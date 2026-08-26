# crown AudioTrack fix — handoff (2026-08-26, end of session)

Written for picking this back up cold, after clearing context.
`docs/echo-show-8-audio-freeze-handoff.md` (the original freeze
investigation, same day, earlier) is now **superseded for its core
question** — the freeze it documents is fixed — but its pstore evidence
and the four freeze repros are still the reference for *why* this fix
exists. `docs/echo-show-8-freeze-scenarios.md` has the full scenario
comparison and verdicts across six candidate fixes. `docs/echo-show-8-audiotrack-design.md`
has the complete design + every bug found while building it, in the order
found — read that one in full before touching this code, it's the real
record.

## The one-line status

**The crown (Echo Show 8) freeze is fixed and validated live on hardware.**
Speaker playback no longer opens `/dev/snd` directly on crown; it goes
through a Unix socket to a real `AudioTrack` in `crown_launcher`, so
`mediaserver` arbitrates the DL1 hardware path normally instead of
contending with an exclusive hold. Confirmed clean: concurrent browser
audio, a full wake→turn→TTS reply under that load, and a 10-minute
unattended soak (including a second turn fired by wake word while the user
was away) — zero freezes, zero drops, one stable connection throughout,
pstore clean at every check.

## What's committed, on `echo-show-8-support`

Three commits from this session, each independently revertible:

1. `af0b000` — `crown_launcher/` itself (previously untracked from an
   earlier session) plus the new `PlaybackServer.java` (binds the socket,
   tested live) and `AudioProbeReceiver.java` (throwaway diagnostic that
   proved `AudioTrack` low-latency mode is actually granted on this
   SoC/ROM). Not yet wired to the daemon at this point.
2. `dfb773b` — the actual fix: `pcm_speaker_crown.go` rewired to
   `socketPCM` instead of `alsa.Open`, behind a 2-method `pcmWriter`
   interface so the mixer/duck/ring-buffer code needed zero changes.
   Includes the write-pacing fix (see "the real bug" below).
3. `f82af15` — docs recording the validation tests (real TTS turn,
   10-minute soak).

All three tests green at time of writing: `go vet`/`go test -tags crown`
(clean), `go vet`/`go test -tags server` (fails identically to before this
session — pre-existing host limitation, no FireOS cgo sysroot, unrelated
to this diff), `pytest controller/tests/` (734 passed, 3 skipped, same
count as before).

## The one thing worth reading in full before changing this code

`docs/echo-show-8-audiotrack-design.md`'s "Real implementation, built and
tested live" section. The short version: a write-pacing bug in
`socketPCM.Write` (device/internal/bindings/speaker/socket_pcm_crown.go)
produced a **completely convincing false conclusion mid-session** — "sustained
AudioTrack streaming itself stalls on this ROM after ~1s, silently, with
zero error anywhere in the log" — reproducible on both `AudioTrack`
performance modes, looking exactly like a platform-level dead end. It
wasn't: raw ALSA used to pace the mix loop for free (blocking `Write` at
hardware rate), and nothing replaced that pacing when the write moved to a
socket, so it produced bursty writes instead of one period every ~32ms. A
bypass test (discard the bytes, skip `AudioTrack` entirely) ran clean for
6+ seconds and is what actually isolated it. Fixed with explicit real-time
pacing in `socketPCM.Write`. If sustained `AudioTrack` streaming looks
broken again in the future, check pacing before concluding it's the
platform.

## What's NOT done — the actual next steps

1. ~~Cross-app ducking~~ — **done**, `15c1704`. `PlaybackServer.java`
   requests `AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK` around each connection's
   `track.play()`/`track.stop()`; other apps duck automatically while
   focus is held, matching the pre-fix ducking behaviour from the
   outside. Not yet validated live with a second app actually playing
   during a turn — build is clean, but nobody has watched it duck on
   hardware yet.
2. ~~`AudioProbeReceiver`~~ — **removed**, `6079f3f`. Findings kept as a
   comment in `PlaybackServer.buildTrack()`; the receiver and its
   exported `PROBE_AUDIO` broadcast are gone from the manifest.
3. **Everything from the *previous* handoff doc that was never about the
   freeze itself is still open**: the ueventd `/dev/snd`+`/dev/input`
   permission patch is still a manual, undocumented step on this one test
   unit (not in `provision_crown.sh`); `device/.android-sdk/` and
   `crown_launcher/build/` are gitignored now but the SDK itself is still
   a manual local install. `provision_crown.sh` itself was rewritten to
   install `crown_launcher.apk` instead of the dead raw-init `.rc` path
   and committed this session (`88a99a3`) — that part of the previous
   handoff's open list is closed.
4. **Longer-horizon, not urgent**: the 10-minute soak is solid evidence
   but is still short next to "ran for days" — if this ships to a real
   fleet rather than staying a one-device experiment, worth a much longer
   unattended run at some point, ideally with the device also doing
   ordinary tablet things (browsing, notifications) rather than just idle.
5. **New**: branch not yet opened as a PR against `main`. See below.

## Live device state as of writing this

- Device: crown test unit, serial `G0916D10014507JS`, **not currently
  frozen** — last known state fully responsive, running the fixed build.
- `crown_launcher` app: enabled, `ServerService` running with the real
  `PlaybackServer` socket active.
- `/data/local/bin/server`: the just-built crown binary
  (`20260826-2048-dev` or later if rebuilt again this session — check
  `[control] Registered as ... (version ...)` in
  `/data/local/tmp/echomuse.log` for the exact version actually running).
- Controller: running on host `thinkpad` (per the original handoff doc),
  `.env` has `DEVICE_APPROVAL=auto`.
- Repo: `echo-show-8-support` branch, three new commits this session (see
  above), working tree otherwise matches whatever was left from the
  earlier session (`device/scripts/provision_crown.sh` modifications from
  before this session are still uncommitted — not touched today).
