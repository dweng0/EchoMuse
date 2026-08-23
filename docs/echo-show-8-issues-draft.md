# DRAFT — Echo Show 8 issue breakdown (review before publishing)

Tracer-bullet slices from `echo-show-8-plan.md`, dependency-ordered. Target:
**dweng0/EchoMuse** (fork), not upstream. HITL = needs the real device / a human;
AFK = an agent can do it unattended.

1. **Hardware Map for the Show 8** — HITL — blocked by: none
   Discovery on the real device: ALSA card/device + format for mic and speaker,
   input-device paths for buttons and mute, arch/API level, autostart mechanism.
   Output: a docs file. Unblocks every binding.

2. **`hoya` build target + cross-compile toolchain** — HITL — blocked by: 1
   New build tag + compiler image for LineageOS's API level; produces an
   installable arm binary. Verified by it running on the device.

3. **Provisioning script: install + autostart** — HITL — blocked by: 2
   One script: adb-push binary + TLS creds, set auto-start. Run by hand for now.

4. **Device connects & is recognised as "Echo Show 8" in HA** — HITL — blocked by: 3
   Reuse TLS/client unchanged; announce minimal capabilities; add the decorative
   model label across Go + Python + the capability-match test. Appears as pending
   in the dashboard and discoverable in Home Assistant. (This is the literal
   "recognise Echo 8".)

5. **Speaker binding: hear a spoken test through the Show** — HITL — blocked by: 4
   Implement `speaker.Speaker` (selfish grab) against the Hardware Map. Announce
   `speaker`. Push-TTS / audio test plays through the Show's speaker.

6. **Mic binding: wake → Assist → spoken reply** — HITL — blocked by: 5
   Implement `mic.Microphone`, single channel, no beamforming. Announce `mic`.
   Controller-side wake word → Assist → reply. **The "it works" milestone.**

7. **Buttons + mute + platform glue** — AFK(mostly) — blocked by: 4
   `buttons.Controller` (vol ±, mute); replace FireOS-only `main()` glue with
   LineageOS equivalents / no-ops behind the tag.

8. **Capture quality: mic gain + beamformer for Show geometry** — HITL — blocked by: 6
   Set gain; re-tune / add beamforming for the Show's mic array.

9. **Echo cancellation + barge-in** — HITL — blocked by: 6
   Port the speexdsp AEC path (reference tapped at the speaker write); barge-in.

10. **Borrowed Screen: Voice Turn Overlay** — AFK(mostly) — blocked by: 6
    `display` capability + a status-light edge bar drawn only during a turn,
    driven by the same turn-state signal as the LED scenes.

## Status
Slices 1–6 **published** to dweng0/EchoMuse (label `show8`). Slices 7–10 held.

> **CHECKPOINT — after slices 1–6 land:** revisit slices 7–10 below, refine them
> against what we learned building the MVP, then publish them. Do not skip this —
> it's the "take stock and pivot if needed" gate.

### Held slices (7–10) — refine + publish at checkpoint
7. Buttons + mute + platform glue — AFK(mostly) — blocked by: 4
8. Capture quality: mic gain + beamformer for Show geometry — HITL — blocked by: 6
9. Echo cancellation + barge-in — HITL — blocked by: 6
10. Borrowed Screen: Voice Turn Overlay — AFK(mostly) — blocked by: 6

## Backlog (not issues yet)
- Dashboard wizard that wraps the provisioning script and streams its output.
- Shared audio via Android's audio stack (reverses ADR-0002).
