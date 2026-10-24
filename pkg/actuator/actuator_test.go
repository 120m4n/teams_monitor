package actuator

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHTTPActuator(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
	}))
	defer srv.Close()

	a := NewHTTPActuator(srv.URL)
	if err := a.SetCallState(true); err != nil {
		t.Fatal(err)
	}
	if err := a.SetCallState(false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"CALL_ON\n", "CALL_OFF\n"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}

	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	if err := a.SetCallState(true); err == nil {
		t.Fatal("expected error on 500")
	}
}

type fakePort struct {
	written []string
	broken  bool
	closed  bool
}

func (p *fakePort) Write(b []byte) (int, error) {
	if p.broken {
		return 0, errors.New("device unplugged")
	}
	p.written = append(p.written, string(b))
	return len(b), nil
}
func (p *fakePort) Close() error { p.closed = true; return nil }

func TestSerialReconnect(t *testing.T) {
	var ports []*fakePort
	a := &SerialActuator{open: func() (io.WriteCloser, error) {
		p := &fakePort{}
		ports = append(ports, p)
		return p, nil
	}}

	if err := a.SetCallState(true); err != nil {
		t.Fatal(err)
	}
	ports[0].broken = true // unplugged
	if err := a.SetCallState(false); err != nil {
		t.Fatal(err)
	}

	if len(ports) != 2 || !ports[0].closed {
		t.Fatalf("expected stale port closed and reopened, got %d opens", len(ports))
	}
	if ports[0].written[0] != "CALL_ON\n" || ports[1].written[0] != "CALL_OFF\n" {
		t.Fatalf("writes: %q / %q", ports[0].written, ports[1].written)
	}
}

type fakeActuator struct {
	sent []string
	err  error
}

func (f *fakeActuator) SetCallState(active bool) error { return f.Send(payload(active)) }
func (f *fakeActuator) Send(msg string) error {
	f.sent = append(f.sent, msg)
	return f.err
}

func TestGroupLogsErrorChangesOnly(t *testing.T) {
	a := &fakeActuator{}
	g := NewGroup(a)
	var logs []string
	g.Logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }

	g.SetCallState(true)
	a.err = errors.New("timeout")
	g.Sync(func() bool { return true }) // board gone: logged once...
	g.Sync(func() bool { return true }) // ...not on every resync
	if err := g.Send("FX 4\n"); err == nil {
		t.Error("Send with the only actuator down: want an error")
	}
	a.err = nil
	if err := g.Send("FX 4\n"); err != nil { // back
		t.Error(err)
	}
	g.SetCallState(false)

	if want := []string{"CALL_ON\n", "CALL_ON\n", "CALL_ON\n", "FX 4\n", "FX 4\n", "CALL_OFF\n"}; !reflect.DeepEqual(a.sent, want) {
		t.Errorf("sent %q, want %q", a.sent, want)
	}
	if want := []string{"actuator: timeout", "actuator: ok again"}; !reflect.DeepEqual(logs, want) {
		t.Errorf("logs %q, want %q", logs, want)
	}
}

func TestGroupSendOneUpIsEnough(t *testing.T) {
	down, up := &fakeActuator{err: errors.New("timeout")}, &fakeActuator{}
	g := NewGroup(down, up)
	g.Logf = func(string, ...any) {}
	if err := g.Send("FX 1\n"); err != nil {
		t.Fatalf("serial up, http down: %v", err)
	}
}
