// Package web serves a one-button page that lets teammates flash the LED ring. It listens on
// loopback and is published with Tailscale Funnel (docs/INSTALL.md). Each client gets one ping
// per cooldown, and at most MaxQueue pings wait their turn. GET /status tells a client where its
// ping is in the queue.
package web

import (
	_ "embed"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed index.html
var page []byte

const MaxQueue = 10

type server struct {
	cooldown time.Duration
	wake     chan struct{} // nudges the worker after an enqueue

	mu      sync.Mutex
	last    map[string]time.Time // client -> time of its last accepted ping
	waiting []string             // clients whose pings wait, oldest first
	playing string               // client whose ping is playing now, "" if none
}

// New returns the handler. ping plays the effect and blocks for its duration; pings run one at a time.
func New(ping func(), cooldown time.Duration) http.Handler {
	s := &server{cooldown: cooldown, wake: make(chan struct{}, 1), last: map[string]time.Time{}}
	go s.work(ping)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("POST /ping", s.ping)
	mux.HandleFunc("GET /status", s.status)
	return mux
}

func (s *server) work(ping func()) {
	for range s.wake {
		for {
			s.mu.Lock()
			if len(s.waiting) == 0 {
				s.playing = ""
				s.mu.Unlock()
				break
			}
			s.playing, s.waiting = s.waiting[0], s.waiting[1:]
			who := s.playing
			s.mu.Unlock()
			log.Printf("web: ping from %s", who)
			ping()
		}
	}
}

type reply struct {
	Position int    `json:"position,omitempty"` // pings waiting, this one included
	Error    string `json:"error,omitempty"`
	Retry    int    `json:"retry"` // seconds until this client may ping again
}

func (s *server) ping(w http.ResponseWriter, r *http.Request) {
	who := clientIP(r)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.last[who]; ok && now.Sub(t) < s.cooldown {
		send(w, http.StatusTooManyRequests, reply{Error: "Demasiado pronto", Retry: secs(s.cooldown - now.Sub(t))})
		return
	}
	if len(s.waiting) >= MaxQueue {
		send(w, http.StatusTooManyRequests, reply{Error: "Cola llena", Retry: 10})
		return
	}
	s.waiting = append(s.waiting, who)
	select {
	case s.wake <- struct{}{}:
	default: // the worker is already awake and drains the whole queue
	}
	for k, t := range s.last { // forget expired clients so the map stays small
		if now.Sub(t) >= s.cooldown {
			delete(s.last, k)
		}
	}
	s.last[who] = now
	send(w, http.StatusAccepted, reply{Position: len(s.waiting), Retry: secs(s.cooldown)})
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
	for i, c := range s.waiting {
		if c == who {
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
