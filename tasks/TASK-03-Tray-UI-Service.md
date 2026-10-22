# TASK 03: Windows System Tray & Toast Notification UI

## Description
Provide user interface feedback via Windows System Tray icon, menu, and popup notifications.

## Requirements
1. System tray icon using `github.com/getlantern/systray`.
2. Tray Menu options:
   - Status Indicator (e.g. "Status: Idle" / "Status: In Call")
   - Toggle Auto-Start on Windows Boot
   - Exit
3. Trigger Windows Toast Notification whenever Teams call activates or ends.
4. Support Windows Service wrapper mode using `golang.org/x/sys/windows/svc`.

## File Structure Target
`cmd/app/main.go`, `pkg/ui/tray.go`, `pkg/ui/toast.go`

## Implementation Checklist
- [ ] Create system tray icon asset loader.
- [ ] Connect event manager callback to tray status text update and toast message.
- [ ] Display Windows Toast: `"Teams Call Active! Actuating ESP32 LED Ring."`
- [ ] Build Windows Service registration flag `--service`.
