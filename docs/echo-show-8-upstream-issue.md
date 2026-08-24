# DRAFT — upstream issue (review before posting)

> Intended for the upstream repo (wilbowes/EchoMuse) to describe the plan and
> ask for feedback. **Do not post until reviewed.** Trim/adjust tone to taste.

---

**Title:** Adding a second board: Echo Show 8 (1st gen) on LineageOS

**Body:**

I'd like to add support for the **Echo Show 8 (1st gen)** running LineageOS as a
second device alongside the Echo Dot 2nd Gen, and wanted to share the intended
shape early to get your feedback before writing much code.

**Approach.** The controller is model-agnostic (capability-negotiated), so this
is almost entirely a second set of device **bindings**. The plan is to reuse the
whole board-agnostic core (wake word, beamformer, AEC, networking, server) and
write fresh hardware bindings for the Show behind a new `crown` build tag,
mirroring the existing `server` tag — one binary per board, no runtime hardware
detection.

**Key decisions so far:**
- One binary per board, selected by build tag (not runtime detection).
- The Show owns the audio hardware exclusively for now, exactly like the Dot;
  sharing with other Android apps via the Android audio stack is deferred.
- A decorative model label ("Echo Show 8") in the handshake — display only,
  nothing branches on it; behaviour stays capability-driven.
- No LED ring on the Show; instead a later, minimal "voice turn" status-light
  overlay on the display, drawn only during a turn (the screen otherwise belongs
  to the user's own software). Deferred past MVP.

**MVP milestone:** boots → connects → shows in Home Assistant → wake word →
Assist → spoken reply. Single mic, controller-side wake word, installed via a
small provisioning script.

**Where I'd love input:** the very first step is a *hardware map* of the Show 8 —
which ALSA card/device the mic and speaker are on, their formats and channel
counts, and the input-device paths for buttons and mute. **Do you (or anyone)
already have this for the Show 8?** It would save the discovery work.

Full plan and design notes attached below / in the linked branch.

---

_(Attach or paste: `docs/echo-show-8-plan.md` and `CONTEXT.md`.)_
