# Install (Windows 11)

## 1. Build

From any OS (no cgo needed for Windows):

```sh
GOOS=windows GOARCH=amd64 go build -ldflags -H=windowsgui -o teams_monitor.exe ./cmd/app
```

`-H=windowsgui` hides the console window. Logs go to `%LOCALAPPDATA%\teams_monitor\teams_monitor.log`.

Copy `teams_monitor.exe` somewhere stable, e.g. `C:\Tools\teams_monitor.exe`.

## 2. Flags

| Flag | Default | |
|---|---|---|
| `--serial COM3` | off | ESP32 over USB serial |
| `--baud 115200` | 115200 | serial baud rate |
| `--http http://192.168.1.50/led` | off | ESP32 over HTTP (POST `CALL_ON\n` / `CALL_OFF\n`) |
| `--headless` | false | no tray icon / toasts |
| `--interval 1s` | 1s | poll interval |
| `--debounce 2` | 2 | consecutive readings to change state |

Try it by hand first: `teams_monitor.exe --serial COM3`, join a Teams call, and watch the tray icon turn red.

## 3. Auto-start: pick one

Only one copy runs at a time (named mutex), so if both are set up the second one just exits.

### A. Task Scheduler (recommended: restarts if it crashes)

From the repo folder, in a normal (non-admin) PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File deploy\install-task.ps1 -Exe C:\Tools\teams_monitor.exe -Arguments "--serial COM3"
```

This registers the `TeamsMonitor` task for the current user and starts it:
- **At logon**, in your session, so the tray icon, toasts and Teams detection all work.
- **Restart on failure**: retries every 1 min, up to 999 times.
- **Watchdog**: the trigger repeats every 5 min. If the app is already running, nothing happens (`IgnoreNew`). If it died, for example with a crash the restart-on-failure setting didn't catch, it starts again.

Manage it:

```powershell
schtasks /Query  /TN TeamsMonitor /V /FO LIST   # status, last result
schtasks /Run    /TN TeamsMonitor               # start now
schtasks /End    /TN TeamsMonitor               # kill it (LED ring stays as-is)
schtasks /Change /TN TeamsMonitor /Disable      # stop auto-starting / watchdog
schtasks /Change /TN TeamsMonitor /Enable
schtasks /Delete /TN TeamsMonitor /F            # uninstall
```

> Because of the watchdog, **Exit** in the tray menu only stops it for up to 5 minutes. To stop it for good, run `/Disable` or `/Delete` (see above).

### B. Tray menu "Start with Windows"

Writes `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\TeamsMonitor` with the exe path and the flags it was started with. Simple, but there's no restart on crash.
