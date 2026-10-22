// Package events turns raw detector readings into debounced call-state transitions.
package events

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type CallState int32

const (
	StateIdle CallState = iota
	StateInCall
)

func (s CallState) String() string {
	if s == StateInCall {
		return "IN_CALL"
	}
	return "IDLE"
}

// Manager polls Check and fires listeners once Debounce consecutive readings
// disagree with the current state.
type Manager struct {
	Interval time.Duration // default 1s
	Debounce int           // default 2
	Check    func() (bool, error)

	state     atomic.Int32
	streak    int // only touched by the Run goroutine
	mu        sync.Mutex
	listeners []func(CallState)
}

func NewManager(check func() (bool, error)) *Manager {
	return &Manager{Interval: time.Second, Debounce: 2, Check: check}
}

// OnStateChange registers fn; it is called on the Run goroutine for each transition.
func (m *Manager) OnStateChange(fn func(newState CallState)) {
	m.mu.Lock()
	m.listeners = append(m.listeners, fn)
	m.mu.Unlock()
}

// State is safe to call from any goroutine.
func (m *Manager) State() CallState { return CallState(m.state.Load()) }

// Run polls until ctx is cancelled. Check is always called from the goroutine
// running Run, so COM (detector.InitCOM) must be initialized on it beforehand.
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(m.Interval)
	defer t.Stop()
	for {
		m.poll()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) poll() {
	active, err := m.Check()
	if err != nil {
		log.Printf("events: check failed: %v", err) // keep current state, don't count toward debounce
		return
	}
	reading := StateIdle
	if active {
		reading = StateInCall
	}
	if reading == m.State() {
		m.streak = 0
		return
	}
	if m.streak++; m.streak < m.Debounce {
		return
	}
	m.streak = 0
	m.state.Store(int32(reading))

	m.mu.Lock()
	fns := append([]func(CallState){}, m.listeners...)
	m.mu.Unlock()
	for _, fn := range fns {
		fn(reading)
	}
}
