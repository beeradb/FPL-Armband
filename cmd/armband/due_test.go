package main

import (
	"testing"
	"time"

	"armband/internal/fpl"
)

// The gate exists to stop a scheduled run billing repeatedly. Every case here
// is a way it could quietly spend money it should not.
func TestCheckDueOnlyFiresInsideTheWindow(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	at := func(h float64) *fpl.Event {
		return &fpl.Event{ID: 1, DeadlineTime: now.Add(time.Duration(h * float64(time.Hour)))}
	}

	for _, tc := range []struct {
		name string
		next *fpl.Event
		want bool
	}{
		{"well outside the window", at(72), false},
		{"just outside", at(6.5), false},
		{"inside", at(3), true},
		{"minutes to spare", at(0.25), true},
		{"deadline passed", at(-1), false},
		{"no upcoming gameweek", nil, false},
	} {
		got := checkDue(tc.next, 6, now)
		if got.Run != tc.want {
			t.Errorf("%s: Run=%v, want %v (%s)", tc.name, got.Run, tc.want, got.Reason)
		}
		if !got.Run && got.Reason == "" {
			t.Errorf("%s: declined without saying why", tc.name)
		}
	}
}

// A deadline that has already passed must never fire. Running after one spends
// tokens deciding a gameweek that can no longer be changed.
func TestCheckDueNeverFiresAfterTheDeadline(t *testing.T) {
	now := time.Now()
	past := &fpl.Event{ID: 7, DeadlineTime: now.Add(-time.Minute)}
	if v := checkDue(past, 6, now); v.Run {
		t.Error("fired after the deadline had passed")
	}
	// Even with an absurd lead time, which would otherwise swallow the whole
	// season and make every gameweek look imminent.
	if v := checkDue(past, 10000, now); v.Run {
		t.Error("a long lead time resurrected a passed deadline")
	}
}
