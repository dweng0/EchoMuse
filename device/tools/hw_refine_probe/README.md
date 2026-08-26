# hw_refine_probe — ask the driver, don't infer from the HAL

## What this is for

Every mic/speaker config used elsewhere in crown's discovery (`capture_mics`,
`aec_probe`) was HAL-observed or "it didn't error" — nobody had actually
asked the ALSA driver what it really supports. This calls `HW_REFINE`
directly: opens the PCM briefly, seeds a wide-open parameter set, and reads
back the driver's real ranges for format, channels, rate, period size and
period count. The same method `docs/checkers-port.md` (PR #36) used to pin
its constants — this is the first time it's been run on crown.

It never reads or writes audio — only opens the device long enough to
negotiate parameters, so it's safe to run even while something else has the
PCM (it will simply fail to open, same as any other contending opener).

## Why it needs the whole module mounted

Unlike `capture_mics`/`aec_probe` (standalone modules using only GoTinyAlsa),
this imports `internal/alsa` — the dependency-free ALSA client from PR #36,
vendored here purely for its `Capabilities()` call. `internal/alsa` is an
**internal package**: Go only allows importing it from inside
`github.com/wilbowes/EchoMuse`, so this tool has no `go.mod` of its own and
builds as part of the main device module, exactly like `oww_probe` (see
`../README.md`).

## Build

```bash
cd device
REPO_ROOT=$(git rev-parse --show-toplevel)
docker run --rm \
  --entrypoint bash \
  -e CGO_LDFLAGS="-Wl,--hash-style=both" \
  -v "$(pwd)":/sdk \
  -v "$REPO_ROOT/GoTinyAlsa":/GoTinyAlsa \
  echomuse-compiler \
  -c "cd /sdk && go build -tags server -o build/hw_refine_probe ./tools/hw_refine_probe"
```

Or via `./build_tools.sh` from `device/tools/`, which builds it alongside
the others.

## Run

No root needed — `HW_REFINE` doesn't require exclusive access the way a real
capture/playback stream does.

```bash
adb push build/hw_refine_probe /data/local/tmp/hw_refine_probe
adb shell chmod 755 /data/local/tmp/hw_refine_probe

# mic (capture)
adb shell /data/local/tmp/hw_refine_probe -card 0 -device 22

# speaker (playback)
adb shell /data/local/tmp/hw_refine_probe -card 0 -device 0 -playback
```

## Reading the result

Prints the driver's actual `Formats` / `Channels` / `Rate` / `Period size` /
`Periods` ranges. Compare against whatever config a binding or probe tool is
using — a config that only *happens* to sit inside range isn't verified until
you've actually seen the range. `Channels: N..N` (min == max) means the
driver has no choice, not that N was a good guess.

On crown, the mic's period-size and period-count ranges came back
numerically identical to checkers' documented driver range — worth checking
here first before assuming a new board's driver constraints are unknown;
they may already be recorded for a sibling board on the same SoC family.
