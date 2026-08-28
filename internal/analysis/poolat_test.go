package analysis

import "testing"

// PoolAt exists because AllMetrics answers a different question, and the
// difference is not cosmetic: only at horizon 1 does FixtureLoad reach Score, so
// a horizon ranking cannot tell a double gameweek from an ordinary one. If this
// ever stops holding, PoolAt is a slower spelling of AllMetrics and every weekly
// ranking built on it is silently the horizon average again.
func TestPoolAtScoresTheWeekAndAllMetricsScoresTheHorizon(t *testing.T) {
	e := testEngine(t)
	skipDuringLiveGW1Gap(t, e)
	if e.Weights.Horizon <= 1 {
		t.Skipf("shipped horizon is %d, so there is no difference to detect",
			e.Weights.Horizon)
	}
	next := e.Boot.NextEvent()
	if next == nil {
		t.Skip("no gameweek open")
	}

	week := e.PoolAt(next.ID)
	if len(week) == 0 {
		t.Fatal("PoolAt returned nothing")
	}
	for _, m := range week {
		if !m.loadInScore {
			t.Fatalf("%s: PoolAt must score at horizon 1, where fixture load reaches "+
				"Score; this row was scored without it", m.Name)
		}
	}
	for _, m := range e.AllMetrics() {
		if m.loadInScore {
			t.Fatalf("%s: AllMetrics carried fixture load at horizon %d, so the two "+
				"functions no longer differ and PoolAt's reason for existing is gone",
				m.Name, e.Weights.Horizon)
		}
	}
}

// The pool is the pool: PoolAt ranks every player in the game, not a squad.
// It is built from engineAt rather than from WeekViews precisely so that no XI
// picker can narrow it, and a regression there would show up here as a count of
// fifteen.
func TestPoolAtCoversTheWholePoolRatherThanASquad(t *testing.T) {
	e := testEngine(t)
	skipDuringLiveGW1Gap(t, e)
	next := e.Boot.NextEvent()
	if next == nil {
		t.Skip("no gameweek open")
	}
	if got, want := len(e.PoolAt(next.ID)), len(e.AllMetrics()); got != want {
		t.Errorf("PoolAt returned %d players, AllMetrics %d; they score the same "+
			"pool and must cover it identically", got, want)
	}
}
