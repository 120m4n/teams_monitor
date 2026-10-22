# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Spec in `docs/README.md`, task specs in `tasks/TASK-0*.md` (TASK-01, TASK-02 done). Read the relevant task file before implementing a component — each one names its target file paths and checklist.

## What this is

A Windows 11 PoC in Go: detects an active Microsoft Teams call by inspecting WASAPI capture sessions on the default microphone, then shows tray/toast feedback and drives an ESP32-S3 LED ring. Single binary, tray mode by default, Windows Service mode with `--service`.

## Architecture (pipeline)

1. **Detector** (`pkg/detector/wca_detector.go`, TASK-01) — `go-wca`: init COM on the worker goroutine → `GetDefaultAudioEndpoint(eCapture, eCommunications)` → `IAudioSessionManager2` → enumerate sessions → `IAudioSessionControl2.GetProcessId()` → resolve exe name via `QueryFullProcessImageNameW` (`golang.org/x/sys/windows`). Call is active if any of `ms-teams.exe`, `Teams.exe`, `msteams.exe` has state `AudioSessionStateActive` (1). Exposes `IsTeamsCallActive() (bool, error)`.
2. **Event manager** (`pkg/events/event_manager.go`, TASK-02) — polls the detector (default 1000ms), debounces (default 2 consecutive matching reads; mic flickers, so IN_CALL→IDLE must be stabilized), emits `StateIdle`/`StateInCall` transitions via `OnStateChange(func(CallState))` callbacks. Takes a `Check func() (bool, error)` so it has no Windows dependency; `Run` calls Check on its own goroutine, so the caller must `detector.InitCOM()` on that goroutine first.
3. **Consumers** of state changes, independent of each other:
   - **UI** (`cmd/app/main.go`, `pkg/ui/tray.go`, `pkg/ui/toast.go`, TASK-03) — `getlantern/systray` menu (status, auto-start toggle, exit), toast on call start/end, `--service` via `golang.org/x/sys/windows/svc`.
   - **Actuator** (`pkg/actuator/`, TASK-04) — `Actuator` interface `SetCallState(active bool) error`; `SerialActuator` (`go.bug.st/serial`, e.g. `COM3`, auto-reconnect) and `HTTPActuator` (e.g. `http://192.168.1.50/led`). Payload is the line `CALL_ON` / `CALL_OFF` (newline-terminated).

## Build notes

- Target is Windows only (COM/WASAPI, systray, svc). Development machine is macOS, so cross-compile: `GOOS=windows GOARCH=amd64 go build ./cmd/app`. The detector can't run or be integration-tested on macOS; keep platform-agnostic logic (debounce/state machine, actuator payloads) testable with plain `go test ./pkg/events/... ./pkg/actuator/...`.
- Go 1.21+ (module `teams_monitor`).
- Windows-only code uses `//go:build windows` or `_windows` filenames; keep pure logic in untagged files so it tests on macOS (e.g. `pkg/detector/teams.go`).
- Commands:
  - `go test ./...` — macOS-runnable tests
  - `go test -run TestIsTeamsProcess ./pkg/detector` — single test
  - `GOOS=windows go vet ./...` — type-check Windows code from macOS
  - On Windows: `go test -v -run TestIsTeamsCallActive ./pkg/detector` — live mic-session check (logs result)
- go-wca gotcha: `GetProcessId` returns an error for multi-process sessions (`AUDCLNT_S_NO_SINGLE_PROCESS` is a success HRESULT but go-wca treats non-zero as error); the detector skips such sessions.
