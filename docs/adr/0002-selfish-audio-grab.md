# Hoya owns the audio hardware exclusively (for now)

On the Hoya (Echo Show 8 on LineageOS) EchoMuse seizes the microphone and
speaker hardware directly and exclusively — exactly as the Biscuit does — rather
than routing through Android's audio system so other apps can share the sound
devices. Sharing is deferred as a possible later phase.

## Why

The direct grab is largely a port of the Biscuit's existing audio bindings, so
it reuses the most code and is days of work, most of it discovery on real
hardware. Going through Android's audio APIs instead would mean brand-new
bindings written from Go against the NDK, weeks of work, and Android tends to
impose its own microphone pre-processing that fights our beamformer and echo
canceller — a real risk to capture quality. The intended use is EchoMuse-as-the-
audio with the screen showing a silent visual kiosk, so nothing else needs to
make sound concurrently.

## Mic is held; the speaker is what's grabbed and released

"Exclusive" cuts two ways, and the two halves are not symmetric:

- **The mic is held continuously**, not per-conversation. MVP wake word is
  controller-side, so the device streams mic PCM the whole time it runs — it has
  to, or it goes deaf to the next wake word. Releasing the mic between turns
  would require push-to-talk or an on-device always-listening wake path, both
  deferred past MVP.
- **The speaker is grabbed for a turn and released to idle.** The device claims
  the speaker for the spoken reply and for any media the turn asked for ("play
  X"), then releases it when the turn (and any resulting playback) finishes —
  the Alexa model. This is a turn-ownership/ducking claim, not a device the
  other software could have used anyway, since EchoMuse owns the sound hardware
  outright while it runs.

## Consequences

- While EchoMuse runs, no other app on the Hoya can play audio or capture the
  mic. This is deliberate: EchoMuse owns the audio, and is only a guest on the
  screen (the Borrowed Screen; see CONTEXT.md).
- EchoMuse's own media playback and its self-ducking under a voice answer are
  unaffected — those never needed a third party.
- If concurrent third-party audio ever becomes a requirement, revisit with an
  Android-audio binding as its own phase.
