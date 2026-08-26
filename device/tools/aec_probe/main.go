//go:build server

// aec_probe: plays a click train through the speaker while simultaneously
// capturing the mic array, in one process — so there's no cross-process
// timing skew between what was played and what was captured. Writes both
// to /data/local/tmp/ for offline cross-correlation.
//
// This is the on-hardware test docs/checkers-port.md used to characterise
// checkers' ch2/ch3 as a sample-aligned hardware AEC reference (measured:
// 40-sample delay, -13dB, 0.83 correlation to the mics). Same method here,
// aimed at crown's ch4/ch5 — logged as "quiet, ~15dB down" in the plain
// capture_mics run, unconfirmed as a reference channel or not.
//
// Usage:
//   aec_probe [seconds] [-mic-card N] [-mic-device N] [-mic-channels N]
//                       [-spk-card N] [-spk-device N]
//   defaults: 8s, mic 0/22/6 (crown), speaker 0/0 (crown)
//
// Output: /data/local/tmp/aec_probe_mic.raw   (S24_3LE, N channels, 16kHz)
//         /data/local/tmp/aec_probe_click.txt (click times, seconds from start)
//
// Build inside echomuse-compiler Docker container:
//   go build -tags server -o aec_probe .

package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/Binozo/GoTinyAlsa/pkg/pcm"
	"github.com/Binozo/GoTinyAlsa/pkg/tinyalsa"
)

const (
	micSampleRate = 16000
	micPeriodSize = 512
	micPeriodCnt  = 5

	spkSampleRate = 48000
	spkChannels   = 2

	clickIntervalMs = 500 // one click every 500ms
	clickDurationMs = 8   // short, sharp — good for cross-correlation
	clickFreqHz     = 2000

	micOutPath   = "/data/local/tmp/aec_probe_mic.raw"
	clickLogPath = "/data/local/tmp/aec_probe_click.txt"
)

func stopMixerCmd() {
	// same requirement as capture_mics / EchoMuse pcm_microphone.go
	cmd := exec.Command("stop", "mixer")
	if err := cmd.Run(); err != nil {
		log.Printf("stop mixer: %v (continuing)", err)
	}
}

func main() {
	micCard := flag.Int("mic-card", 0, "mic ALSA card")
	micDevice := flag.Int("mic-device", 22, "mic ALSA device (24=biscuit, 22=crown)")
	micChannels := flag.Int("mic-channels", 6, "mic channel count (9=biscuit, 6=crown)")
	spkCard := flag.Int("spk-card", 0, "speaker ALSA card")
	spkDevice := flag.Int("spk-device", 0, "speaker ALSA device")
	flag.Parse()

	durationSecs := 8
	if flag.NArg() > 0 {
		n, err := strconv.Atoi(flag.Arg(0))
		if err != nil || n < 2 || n > 60 {
			log.Fatalf("usage: aec_probe [seconds 2-60] [-mic-card N] [-mic-device N] [-mic-channels N] [-spk-card N] [-spk-device N]")
		}
		durationSecs = n
	}

	fmt.Println("Stopping mixer service...")
	stopMixerCmd()

	// --- mic capture, started first so it's warm before playback begins ---
	micDev := tinyalsa.NewDevice(*micCard, *micDevice, pcm.Config{
		Channels:    *micChannels,
		SampleRate:  micSampleRate,
		PeriodSize:  micPeriodSize,
		PeriodCount: micPeriodCnt,
		Format:      tinyalsa.PCM_FORMAT_S24_3LE,
	})

	micFile, err := os.Create(micOutPath)
	if err != nil {
		log.Fatalf("failed to create %s: %v", micOutPath, err)
	}
	defer micFile.Close()

	clickLog, err := os.Create(clickLogPath)
	if err != nil {
		log.Fatalf("failed to create %s: %v", clickLogPath, err)
	}
	defer clickLog.Close()

	stream := make(chan []byte, 64)
	errCh := make(chan error, 1)
	go func() {
		if err := micDev.GetAudioStream(micDev.DeviceConfig, stream); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// let the capture stabilise before playback starts
	time.Sleep(500 * time.Millisecond)
	start := time.Now()

	// --- speaker playback, click train, in its own goroutine ---
	// AudioSession/Pump, not SendAudioStream: SendAudioStream opens+closes
	// the PCM on every call and fails outright on this card, matching
	// production's pcm_speaker.go, which streams via one long-lived session.
	spkDev := tinyalsa.NewDevice(*spkCard, *spkDevice, pcm.Config{
		Channels:    spkChannels,
		SampleRate:  spkSampleRate,
		PeriodSize:  1536,
		PeriodCount: 4,
		Format:      tinyalsa.PCM_FORMAT_S16_LE,
	})

	session, err := spkDev.NewAudioSession()
	if err != nil {
		log.Fatalf("speaker session open failed: %v", err)
	}
	defer session.Close()

	periodBytes := session.BufferSize()
	if periodBytes <= 0 {
		periodBytes = 1536 * spkChannels * 2
	}

	go func() {
		click := makeClick(spkSampleRate, spkChannels, clickDurationMs, clickFreqHz)
		silenceLen := (spkSampleRate * spkChannels * 2 * clickIntervalMs / 1000) - len(click)
		if silenceLen < 0 {
			silenceLen = 0
		}
		silence := make([]byte, silenceLen)

		pump := func(data []byte) {
			for off := 0; off < len(data); off += periodBytes {
				end := off + periodBytes
				if end > len(data) {
					end = len(data)
				}
				chunk := data[off:end]
				if len(chunk) < periodBytes {
					padded := make([]byte, periodBytes)
					copy(padded, chunk)
					chunk = padded
				}
				if err := session.Pump(chunk); err != nil {
					log.Printf("speaker pump error (continuing): %v", err)
					return
				}
			}
		}

		ticker := time.NewTicker(time.Duration(clickIntervalMs) * time.Millisecond)
		defer ticker.Stop()
		for t := range ticker.C {
			elapsed := t.Sub(start).Seconds()
			fmt.Fprintf(clickLog, "%.4f\n", elapsed)
			pump(click)
			pump(silence)
		}
	}()

	deadline := time.After(time.Duration(durationSecs) * time.Second)
	bytesWritten := 0

	fmt.Printf("Capturing %ds mic + click train to speaker ...\n", durationSecs)

loop:
	for {
		select {
		case <-deadline:
			break loop
		case err := <-errCh:
			if err != nil {
				log.Fatalf("mic stream error: %v", err)
			}
			break loop
		case buf, ok := <-stream:
			if !ok {
				break loop
			}
			n, err := micFile.Write(buf)
			if err != nil {
				log.Fatalf("write error: %v", err)
			}
			bytesWritten += n
		}
	}

	framesWritten := bytesWritten / (*micChannels * 3)
	fmt.Printf("Done.\n  Frames: %d  Duration: %dms  Bytes: %d\n",
		framesWritten, framesWritten*1000/micSampleRate, bytesWritten)
	fmt.Printf("Pull with:\n  adb pull %s\n  adb pull %s\n", micOutPath, clickLogPath)
}

// makeClick builds one short S16_LE tone burst, interleaved to N channels,
// windowed (linear ramp in/out) so it doesn't click the DAC itself.
func makeClick(sampleRate, channels, durationMs, freqHz int) []byte {
	nSamples := sampleRate * durationMs / 1000
	rampSamples := nSamples / 8
	buf := make([]byte, nSamples*channels*2)
	for i := 0; i < nSamples; i++ {
		amp := 1.0
		if i < rampSamples {
			amp = float64(i) / float64(rampSamples)
		} else if i > nSamples-rampSamples {
			amp = float64(nSamples-i) / float64(rampSamples)
		}
		v := int16(amp * 0.8 * 32767 * math.Sin(2*math.Pi*float64(freqHz)*float64(i)/float64(sampleRate)))
		off := i * channels * 2
		for c := 0; c < channels; c++ {
			buf[off+c*2] = byte(v)
			buf[off+c*2+1] = byte(v >> 8)
		}
	}
	return buf
}
