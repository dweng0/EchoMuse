//go:build crown

package led

import "github.com/wilbowes/EchoMuse/pkg/led"

// crown has a screen, not an LED ring — no i2c frame device, no gpio444
// mute-button LED (both are biscuit-specific hardware, see i2c_controller.go
// and mute_button.go). The "leds" capability still stays on (CLAUDE.md: the
// controller's listening-ring hint drives a device's wake cue whether or not
// there is a physical ring to paint); a screen status surface is deferred
// past MVP (interface doc, crown capability table). Until then this is a
// deliberate no-op, not a stub to fill in by accident.
type nullController struct{}

func (nullController) Init() error                   { return nil }
func (nullController) GetNumLEDs() (int, error)      { return 0, nil }
func (nullController) SetLEDs(leds ...led.Led) error { return nil }

func NewDefaultController() (led.Controller, error) {
	return nullController{}, nil
}

// InitMuteButtonLED / SetMuteButtonLED: no discrete mute-button LED on this
// board. Mute itself still works (it's ADC-mute plus, for crown, no ring to
// redden) — this only stubs the button's own indicator.
func InitMuteButtonLED() error       { return nil }
func SetMuteButtonLED(on bool) error { return nil }
