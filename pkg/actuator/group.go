package actuator

import (
	"log"
	"sync"
)

// Group sends each command to several actuators, one command at a time, so a periodic resync
// can't land after a newer state. It logs an actuator's error only when it changes, and once
// more when the actuator recovers, so a board that is off for hours doesn't flood the log.
type Group struct {
	Logf func(format string, args ...any)

	mu      sync.Mutex
	acts    []Actuator
	lastErr []string
}

func NewGroup(acts ...Actuator) *Group {
	return &Group{Logf: log.Printf, acts: acts, lastErr: make([]string, len(acts))}
}

func (g *Group) SetCallState(active bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.each(func(a Actuator) error { return a.SetCallState(active) })
}

func (g *Group) Send(msg string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.each(func(a Actuator) error { return a.Send(msg) })
}

// Sync sends the state active() reports, reading it under the lock so it is never stale.
func (g *Group) Sync(active func() bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	v := active()
	g.each(func(a Actuator) error { return a.SetCallState(v) })
}

func (g *Group) each(do func(Actuator) error) {
	for i, a := range g.acts {
		msg := ""
		if err := do(a); err != nil {
			msg = err.Error()
		}
		switch {
		case msg == g.lastErr[i]:
		case msg == "":
			g.Logf("actuator: ok again")
		default:
			g.Logf("actuator: %s", msg)
		}
		g.lastErr[i] = msg
	}
}
