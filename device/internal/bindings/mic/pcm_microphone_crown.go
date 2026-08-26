//go:build crown

package mic

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/wilbowes/EchoMuse/internal/alsa"
	pkgmic "github.com/wilbowes/EchoMuse/pkg/mic"
)

// crown: mic is card0,device22 (2x TLV320AIC3101, 6ch/16kHz/S24_3LE) —
// proven on real hardware 2026-08-26 (device/tools/capture_mics), driver
// range confirmed by HW_REFINE to match checkers exactly. See
// docs/echo-show-8-hardware-map.md. Only 4 of the 6 channels carry a live
// mic (ch4/ch5 measured as idle TDM slots, not an AEC reference); the
// beamformer/channel selection above this package decides what to do with
// that, this binding just delivers all 6.
const cardNr = 0
const deviceNr = 22
const channels = 6

// periodFrames/periods are carried from the working capture_mics config
// (well inside the HW_REFINE range 257..2570 frames / 1..10 periods);
// checkers needed 8 periods after measuring arrival-gap drops at 4 under
// playback load — crown's margin under load is unmeasured, so this starts at
// the same margin rather than the untested minimum.
const periodFrames = 512
const periods = 8

// PcmMicrophone is crown's capture binding: same fan-out-to-subscribers shape
// as biscuit's (pcm_microphone.go), over internal/alsa's blocking Read
// instead of GoTinyAlsa's channel-based GetAudioStream — this driver has
// never been run against GoTinyAlsa/tinyalsa, and internal/alsa is already
// proven against it via HW_REFINE and capture_mics.
type PcmMicrophone struct {
	pcm  *alsa.PCM
	mu   sync.Mutex
	subs []chan []byte
}

func NewMicrophone() (*PcmMicrophone, error) {
	m := &PcmMicrophone{}
	if err := m.Init(); err != nil {
		return nil, err
	}
	return m, nil
}

// Init opens the capture PCM and starts the permanent read loop.
//
// No init service to stop first: `stop mixer` on this platform returned
// "exit status 1" against a real unit — there is no such service — and per
// TestNeverStopAudioserver's reasoning (device/internal/profile is gone, but
// the finding stands, see docs/echo-show-8-hardware-map.md), nothing in the
// audioserver family may ever be added here.
func (m *PcmMicrophone) Init() error {
	pcm, err := alsa.Open(alsa.Config{
		Card: cardNr, Device: deviceNr, Playback: false,
		Channels: channels, Format: alsa.FormatS24_3LE, Rate: 16000,
		PeriodSize: periodFrames, Periods: periods,
	})
	if err != nil {
		return err
	}
	m.pcm = pcm
	go m.readLoop()
	return nil
}

// readLoop reads fixed-size periods forever and fans each one out to every
// current subscriber, mirroring biscuit's readLoop (pcm_microphone.go) —
// same stall/clock telemetry, same copy-per-subscriber, same close-all-on-
// death behaviour — over a plain blocking Read instead of a stream channel.
func (m *PcmMicrophone) readLoop() {
	periodBytes := periodFrames * channels * 3 // S24_3LE
	buf := make([]byte, periodBytes)

	rate := int64(16000)
	var (
		firstArrival time.Time
		lastArrival  time.Time
		lastReport   time.Time
		framesTotal  int64
		stalls       uint64
		subDrops     uint64
	)

	for {
		n, err := m.pcm.Read(buf)
		if err != nil {
			log.Printf("mic: ALSA read error: %v", err)
			break
		}
		if n == 0 {
			continue // EPIPE recovery in alsa.PCM.Read already re-armed the stream
		}

		now := time.Now()
		frames := int64(n / (channels * 3))
		batchDur := time.Duration(frames) * time.Second / time.Duration(rate)
		if firstArrival.IsZero() {
			firstArrival, lastReport = now, now
		} else if gap := now.Sub(lastArrival); gap > 2*batchDur {
			stalls++
			log.Printf("[mic] capture stall: %dms between %dms batches — ~%dms lost to ALSA overrun (stalls=%d)",
				gap.Milliseconds(), batchDur.Milliseconds(),
				(gap - batchDur).Milliseconds(), stalls)
		}
		lastArrival = now
		framesTotal += frames
		if now.Sub(lastReport) >= time.Minute {
			wall := now.Sub(firstArrival)
			audioDur := time.Duration(framesTotal) * time.Second / time.Duration(rate)
			log.Printf("[mic] clock: %.1fs audio over %.1fs wall (deficit %+dms, stalls=%d, sub_drops=%d)",
				audioDur.Seconds(), wall.Seconds(), (wall - audioDur).Milliseconds(), stalls, subDrops)
			lastReport = now
		}

		out := make([]byte, n)
		copy(out, buf[:n])

		m.mu.Lock()
		for _, ch := range m.subs {
			select {
			case ch <- out:
			default:
				subDrops++
				if subDrops == 1 || subDrops%64 == 0 {
					log.Printf("[mic] subscriber channel full — batch dropped (sub_drops=%d)", subDrops)
				}
			}
		}
		m.mu.Unlock()
	}

	m.mu.Lock()
	log.Printf("mic: ALSA stream closed — notifying %d subscribers", len(m.subs))
	for _, ch := range m.subs {
		close(ch)
	}
	m.subs = nil
	m.mu.Unlock()
}

func (m *PcmMicrophone) Subscribe() chan []byte {
	ch := make(chan []byte, 32)
	m.mu.Lock()
	m.subs = append(m.subs, ch)
	m.mu.Unlock()
	return ch
}

func (m *PcmMicrophone) Unsubscribe(ch chan []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.subs {
		if s == ch {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (m *PcmMicrophone) Listen(callback pkgmic.AudioCallback, ctx context.Context) error {
	if callback == nil {
		return errors.New("callback can't be nil")
	}
	ch := m.Subscribe()
	defer m.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return nil
		case audio, ok := <-ch:
			if !ok {
				return nil
			}
			callback(audio)
		}
	}
}
