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

## 4. ESP8266 firmware (`firmware/esp8266`, PlatformIO)

Wiring for a 12-LED WS2812B ring (e.g. WCMCU-2812B-12). Go by the pad labels on the ring, not the wire colors:

| Ring pad | NodeMCU / D1 mini |
|---|---|
| DI / DIN / IN | D4 (GPIO2) |
| 5V / VCC | VIN on NodeMCU, 5V on D1 mini (5V from USB) |
| GND | G / GND |
| DO / DOUT | not connected (only for chaining another ring) |

- Connect GND and 5V before the data wire.
- If the data wire is soldered to DO instead of DI, the ring stays dark.
- If the ring flickers, try powering it from 3V3 instead: WS2812B usually runs at 3.3 V, which also matches the data level. At the default brightness (80/255, red) the ring draws ~75 mA.

- D4 is also the onboard LED, so it flickers when data is sent. That's harmless.
- A 300–500 Ω resistor in series on DIN and a 470–1000 µF capacitor across 5V/GND are recommended.
- If colors glitch, the 3.3 V data signal is too weak for the 5 V ring: add a level shifter (e.g. 74AHCT125).

Flash it:

```sh
cd firmware/esp8266
cp src/secrets.example.h src/secrets.h   # optional: enables HTTP; edit SSID/password (2.4 GHz only)
pio run -t upload                        # add --upload-port COM3 (Windows) if needed
pio device monitor                       # shows "teams_led ready" and, with WiFi, the HTTP URL
```

Without `secrets.h` the board only accepts serial commands. In the serial monitor, type `CALL_ON` or `CALL_OFF`. The ring turns red or off, and the board replies `OK ...`.

Then run the PC app with `--serial COM3` or `--http http://<ip printed above>/led`. Reserve that IP in your router so it doesn't change. A board other than NodeMCU (e.g. D1 mini) needs `board = d1_mini` in `platformio.ini`.
