//go:build crown

package buttons

// crown: live-confirmed with `getevent -lt` 2026-08-26
// (docs/echo-show-8-hardware-map.md). No separate action button — the
// mic/action button rides the "gating" driver as KEY_POWER (code 116), not
// gpio-keys, and there is no code-138 "dot" event as on biscuit. Volume
// shares gpio-keys with the camera shutter switch, same evdev node.
//
// KNOWN GAP: KEY_POWER (116) does not match pkg/buttons.MuteClick (113), so
// SubscribeToButton's mute intercept (evdev_controller.go) does not fire for
// it yet — a press currently surfaces as an ordinary ButtonClickEvent rather
// than toggling mute, even though "firmware toggles mute in software" is the
// intended behaviour per the hardware map. Deferred rather than guessed at:
// fixing it means either mapping 116 onto MuteClick for this board or giving
// the dot device its own click-type table, and that's a UX decision, not a
// wiring one.
const dotButton = "/dev/input/event0"
const volumeButton = "/dev/input/event6"

// Init: no native button service to stop. `stop acebutton` (biscuit's
// equivalent) returned "exit status 1" against a real crown unit — there is
// no such service on LineageOS — so this is a no-op rather than a
// best-effort exec that would only ever fail.
func (e *EvDevController) Init() error { return nil }
