# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Spec in `docs/README.md`, task specs in `tasks/TASK-0*.md` (all four done; untested on real Windows hardware yet). Install/auto-start docs: `docs/INSTALL.md`. Read the relevant task file before implementing a component — each one names its target file paths and checklist.

## What this is

A Windows 11 PoC in Go: detects an active Microsoft Teams call by inspecting WASAPI capture sessions on the default microphone, then shows tray/toast feedback and drives an ESP32-S3 LED ring. Single binary, tray mode by default, `--headless` for no UI. Deliberately **no Windows Service**: Session 0 has no tray/toast and may not see Teams' mic session; auto-start is via the `HKCU\...\Run` registry key instead.

## Architecture (pipeline)

1. **Detector** (`pkg/detector/wca_detector.go`, TASK-01) — `go-wca`: init COM on the worker goroutine → `GetDefaultAudioEndpoint(eCapture, eCommunications)` → `IAudioSessionManager2` → enumerate sessions → `IAudioSessionControl2.GetProcessId()` → resolve exe name via `QueryFullProcessImageNameW` (`golang.org/x/sys/windows`). Call is active if any of `ms-teams.exe`, `Teams.exe`, `msteams.exe` has state `AudioSessionStateActive` (1). Exposes `IsTeamsCallActive() (bool, error)`.
2. **Event manager** (`pkg/events/event_manager.go`, TASK-02) — polls the detector (default 1000ms), debounces (default 2 consecutive matching reads; mic flickers, so IN_CALL→IDLE must be stabilized), emits `StateIdle`/`StateInCall` transitions via `OnStateChange(func(CallState))` callbacks. Takes a `Check func() (bool, error)` so it has no Windows dependency; `Run` calls Check on its own goroutine, so the caller must `detector.InitCOM()` on that goroutine first.
3. **Consumers** of state changes, independent of each other:
   - **UI** (`cmd/app/main.go`, `pkg/ui/tray.go`, `pkg/ui/toast.go`, TASK-03) — `getlantern/systray` menu (status, auto-start toggle, exit), toast on call start/end, `--headless` flag, auto-start via `HKCU\...\Run`.
   - **Actuator** (`pkg/actuator/`, TASK-04) — `Actuator` interface `SetCallState(active bool) error`; `SerialActuator` (`go.bug.st/serial`, e.g. `COM3`, auto-reconnect) and `HTTPActuator` (e.g. `http://192.168.1.50/led`). Payload is the line `CALL_ON` / `CALL_OFF` (newline-terminated). HTTP sends it as a `text/plain` POST body. Serial opens the port lazily and on a write error closes, reopens and retries once.

## Build notes

- Target is Windows only (COM/WASAPI, systray, svc). Development machine is macOS, so cross-compile: `GOOS=windows GOARCH=amd64 go build ./cmd/app`. The detector can't run or be integration-tested on macOS; keep platform-agnostic logic (debounce/state machine, actuator payloads) testable with plain `go test ./pkg/events/... ./pkg/actuator/...`.
- Go 1.21+ (module `teams_monitor`).
- Windows-only code uses `//go:build windows` or `_windows` filenames; keep pure logic in untagged files so it tests on macOS (e.g. `pkg/detector/teams.go`).
- Commands:
  - `go test ./...` — macOS-runnable tests
  - `GOOS=windows GOARCH=amd64 go build -ldflags -H=windowsgui -o teams_monitor.exe ./cmd/app` — release build (no console window; logs in `%LOCALAPPDATA%\teams_monitor\teams_monitor.log`)
  - `go test -run TestIsTeamsProcess ./pkg/detector` — single test
  - `GOOS=windows go vet ./...` — type-check Windows code from macOS
  - On Windows: `go test -v -run TestIsTeamsCallActive ./pkg/detector` — live mic-session check (logs result)
- go-wca gotcha: `GetProcessId` returns an error for multi-process sessions (`AUDCLNT_S_NO_SINGLE_PROCESS` is a success HRESULT but go-wca treats non-zero as error); the detector skips such sessions.
- `cmd/app` starts the manager goroutine (which owns COM) before the tray; the tray registers its listener in `onReady` and syncs to `mgr.State()`. A named mutex (`Local\TeamsMonitor`) enforces a single instance, because the HKCU Run key and the scheduled task (`deploy/install-task.ps1`) can both launch it.
- Config: `teams_monitor.xml` next to the exe (`pkg/config`, example `teams_monitor.example.xml`, gitignored) sets flag defaults: processes → `detector.TeamsExes`, `actuator mode` serial/http/both → `--serial`/`--http` (`http://<wifi ip>/led`), `fx`. `deploy/flash.ps1` reads the same file to write `secrets.h` (incl. optional static IP `WIFI_IP`/`WIFI_GATEWAY`/`WIFI_SUBNET`) and flashes, stopping/restarting the app around the upload.
- Teammate ping page: `pkg/web` (embedded `index.html`, `POST /ping`, per-client cooldown + bounded queue, client = last `X-Forwarded-For` entry only when from loopback). `--web 127.0.0.1:8080`, published via `tailscale funnel --bg 8080` (docs/INSTALL.md §5). A ping sends `FX 4` for 5 s, then restores `mgr.State()`.
- Tray icons are `pkg/ui/icons/{idle,call}.ico` (multi-size, PNG-embedded), loaded with `go:embed`.
- Firmware: `firmware/esp8266` (PlatformIO, Arduino core, Adafruit NeoPixel), 12× WS2812 on D4. The actual board is an ESP8266, not the ESP32-S3 the spec names. Build with `cd firmware/esp8266 && pio run`; flash with `pio run -t upload`. WiFi/HTTP compiles in only if the gitignored `src/secrets.h` exists (`__has_include`); otherwise it's serial-only.
