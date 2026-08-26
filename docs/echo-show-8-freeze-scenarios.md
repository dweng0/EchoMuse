# crown audio freeze — candidate fix scenarios (2026-08-26)

Companion to `docs/echo-show-8-audio-freeze-handoff.md` (read that first — this
doc assumes its findings as given, doesn't re-derive them). Purpose: lay out
every distinct route out of the freeze as an independent, agent-sized
investigation, so each can be explored in parallel without one blocking
another.

## Established, not up for debate in any scenario below

- Freeze is real hardware/kernel lockup (`mtk_pcm_I2S0dl1` thrashing in
  pstore), not app-uid, not `stop mixer`, not the ueventd patch, not the
  launcher mechanism itself (11 clean idle minutes with it enabled).
- Happens identically whether the daemon is launched via the APK or a plain
  root shell exec — rules out anything specific to `crown_launcher/`.
- Our side of the conflict is `pcm_speaker_crown.go`'s permanent
  `silenceLoop`, opening the DL1 playback PCM once in `Init()` and never
  closing it, so it competes with `mediaserver` for the same exclusive
  hardware path for the life of the process.
- Direction constraint from `CLAUDE.md`: **no dependency on Amazon's
  software**, and any option here is judged the same way — a fix that makes
  a vendor blob (or a from-Amazon binary) load-bearing is the wrong
  direction unless it's an explicit opt-in with old behaviour untouched.
  crown isn't Amazon software to begin with (stock is LineageOS-derived),
  but the same posture applies to *any* vendor blob dependency it might
  introduce.

## Scenario A — stop holding the speaker open; open on demand

Change `pcm_speaker_crown.go` so DL1 is opened only when audio is actually
about to play, and closed again after, instead of the standing
`silenceLoop`. Removes the standing conflict at the root — no long-lived
handle for `mediaserver` to contend with.

- **Cost**: first-sound latency on every TTS reply (PCM device open/HW_PARAMS
  negotiation time, unmeasured — needs benchmarking on this board).
- **Risk**: does the freeze *also* need the open/close race itself (i.e. does
  opening DL1 while mediaserver holds it *also* wedge the DSP, just less
  often)? Handoff's pstore evidence shows contention symptoms from repeated
  open/close, which is unsettlingly close to what this scenario deliberately
  introduces on our side, just less frequently. Needs to be tested, not
  assumed safe.
- **Investigation tasks**: measure open latency; test open/close under
  concurrent mediaserver load specifically (the YouTube-playing repro);
  check whether closing on every turn boundary vs. keeping open with a
  short idle-timeout changes the picture.

## Scenario B — find a shared/software-mixed ALSA path (dmix-equivalent)

If the MT8163 driver or a userspace mixing layer exposes a shared node both
processes can hold concurrently (ALSA `dmix`, a `snd_pcm_plug` shim, or a
board-specific multi-client path), both sides keep continuous access with no
exclusive-open conflict — closest to biscuit's model, ported correctly this
time.

- **Investigation tasks**: enumerate `/proc/asound/pcm`, `/proc/asound/cards`,
  and `alsa-lib` config (`/system/etc/asound.conf` or equivalent) on the live
  device for any softvol/dmix/plug node already defined (stock Android audio
  HALs often route through one internally even when userspace tools don't
  show it). Check whether `tinyalsa`'s `pcm_open` on this board accepts a
  device index other than the raw hardware node. If none exists, this
  scenario is dead — report that clearly rather than forcing it.
- **Risk**: may simply not exist on this driver — handoff already flags this
  as "not yet investigated, may not exist at all."

## Scenario C — use Android's own audio APIs instead of raw ALSA on crown

Route crown's *playback* through `AudioTrack`/`AudioManager` (via a thin JNI
or a Binder shim from the exec'd native binary) instead of opening
`/dev/snd` directly, so `mediaserver` arbitrates access the way it does for
every other app, and our daemon gets normal audio-focus behavior instead of
fighting for the raw device node.

- **Investigation tasks**: what's the minimum surface needed — a small
  Java-side playback service in `crown_launcher` that the Go daemon feeds
  PCM to over a local socket, or can this be done from Go via cgo bindings
  to `AAudio`/`OpenSL ES` (NDK, no privileged permission needed)? Check
  achievable output latency (this is a voice assistant — barge-in and
  end-of-turn responsiveness matter) against Scenario A's raw numbers.
  Keep *capture* (mic) as raw ALSA regardless — the freeze evidence points at
  the playback (DL1) side; don't fix what isn't shown broken.
- **Judgment call flagged for later**: this is the one scenario that adds a
  real dependency on Android's own media framework where today there is
  none. Under this project's portability direction that's a **cost to be
  justified explicitly, not a free win** — worth having the exploring agent
  say plainly whether it's justified for crown specifically (a real Android
  tablet where sharing the stack is the platform's actual model) vs. a
  precedent to avoid setting for `device/` in general.

## Scenario D — patch/replace the audio HAL or kernel driver via a custom recovery flash

Boot a custom recovery (TWRP or crown's `fastboot boot` equivalent — stock
recovery status unconfirmed, needs checking), and either (a) flash a patched
`audio.primary.mt8163.so`-equivalent HAL, or (b) flash a patched kernel
module for `mtk_pcm_I2S0dl1` that fixes the underflow/retry storm at the
driver level instead of working around it in our own daemon.

- **Investigation tasks**: does crown/checkers's board have a
  publicly-unlockable bootloader and a working TWRP port, or would this
  require building one from scratch (large undertaking, likely
  disproportionate)? Is the underlying MT8163 audio HAL source available
  anywhere (MediaTek's BSP release, or already patched by another
  device/ROM using the same SoC family)? This is the most invasive, highest
  time-cost option here and should only be pursued if A/B/C all prove
  insufficient.
- **Direction check**: flashing a MediaTek vendor HAL blob, even a patched
  one, is squarely the kind of dependency `CLAUDE.md` flags as "going the
  wrong way" for the project's stated direction, unless what actually gets
  fixed is open kernel driver code (postmarkOS's `amazon-biscuit` precedent
  is instructive here even though that's a different board) rather than a
  vendor binary. Worth being explicit in the findings about which of (a) or
  (b) an agent actually finds feasible, since they're not equally aligned
  with the project's direction.

## Scenario E — full custom ROM / LineageOS rebuild for crown specifically

Build a from-source LineageOS (or the postmarketOS-style approach already
used for the biscuit ALS diagnosis) targeting this exact device, with the
audio driver patched at the source level rather than the stock vendor image
patched live on-device.

- **Investigation tasks**: does a device tree already exist for this Echo
  Show 8 variant (check XDA, LineageOS gerrit, postmarketOS pmaports) or
  would one need to be built from the stock dump? What's the actual size of
  this undertaking relative to the payoff — this is almost certainly a
  multi-week task, not a fix.
- **Likely outcome**: probably too large to justify next, but worth having
  an agent spend a bounded amount of time (say, one search pass) confirming
  whether prior art already exists that would make this cheap, before ruling
  it out. Don't spend real build time here without checking that first.

## Scenario F — search for prior reports of the same instability

Handoff already found one XDA-reported symptom ("capture works once after
boot, then goes silent until `audioserver` restarts") that's plausibly the
same root fragility at a milder severity. Before designing any fix, worth
knowing whether this exact freeze — `mtk_pcm_I2S0dl1` underflow/retry loop
into a full hang — has been reported and solved anywhere: XDA threads for
this device/chipset, LineageOS bug tracker, MediaTek MT8163 kernel forks,
other Echo Show 8 rooting/ROM communities, or generic "Android tablet
freezes when two apps touch the speaker at once on MT816x" reports.

- **Investigation tasks**: targeted web search across those sources; if a
  known fix or workaround turns up (a kernel patch, a specific
  `audio_policy.conf` tweak, a known-bad-underlying-firmware-version note),
  report it verbatim with a source link rather than paraphrasing — this is
  exactly the kind of finding where the actual patch/commit matters more
  than a summary of it.
- **Do this one first, or in parallel with A/B** — it's cheap and could
  short-circuit a lot of the more expensive scenarios below it if a known
  fix already exists.

## Scenario G — mitigate rather than fix: detect and recover instead of prevent

If no clean prevention is found (or as a defense-in-depth alongside
whichever fix is chosen), have the daemon watch for the early warning sign
already captured in pstore (rapid `mUserCount` toggling / underflow storm on
its own PCM handle) and voluntarily release+reopen its own path, or back off
entirely for a cooldown window, before the DSP wedges.

- **Investigation tasks**: is the underflow/retry pattern observable from
  userspace *before* the freeze (kernel log via `dmesg`/`logcat`, or does it
  only show up after the fact in pstore)? If it's only visible post-hoc via
  pstore, this scenario isn't viable as *prevention* — only as faster
  detection/auto-recovery after a milder version of the event. Treat this as
  a fallback, not a primary fix — it doesn't address the actual contention,
  just tries to duck out of it faster.

## Suggested agent split

Run **F first** (cheap, might invalidate or redirect the rest). Then in
parallel: **A**, **B**, **C** as independent technical-feasibility spikes
(each should end with a clear recommendation + concrete cost estimate, not
just "this is possible"). Hold **D** and **E** unless A/B/C all come back
negative or infeasible — both are large enough that starting them
speculatively wastes real time. **G** is a fallback to write up regardless,
since some form of defensive recovery is worth having no matter which
primary fix (if any) gets chosen.

Every agent's report should end with an explicit recommendation
(pursue / deprioritize / dead end) and say what evidence would change that
verdict — not just describe the option.

## Verdicts (2026-08-26, first pass — F + A + B + C run in parallel)

| Scenario | Verdict | Why |
|---|---|---|
| **F** — prior art | No fix found anywhere | XDA confirms the milder "capture silent after first recording until `audioserver` restart" bug exists and is *unsolved* — corroborates the freeze doc's root-cause theory, but no patch/workaround for either the mild or severe form turned up on XDA, LineageOS gerrit, MT8163 kernel forks, or postmarketOS's echo port. Driver code (`mtk-soc-pcm-dl1-i2s0Dl1.c`) is public MediaTek `common_int/` code shared across the mt6761/mt8163 family, but no fix history exists for it anywhere searched. |
| **B** — shared/dmix ALSA path | **Dead** | No `dmix`/alsa-lib config anywhere in the codebase or (per architecture reasoning) on this class of Android device — `internal/alsa` talks straight to the raw `/dev/snd` node, and AudioFlinger's mixing happens *above* the HAL in userspace, not as a shared kernel node underneath it our raw open could join. Our exclusive open and mediaserver's HAL are just two competing exclusive owners of one node. One near-free hardware check left un-run (`adb shell cat /proc/asound/pcm`, looking for a second unused device index) but unlikely to overturn this. |
| **A** — stop holding DL1 open, open on demand | **Deprioritize** | Buildable (call boundary exists at the first per-stream `PumpPeriod`/`PumpMusic`), but two real problems: (1) `waitForFreePcm`'s existing timeout is up to 10s and *unbounded after that* — written for a one-time boot wait, would become a per-turn latency cliff exactly when mediaserver is contending; (2) the pstore evidence shows the storm was **mediaserver retrying against a permanently-held path**, not caused by repeated open/close on our side — shrinking our hold to per-turn narrows the collision *window*, it doesn't obviously remove the failure mode. Unproven either way without a live soak test under the YouTube-repro conditions. |
| **C** — route playback through Android's own audio stack (AudioTrack/AAudio) | **Pursue** (after A's soak test is tried; see below) | Two designs compared: (a) NDK AAudio/OpenSL ES via cgo directly in the Go binary — rejected, adds a load-bearing compiled ABI dependency with no fallback, a heavier and more novel form of coupling than anything else in `device/`; (b) a small `AudioTrack`-based Java service inside the existing `crown_launcher` APK, fed raw PCM by the Go daemon over a local socket — Go-side code otherwise unchanged, all new Android surface confined to the existing launcher isolation boundary. (b) is the recommended shape. Judged directionally acceptable for crown specifically (a real multi-app Android tablet where shared audio arbitration is the platform's actual model, unlike biscuit) but explicitly **not** a project-wide pattern — crown-only via build tag, biscuit's raw-ALSA path stays the reference for future non-Android boards. Real open costs: first structural (non-shell-out) Android dependency in `device/`; barge-in/duck latency across the new process boundary is unmeasured and needs a live probe before trusting it against the currently-proven raw-path barge-in behavior. |
| **D** — patch HAL/kernel via TWRP/custom recovery | Hold, weaker than before | F found no existing patch or fix history for this exact driver anywhere, meaning this would be true reverse-engineering of MediaTek's DSP resource management from zero prior art — bigger and riskier than the doc originally scoped it. |
| **E** — full custom LineageOS build | Hold, weaker than before | Same reasoning as D — no device tree, no prior art, nothing to build on. Not worth even the bounded prior-art spend originally suggested; F already did that search. |
| **G** — detect and self-recover | Still a fallback, worth keeping regardless | F's XDA finding (silent-after-first-recording, fixed by `audioserver` restart) shows this fragility already partially "self-heals" on other users' devices some of the time via exactly this kind of mechanism — reinforces it as a defense-in-depth, not a primary fix. |

**Net read**: B is closed for good. A is weaker than hoped — likely reduces frequency, not root cause. C is the strongest surviving lead and the direction actually being pursued next, pending the architecture discussion below and a live latency/soak probe on hardware. D/E stay parked.

## Scenario C — proven / disproven so far (2026-08-26, live on hardware)

Full design in `docs/echo-show-8-audiotrack-design.md`. Status of its three
named open questions:

- **Q1 (socket permissions across both launch paths) — PROVEN, by code
  inspection, no device needed.** `ServerService` exec's the daemon at the
  service's own uid; a filesystem-namespace socket under the app's private
  (`0700`) dir is reachable by a same-uid child with zero permission
  changes, and the root-shell-exec launch path bypasses DAC checks
  entirely regardless. No ueventd-style fix required, unlike the earlier
  `/dev/snd` node issue.
- **Q2 (does this SoC/ROM actually grant AudioTrack low-latency mode) —
  PROVEN LIVE.** Ran the throwaway `AudioProbeReceiver` on
  `G0916D10014507JS`: `getPerformanceMode()` returned
  `PERFORMANCE_MODE_LOW_LATENCY` exactly as requested (not a silent
  fallback to the shared/legacy mixer), native fast-mixer period is 16ms
  (`PROPERTY_OUTPUT_FRAMES_PER_BUFFER=768` @ 48kHz), and the test tone was
  written successfully and **audibly heard** on the real device. This
  clears the single biggest risk to Scenario C — the worst case (silent
  legacy-path fallback making barge-in worse) is ruled out. Full log and
  detail in the design doc.
- **Q3 (failure handling design) — designed, not yet load-tested.**
  Write-deadline-per-period + reconnect-with-backoff on the Go side,
  single-connection-accept + recreate-on-error on the Java side. Sound on
  paper; only a real implementation under the YouTube-concurrent repro will
  confirm it degrades to dropped audio rather than a hang.

**Not yet proven**: end-to-end barge-in/duck latency through the real
mixer→socket→AudioFlinger path (the probe only proves the *mode* is
available, not the full round-trip cost).

### The contention test — run live, and it survived

Ran the actual repro this whole investigation is about: browser
audio/video playing on the device, with 20 rounds of the `AudioTrack`
probe fired concurrently over ~50 seconds (`adb shell am broadcast` in a
loop, `adb shell echo alive` checked after every round).

**Result: no freeze, no pstore write, device fully responsive throughout.**
The user heard the probe's beep mixing audibly with the browser's audio —
confirming two live audio clients were genuinely contending for output at
the same time, not just running back-to-back. `dmesg` afterwards showed the
DL1 driver behaving completely differently from the freeze evidence: each
probe write produced one clean, orderly open→underflow(expected, short
buffer)→stop→close cycle (`mUserCount` moving 0→1→0 in sequence) —
**not** the freeze pstore capture's signature of dozens of rapid 1↔2
toggles within ~124ms. Same kernel driver, qualitatively different traffic
pattern, no sign of the storm that precedes a freeze.

This is the strongest evidence yet for Scenario C: routing playback through
`AudioTrack` instead of raw `/dev/snd` appears to remove the actual
collision condition, not just reduce its frequency (contrast with
Scenario A, where the same open/close pattern on the *raw* ALSA path was
flagged as plausibly reproducing the storm). Not yet a full proof — this
was the throwaway probe (short tone bursts), not the real socket-fed
service under a full voice turn — but it's real concurrent-load evidence
on real hardware, not a hypothesis.

**Next to actually close this out**: build the real `LocalServerSocket` +
`AudioTrack` service per the design doc, wire `pcm_speaker_crown.go`'s
output to it, and re-run this exact contention test against full voice
turns (not just probe tones) — including a long soak, not just 50 seconds,
since freeze #2 in the original handoff had no confirmed trigger at all and
a short clean run doesn't rule out a rarer failure mode.
