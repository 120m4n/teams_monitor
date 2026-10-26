//go:build windows

// Command app detects Teams calls and drives the tray icon, toasts and ESP32 LED ring.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	_ "time/tzdata" // Windows has no zoneinfo for time.LoadLocation unless Go is installed

	"golang.org/x/sys/windows"

	"teams_monitor/pkg/actuator"
	"teams_monitor/pkg/config"
	"teams_monitor/pkg/detector"
	"teams_monitor/pkg/events"
	"teams_monitor/pkg/ui"
	"teams_monitor/pkg/web"
)

func main() {
	setupLog()

	// teams_monitor.xml next to the exe sets the flag defaults; explicit flags still win.
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	cfgPath := filepath.Join(filepath.Dir(exe), "teams_monitor.xml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if len(cfg.Processes) > 0 {
		detector.TeamsExes = cfg.Processes
	}

	headless := flag.Bool("headless", false, "no tray icon or toasts; detection + actuator only")
	serialPort := flag.String("serial", cfg.SerialPort(), "ESP32 serial port, e.g. COM3")
	baud := flag.Int("baud", cfg.Actuator.Serial.Baud, "serial baud rate")
	httpURL := flag.String("http", cfg.HTTPURL(), "ESP32 HTTP endpoint, e.g. http://192.168.1.50/led")
	interval := flag.Duration("interval", time.Second, "detector poll interval")
	debounce := flag.Int("debounce", 2, "consecutive readings needed to change state")
	resync := flag.Duration("resync", 30*time.Second, "resend the current state this often, so a board that rebooted catches up; 0 disables")
	webAddr := flag.String("web", cfg.Web.Listen, "serve the teammate ping page here, e.g. 127.0.0.1:8080 (publish with tailscale funnel)")
	webCooldown := flag.Duration("web-cooldown", time.Duration(cfg.Web.Cooldown)*time.Second, "minimum time between pings from one client")
	webDuration := flag.Duration("web-duration", time.Duration(cfg.Web.Duration)*time.Second, "how long one ping plays, 1s-60s")
	webFX := flag.Int("web-fx", cfg.Web.FX, "default ping effect, an index into web.Effects (0 fade-in ... 4 disco, 5 siren ... 10 comet)")
	webName := flag.String("web-name", cfg.Web.Name, "whose ring it is, shown on the ping page")
	webPickFX := flag.Bool("web-pickfx", cfg.Web.PickFX, "let teammates pick the ping effect on the page")
	webTZ := flag.String("web-tz", cfg.Web.Timezone, "IANA time zone whose midnight expires teammates' nicks, e.g. America/Bogota")
	fx := flag.Int("fx", cfg.FX, "force LED effect during calls (0 fade-in, 1 clock, 2 voice, 3 scanner, 4 disco, 5-10 see README); -1 follows call length")
	flag.Parse()

	log.Printf("config %s: serial=%q http=%q processes=%v", cfgPath, *serialPort, *httpURL, detector.TeamsExes)
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
	ring := actuator.NewGroup(acts...)
	actuate := ring.SetCallState
	var pinging atomic.Bool // a teammate ping is playing: the resync must not cut it short

	mgr := events.NewManager(detector.IsTeamsCallActive)
	mgr.Interval, mgr.Debounce = *interval, *debounce
	mgr.OnStateChange(func(s events.CallState) {
		log.Printf("state: %v", s)
		actuate(s == events.StateInCall)
	})

	if *webAddr != "" {
		if *webDuration < time.Second || *webDuration > time.Minute {
			log.Fatalf("web duration %v: want 1s-60s (each ping holds up the queue this long)", *webDuration)
		}
		if *webFX < 0 || *webFX >= len(web.Effects) {
			log.Fatalf("web fx %d: want 0-%d", *webFX, len(web.Effects)-1)
		}
		loc, err := time.LoadLocation(*webTZ)
		if err != nil {
			log.Fatalf("web tz: %v", err)
		}
		start := func(fx int, nick string) error {
			pinging.Store(true)
			err := ring.Send(fmt.Sprintf("FX %d\n", fx))
			if err != nil {
				pinging.Store(false) // board unreachable: the web queue holds the ping and retries
				return err
			}
			if !*headless {
				go ui.Notify("Ping de "+nick, "Efecto: "+web.Effects[fx])
			}
			return nil
		}
		stop := func() {
			ring.Sync(func() bool { return mgr.State() == events.StateInCall }) // back to the real state (restarts the call-length timeline)
			pinging.Store(false)
		}
		opts := web.Options{Cooldown: *webCooldown, Duration: *webDuration, FX: *webFX, PickFX: *webPickFX, Name: strings.TrimSpace(*webName), Loc: loc}
		go func() { log.Printf("web: %v", http.ListenAndServe(*webAddr, web.New(start, stop, opts))) }()
		log.Printf("web: ping page on http://%s", *webAddr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	actuate(false) // sync the LED ring with our initial Idle state
	if *resync > 0 {
		// The board only hears about transitions. If it reboots mid-call (brownout, crash, WiFi
		// drop) it wakes up dark, so resend the state; a repeated CALL_ON keeps the call timer.
		go func() {
			t := time.NewTicker(*resync)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if !pinging.Load() {
						ring.Sync(func() bool { return mgr.State() == events.StateInCall })
					}
				}
			}
		}()
	}
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
