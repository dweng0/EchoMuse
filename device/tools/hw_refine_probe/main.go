//go:build server

// hw_refine_probe: asks the ALSA driver to refine a wide-open parameter set
// (HW_REFINE) for a given PCM, so a profile's Channels/SampleRate/PeriodSize
// are driver-verified rather than HAL-observed or copied from a working
// config that happened not to error.
//
// Uses device/internal/alsa (the dependency-free client from PR #36 /
// docs/checkers-port.md) purely for its Capabilities() call — no PCM is
// opened for read/write here, so this never contends with a running
// capture/playback stream.
//
// Usage:
//   hw_refine_probe -card N -device N [-playback]
//
// Build inside echomuse-compiler Docker container, module-mounted like
// oww_probe (internal/alsa is an internal package, only importable from
// inside github.com/wilbowes/EchoMuse):
//   go build -o hw_refine_probe ./tools/hw_refine_probe

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/wilbowes/EchoMuse/internal/alsa"
)

func main() {
	card := flag.Int("card", 0, "ALSA card number")
	device := flag.Int("device", 22, "ALSA device number")
	playback := flag.Bool("playback", false, "probe the playback (p) substream instead of capture (c)")
	flag.Parse()

	direction := "capture"
	if *playback {
		direction = "playback"
	}

	caps, err := alsa.Capabilities(*card, *device, *playback)
	if err != nil {
		log.Fatalf("HW_REFINE failed on card%d,device%d (%s): %v", *card, *device, direction, err)
	}

	fmt.Printf("card%d,device%d (%s) — driver-refined capabilities:\n", *card, *device, direction)
	fmt.Printf("  Formats:      %v\n", caps.Formats)
	fmt.Printf("  Channels:     %d..%d\n", caps.ChannelsMin, caps.ChannelsMax)
	fmt.Printf("  Rate:         %d..%d Hz\n", caps.RateMin, caps.RateMax)
	fmt.Printf("  Period size:  %d..%d frames\n", caps.PeriodSizeMin, caps.PeriodSizeMax)
	fmt.Printf("  Periods:      %d..%d\n", caps.PeriodsMin, caps.PeriodsMax)
}
