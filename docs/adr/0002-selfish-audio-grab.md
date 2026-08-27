# Crown owns the audio hardware exclusively (for now)

**Status, 2026-08-27: the speaker half of this ADR is superseded by
implementation. The mic half still stands as written.** Continuous concurrent
use of the device as an ordinary Android tablet (browser audio, etc.)
alongside the raw-ALSA exclusive grab this ADR describes hard-froze the whole
board — not just the daemon, the SoC — documented in
`docs/echo-show-8-audio-freeze-handoff.md` and root-caused via pstore in
`docs/echo-show-8-journal.md` (2026-08-26). The fix
(`docs/echo-show-8-audiotrack-design.md`, commits `af0b000`/`dfb773b`) routes
playback through a real Android `AudioTrack` instead, via a socket to
`crown_launcher`'s `PlaybackServer` — deliberately the opposite of "exclusive
and direct" for exactly the reason this ADR gave for not doing that (avoiding
weeks of NDK binding work) turned out not to be the deciding cost once a
freeze was on the table. `AudioFocus` ducking (`bb80677`) replaced the
turn-ownership model described below for the speaker.

The mic still does the raw-ALSA exclusive grab described here, unmodified,
and the same class of collision is the leading (unconfirmed) hypothesis for
an open bug — mic frames never reaching the controller at all — see
`docs/echo-show-8-journal.md`'s final entry. If that hypothesis holds, the
mic side of this ADR will need the same reversal the speaker side already
got; this file should be revisited then rather than treated as settled.

On the Crown (Echo Show 8 on LineageOS) EchoMuse seizes the microphone and
speaker hardware directly and exclusively — exactly as the Biscuit does — rather
than routing through Android's audio system so other apps can share the sound
devices. Sharing is deferred as a possible later phase.

**(Original ADR text follows, describing the design as first built. See the
status note above for what has since changed.)**

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

- While EchoMuse runs, no other app on the Crown can play audio or capture the
  mic. This is deliberate: EchoMuse owns the audio, and is only a guest on the
  screen (the Borrowed Screen; see CONTEXT.md).
- EchoMuse's own media playback and its self-ducking under a voice answer are
  unaffected — those never needed a third party.
- If concurrent third-party audio ever becomes a requirement, revisit with an
  Android-audio binding as its own phase.
