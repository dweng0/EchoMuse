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

**Verified live, from a genuine factory reset (not a patched-up test unit),
including the full voice loop:**
- Full wizard run start to finish with zero manual `adb shell` intervention,
  repeated twice
- `go vet`/`pytest controller/tests/` clean throughout (735 passed, 3
  skipped), `esbuild` syntax-checks `dashboard.jsx` clean at every step
- Daemon starts, registers, connects over `wss://` with TLS credentials
  installed, speaker path initialises correctly
- **A real end-to-end voice turn**: wake word detected (`score=0.727`), STT
  transcript `'What is the time?'`, TTS generated and played back,
  device-confirmed. The mic-frames scare below turned out to be testing
  aimed at the wrong device, not a bug — see the correction.

**Resolved, not a bug** — recorded here because it produced real, confusing
symptoms and is worth knowing about if it looks like it's recurring:
earlier in this session, wake word appeared to never fire (`hey_jarvis` and
`Winston` both silent, controller logging `OWW: no mic frames for 10s`
repeating on every connect) after several factory-reset test cycles. Root
cause was **testing setup, not the mic pipeline**: the user was speaking near
an old Echo Dot in the same room, and HA/ESPHome was pointed at the wrong
device entity while being configured. Once both were corrected, the very
next attempt worked cleanly. Full account, including what's still a loose
end (the "no mic frames" log lines themselves weren't fully explained by the
wrong-device testing and didn't reproduce on the working connection), is in
`docs/echo-show-8-journal.md`'s final two entries — read the resolution
entry, not just the investigation that preceded it.

**Added as a side effect of chasing this** (kept regardless, since they're
useful diagnostics independent of whether this was ever a real bug):
`PcmMicrophone (crown) initialised` log line (previously absent — the
speaker binding had one, the mic binding didn't), and rate-limited
EPIPE-recovery counters in `internal/alsa`'s `Read`/`Write` (previously
completely silent, which would have hidden a genuine thrashing-recovery
failure mode indistinguishable from "healthy" in the log).

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

1. Pick back up the still-unconfirmed status-bar items from the previous
   handoff (live wake-turn lighting the bar, wake-from-lock, custom LED
   colours) — now unblocked, since wake word and the full voice loop are
   confirmed working.
2. If `OWW: no mic frames for 10s` ever recurs, it's a real loose end worth
   a proper look (see the journal's resolution entry) — the two new log
   lines added this session should make it much faster to root-cause.
3. Open the branch as a PR — repeatedly flagged as outstanding across three
   consecutive handoff docs now, and there is no longer an open bug blocking
   it.
4. A same-day review (spawned 2026-08-27, four personas) surfaced several
   independent findings across architecture/testing/security/hardware —
   some fixed same-session (ADR-0002 amended, `capabilities_crown.go`'s
   `display` capability + `DeviceIcon`'s model-regex replaced, evdev button
   nodes resolved by name, a `-tags crown` CI job added), some still open
   (unauthenticated launcher sockets, `ServerService` not supervising its
   child process, the ADB private key's indefinite `localStorage` lifetime,
   a couple of test-coverage gaps). None of the open ones are blocking; see
   the review's findings in this session's conversation log for the full
   list if picking them up.
