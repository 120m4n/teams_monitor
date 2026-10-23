package web

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var opts = Options{Cooldown: time.Hour, Duration: 5 * time.Second, FX: 4, PickFX: true}

func post(h http.Handler, remote, xff string) int { return postURL(h, "/ping", remote, xff) }

func postURL(h http.Handler, url, remote, xff string) int {
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
	h := New(func(int) { <-block }, opts) // the first ping never finishes, so the queue fills up

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

func getStatus(t *testing.T, h http.Handler, remote string) status {
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
	h := New(func(int) { <-release }, opts)
	a, b, c := "203.0.113.1:1", "203.0.113.2:1", "203.0.113.3:1"

	post(h, a, "")
	for deadline := time.Now().Add(time.Second); !getStatus(t, h, a).Playing; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("a's ping never started playing")
		}
	}
	post(h, b, "")
	post(h, c, "")
	if st := getStatus(t, h, b); st != (status{Position: 1, Ahead: 1}) {
		t.Errorf("b: %+v", st)
	}
	if st := getStatus(t, h, c); st != (status{Position: 2, Ahead: 2}) {
		t.Errorf("c: %+v", st)
	}
	if st := getStatus(t, h, "198.51.100.1:1"); st != (status{}) {
		t.Errorf("stranger: %+v", st)
	}

	release <- struct{}{} // a done, b plays
	for deadline := time.Now().Add(time.Second); !getStatus(t, h, b).Playing; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("b's ping never started playing")
		}
	}
	if st := getStatus(t, h, a); st != (status{}) {
		t.Errorf("a after playing: %+v", st)
	}
	if st := getStatus(t, h, c); st != (status{Position: 1, Ahead: 1}) {
		t.Errorf("c after a: %+v", st)
	}
	close(release)
}

func TestPickFX(t *testing.T) {
	played := make(chan int, 4)
	h := New(func(fx int) { played <- fx }, opts)
	for _, c := range []struct {
		url  string
		code int
		fx   int
	}{
		{"/ping?fx=2", http.StatusAccepted, 2},
		{"/ping", http.StatusAccepted, 4}, // default
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
	h = New(func(fx int) { played <- fx }, off)
	postURL(h, "/ping?fx=2", "203.0.113.50:1", "")
	if fx := <-played; fx != 4 {
		t.Errorf("pickfx off played fx %d, want the default 4", fx)
	}
}

func TestPageName(t *testing.T) {
	o := opts
	o.Name = `Ana "<b>"`
	w := httptest.NewRecorder()
	New(func(int) {}, o).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	body, _ := io.ReadAll(w.Body)
	if !strings.Contains(string(body), `data-name="Ana &#34;&lt;b&gt;&#34;"`) || strings.Contains(string(body), "{{NAME}}") {
		t.Fatalf("name not filled in escaped: %.200s", body)
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
