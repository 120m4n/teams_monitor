# TASK 04: ESP32-S3 Hardware Actuation Interface (Serial COM / HTTP)

## Description
Provide abstraction layer to send `ACTIVATE` / `DEACTIVATE` signals to ESP32-S3 via Serial UART (USB COM Port) or HTTP REST.

## Requirements
1. Configurable output target: Serial Port (e.g. `COM3`) or HTTP Endpoint (e.g. `http://192.168.1.50/led`).
2. Protocol payload simple string: `"CALL_ON
"` / `"CALL_OFF
"`.

## File Structure Target
`pkg/actuator/actuator.go`, `pkg/actuator/serial.go`, `pkg/actuator/http.go`

## Implementation Checklist
- [x] Implement `Actuator` interface: `SetCallState(active bool) error`.
- [x] Implement `SerialActuator` with auto-reconnect logic.
- [x] Implement `HTTPActuator` for wireless setups.
