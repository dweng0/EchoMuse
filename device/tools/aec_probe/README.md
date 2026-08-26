# aec_probe — click-train speaker/mic correlation test

## What this is for

Tests whether a mic PCM's "extra" channels are a hardware AEC reference (a
sample-aligned loopback of the playback signal) or genuinely unused/idle
slots. PR #36 (`docs/checkers-port.md`) found checkers' spare channels ARE
such a reference — sample-aligned, measured 40-sample delay, −13dB, 0.83
correlation to the mics. Built 2026-08-26 to test the same hypothesis for
crown's ch4/ch5, which had shown up quiet-but-nonzero in a plain
`capture_mics` run (music playing in the room).

Result on crown: **negative**. ch4/ch5 are 99.1% exact digital zero with
rare uncorrelated glitches — nothing like checkers' continuously-live
reference. See `docs/echo-show-8-hardware-map.md` for the full writeup.

## Why one process, not two

Plays the click train and captures the mic array in the same process,
sharing one clock (`start := time.Now()`, click timestamps logged relative
to it). Running playback and capture as separate `adb shell` invocations
would add unknown, variable latency between "the click played" and "the
capture started" — exactly what would need to be ruled out before trusting a
sample-delay measurement.

## Why `AudioSession`/`Pump`, not `SendAudioStream`

GoTinyAlsa's `SendAudioStream` opens and closes the PCM device on *every*
call and failed outright (`EOF`) against this card. `AudioSession`/`Pump`
holds one long-lived session and streams — the same API the production
speaker binding (`device/internal/bindings/speaker/pcm_speaker.go`) already
uses. If you're adding playback to a new probe tool, use this one.

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
  -c "cd /sdk/tools/aec_probe && go build -tags server -o aec_probe ."
```

## Run

Needs root, same as capture_mics. **Plays audibly through the speaker** —
warn whoever's near the device first.

```bash
adb push aec_probe /data/local/tmp/aec_probe
adb shell chmod 755 /data/local/tmp/aec_probe

adb shell /data/local/tmp/aec_probe \
  -mic-card 0 -mic-device 22 -mic-channels 6 \
  -spk-card 0 -spk-device 0 \
  8

adb pull /data/local/tmp/aec_probe_mic.raw
adb pull /data/local/tmp/aec_probe_click.txt
```

Defaults are crown's values (mic 0/22/6, speaker 0/0) — pass `-mic-device 24
-mic-channels 9` for biscuit.

If the speaker doesn't play, check `Ext_Speaker_Amp_Switch` — some boards
(crown, checkers; anything on an RT5616) have it inverted: `On` silences,
`Off` is audible. `adb shell tinymix 5 Off`.

## Reading the result

`aec_probe_click.txt` has one timestamp per click, seconds from capture
start (0.5s pre-roll before the first click). Cross-correlate each click's
arrival on a known-good mic channel against the candidate reference
channel, looking for a small, *consistent* delay — not just correlation,
since loud program material can bleed correlated noise onto genuinely idle
channels (this is what made the first plain-capture reading of crown's
ch4/ch5 misleading). A real hardware reference shows a fixed sample delay
across every click; an idle channel with bleed does not.

```python
# see docs/echo-show-8-hardware-map.md's "ch4/ch5 tested against checkers'
# AEC-reference pattern" section for the full analysis script used here.
```
