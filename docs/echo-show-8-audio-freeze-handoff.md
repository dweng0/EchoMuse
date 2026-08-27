# crown audio freeze — handoff (2026-08-26)

Written for picking this back up cold. Read `docs/echo-show-8-hardware-map.md`
and the 2026-08-26 entries in `docs/echo-show-8-journal.md` first — this doc
assumes that context and only covers the freeze investigation in detail.

## The problem, one line

**crown (Echo Show 8) hard-freezes the whole device — not just the daemon,
the entire Android system, needs a physical power-cycle — when the EchoMuse
audio pipeline and Android's own audio stack (`mediaserver`) both touch the
speaker hardware at or near the same time.** Confirmed to happen regardless
of how the daemon was launched (app-exec'd via the new launcher APK, or a
plain `adb root` shell exec — the exact method that "worked fine" on every
earlier session). This is new: it never came up before today because nobody
had previously run the daemon continuously *while also* using the device as
an ordinary Android tablet (playing something through the browser, etc.).

**This is very likely a design-level conflict, not a one-line bug.** crown's
speaker binding (`pcm_speaker_crown.go`) deliberately holds the playback PCM
device open and streaming *continuously* (a permanent "silence stream", for
low playback latency — see its `silenceLoop` and the package comment). That
is fine in isolation and mirrors biscuit's approach. The difference is that
biscuit (Echo Dot Gen 2, FireOS 5) is a single-purpose appliance where
nothing else on the box competes for the speaker; crown is a real Android 11
tablet where `mediaserver` legitimately expects to be able to grab the same
hardware whenever any app plays audio. Holding it open exclusively and
permanently puts EchoMuse in a standing conflict with that.

## What is CONFIRMED fixed and working (do not re-litigate these)

Everything below was tested live on hardware today and is solid:

1. **Raw `init.rc` cannot exec `/data/local` binaries on crown at all** —
   `service echomuse /data/local/bin/server` in `init.rc` fails every time,
   "Permission denied", exit 127, independent of SELinux mode (confirmed
   Permissive). Hard Android restriction, not a permissions/SELinux issue we
   can fix. biscuit never hits this because `start_server.sh` runs via
   Magisk's `service.d`, sidestepping raw init; crown has real root
   (`adb root`, userdebug) but no Magisk at all, so that escape hatch does
   not exist here. Already documented as its own commit before this session
   started (`15250d3`).

2. **`device/crown_launcher/` — a minimal Android launcher APK** now exists
   and works: `BootReceiver` (BOOT_COMPLETED) → `ServerService`
   (foreground service) → execs `/data/local/bin/server`, `START_STICKY`.
   Built with **no gradle, no Android Studio** — `device/crown_launcher/build.sh`
   goes `aapt2 link` → `javac` → `d8` → `zipalign` → `apksigner`, against a
   minimal SDK at `device/.android-sdk` (cmdline-tools + `platforms;android-30`
   + `build-tools;30.0.3` — crown reports API 30 live). Untracked in git
   currently (`device/crown_launcher/`, `device/.android-sdk/` both show as
   `??` in `git status`) — **the SDK directory should probably be gitignored,
   not committed** (it's a downloaded toolchain, ~150MB+); the launcher
   source/build.sh should be committed.

3. **Fixed: a freshly-installed app never gets `BOOT_COMPLETED`.** Android
   withholds implicit broadcasts from an app in the "stopped" package state
   until it has run at least once (confirmed live: installed, rebooted,
   nothing started; `dumpsys package` showed `stopped=true`). Fix: run
   `adb shell am start-foreground-service -n com.echomuse.crownlauncher/.ServerService`
   once at install/provision time — this both starts it immediately AND
   permanently clears the stopped flag so future real reboots work. This is
   now in `device/scripts/provision_crown.sh` (modified, uncommitted). The
   old dead raw-`init.rc` install step was removed from that script entirely
   since step 1 above proved it does nothing.

4. **Fixed: `/dev/snd/*` and `/dev/input/*` permission denied for the
   app-launched process.** The exec'd binary runs as the launcher app's own
   sandboxed uid (`u0_a148`), not root. Those device nodes are
   `system:audio` / `root:input`, mode `0660` — no group for a third-party
   app uid. **Two things that do NOT fix this, confirmed by testing, in case
   they get re-proposed:**
   - Granting `android.permission.RECORD_AUDIO` does nothing — confirmed via
     `adb shell id u0_a148` showing no `audio` gid even after `pm grant`.
     That permission gates the Binder/`AudioRecord` path; this daemon opens
     `/dev/snd` directly (tinyalsa-equivalent), which isn't mediated by it.
   - Making the launcher a `/vendor/priv-app` privileged app would not
     help either (reasoned through, not built) — privileged-app allowlisting
     only ungates Android *permission* checks, not Linux device-node gid
     membership. Real system audio processes get the `audio` gid from a
     build-time AID baked into `android_filesystem_config.h`; a pushed APK
     cannot acquire that after the fact.
   - **Actual fix**: crown has no separate vendor partition (`/vendor` is a
     symlink to `/system/vendor`, confirmed via `stat`), so the live
     ueventd rule is in `/system/etc/ueventd.rc`. Patched
     `/dev/snd/*` and `/dev/input/*` from mode `0660` to `0666` directly on
     the live device (`adb remount` + edit + `adb push`). Original backed up
     at `device/crown_launcher/ueventd.rc.crown-orig` (untracked). Survives
     a normal reboot and even a factory reset (both only touch `/data`);
     will NOT survive a ROM/vendor reflash. **This has not been formalized
     into a repeatable provisioning step** — it was a one-off manual edit
     directly on the test unit. If this device gets reflashed, someone needs
     to redo this by hand, or (better, not done) add it as a step to
     `provision_crown.sh`.

5. **Fixed: missing `android.permission.INTERNET`.** Without it, the app's
   uid has no `inet` gid (3003), and Android's netfilter owner-match rules
   silently drop **every** `socket()` call for that uid at the kernel level
   — not just from the Java side, from the exec'd native child too, since it
   inherits the uid. This was the actual reason mDNS discovery (and would
   have been the controller WebSocket dial) never worked from the launcher
   path. Confirmed via `tcpdump` on the device's own `wlan0`: the
   controller's periodic mDNS announce arrived fine (ruling out the network),
   but a 25s targeted capture for **outbound** traffic from the device's own
   IP on port 5353 showed zero packets. Added the permission to
   `device/crown_launcher/AndroidManifest.xml`, confirmed
   `dumpsys package ... | grep gids` now shows `gids=[3003]`. This one is
   committed in the sense that it's in the (untracked, not-yet-committed)
   `AndroidManifest.xml` — **still needs a real `git add` + commit**, it is
   currently sitting as an untracked new file only.

6. **End-to-end confirmed working from the app-launched path** (before the
   freeze problem below was found): registered with the controller, config
   pushed, `Winston` OWW model loaded, mic streaming, one full voice turn
   completed (wake → barge watcher → turn complete), all with zero manual
   scripts — purely BOOT_COMPLETED → exec.

## The freeze — what's known, in order of confidence

**Confirmed real, not a testing artifact.** The device has hard-frozen (no
adb, no USB re-enumeration recovering it, needs a physical power-cycle) at
least **four** separate times today:

1. Right after `adb reboot` was issued while the daemon was actively
   streaming through the AEC pipeline (self-inflicted: Android's shutdown
   sequence may have stalled on a raw ALSA fd our exec'd process never
   released cleanly — it has no real Android service lifecycle, no
   `onDestroy`-equivalent hook for a reboot signal).
2. Spontaneously, sometime after a normal boot had already settled — no
   reboot in flight, **no confirmed concurrent trigger of any kind**. This
   one is the reason "needs YouTube playing" cannot be the whole story.
3. Within ~15 seconds of the daemon being (re-)started via the launcher APK,
   coincident with the user actively playing YouTube in the browser
   (`mediaserver` actively live).
4. Within seconds of the user **opening the browser** (not yet confirmed
   playing anything — "crashed while i opened the browser") — this happened
   with the daemon running via a **plain `adb root` shell exec**, the exact
   launch method that has "always worked" in every prior session. This is
   the most important data point: **it rules out the launcher APK / app uid
   as the cause.** The freeze happens with the old, previously-trusted
   launch method too, the moment Android's own audio-capable app stack gets
   exercised while the daemon is running.

**Root cause hypothesis (not yet proven, but the strongest lead so far)**:
pulled `/sys/fs/pstore/console-ramoops` (pstore/ramoops survives a hard
power-cycle — confirmed present on this device) immediately after freeze #3
and got real kernel console output from milliseconds before the hang:

```
[  591.115007] +SetIrqEnable(), Irqmode = 0, bEnable = 0
[  591.115637] SetMemoryPathEnable Aud_block = 0 mUserCount = 1 mState = 1
[  591.116462] ClearMemBlock MemBlock = 0
[  591.116933] ClearMemBlock MemBlock 0 reset done
[  591.120556] +SetIrqEnable(), Irqmode = 0, bEnable = 1
[  591.121186] SetMemoryPathEnable Aud_block = 0 mUserCount = 2 mState = 1
[  591.122108] mtk_pcm_I2S0dl1_pointer underflow
[  591.122651] mtk_pcm_I2S0dl1_stop
... (repeats dozens of times over ~124ms, mUserCount toggling 1<->2 each time)
```

This is the MT8163's `mtk_pcm_I2S0dl1` driver — the playback/"downlink 1"
I2S path — being opened, underflowing immediately, and closed, **in a tight
loop, dozens of times within about 150ms**, right before the freeze. This
pattern (`mUserCount` toggling 1→2→1 rapidly) reads as two different clients
fighting to open the same physical playback path: our daemon already holds
it open continuously (`silenceLoop` in `pcm_speaker_crown.go` opens it once
in `Init()` and never closes it — confirmed by reading that code, it is
**not** the one doing the rapid open/close), and something else — almost
certainly `mediaserver`'s own HAL — is repeatedly trying to also open it,
failing/getting kicked, and retrying fast enough to thrash the DSP's shared
memory-block allocator (`ClearMemBlock`/`SetMemoryPathEnable` are low-level
DSP resource management calls) into a wedged state hard enough to freeze the
whole SoC, not just the audio subsystem.

**This would explain all four freezes as one underlying issue**: our
daemon holding the DL1 path open permanently is fundamentally incompatible
with this being a real Android device other software also expects to use
the speaker on. Freeze #3 had an obvious trigger (YouTube). Freeze #4 needed
only "open the browser" — plausibly enough for Android to make some incidental
audio-stack touch (a UI sound, a focus-check, background HAL activity)
without any video actually playing. Freeze #2's total lack of a visible
trigger is also consistent with this — Android's own audio stack can poke
the hardware on its own schedule without the user doing anything that looks
like "playing audio."

**What is NOT the cause, ruled out with evidence:**
- Not the app/launcher-uid vs root distinction — freeze #4 happened via the
  same root-shell-exec method that "always worked" before.
- Not `stop media`/`stop mixer` needing root and failing silently — checked
  the actual crown-specific bindings (`pcm_speaker_crown.go`,
  `pcm_microphone_crown.go`); **neither one calls `stop` on anything.** A
  comment in the mic binding already documents that `stop mixer` returns
  "exit status 1" on crown (no such service exists) and explicitly says
  "nothing in the audioserver family may ever be added here" — deliberate,
  because stopping it on real Android 11 breaks the system rather than
  freeing the audio path the way the equivalent trick works on FireOS.
- Not the ueventd `/dev/snd`+`/dev/input` permission changes — a full clean
  boot happened successfully with that same file in place before the first
  freeze; ruled out by ordering.
- **Not fully proven yet**: whether this is *specifically* the DL1/playback
  path, or also implicates the capture (mic) path opened by
  `pcm_microphone_crown.go` — the pstore snippet only shows the DL1
  (playback) driver thrashing, but a capture-side conflict hasn't been
  separately captured/ruled out.

## What is already known and pre-dates this session (context, not new)

`docs/echo-show-8-hardware-map.md` already documents: *"Known LineageOS bug:
capture works for the first recording after boot, then goes silent until
`audioserver` restarts (reported on XDA)."* This freeze is very plausibly the
**same underlying fragility**, just escalating from "goes silent" (the
XDA-reported symptom, presumably observed by people not holding the device
open continuously the way our daemon does) to "hangs the whole board" under
real sustained contention. Worth treating as the same root platform issue,
not a coincidence.

## What was tried as a live isolation test, and its result

1. Disabled the launcher app entirely (`pm disable`), left the device
   completely idle (nothing of ours running) for **11 minutes straight** —
   completely stable, no drops, no re-enumeration.
2. Re-enabled the launcher, it froze within ~15 seconds, coincident with
   YouTube playing. This ruled the launcher mechanism itself IN as fine
   (11 clean minutes without it) and pointed at the daemon's continuous
   audio use as the live suspect.
3. As a further control (**in progress when context was cut**): disabled
   the launcher again, started the daemon manually via `adb root` shell
   exec instead (the pre-APK, "always worked" method) to check whether the
   freeze is really launch-method-independent. **Result: froze again**,
   within moments of the user opening the browser. This is the strongest
   evidence yet that the freeze is a genuine platform/hardware-contention
   issue, not anything introduced by today's launcher/APK work.

## Diagnostic capability now available, use it

`/sys/fs/pstore/console-ramoops` and `/sys/fs/pstore/pmsg-ramoops-0` exist on
this device and survive a hard power-cycle. **Read these immediately after
any future freeze, before doing anything else** (including before letting
the device fully reboot again if avoidable — Android's pstore service may
consume/clear entries on boot):

```bash
adb root && adb wait-for-device
adb shell cat /sys/fs/pstore/console-ramoops
adb shell cat /sys/fs/pstore/pmsg-ramoops-0
```

A second capture attempt (triggered by freeze #4, the browser-open one) came
back **empty**: `/sys/fs/pstore/console-ramoops` no longer existed by the
time `adb` reconnected after the power-cycle. Two possible explanations,
neither confirmed:

- Android's own pstore-reading service consumed/cleared the file early in
  boot before this session's `adb root && cat` won the race (the risk this
  doc already flagged — read pstore *immediately*, ideally before the boot
  animation finishes).
- Or freeze #4's recovery didn't populate ramoops at all — ramoops/pstore
  typically only writes on a genuine kernel panic or watchdog-triggered
  reset, not on every hang. If the physical recovery this time (however the
  device was power-cycled) was gentler than whatever happened for freeze #3
  — e.g. a watchdog eventually kicking in and rebooting on its own vs. a
  true held-power hard cut — there may be nothing for it to have captured.

**So freeze #3 is still the only pstore evidence in hand.** It is a single
data point, not yet corroborated by a second capture. Whoever picks this up
should try again on the next freeze, reading pstore as fast as possible
after the device is reachable again — and if it comes back empty a second
time, that itself is informative (points toward "not every freeze is a
kernel-panic-with-crash-dump," which would mean some freezes are a silent
deadlock the SoC's own watchdog isn't even catching, a worse situation than
"panics and recovers").

## Live device state as of writing this

- Device: crown test unit, serial `G0916D10014507JS`, currently **frozen**
  (freeze #4), needs a physical power-cycle to recover.
- Controller: running on host `thinkpad` (SSH alias, `192.168.1.120`),
  container name `echomuse-controller`, host networking, `.env` created
  this session with `DEVICE_APPROVAL=auto` and `SERVER_IP=192.168.1.122`
  (this dev machine's LAN IP — **irrelevant/leftover**, since the real
  controller in use is the one already running on `thinkpad`, not a locally
  built one; a local build was attempted and failed harmlessly — no GPU
  driver — no local container is running).
- crown_launcher app: currently **disabled** on-device
  (`pm disable com.echomuse.crownlauncher`) as part of the isolation test in
  progress.
- Repo state: `JOURNAL.md` and `device/scripts/provision_crown.sh` modified
  but uncommitted; `device/crown_launcher/` and `device/.android-sdk/`
  present but untracked. **Nothing from today has been committed.**

## Suggested next steps, in order

1. Power-cycle the device, immediately pull both pstore files, compare
   against the freeze #3 capture. Confirm or refute the DL1-contention
   theory with a second data point.
2. If confirmed: this needs a real design decision, not a quick patch — the
   options are (a) stop holding the speaker path open permanently, opening
   only when actually about to play something (costs first-sound latency,
   removes the standing conflict), or (b) find whether this board exposes
   any shared/software-mixed ALSA path (dmix-equivalent) that would let both
   our daemon and `mediaserver` hold the device concurrently without
   fighting for exclusive access — not yet investigated, may not exist on
   this driver at all. The user's instinct (quoted from this session):
   *"this feels like a huge divergence from how the echo dot does it"* — worth
   taking seriously; biscuit's model of exclusive continuous ownership may
   just not be portable to a device Android itself needs to share.
3. Separately and lower-priority: formalize the ueventd `/system/etc/ueventd.rc`
   patch into `provision_crown.sh` (or document it as a required manual step)
   so a reflashed or new crown unit doesn't silently regress on mic/button
   access.
4. Commit today's actual working fixes (`crown_launcher/`, the
   `provision_crown.sh` changes, `JOURNAL.md`) once the freeze investigation
   isn't actively in-flight — right now the tree is uncommitted and mid-test,
   deliberately left that way in case any of it needs to change based on
   what the next pstore capture shows.
5. Add `device/.android-sdk/` to `.gitignore` before committing anything —
   it's a downloaded toolchain, not source.
