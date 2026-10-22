# TASK 01: CoreAudio Listener & WASAPI Teams Inspector (go-wca)

## Description
Implement the core audio capture session inspector using `github.com/moutend/go-wca`.

## Requirements
1. Initialize COM library on the worker goroutine.
2. Get the default capture device (Microphone / `eCapture`).
3. Access `IAudioSessionManager2` to enumerate active audio sessions.
4. Iterate over sessions, extract PID via `IAudioSessionControl2.GetProcessId()`.
5. Map PID to executable process name using Windows API (`QueryFullProcessImageNameW`).
6. Detect if any process matching `ms-teams.exe`, `Teams.exe`, or `msteams.exe` is in `AudioSessionStateActive`.

## File Structure Target
`pkg/detector/wca_detector.go`

## Implementation Checklist
- [x] Implement `InitCOM()` and `ReleaseCOM()`.
- [x] Implement `IsTeamsCallActive() (bool, error)`.
- [x] Implement process name lookup given PID (`getProcessNameByID(pid uint32)`).
- [x] Add unit/integration test with mock or direct output logging.
