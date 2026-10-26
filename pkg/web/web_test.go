package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var opts = Options{Cooldown: time.Hour, Duration: time.Millisecond, FX: 4, PickFX: true}

func started(int, string) error { return nil }
func stopped()                  {}

func post(h http.Handler, remote, xff string) int { return postURL(h, "/ping", remote, xff) }

// postURL pings with a valid nick added; postRaw sends url as is.
func postURL(h http.Handler, url, remote, xff string) int {
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return postRaw(h, url+sep+"nick=tester", remote, xff)
}

func postRaw(h http.Handler, url, remote, xff string) int {
	r := httptest.NewRequest("POST", url, nil)
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
	h := New(started, func() { <-block }, opts) // the first ping never finishes, so the queue fills up

	for _, url := range []string{"/ping", "/ping?nick=", "/ping?nick=a", "/ping?nick=ana%20b", "/ping?nick=a%C3%B1o",
		"/ping?nick=ana!", "/ping?nick=abcdefghijklmnopq"} { // missing, short, space, ñ, symbol, 17 chars
		if c := postRaw(h, url, "203.0.113.200:1", ""); c != http.StatusBadRequest {
			t.Fatalf("%s: %d, want 400", url, c)
		}
	}
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

func getStatus(t *testing.T, h http.Handler, remote string) place {
	return getQueue(t, h, remote).place
}

func getQueue(t *testing.T, h http.Handler, remote string) status {
	r := httptest.NewRequest("GET", "/status", nil)
	r.RemoteAddr = remote
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var st status
	if err := json.NewDecoder(w.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestStatus(t *testing.T) {
	release := make(chan struct{})
	h := New(started, func() { <-release }, opts)
	a, b, c := "203.0.113.1:1", "203.0.113.2:1", "203.0.113.3:1"

	postRaw(h, "/ping?nick=ana", a, "")
	for deadline := time.Now().Add(time.Second); !getStatus(t, h, a).Playing; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("a's ping never started playing")
		}
	}
	postRaw(h, "/ping?nick=bob", b, "")
	postRaw(h, "/ping?nick=Carlos2024xyz", c, "")
	if q := getQueue(t, h, b).Queue; !slices.Equal(q, []string{"ana", "bob", "Carlos2024xyz"}) {
		t.Errorf("queue: %q", q)
	}
	if st := getStatus(t, h, b); st != (place{Position: 1, Ahead: 1}) {
		t.Errorf("b: %+v", st)
	}
	if st := getStatus(t, h, c); st != (place{Position: 2, Ahead: 2}) {
		t.Errorf("c: %+v", st)
	}
	if st := getStatus(t, h, "198.51.100.1:1"); st != (place{}) {
		t.Errorf("stranger: %+v", st)
	}

	release <- struct{}{} // a done, b plays
	for deadline := time.Now().Add(time.Second); !getStatus(t, h, b).Playing; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("b's ping never started playing")
		}
	}
	if st := getStatus(t, h, a); st != (place{}) {
		t.Errorf("a after playing: %+v", st)
	}
	if st := getStatus(t, h, c); st != (place{Position: 1, Ahead: 1}) {
		t.Errorf("c after a: %+v", st)
	}
	if q := getQueue(t, h, c).Queue; !slices.Equal(q, []string{"bob", "Carlos2024xyz"}) {
		t.Errorf("queue after a: %q", q)
	}
	close(release)
}

func TestPickFX(t *testing.T) {
	played := make(chan int, 4)
	h := New(func(fx int, nick string) error {
		if nick != "tester" {
			t.Errorf("start got nick %q, want tester", nick)
		}
		played <- fx
		return nil
	}, stopped, opts)
	for _, c := range []struct {
		url  string
		code int
		fx   int
	}{
		{"/ping?fx=2", http.StatusAccepted, 2},
		{"/ping", http.StatusAccepted, 4},        // default
		{"/ping?fx=10", http.StatusAccepted, 10}, // comet, the last effect
		{"/ping?fx=11", http.StatusBadRequest, -1},
		{"/ping?fx=x", http.StatusBadRequest, -1},
	} {
		if code := postURL(h, c.url, "203.0.113."+c.url[len(c.url)-1:]+":1", ""); code != c.code {
			t.Fatalf("%s: %d, want %d", c.url, code, c.code)
		}
		if c.fx >= 0 {
			if fx := <-played; fx != c.fx {
				t.Errorf("%s played fx %d, want %d", c.url, fx, c.fx)
			}
		}
	}

	off := opts
	off.PickFX = false
	h = New(func(fx int, _ string) error { played <- fx; return nil }, stopped, off)
	postURL(h, "/ping?fx=2", "203.0.113.50:1", "")
	if fx := <-played; fx != 4 {
		t.Errorf("pickfx off played fx %d, want the default 4", fx)
	}
}

func TestRingOffline(t *testing.T) {
	retryEvery = time.Millisecond
	defer func() { retryEvery = 5 * time.Second }()
	var mu sync.Mutex
	up := false
	played := make(chan int, 4)
	h := New(func(fx int, _ string) error {
		mu.Lock()
		defer mu.Unlock()
		if !up {
			return errors.New("timeout")
		}
		played <- fx
		return nil
	}, stopped, opts)
	a, b := "203.0.113.1:1", "203.0.113.2:1"

	postURL(h, "/ping?fx=1", a, "")
	for deadline := time.Now().Add(time.Second); !getStatus(t, h, a).RingOffline; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("status never reported the ring offline")
		}
	}
	postURL(h, "/ping?fx=2", b, "")
	if st := getStatus(t, h, a); st != (place{Position: 1, RingOffline: true}) {
		t.Errorf("a while offline: %+v", st)
	}
	if st := getStatus(t, h, b); st != (place{Position: 2, Ahead: 1, RingOffline: true}) {
		t.Errorf("b while offline: %+v", st)
	}

	mu.Lock()
	up = true // the board is back: the held pings play in order
	mu.Unlock()
	for _, want := range []int{1, 2} {
		select {
		case fx := <-played:
			if fx != want {
				t.Errorf("played fx %d, want %d", fx, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("fx %d never played", want)
		}
	}
	if st := getStatus(t, h, b); st.RingOffline {
		t.Errorf("still offline after a ping played: %+v", st)
	}
}

func TestExpire(t *testing.T) {
	now := time.Now()
	s := &server{waiting: []entry{{"old", "o", 0, now.Add(-maxWait)}, {"new", "n", 1, now.Add(-time.Minute)}}}
	s.expire(now)
	if len(s.waiting) != 1 || s.waiting[0].who != "new" {
		t.Fatalf("waiting after expire: %+v", s.waiting)
	}
}

func TestPageName(t *testing.T) {
	o := opts
	o.Name = `Ana "<b>"`
	h := New(started, stopped, o)
	for _, url := range []string{"/", "/welcome"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		body, _ := io.ReadAll(w.Body)
		if !strings.Contains(string(body), `data-name="Ana &#34;&lt;b&gt;&#34;"`) || strings.Contains(string(body), "{{NAME}}") {
			t.Fatalf("%s: name not filled in escaped: %.200s", url, body)
		}
	}
}

func TestEffectsMatchFirmware(t *testing.T) {
	src, err := os.ReadFile("../../firmware/esp8266/src/main.cpp")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`FX_NAMES\[FX_COUNT\] = \{([^}]*)\}`).FindSubmatch(src)
	if m == nil {
		t.Fatal("FX_NAMES not found in main.cpp")
	}
	var fw []string
	for _, q := range regexp.MustCompile(`"([^"]*)"`).FindAllSubmatch(m[1], -1) {
		fw = append(fw, string(q[1]))
	}
	if !slices.Equal(fw, Effects) {
		t.Fatalf("firmware FX_NAMES %q != web.Effects %q", fw, Effects)
	}
	for _, name := range Effects { // the page needs a label (EFFECTS) and an animation (FX_LOOKS) for each
		key := regexp.MustCompile(`(?m)^\s*'?` + regexp.QuoteMeta(name) + `'?: `)
		if n := len(key.FindAllIndex(page, -1)); n < 2 {
			t.Errorf("index.html has %d entries for effect %q, want EFFECTS and FX_LOOKS", n, name)
		}
	}
}

func TestDayEnd(t *testing.T) {
	bogota, err := time.LoadLocation("America/Bogota")
	if err != nil {
		t.Fatal(err)
	}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		now  time.Time
		loc  *time.Location
		want string
	}{
		{time.Date(2026, 10, 27, 4, 59, 0, 0, time.UTC), bogota, "2026-10-27T00:00:00-05:00"}, // 23:59 on the 26th in Bogota
		{time.Date(2026, 10, 27, 5, 0, 0, 0, time.UTC), bogota, "2026-10-28T00:00:00-05:00"},  // midnight: a new day
		{time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), madrid, "2026-10-25T00:00:00+02:00"}, // DST ends on the 25th at 03:00
		{time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC), madrid, "2026-10-26T00:00:00+01:00"},
		{time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC), nil, "2026-10-27T00:00:00Z"},
	} {
		if got := dayEnd(c.now, c.loc).Format(time.RFC3339); got != c.want {
			t.Errorf("dayEnd(%v, %v) = %s, want %s", c.now, c.loc, got, c.want)
		}
	}
}

func TestConfigNickUntil(t *testing.T) {
	w := httptest.NewRecorder()
	New(started, stopped, opts).ServeHTTP(w, httptest.NewRequest("GET", "/config", nil))
	var c struct {
		NickUntil int64 `json:"nick_until"`
	}
	if err := json.NewDecoder(w.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	if left := time.Until(time.UnixMilli(c.NickUntil)); left <= 0 || left > 24*time.Hour {
		t.Fatalf("nick_until is %v away, want within the next day", left)
	}
}
