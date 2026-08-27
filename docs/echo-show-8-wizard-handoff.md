# crown provisioning wizard — handoff (2026-08-27, end of session)

Written for picking this back up cold. Supersedes nothing — the status-bar
handoff (`docs/echo-show-8-status-bar-handoff.md`) and the AudioTrack/freeze
handoffs are separate and still valid; this covers today's session only,
which was entirely about getting a browser-based provisioning path for crown
and testing it against a genuinely fresh device for the first time.

## What shipped today

1. **`CrownProvisionWizard`** (`controller/static/dashboard.jsx`) — a new,
   separate wizard component (not a device-type branch through the existing
   biscuit `ProvisionWizard`) implementing the same six steps
   `provision_crown.sh` already does: connect + `adb root`, install server
   binary, TLS credentials, install launcher APK, grant overlay permission,
   start. A device-type picker now shows ahead of both wizards.
2. **`_ADB.Client.root()` / `reconnectSilent()`** — the WebUSB ADB layer's
   equivalent of `adb root` (open the real `root:` service, then reconnect to
   the same already-permitted USB device with no picker, matching real
   `adb`'s own silent-reconnect behaviour).
3. **On-failure diagnostics for crown** (`_CROWN_PROVISION_PROBES` in both
   `em_support.py` and `dashboard.jsx`, pinned together by test) — mirrors
   biscuit's existing on-failure probe capture, with crown's own shorter
   allowlist (no TWRP/Magisk/Alexa/WiFi surface to probe) plus two
   crown-specific reads: the ueventd device-node patch and the launcher's own
   log.
4. **Two real, previously-invisible bugs found and fixed by actually running
   this against hardware** (full account in `JOURNAL.md` and
   `docs/echo-show-8-journal.md`, 2026-08-27 entries):
   - The wizard's ADB authentication had never worked, on **either** device,
     ever — `Transport.authenticate()` was never given a `credentialStore`.
     Fixed with a minimal `_WebCredentialStore` (WebCrypto + `localStorage`).
   - `ServerService`'s exec of the daemon silently fails on a genuinely fresh
     device: `/data/local/tmp` is `drwxrwx--x`, the app's own uid can open an
     existing file there but can't create one, so the log-redirect throws and
     is swallowed with no crash, no log line, nothing visibly wrong. Fixed by
     pre-creating the log file before starting, in both the wizard and
     `provision_crown.sh` (which had also independently drifted back to a
     known-bad `am start-service`, now corrected to
     `am start-foreground-service`).

## What's verified vs NOT

**Verified live, twice, from a genuine factory reset (not a patched-up test
unit):**
- Full wizard run start to finish with zero manual `adb shell` intervention
- `go vet`/`pytest controller/tests/` clean throughout (735 passed, 3
  skipped), `esbuild` syntax-checks `dashboard.jsx` clean at every step
- Daemon starts, registers, connects over `wss://` with TLS credentials
  installed, speaker path initialises correctly

**NOT verified — the actual next thing to chase:**
- **Wake word does not fire at all**, on either `hey_jarvis` or the custom
  `Winston` model, and it isn't a threshold/model-selection problem — the
  controller log shows `OWW: no mic frames for 10s on the continuous wake
  stream` repeating on every single connect, escalating to a forced
  disconnect ~90–120s in. Mic frames are not reaching the controller AT ALL.
  Full detail and the suggested next steps (in order) are in
  `docs/echo-show-8-journal.md`'s final entry — start there, not from
  scratch. **This means the full voice loop is not re-confirmed since the
  crown mic/speaker rewrite** — only speaker output and registration are
  proven on this exact provisioning path so far.
- Whether the recurring control-plane keepalive timeout is the SAME root
  cause as the missing mic frames (a hung capture path stalling the process
  enough to miss its own pings) or a coincidence — reasoned about, not yet
  investigated.
- HA was not pointed at this device this session (a real but separate
  integration step, not a bug — see the 2026-08-26 entry's identical false
  alarm) — irrelevant to the mic-frames issue since wake scoring is
  controller-side and doesn't need HA at all, but still needs doing before an
  actual end-to-end assist turn can be tested.

## Live device state as of writing this

- Device: crown test unit, serial `G0916D10014507JS`, freshly provisioned via
  the wizard, currently connected to the controller (label "Door" — the last
  of several re-provisions this session, each deleted and redone).
- `crown_launcher.apk` + `/data/local/bin/server` (`20260826-2201-dev`):
  installed and running via the launcher path, confirmed via
  `/data/local/tmp/echomuse.log`.
- Controller: thinkpad's `echomuse-controller` container running the
  `echomuse-controller:crown-dev` local dev-build image, rebuilt from
  `/home/jay/projects/EchoMuse-crown-fix` (branch `echo-show-8-support`,
  fast-forwarded to `749b7e3` as of this session) — same temporary swap
  documented in the previous handoff, still not the published image.
- Repo: `echo-show-8-support`, pushed to `origin` through `749b7e3`. Still not
  opened as a PR.

## Suggested next steps, in order

1. Pick up the mic-frames investigation exactly where
   `docs/echo-show-8-journal.md`'s last entry leaves it — capture the device
   log from a fresh connect specifically watching for whether
   `PcmMicrophone` (or equivalent) ever logs an init line at all.
2. Once wake word fires again, point HA at this device's freshly-allocated
   ESPHome port (one-time integration step) and confirm a real end-to-end
   assist turn.
3. Only after both of those: pick back up the still-unconfirmed status-bar
   items from the previous handoff (live wake-turn lighting the bar,
   wake-from-lock, custom LED colours) — none of that can be tested without
   wake word working first.
4. Open the branch as a PR — repeatedly flagged as outstanding across three
   consecutive handoff docs now.
