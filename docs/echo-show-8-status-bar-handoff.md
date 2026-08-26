# crown status-bar + fresh-provisioning handoff (2026-08-26, end of session)

Written for picking this back up cold. Supersedes nothing — the AudioTrack
freeze fix (`docs/echo-show-8-audiotrack-handoff.md`) is separate and still
fully valid; this is the session after it, same day.

## What shipped tonight

1. **AudioFocus ducking fix + provisioning/probe-receiver cleanup** — see
   `docs/echo-show-8-audiotrack-handoff.md`'s updated "What's NOT done"
   section, all closed.
2. **Dashboard: crown gets its own device icon** (`ScreenRing`, screen body
   instead of `LedRing`'s puck), and `device.model` now persists to the DB
   (schema v20) instead of vanishing when a device goes offline. Deployed
   live to thinkpad's controller (dev-build swap, see below) and confirmed
   rendering correctly.
3. **On-device status indicator — the real news.** crown has no LED ring
   (`led_crown.go`'s controller was a deliberate no-op). Built one:
   `StatusOverlay` in `crown_launcher/` draws a glowing, pulsing bar along
   the screen's top edge over whatever app has focus, fed by
   `led_crown.go`'s `overlayController` over a new Unix socket
   (`com.echomuse.crownlauncher/led`), same shape as `PlaybackServer` for
   audio. Also wakes the screen for the duration of a turn (`WakeLock`,
   acquire on colour-on, release on colour-off, 180s dead-man cap).

   Went through three visual shapes live before landing on the bar — full
   detail and the reasoning for each is in commit `229c12c`'s message,
   which is long on purpose; read it before changing this file further.

## What's verified vs NOT

**Verified live tonight:**
- `go vet`/`go test -tags crown` clean, `./compile.sh crown` clean
- APK rebuilds clean, installs, `EchoMuseOverlay` logs "listening" +
  "daemon connected" on every boot
- `SYSTEM_ALERT_WINDOW` granted via `appops set` survives an ordinary
  reboot — confirmed by rebooting twice with the overlay attaching cleanly
  both times, no `BadTokenException`

**NOT verified — first thing to check tomorrow:**
- An actual wake-word turn with the bar visibly lighting and pulsing.
  Never triggered one after the last rebuild — ran out of session time.
- The wake-from-locked-screen behaviour specifically (lock the screen,
  trigger a turn, confirm it wakes and the bar is visible). This is the
  feature the user actually asked for last — code is in place
  (`WakeLock` in `StatusOverlay.applyColor`) but zero live confirmation.
- Whether `ledListenColor`/`ledThinkColor` custom colours (Config → Ring)
  actually reach the strip. Reasoned through why they should (crown falls
  back to the legacy streamed-frames path since it doesn't advertise
  `led_anim`) but never watched it happen.

## The appops trap — read this before re-provisioning tomorrow

**`SYSTEM_ALERT_WINDOW` granted via `appops set` does NOT survive a
reinstall.** Confirmed live: install → grant → confirmed `allow` →
reboot → read back as `default` (reset). The grant DOES survive an
ordinary reboot with no reinstall in between — confirmed separately,
twice.

This means **provisioning order matters and is already correct in
`provision_crown.sh`**: install APK, THEN grant appops (never the other
way around), and the grant step is unconditional, not gated on "if not
already granted" — because a fresh install always needs it, and running
it again on an already-granted device is a no-op. But it also means: if
tomorrow's fresh-install test does the APK install more than once (e.g.
retry after a failure, or a `-r` reinstall for any reason), **the appops
grant must be re-run after the LAST install**, or the overlay silently
never appears (logs `BadTokenException`, everything else — mic, speaker,
wake word — keeps working fine, so this is easy to miss).

## Tomorrow's plan, as stated by the user tonight

1. **Fresh install end to end** — wipe/re-provision a device from
   scratch using `provision_crown.sh` as it exists on this branch right
   now, and confirm every step actually works in sequence (not just that
   each piece works in isolation, which is all that's been tested so
   far). This is the first real test of the appops-after-install
   ordering above under real conditions.
2. **Confirm the "setup" process includes crown's steps.** Unclear yet
   exactly what "the setup process" refers to — likely the **dashboard's
   provisioning wizard** (`dashboard.jsx`'s `_WIZARD_STEPS`, documented
   in `controller/CLAUDE.md`'s "Provisioning wizard" section), which as
   of tonight has **no crown-specific path at all** — it was built
   entirely around biscuit (Echo Dot) and its Amazon-OOBE-bypass,
   debloat, and `pm`-based steps, none of which apply to crown (no OOBE
   to race, no Alexa stack to disable, LineageOS not stock FireOS). Needs
   scoping tomorrow: does crown get its own wizard path, or does
   `provision_crown.sh` stay a separate standalone script (as it is now)
   with the wizard left biscuit-only? Worth raising with the user rather
   than assuming — this is a real fork in how much work tomorrow is.

## Live device state as of writing this

- Device: crown test unit, serial `G0916D10014507JS`, running the
  status-bar build, believed working but the last two features
  (wake-from-lock, live pulse) are unconfirmed per above.
- `crown_launcher.apk`: latest build installed, `SYSTEM_ALERT_WINDOW`
  granted, survived the last reboot cleanly.
- `/data/local/bin/server`: crown-tagged build `20260826-2201-dev` (or
  later if rebuilt again — check the `[control] Registered as ...`
  line in `/data/local/tmp/echomuse.log` for the exact version).
- Controller: thinkpad's `echomuse-controller` container is running a
  **temporary local dev-build swap** (`echomuse-controller:crown-dev`,
  built from this branch), NOT the published image
  `docker-compose.deploy.yml` normally pulls. This is intentional per
  tonight's session but is easy to forget — a `docker compose pull`
  against the deploy compose file would silently revert to the old image
  and old model label. The dev-build compose override lives at
  `/home/jay/echomuse/docker-compose.crown-dev.yml` on thinkpad; the
  source checkout used to build it is
  `/home/jay/projects/EchoMuse-crown-fix` on thinkpad, tracking this
  branch.
- Repo: `echo-show-8-support`, pushed to `origin` (your fork,
  `dweng0/EchoMuse`) through commit `229c12c`. Not opened as a PR
  anywhere yet — still an open item from the previous handoff doc too.
