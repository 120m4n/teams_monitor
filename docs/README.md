# SPEC KIT: Proof of Concept (PoC) - MS Teams Call Detector Daemon & Tray App

## 1. Executive Summary & Objective
This Spec Kit outlines the technical specification, architectural design, and step-by-step developer tasks for building a lightweight Windows 11 Proof of Concept (PoC) application written in **Go (Golang)** using `go-wca` (Windows Core Audio API binding).

The application runs both as a **System Tray Application** (with option to operate headless as a Windows Service/Daemon) that monitors active WASAPI audio capture streams on Windows 11 to detect when Microsoft Teams (`ms-teams.exe` / `Teams.exe`) opens an active microphone audio session (indicating an active call). Upon detecting a active call, it displays an on-screen notification / log message and prepares the triggers for ESP32-S3 LED Ring actuation.

---

## 2. Architecture & System Flow

```
+-----------------------------------------------------------------------+
|                            WINDOWS 11 OS                              |
|                                                                       |
|  [ MS Teams ]  ──(Open Mic Session)──> [ WASAPI / CoreAudio API ]     |
|                                                     │                 |
|                                                     ▼                 |
|  +-----------------------------------------------------------------+  |
|  |                 GOLANG DETECTOR APP / TRAY-ICON                 |  |
|  |                                                                 |  |
|  |  +-----------------------------------------------------------+  |  |
|  |  |  Task 1: CoreAudio Listener Loop (go-wca)                 |  |  |
|  |  |  - Enumerate IMMDeviceEnumerator (eCapture / eRender)    |  |  |
|  |  |  - Session Control & Process ID -> Executable Matching     |  |  |
|  |  +------------------------------┬----------------------------+  |  |
|  |                                 │ (Event: Call State Changed)  |  |
|  |                                 ▼                              |  |
|  |  +-----------------------------------------------------------+  |  |
|  |  |  Task 2: State Machine & Event Manager                    |  |  |
|  |  |  - State: BUSY_IN_CALL / IDLE                              |  |  |
|  |  |  - Debounce (2s stabilization)                            |  |  |
|  |  +------------------------------┬----------------------------+  |  |
|  |                                 │                              |  |
|  |                 ┌───────────────┴───────────────┐              |  |
|  |                 ▼                               ▼              |  |
|  |  +-----------------------------+  +--------------------------+  |  |
|  |  | Task 3: Tray Icon & Win UI  |  | Task 4: ESP32 Actuation  |  |  |
|  |  | - System Tray (fyne/getlantern)| - Serial/COM or HTTP     |  |  |
|  |  | - Toast/Balloon Notifications|  |   Control Commands       |  |  |
|  |  +-----------------------------+  +--------------------------+  |  |
|  +-----------------------------------------------------------------+  |
+-----------------------------------------------------------------------+
```

---

## 3. Tech Stack & Dependencies
- **Language:** Go (Golang) v1.21+
- **Windows Audio API Binding:** `github.com/moutend/go-wca`
- **Process Lookup:** `golang.org/x/sys/windows` (OpenProcess, QueryFullProcessImageName)
- **System Tray:** `github.com/getlantern/systray` or `fyne.io/systray`
- **Notifications:** `github.com/go-toast/toast` or Windows native balloon notifications.
- **Serial Port (Optional for Actuator):** `go.bug.st/serial`

---

## 4. Key Architectural Decisions & Technical Guidelines
1. **Audio Polling & Session Enumeration:**
   - Initialize COM with `wca.CoInitializeEx(0, wca.COINIT_APARTMENTTHREADED)` or `COINIT_MULTITHREADED`.
   - Retrieve Default Microphone Capture Device via `IMMDeviceEnumerator::GetDefaultAudioEndpoint(eCapture, eCommunications)`.
   - Enumerate `IAudioSessionManager2` -> `IAudioSessionEnumerator`.
   - For each active session, retrieve `IAudioSessionControl2::GetProcessId()`.
   - Match PID against process names: `ms-teams.exe`, `Teams.exe`, `msteams.exe`.
   - Check state via `GetState()`: if `AudioSessionStateActive` (1), Teams call is ACTIVE.
2. **Debounce / Hysteresis:**
   - Microphones can flicker between active and quiet. Maintain a 1.5–2s debounce buffer before signaling state transition from `IN_CALL` to `IDLE`.
3. **Tray & Service Architecture:**
   - Single binary supporting multi-mode: GUI Tray app mode (default) and Windows Service mode (when invoked with `--service` via `golang.org/x/sys/windows/svc`).
