package main

import "armband/internal/fpl"

// importWindow reports the gameweek whose picks may be imported, and whether the import
// feature should be offered at all right now.
//
// # Why one field answers two questions
//
// FPL only returns picks for gameweek N once N's deadline has passed — which is the same
// instant IsNext moves off gameweek 1 and IsCurrent starts naming a real gameweek. So
// "is the feature on" and "which gameweek's picks are fetchable" are one fact, not two:
// once IsCurrent names a gameweek, that gameweek's picks exist to be imported, and the
// GW1 deadline having passed is exactly the condition under which that first becomes true.
// This is deliberate, not a coincidence to simplify away — do not split it into a separate
// "is it open" flag computed some other way, or the two can drift and offer an import the
// network call behind it cannot answer.
//
// # Why every empty or missing case closes the gate
//
// No events, no IsNext event, or no IsCurrent event all answer closed. That is exactly
// today's shipped GW1 behaviour — before the season's first deadline there is no squad
// anywhere to import — so nobody should "fix" a nil case here into an open default; doing
// so would offer an import that has nothing behind it. A finished season (no IsNext at
// all, every gameweek played) also closes: there is nothing left to plan for, so the
// planner's own opening-squad flow is the right one regardless of whether last season's
// picks could technically still be fetched.
//
// # Why "open" can lag the real deadline in production
//
// The gate is only as fresh as the bootstrap this process is holding, and
// fpl.Client.Bootstrap memoizes its result for the life of the process (see its own doc
// comment). So in `armband serve` this effectively opens on the first process restart
// after GW1's deadline, not at the deadline itself. That is an operational fact — rolling
// the deployment after GW1 closes is how the gate actually opens on time — not a code bug,
// and nothing here should grow a defensive re-fetch to paper over it.
func importWindow(events []fpl.Event) (importEvent, nextEvent int, open bool) {
	var next, cur *fpl.Event
	for i := range events {
		if events[i].IsNext {
			next = &events[i]
		}
		if events[i].IsCurrent {
			cur = &events[i]
		}
	}
	if next == nil || cur == nil || next.ID < 2 {
		return 0, 0, false
	}
	return cur.ID, next.ID, true
}
