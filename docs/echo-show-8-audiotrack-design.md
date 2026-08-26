# crown speaker via AudioTrack — design spec (Scenario C)

Follows `docs/echo-show-8-freeze-scenarios.md` (Scenario C verdict: pursue).
Covers only the **playback** path — `pcm_microphone_crown.go` and the mic
side of the protocol are untouched, staying raw ALSA, per the freeze doc's
own reasoning (pstore evidence implicates DL1/playback, not capture).

## Why this instead of A

A (open DL1 on demand) narrows the collision *window* but doesn't remove the
collision *condition* — mediaserver still contends for the same exclusive
node, just less often. C removes the condition itself: playback goes through
`AudioTrack`, which `AudioFlinger` mixes with everything else in userspace
*before* touching the HAL/kernel node, so there's no second exclusive opener
of `mtk_pcm_I2S0dl1` at all. If this works, the "hold it open forever"
model A was trying to avoid becomes safe again — see below.

## Split of responsibility

| Layer | Stays the same | Changes |
|---|---|---|
| `data.go`, WS protocol, turn state machine | ✅ unchanged | — |
| `pcm_speaker_crown.go`: `audioStream`, `Mixer`, `duckTarget`, ring buffers | ✅ unchanged | — |
| `pcm_speaker_crown.go`: final write call | — | `p.pcm.Write(out)` → socket write |
| `pcm_microphone_crown.go` | ✅ unchanged (raw ALSA) | — |
| New: `crown_launcher` Java playback service | — | new |

Everything upstream of the current `p.pcm.Write(out)` call in `silenceLoop()`
(`device/internal/bindings/speaker/pcm_speaker_crown.go:163`) is reused
as-is. The `alsa.PCM` field becomes a small interface (`Write([]byte) (int,
error)`, `Close()`) so a socket-backed implementation can satisfy it without
touching the mixing loop.

## Format (fixed, no handshake)

Current ALSA config (`pcm_speaker_crown.go:88-92`): 48000 Hz, stereo, S16LE,
period = 1536 frames = 6144 bytes. The socket protocol reuses this exactly —
no negotiation needed, both sides are built from the same repo and deployed
together. If this ever needs to change, bump a version byte in the frame
header rather than adding a runtime handshake.

## Transport: filesystem-namespace Unix domain socket

Path: `<launcher's getFilesDir()>/pcm.sock` — e.g.
`/data/data/com.echomuse.crownlauncher/files/pcm.sock`. Created by the Java
service via `LocalServerSocket`/`LocalSocketAddress.Namespace.FILESYSTEM`
(not Android's default abstract namespace — a real path is what lets the Go
side use a plain `net.Dial("unix", path)` with no Android-specific socket
API on that end).

Framing: 4-byte little-endian length prefix + that many raw PCM bytes. One
frame per period (6144 bytes) is the expected steady state; the length
prefix exists so a future format-version bump or partial/short frame
doesn't have to guess.

## Q1 — does the socket work across both launch paths? **Answered, no device needed.**

Two ways the daemon gets launched today (handoff doc, both confirmed live):

1. **Via `ServerService`** (`ServerService.java:40`): `new
   ProcessBuilder(BINARY).start()` — the child inherits the **service's own
   uid** (the app's sandboxed uid, `u0_a148` per the handoff doc). The
   socket file lives under `getFilesDir()`, which Android creates as
   `0700`, owned by that same uid — a same-uid child can always open it, no
   special permission needed.
2. **Via plain `adb root` shell exec** (the "always worked" pre-APK method,
   also used throughout today's freeze testing): the process runs as
   **root**. Root bypasses DAC permission checks entirely — it can open any
   socket file regardless of owning uid.

Both paths connect successfully with **zero permission changes** needed
anywhere (no ueventd-style ownership patch, unlike the earlier `/dev/snd`
fix). This was fully answerable from the existing `ServerService.java` and
the handoff doc's uid findings — confirmed without touching hardware.

## Q3 — failure handling. **Designed, no device needed to validate the logic.**

The whole point of this change is to stop wedging the DSP when something
goes wrong; a design that can itself hang because the *socket* misbehaves
would be self-defeating. Rules:

- **Go side never blocks indefinitely on the socket.** `silenceLoop`'s
  current `p.pcm.Write(out)` is a blocking ALSA write paced by the hardware
  ring — the new socket writer gets a **write deadline** (one period's worth
  of wall-clock time, ~32ms at this rate) via `SetWriteDeadline`. A missed
  deadline drops that period (counts as an underrun, same accounting
  `report()` already does) rather than stalling the mix loop.
- **Reconnect, don't crash.** If the socket is closed or the Java service is
  mid-restart (it's `START_STICKY`, per `ServerService.java:47` — Android
  restarts it, but not instantly), the Go side keeps mixing and drops
  periods (writes to nothing / drops on deadline) while attempting a
  reconnect on a short backoff (e.g. 250ms, capped) in the background,
  swapping the live connection in once it succeeds. No period is ever
  queued waiting for a connection that might not come back.
- **Java side accepts one connection at a time**, closes any prior one on a
  new connect (the daemon only ever has one instance), and on socket error
  releases the `AudioTrack` and recreates it on the next connection —
  mirroring `ServerService`'s existing "no supervisor, minimal" philosophy
  rather than adding new retry/health-check machinery on that side.
- **Nothing here calls into `stop <service>` or touches any other app** —
  consistent with the mic binding's existing comment that "nothing in the
  audioserver family may ever be added here."

This is a straightforward reconnect-with-backoff + deadline-write pattern;
none of it needs a real device to design or code-review, only to confirm the
measured deadline value is sane once real latency numbers exist (see Q2).

## Q2 — is low-latency mode actually granted on this hardware? **Needs a device. Probe is ready.**

This is the one question that can kill the whole scenario: if
`AudioTrack` silently falls back to the shared/legacy mixer path on this
SoC/ROM instead of the fast/low-latency path, the extra buffering could
make barge-in noticeably worse than the current raw-ALSA behavior — and
there's no way to know without asking the actual device.

**Throwaway probe added**: `device/crown_launcher/src/com/echomuse/crownlauncher/AudioProbeReceiver.java`
(new, see below) — a broadcast receiver, triggered by
`adb shell am broadcast -a com.echomuse.crownlauncher.PROBE_AUDIO
-n com.echomuse.crownlauncher/.AudioProbeReceiver`, that:

1. Builds an `AudioTrack` with the exact format/usage this design would
   actually use (48kHz stereo S16LE, `USAGE_ASSISTANT`,
   `CONTENT_TYPE_SPEECH`, `PERFORMANCE_MODE_LOW_LATENCY` requested).
2. Logs (`adb logcat -s EchoMuseProbe`) what was **actually granted**:
   `getPerformanceMode()` (does it agree with what was requested, or fall
   back to `PERFORMANCE_MODE_NONE`?), `getBufferSizeInFrames()`, and the
   system-wide fast-mixer indicators `AudioManager.PROPERTY_OUTPUT_FRAMES_PER_BUFFER`
   / `PROPERTY_OUTPUT_SAMPLE_RATE`.
3. Plays a short (~1s) test tone and releases — confirms the track actually
   produces sound at all on this build, not just that it constructs
   without throwing.

Answer takes under a minute once a device is attached: install the APK,
run the broadcast, read three log lines. **Run this before writing a single
line of the real socket/service code** — a `PERFORMANCE_MODE_NONE` result
doesn't kill Scenario C outright (legacy-mixer AudioTrack is still strictly
safer than raw ALSA contention), but it does mean the "keep it open
continuously, no latency regression" assumption above needs re-checking
against a measured number instead of an aspiration.

### Result (2026-08-26, run live on `G0916D10014507JS`)

```
AudioManager.PROPERTY_OUTPUT_SAMPLE_RATE=48000
AudioManager.PROPERTY_OUTPUT_FRAMES_PER_BUFFER=768
AudioTrack.getMinBufferSize=18464 bytes
requested PERFORMANCE_MODE_LOW_LATENCY, granted: LOW_LATENCY (granted as requested)
getBufferSizeInFrames=4616
getSampleRate=48000
getState=1 (1=INITIALIZED expected)
wrote 96000 of 96000 samples
```

**PROVEN: low-latency mode is granted, not simulated, not a fallback.**
`getPerformanceMode()` returned exactly what was requested — this ROM/SoC
does have a working fast-mixer path, contrary to the risk this section
flagged going in. `PROPERTY_OUTPUT_FRAMES_PER_BUFFER=768` at 48kHz is a
16ms native mixer period, in line with a real fast-mixer config, not the
larger legacy-path buffer sizes (typically 20-40ms+) that would have shown
up under `PERFORMANCE_MODE_NONE`. Test tone played and was **audibly heard**
on the actual device speaker (confirmed by the user live), not just
constructed without throwing.

One number to note, not yet fully explained: `getBufferSizeInFrames=4616`
(~96ms) is considerably larger than the requested `getMinBufferSize`
translated to frames (18464 bytes / 4 bytes-per-frame = 4616 frames —
**they're identical**, so this is simply what was asked for via
`.setBufferSizeInBytes(Math.max(minBuf, 4096))`, not something the low-latency
path silently inflated). The *native* low-latency period is the 768-frame
(16ms) figure above; the 4616-frame client buffer is just this probe's own
conservative sizing and isn't representative of what the real design should
request — the real service should size its `AudioTrack` buffer close to the
768-frame native period, not reuse this probe's oversized default, to get
the actual latency win this mode is proving is available.

**This clears the biggest open risk in Scenario C.** Barge-in/duck latency
still needs measuring end-to-end once the real socket path exists (this
probe only proves the mode is available, not the full round-trip cost
through our own mixer + socket + AudioFlinger), but the worst-case outcome
this section worried about — silent fallback to the shared legacy mixer —
is ruled out.

## Real implementation, built and tested live (2026-08-26)

`PlaybackServer.java` (crown_launcher) + `socket_pcm_crown.go` (device)
implement the design above. Three real bugs found and fixed by testing on
hardware, not by inspection — recorded because each one would have been a
believable-looking wrong conclusion if testing had stopped early:

1. **`LocalServerSocket`'s filesystem-namespace attempt silently became
   abstract-namespace instead.** Its String constructor only ever binds
   abstract; `LocalSocketAddress(...).getName()` just returns the raw name,
   dropping the namespace choice. Confirmed via `/proc/<pid>/net/unix`
   showing an `@`-prefixed entry, not a real dentry (`ls`/`find` on the
   intended path found nothing). Not worth fighting — abstract sockets need
   no stale-file cleanup and aren't gated by filesystem permission bits at
   all, which only strengthens Q1's answer. Go dials it with the `@name`
   convention.

2. **A 25ms write deadline mistook AudioTrack backpressure for a dead
   connection.** Raw ALSA gave `silenceLoop` its pacing for free (blocking
   `Write` at hardware rate); a socket write returns as soon as the kernel
   buffer has room, so without that the loop floods far faster than
   AudioTrack drains, backpressure blocks the write, and 25ms is nowhere
   near enough headroom for that ordinary case. Raised to 500ms — ruled
   out as *the* problem, but did not fully fix the drops (see next).

3. **The real cause: bursty delivery, not backpressure blocking.** With the
   deadline fixed, connections still died reliably after ~0.6–1.3s of
   streaming — reproducible on both `PERFORMANCE_MODE_LOW_LATENCY` and
   `PERFORMANCE_MODE_NONE`, with **zero errors anywhere in the system log**
   at the exact stall moment (no AudioFlinger warning, no HAL message,
   nothing — a genuinely silent wedge). Bypassing `AudioTrack.write()`
   entirely (discard the bytes, do nothing else) ran clean for 6+ seconds
   with zero drops, isolating the fault to something about how the write
   was being driven, not the socket/read path. Root cause: Go's writer had
   no pacing of its own — it produced whatever the socket's backpressure
   allowed, in bursts of several periods at once followed by a stall,
   rather than one period every ~32ms. Adding explicit real-time pacing in
   `socketPCM.Write` (one period every `len(buf)/48000/4` seconds,
   matching the ALSA config exactly) fixed it outright: **2+ minutes
   sustained, one connection, zero drops**, confirmed live with the daemon
   log's own `[mic] clock` line as an independent time reference
   (`120.2s audio over 120.1s wall`).

   **This means the earlier live conclusion — "AudioTrack itself stalls
   under sustained streaming on this ROM" — was wrong**, reached mid-session
   before pacing was tried. Worth recording exactly because it looked
   completely convincing at the time (reproducible, silent, present under
   both performance modes) and would have wrongly closed off Scenario C
   as non-viable if it had been accepted instead of chased one step
   further.

**Concurrent-load retest against the real (non-probe) service**: with the
daemon's normal continuous speaker plane running through the real socket +
`AudioTrack` path, browser audio playing concurrently on the device, and a
volume-button press exercised mid-test — 30 seconds, zero freeze, zero
drops, one stable connection, clean pstore. This is the actual production
code path, not the throwaway `AudioProbeReceiver`.

**Real voice turn under concurrent load (2026-08-26, later)**: a full
wake→turn→TTS reply while browser audio played concurrently — 83 periods
of real speech audio, **zero underruns**, zero socket drops, one stable
connection, device fully responsive, pstore clean throughout. This is
actual TTS content through the mixer, not the continuous silence stream —
the strongest evidence yet, since the pacing fix (above) was derived
against the silence stream and needed to hold for real content too.

**10-minute unattended soak (2026-08-26, later)**: 60 liveness checks at
10s intervals, zero freeze, zero drops, one connection the whole run,
pstore clean at the end. The daemon's own `[mic] clock` line independently
confirms ~841s of continuous uptime with near-zero drift (`deficit -142ms`
over 841s audio) — covers the freeze #2 profile from the original handoff
doc (no confirmed trigger, purely time-based), not just the active-load
case. **A second real voice turn was triggered by wake word mid-soak**
(user away from the keyboard) — 86 periods, zero underruns, same clean
profile as the first turn: the wake path, not just a manually-triggered
turn, holds up unattended.

**One known gap, not a regression**: cross-app ducking doesn't happen —
`duckDb`/`Mixer.duckTarget` only ever ducked crown's own voice-vs-music
planes against each other (one process, two internal streams). Now that
the browser's audio is a genuinely separate `AudioTrack` client mixed by
`AudioFlinger`, our mixer has no reach over it, and never did — this was
always true, just newly visible now that concurrent playback survives at
all. The real fix is a transient `AudioFocus` request per turn (Android's
standard mechanism — other apps duck/pause automatically while focus is
held), tracked as its own follow-up in the Sequencing section below, not
folded into the freeze fix itself.

## Sequencing

1. **Run the Q2 probe** the moment a device is available — answers whether
   to budget for a latency regression before any other code is written.
2. Build the real `LocalServerSocket` + `AudioTrack` playback service in
   `crown_launcher`, using the format/framing/failure-handling above.
3. Swap `pcm_speaker_crown.go`'s `alsa.PCM` for the socket-backed writer
   behind the same small interface; mixing/ducking code untouched.
4. Live test: full voice turn + concurrent YouTube/browser load (the
   existing freeze repro) — this is the test that actually validates the
   scenario, everything before it is just de-risking getting there.
5. If 4 holds up, decide whether audio focus (`AudioManager.requestAudioFocus`,
   transient, per turn) should replace the daemon's own `duckDb` logic — a
   separate, smaller follow-up, not blocking the freeze fix itself.
