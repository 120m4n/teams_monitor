package actuator

import (
	"io"
	"sync"

	"go.bug.st/serial"
)

// SerialActuator writes the payload to a COM port (e.g. COM3). The port is opened
// lazily; on a write error it is closed and reopened once, so unplugging and
// replugging the ESP32 recovers on the next state change.
type SerialActuator struct {
	open func() (io.WriteCloser, error)

	mu   sync.Mutex
	port io.WriteCloser
}

func NewSerialActuator(portName string, baud int) *SerialActuator {
	return &SerialActuator{open: func() (io.WriteCloser, error) {
		return serial.Open(portName, &serial.Mode{BaudRate: baud})
	}}
}

func (a *SerialActuator) SetCallState(active bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	err := a.write(payload(active))
	if err != nil { // stale handle after unplug: reconnect and retry once
		err = a.write(payload(active))
	}
	return err
}

func (a *SerialActuator) write(msg string) error {
	if a.port == nil {
		p, err := a.open()
		if err != nil {
			return err
		}
		a.port = p
	}
	if _, err := io.WriteString(a.port, msg); err != nil {
		a.port.Close()
		a.port = nil
		return err
	}
	return nil
}

func (a *SerialActuator) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.port == nil {
		return nil
	}
	err := a.port.Close()
	a.port = nil
	return err
}
