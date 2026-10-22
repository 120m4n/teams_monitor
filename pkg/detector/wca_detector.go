//go:build windows

package detector

import (
	"runtime"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// InitCOM initializes COM on the calling goroutine and pins it to its OS thread.
// Call IsTeamsCallActive only from that goroutine; pair with ReleaseCOM.
func InitCOM() error {
	runtime.LockOSThread()
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if oleErr, ok := err.(*ole.OleError); ok && oleErr.Code() == 1 { // S_FALSE: already initialized
		return nil
	}
	if err != nil {
		runtime.UnlockOSThread()
	}
	return err
}

// ReleaseCOM undoes InitCOM. Must run on the same goroutine.
func ReleaseCOM() {
	ole.CoUninitialize()
	runtime.UnlockOSThread()
}

// IsTeamsCallActive reports whether a Teams process has an active session on the
// default communications capture device (microphone).
func IsTeamsCallActive() (bool, error) {
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return false, err
	}
	defer mmde.Release()

	var mmd *wca.IMMDevice
	if err := mmde.GetDefaultAudioEndpoint(wca.ECapture, wca.ECommunications, &mmd); err != nil {
		return false, err
	}
	defer mmd.Release()

	var asm2 *wca.IAudioSessionManager2
	if err := mmd.Activate(wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &asm2); err != nil {
		return false, err
	}
	defer asm2.Release()

	var ase *wca.IAudioSessionEnumerator
	if err := asm2.GetSessionEnumerator(&ase); err != nil {
		return false, err
	}
	defer ase.Release()

	var count int
	if err := ase.GetCount(&count); err != nil {
		return false, err
	}
	for i := 0; i < count; i++ {
		if sessionIsActiveTeams(ase, i) {
			return true, nil
		}
	}
	return false, nil
}

// sessionIsActiveTeams checks one session; per-session failures (e.g. multi-process
// sessions, exited processes) just mean "not Teams".
func sessionIsActiveTeams(ase *wca.IAudioSessionEnumerator, i int) bool {
	var asc *wca.IAudioSessionControl
	if err := ase.GetSession(i, &asc); err != nil {
		return false
	}
	defer asc.Release()

	var state uint32
	if err := asc.GetState(&state); err != nil || state != wca.AudioSessionStateActive {
		return false
	}

	disp, err := asc.QueryInterface(wca.IID_IAudioSessionControl2)
	if err != nil {
		return false
	}
	asc2 := (*wca.IAudioSessionControl2)(unsafe.Pointer(disp))
	defer asc2.Release()

	var pid uint32
	if err := asc2.GetProcessId(&pid); err != nil || pid == 0 {
		return false
	}
	name, err := getProcessNameByID(pid)
	return err == nil && isTeamsProcess(name)
}

// getProcessNameByID returns the full image path of the process.
func getProcessNameByID(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}
