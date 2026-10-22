# TASK 02: State Machine & Debounce Event Manager

## Description
Build a state machine that periodically polls the detector, handles state stabilization (debounce), and emits clean events on state transition (`ON_CALL_START`, `ON_CALL_END`).

## Requirements
1. Poll interval configurable (default: 1000ms).
2. Debounce buffer (default: 2 consecutive positive or negative checks to confirm transition).
3. Event Channel / Callback hooks for Call State changes.

## File Structure Target
`pkg/events/event_manager.go`

## Implementation Checklist
- [x] Define `CallState` enum (`StateIdle`, `StateInCall`).
- [x] Implement `Manager` struct with channel listeners `OnStateChange(func(newState CallState))`.
- [x] Implement polling ticker loop with thread-safe atomic status updates.
