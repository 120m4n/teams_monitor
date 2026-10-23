// Package web serves a one-button page that lets teammates flash the LED ring. It listens on
// loopback and is published with Tailscale Funnel (docs/INSTALL.md). Each client gets one ping
// per cooldown, and at most MaxQueue pings wait their turn.
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
	queue    chan string

	mu   sync.Mutex
	last map[string]time.Time // client -> time of its last accepted ping
}

// New returns the handler. ping plays the effect and blocks for its duration; pings run one at a time.
func New(ping func(), cooldown time.Duration) http.Handler {
	s := &server{cooldown: cooldown, queue: make(chan string, MaxQueue), last: map[string]time.Time{}}
	go func() {
		for who := range s.queue {
			log.Printf("web: ping from %s", who)
			ping()
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("POST /ping", s.ping)
	return mux
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
	select {
	case s.queue <- who:
	default:
		send(w, http.StatusTooManyRequests, reply{Error: "Cola llena", Retry: 10})
		return
	}
	for k, t := range s.last { // forget expired clients so the map stays small
		if now.Sub(t) >= s.cooldown {
			delete(s.last, k)
		}
	}
	s.last[who] = now
	send(w, http.StatusAccepted, reply{Position: len(s.queue), Retry: secs(s.cooldown)})
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
