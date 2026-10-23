// Package web serves a one-button page that lets teammates flash the LED ring. It listens on
// loopback and is published with Tailscale Funnel (docs/INSTALL.md). Each client gets one ping
// per cooldown, and at most MaxQueue pings wait their turn. GET /status tells a client where its
// ping is in the queue, and GET /config tells the page the duration and effects.
package web

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed index.html
var page []byte // {{NAME}} is replaced by Options.Name

const (
	MaxQueue = 10
	FXCount  = 5 // firmware effects: 0 fade-in, 1 clock, 2 voice, 3 scanner, 4 disco
)

type Options struct {
	Cooldown time.Duration // between pings from one client
	Duration time.Duration // how long one ping plays; the page shows its animation this long
	FX       int           // effect used when the visitor doesn't pick one
	PickFX   bool          // let visitors pick the effect
	Name     string        // whose ring it is, shown on the page; set once, at startup
}

type entry struct {
	who string
	fx  int
}

type server struct {
	Options
	page []byte        // index.html with the name filled in
	wake chan struct{} // nudges the worker after an enqueue

	mu      sync.Mutex
	last    map[string]time.Time // client -> time of its last accepted ping
	waiting []entry              // pings waiting, oldest first
	playing string               // client whose ping is playing now, "" if none
}

// New returns the handler. ping plays effect fx on the ring and blocks for o.Duration; pings run
// one at a time.
func New(ping func(fx int), o Options) http.Handler {
	s := &server{Options: o, wake: make(chan struct{}, 1), last: map[string]time.Time{},
		page: bytes.ReplaceAll(page, []byte("{{NAME}}"), []byte(html.EscapeString(o.Name)))}
	go s.work(ping)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(s.page)
	})
	mux.HandleFunc("POST /ping", s.ping)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Duration int  `json:"duration"` // seconds
			FX       int  `json:"fx"`
			PickFX   bool `json:"pickfx"`
		}{secs(s.Duration), s.FX, s.PickFX})
	})
	return mux
}

func (s *server) work(ping func(fx int)) {
	for range s.wake {
		for {
			s.mu.Lock()
			if len(s.waiting) == 0 {
				s.playing = ""
				s.mu.Unlock()
				break
			}
			e := s.waiting[0]
			s.playing, s.waiting = e.who, s.waiting[1:]
			s.mu.Unlock()
			log.Printf("web: ping from %s, fx %d", e.who, e.fx)
			ping(e.fx)
		}
	}
}

type reply struct {
	Position int    `json:"position,omitempty"` // pings waiting, this one included
	Error    string `json:"error,omitempty"`
	Retry    int    `json:"retry"` // seconds until this client may ping again
}

func (s *server) ping(w http.ResponseWriter, r *http.Request) {
	fx := s.FX
	if v := r.FormValue("fx"); v != "" && s.PickFX { // ignored when picking is off
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n >= FXCount {
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
	s.waiting = append(s.waiting, entry{who, fx})
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
	send(w, http.StatusAccepted, reply{Position: len(s.waiting), Retry: secs(s.Cooldown)})
}

type status struct {
	Position int  `json:"position"` // place of the caller's ping among the waiting ones, 1-based; 0 if not waiting
	Ahead    int  `json:"ahead"`    // pings that play before it, the one playing now included
	Playing  bool `json:"playing"`  // the caller's ping is playing now
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	who := clientIP(r)
	s.mu.Lock()
	st := status{Playing: s.playing == who && who != ""}
	for i, e := range s.waiting {
		if e.who == who {
			st.Position, st.Ahead = i+1, i
			if s.playing != "" {
				st.Ahead++
			}
			break
		}
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
