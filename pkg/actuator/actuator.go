// Package actuator sends call-state commands to the ESP32-S3 LED ring.
package actuator

type Actuator interface {
	SetCallState(active bool) error
}

// OnPayload is sent when a call starts. Set it to e.g. "FX 3\n" before use to force a
// firmware effect instead of the call-length timeline.
var OnPayload = "CALL_ON\n"

func payload(active bool) string {
	if active {
		return OnPayload
	}
	return "CALL_OFF\n"
}
