//go:build windows

// Command app detects Teams calls and drives the tray icon, toasts and ESP32 LED ring.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"teams_monitor/pkg/actuator"
	"teams_monitor/pkg/detector"
	"teams_monitor/pkg/events"
	"teams_monitor/pkg/ui"
)

func main() {
	headless := flag.Bool("headless", false, "no tray icon or toasts; detection + actuator only")
	serialPort := flag.String("serial", "", "ESP32 serial port, e.g. COM3")
	baud := flag.Int("baud", 115200, "serial baud rate")
	httpURL := flag.String("http", "", "ESP32 HTTP endpoint, e.g. http://192.168.1.50/led")
	interval := flag.Duration("interval", time.Second, "detector poll interval")
	debounce := flag.Int("debounce", 2, "consecutive readings needed to change state")
	fx := flag.Int("fx", -1, "force LED effect 0-4 during calls (fade-in, clock, voice, scanner, disco); -1 follows call length")
	flag.Parse()

	setupLog()
	if *fx >= 0 {
		actuator.OnPayload = fmt.Sprintf("FX %d\n", *fx)
	}

	// Auto-start (HKCU Run) and the scheduled task may both launch us; keep one.
	if _, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Local\TeamsMonitor`)); err == windows.ERROR_ALREADY_EXISTS {
		log.Print("already running, exiting")
		return
	}

	var acts []actuator.Actuator
	if *serialPort != "" {
		s := actuator.NewSerialActuator(*serialPort, *baud)
		defer s.Close()
		acts = append(acts, s)
	}
	if *httpURL != "" {
		acts = append(acts, actuator.NewHTTPActuator(*httpURL))
	}
	actuate := func(active bool) {
		for _, a := range acts {
			if err := a.SetCallState(active); err != nil {
				log.Printf("actuator: %v", err)
			}
		}
	}

	mgr := events.NewManager(detector.IsTeamsCallActive)
	mgr.Interval, mgr.Debounce = *interval, *debounce
	mgr.OnStateChange(func(s events.CallState) {
		log.Printf("state: %v", s)
		actuate(s == events.StateInCall)
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	actuate(false) // sync the LED ring with our initial Idle state
	log.Printf("started (headless=%v)", *headless)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := detector.InitCOM(); err != nil {
			log.Fatalf("COM init: %v", err)
		}
		defer detector.ReleaseCOM()
		mgr.Run(ctx)
	}()

	if !*headless {
		ui.RunTray(ctx, func(t *ui.Tray) {
			mgr.OnStateChange(func(s events.CallState) {
				inCall := s == events.StateInCall
				t.SetInCall(inCall)
				if inCall {
					go ui.Notify("Teams Monitor", "Teams Call Active! Actuating ESP32 LED Ring.")
				} else {
					go ui.Notify("Teams Monitor", "Teams call ended.")
				}
			})
			t.SetInCall(mgr.State() == events.StateInCall) // in case we transitioned before the tray was up
		}, stop)
	}
	<-done
	actuate(false)
	log.Print("stopped")
}

// setupLog writes to %LOCALAPPDATA%\teams_monitor\teams_monitor.log (and stderr,
// which is a no-op in a -H=windowsgui build).
func setupLog() {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "teams_monitor")
	os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "teams_monitor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
}
