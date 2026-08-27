# Plan — Echo Show 8 (Crown) support

Working plan for adding Amazon Echo Show 8 (1st gen, LineageOS) support to
EchoMuse.

## Shape of the work

The controller is model-agnostic — it enables features purely from the
**Capability** list a device announces — so it needs almost no changes. The work
is almost entirely a second set of **Bindings** in the device firmware: reuse the
whole **Core** (wake word, beamformer, AEC, networking, server), write fresh
hands for the Show's hardware, gated behind a new `crown` build tag — one
binary per board, chosen at compile time, no runtime detection.

## Phases

### Phase 0 — Hardware Map  ← must come first
Discovery on the real device. Produce the **Hardware Map**: ALSA card/device for
mic and speaker, their formats and channel counts, input-device paths for
buttons and the mute switch, arch/API level, and how to auto-start a service.
Every later phase depends on this. (Lead the upstream issue with this — the
owner may already have it.)

### Phase 1 — Toolchain + "it connects"
New cross-compiler target for LineageOS's API level; `crown` build tag. Reuse the
TLS/client stack unchanged. Success = the binary runs on the Show and appears as
*pending* in the dashboard. No audio yet.

### Phase 2 — MVP voice  ← the "works with it" milestone
- `mic.Microphone` + `speaker.Speaker` **Bindings** against the Hardware Map,
  **selfish grab** of the audio hardware (exclusive, like the Dot).
- Single mic channel, no beamforming yet.
- Controller-side wake word (no on-device wake word yet).
- Announce **Capabilities** `["mic","speaker"]` only.
- Decorative model label "Echo Show 8" in the handshake — display only,
  never branched on; behaviour stays capability-driven.
- **Provisioning Script** (basic): adb-push the binary + TLS credentials and set
  auto-start, run by hand.

**Acceptance:** say the wake word → ask Assist → hear the answer through the
Show's speaker. Multi-turn follow-ups work for free (Core already does them).

### Phase 3 — Buttons + platform glue
`buttons.Controller` for the Show (vol ±, mic mute); replace FireOS-only glue in
`main()` (`GetSerialNo`, `stop mixer/acebutton/ledcontroller/smarthomewifid`,
core-floor, wifi recovery) with LineageOS equivalents or no-ops, behind the tag.

### Phase 4 — Capture quality
Mic gain, then re-tune / add the beamformer for the Show's mic geometry, then
AEC and barge-in (speexdsp path ports once the speaker binding taps its write).

### Phase 5 — Borrowed Screen
Add a `display` capability and a **Voice Turn Overlay**: a status-light edge bar
drawn only during a voice exchange, over whatever the user is running, driven by
the same turn-state signal as the Biscuit's LED scenes. The user's own software
owns the screen at rest.

### Later / nice-to-have
- Dashboard wizard that wraps the **Provisioning Script** and streams its output.
- Shared audio via Android's audio system, so third-party apps can also make
  sound (reverses the exclusive-grab deferral above).

## Guardrails (enforced by the repo)
- A test asserts capability strings match across the Go and Python sources — any
  new capability (`display`) or the model field must be mirrored on both sides
  and the test updated.
- Doctrine: an absent capability degrades to disabled-with-reason, never to a
  wrong answer. The phasing above keeps to that.
