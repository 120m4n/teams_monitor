package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func post(h http.Handler, remote, xff string) int {
	r := httptest.NewRequest("POST", "/ping", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestPing(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	h := New(func() { <-block }, time.Hour) // the first ping never finishes, so the queue fills up

	if c := post(h, "203.0.113.1:5000", ""); c != http.StatusAccepted {
		t.Fatalf("first ping: %d", c)
	}
	if c := post(h, "203.0.113.1:5001", ""); c != http.StatusTooManyRequests {
		t.Fatalf("same client within cooldown: %d", c)
	}
	// From loopback (the Funnel proxy), the last X-Forwarded-For entry is the client; a spoofed first entry is ignored.
	if c := post(h, "127.0.0.1:9", "203.0.113.1, 198.51.100.7"); c != http.StatusAccepted {
		t.Fatalf("proxied client: %d", c)
	}
	if c := post(h, "127.0.0.1:9", "203.0.113.9, 198.51.100.7"); c != http.StatusTooManyRequests {
		t.Fatalf("spoofed XFF bypassed cooldown: %d", c)
	}
	// X-Forwarded-For from a non-loopback client is not trusted.
	if c := post(h, "203.0.113.1:5002", "192.0.2.50"); c != http.StatusTooManyRequests {
		t.Fatalf("untrusted XFF bypassed cooldown: %d", c)
	}

	// Fill the queue with distinct clients until it rejects one.
	accepted := 0
	for i := range MaxQueue + 2 {
		if post(h, "127.0.0.1:9", "10.0.0."+string(rune('a'+i))) == http.StatusAccepted {
			accepted++
		}
	}
	if accepted > MaxQueue {
		t.Fatalf("queue accepted %d more pings, max %d", accepted, MaxQueue)
	}
	if c := post(h, "127.0.0.1:9", "10.9.9.9"); c != http.StatusTooManyRequests {
		t.Fatalf("full queue: %d", c)
	}
}
