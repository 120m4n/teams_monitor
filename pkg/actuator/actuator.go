// Package actuator sends call-state commands to the ESP32-S3 LED ring.
package actuator

type Actuator interface {
	SetCallState(active bool) error
}

func payload(active bool) string {
	if active {
		return "CALL_ON\n"
	}
	return "CALL_OFF\n"
}
