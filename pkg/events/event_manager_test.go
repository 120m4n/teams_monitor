package events

import (
	"errors"
	"reflect"
	"testing"
)

func TestDebounce(t *testing.T) {
	type r struct {
		active bool
		err    error
	}
	readings := []r{
		{true, nil},              // 1st positive: not yet
		{false, nil},             // flicker resets streak
		{true, nil}, {true, nil}, // 2 in a row -> IN_CALL
		{false, nil},                // single drop ignored
		{true, nil},                 //
		{false, errors.New("boom")}, // errors don't count
		{false, nil}, {false, nil},  // 2 in a row -> IDLE
		{false, nil}, // steady, no event
	}
	i := 0
	m := NewManager(func() (bool, error) { x := readings[i]; i++; return x.active, x.err })

	var got []CallState
	m.OnStateChange(func(s CallState) { got = append(got, s) })
	for range readings {
		m.poll()
	}

	if want := []CallState{StateInCall, StateIdle}; !reflect.DeepEqual(got, want) {
		t.Fatalf("transitions = %v, want %v", got, want)
	}
	if m.State() != StateIdle {
		t.Fatalf("final state = %v", m.State())
	}
}
