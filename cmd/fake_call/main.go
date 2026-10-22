//go:build windows

// Command fake_call simulates a Teams call for testing: build it as ms-teams.exe and it
// holds an active capture session on the default communications mic, which is exactly
// what the detector looks for. Audio is never read or stored.
//
//	go build -o fake\ms-teams.exe ./cmd/fake_call
//	.\fake\ms-teams.exe -for 30s
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"

	"teams_monitor/pkg/detector"
)

func main() {
	dur := flag.Duration("for", 30*time.Second, "how long the fake call lasts")
	flag.Parse()

	if err := detector.InitCOM(); err != nil {
		log.Fatalf("COM init: %v", err)
	}
	defer detector.ReleaseCOM()

	var mmde *wca.IMMDeviceEnumerator
	must(wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde), "device enumerator")
	defer mmde.Release()

	var mmd *wca.IMMDevice
	must(mmde.GetDefaultAudioEndpoint(wca.ECapture, wca.ECommunications, &mmd), "default communications mic")
	defer mmd.Release()

	var ac *wca.IAudioClient
	must(mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac), "audio client")
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	must(ac.GetMixFormat(&wfx), "mix format")
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	// 1 s shared-mode buffer (REFERENCE_TIME is in 100 ns units); it just overruns, nobody reads it.
	must(ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, 10_000_000, 0, wfx, nil), "initialize capture")
	must(ac.Start(), "start capture")
	defer ac.Stop()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log.Printf("fake call started for %v (Ctrl+C to hang up)", *dur)
	select {
	case <-time.After(*dur):
	case <-ctx.Done():
	}
	log.Print("fake call ended")
}

func must(err error, what string) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}
