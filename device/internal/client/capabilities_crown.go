//go:build crown

package client

// capabilities is crown's MVP subset (docs/device-controller-interface.md,
// "Board profile — crown"). "leds" stays on despite there being no ring —
// the listening-ring hint still drives the wake cue, it just has nothing to
// paint with today (no screen overlay yet, ADR pending a `display`
// capability). No "led_anim": there is no ring for a local animation engine
// to spin frames for. No oww_shadow/oww_trigger: MVP wake word is
// controller-side. No audio_mix / button_hold / ambient_light yet — not
// implemented, not announced (ADR-0003: never sniff the model to decide
// behaviour, only ever add or omit the capability itself).
func capabilities() []string {
	return []string{"mic", "speaker", "leds", "buttons"}
}
