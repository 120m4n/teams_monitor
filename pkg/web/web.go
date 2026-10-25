// Package web serves a one-button page that lets teammates flash the LED ring. It listens on
// loopback and is published with Tailscale Funnel (docs/INSTALL.md). Each client gets one ping
// per cooldown, and at most MaxQueue pings wait their turn. GET /status tells a client where its
// ping is in the queue, and GET /config tells the page the duration and effects. While the ring is
// unreachable the queue holds: the head ping is retried until the ring takes it or it gets too old.
package web

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed index.html
var page []byte // {{NAME}} is replaced by Options.Name

//go:embed welcome.html
var welcome []byte // asks for the nickname the page needs; {{NAME}} as in page

// Same rule as the pages: letters and digits only, so a nick is safe anywhere it's shown.
var nickRe = regexp.MustCompile(`^[A-Za-z0-9]{2,16}$`)

const MaxQueue = 10

// While the ring is unreachable, the head ping is retried every retryEvery, and pings older than
// maxWait are dropped so one doesn't go off hours later. Vars so tests can shorten them.
var retryEvery, maxWait = 5 * time.Second, 15 * time.Minute

// Effects are the firmware's effect names, indexed by FX number (FX_NAMES in
// firmware/esp8266/src/main.cpp; TestEffectsMatchFirmware keeps them in sync). The page maps
// each name to its own copy of the animation.
var Effects = []string{"fade-in", "clock", "voice", "scanner", "disco",
	"siren", "heartbeat", "pacman", "roulette", "fire", "comet"}

type Options struct {
	Cooldown time.Duration // between pings from one client
	Duration time.Duration // how long one ping plays; the page shows its animation this long
	FX       int           // effect used when the visitor doesn't pick one
	PickFX   bool          // let visitors pick the effect
	Name     string        // whose ring it is, shown on the page; set once, at startup
}

type entry struct {
	who  string
	nick string
	fx   int
	at  time.Time // accepted
}

type server struct {
	Options
	page    []byte        // index.html with the name filled in
	welcome []byte        // welcome.html with the name filled in
	wake chan struct{} // nudges the worker after an enqueue

	mu      sync.Mutex
	last    map[string]time.Time // client -> time of its last accepted ping
	waiting []entry              // pings waiting, oldest first
	playing entry                // ping playing now, zero if none
	offline bool                 // the last attempt to start a ping failed
}

// New returns the handler. start starts effect fx on the ring, or fails if the ring is
// unreachable; stop runs o.Duration after a start that worked. Pings run one at a time.
func New(start func(fx int) error, stop func(), o Options) http.Handler {
	name := []byte(html.EscapeString(o.Name))
	s := &server{Options: o, wake: make(chan struct{}, 1), last: map[string]time.Time{},
		page: bytes.ReplaceAll(page, []byte("{{NAME}}"), name), welcome: bytes.ReplaceAll(welcome, []byte("{{NAME}}"), name)}
	go s.work(start, stop)
	mux := http.NewServeMux()
	serve := func(b []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(b)
		}
	}
	mux.HandleFunc("GET /{$}", serve(s.page))
	mux.HandleFunc("GET /welcome", serve(s.welcome))
	mux.HandleFunc("POST /ping", s.ping)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Duration int      `json:"duration"` // seconds
			FX       int      `json:"fx"`
			PickFX   bool     `json:"pickfx"`
			Effects  []string `json:"effects"`
		}{secs(s.Duration), s.FX, s.PickFX, Effects})
	})
	return mux
}

func (s *server) work(start func(fx int) error, stop func()) {
	for range s.wake {
		for {
			s.mu.Lock()
			s.playing = entry{}
			s.expire(time.Now())
			if len(s.waiting) == 0 {
				s.mu.Unlock()
				break
			}
			e := s.waiting[0] // only this worker removes entries, so it stays at the head
			s.mu.Unlock()
			if err := start(e.fx); err != nil {
				s.mu.Lock()
				if !s.offline {
					log.Printf("web: ring unreachable, holding %d ping(s)", len(s.waiting))
				}
				s.offline = true
				s.mu.Unlock()
				time.Sleep(retryEvery)
				continue
			}
			s.mu.Lock()
			s.offline = false
			s.playing, s.waiting = e, s.waiting[1:]
			s.mu.Unlock()
			log.Printf("web: ping from %s (%s), fx %d", e.nick, e.who, e.fx)
			time.Sleep(s.Duration)
			stop()
		}
	}
}

// expire drops pings that waited maxWait or more. Call with s.mu held.
func (s *server) expire(now time.Time) {
	n := 0
	for _, e := range s.waiting {
		if now.Sub(e.at) < maxWait {
			s.waiting[n] = e
			n++
		}
	}
	if n < len(s.waiting) {
		log.Printf("web: dropped %d ping(s) older than %v", len(s.waiting)-n, maxWait)
		clear(s.waiting[n:])
		s.waiting = s.waiting[:n]
	}
}

type reply struct {
	Position    int    `json:"position,omitempty"` // pings waiting, this one included
	Error       string `json:"error,omitempty"`
	Retry       int    `json:"retry"`                  // seconds until this client may ping again
	RingOffline bool   `json:"ring_offline,omitempty"` // the ping waits until the ring is back
}

func (s *server) ping(w http.ResponseWriter, r *http.Request) {
	nick := r.FormValue("nick")
	if !nickRe.MatchString(nick) { // the page sends the visitor to the welcome page on this
		send(w, http.StatusBadRequest, reply{Error: "Nick no válido"})
		return
	}
	fx := s.FX
	if v := r.FormValue("fx"); v != "" && s.PickFX { // ignored when picking is off
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n >= len(Effects) {
			send(w, http.StatusBadRequest, reply{Error: "Animación desconocida"})
			return
		}
		fx = n
	}
	who := clientIP(r)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.last[who]; ok && now.Sub(t) < s.Cooldown {
		send(w, http.StatusTooManyRequests, reply{Error: "Demasiado pronto", Retry: secs(s.Cooldown - now.Sub(t))})
		return
	}
	if len(s.waiting) >= MaxQueue {
		send(w, http.StatusTooManyRequests, reply{Error: "Cola llena", Retry: 10})
		return
	}
	s.waiting = append(s.waiting, entry{who, nick, fx, now})
	select {
	case s.wake <- struct{}{}:
	default: // the worker is already awake and drains the whole queue
	}
	for k, t := range s.last { // forget expired clients so the map stays small
		if now.Sub(t) >= s.Cooldown {
			delete(s.last, k)
		}
	}
	s.last[who] = now
	send(w, http.StatusAccepted, reply{Position: len(s.waiting), Retry: secs(s.Cooldown), RingOffline: s.offline})
}

type place struct {
	Position    int  `json:"position"`     // place of the caller's ping among the waiting ones, 1-based; 0 if not waiting
	Ahead       int  `json:"ahead"`        // pings that play before it, the one playing now included
	Playing     bool `json:"playing"`      // the caller's ping is playing now
	RingOffline bool `json:"ring_offline"` // the ring is unreachable; waiting pings are held
}

type status struct {
	place
	Queue []string `json:"queue"` // nicks, the one playing now first; the caller's is at index ahead
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	who := clientIP(r)
	s.mu.Lock()
	st := status{place: place{Playing: s.playing.who == who && who != "", RingOffline: s.offline}, Queue: []string{}}
	if s.playing.who != "" {
		st.Queue = append(st.Queue, s.playing.nick)
	}
	for i, e := range s.waiting {
		if e.who == who && st.Position == 0 {
			st.Position, st.Ahead = i+1, len(st.Queue)
		}
		st.Queue = append(st.Queue, e.nick)
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(st)
}

func send(w http.ResponseWriter, code int, r reply) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(r)
}

func secs(d time.Duration) int { return int((d + time.Second - 1) / time.Second) }

// clientIP is the caller's address. Funnel requests reach us from loopback, and the Tailscale
// proxy appends the real client to X-Forwarded-For. Only that last entry is trusted, because the
// client can put anything in the earlier ones.
func clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			parts := strings.Split(xff[len(xff)-1], ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	return host
}
