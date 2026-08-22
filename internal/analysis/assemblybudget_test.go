package analysis

import (
	"strings"
	"testing"
)

// TestAssemblyBudgetIsRealOrItIsRefused pins the four states apart.
//
// The dangerous one is the third: a tracked squad that could not be priced. The
// obvious fallback is £100m and it is wrong in the expensive direction, because
// a squad built with money that does not exist is a recommendation that fails at
// the deadline rather than in the output. It must be an error, and the error has
// to name the two things that cause it — a wrong entry id and an unreachable API
// — because they are indistinguishable from here.
func TestAssemblyBudgetIsRealOrItIsRefused(t *testing.T) {
	e := testEngine(t)

	// No entry: nobody's money is in question, so £100m is a fair answer.
	e.Entry, e.SquadValue, e.Bank, e.HypotheticalBudget = 0, nil, nil, 0
	got, source, err := e.AssemblyBudget()
	if err != nil {
		t.Fatalf("no entry id is a hypothetical, not a failure: %v", err)
	}
	if got != DefaultBudget {
		t.Errorf("budget %d with no entry id, want %d", got, DefaultBudget)
	}
	if source == "" {
		t.Error("no source reported; the reader cannot tell a real budget from an assumed one")
	}

	// The config override, for the mid-season projection that is nobody's team.
	e.HypotheticalBudget = 1035
	if got, _, err = e.AssemblyBudget(); err != nil || got != 1035 {
		t.Errorf("hypothetical budget got (%d, %v), want 1035", got, err)
	}

	// A tracked squad, pre-season: nothing has been bought, so the allowance is
	// the answer and no reconstruction is needed to know it.
	//
	// This is now gated on SeasonHasStarted, not GameweeksPlayed()==0 (see
	// AssemblyBudget's own comment), and testEngine loads real live data — if
	// this test happens to run during the live GW1 gap, real fixtures already
	// have Started=true, so finishGameweeks(e, 0) alone (which only fakes
	// Boot.Events, never Fixture.Started) would no longer read as pre-season.
	// Force every fixture back to not-started too, so this sub-case is
	// genuinely pre-season regardless of what day this test runs.
	e.Entry, e.SquadValue, e.Bank = 12345, nil, nil
	finishGameweeks(e, 0)
	for i := range e.Fixtures {
		e.Fixtures[i].Started = false
		e.Fixtures[i].Finished = false
		e.Fixtures[i].FinishedProvisional = false
	}
	if got, _, err = e.AssemblyBudget(); err != nil || got != DefaultBudget {
		t.Errorf("pre-season with an entry id got (%d, %v), want the %d allowance",
			got, err, DefaultBudget)
	}

	// The same squad in-season, unpriced. This is the case the whole rule
	// exists for, so it is exercised whatever time of year the test runs.
	// Restore Started (the pre-season case above forced it off) — a season with
	// gameweeks finished necessarily has fixtures that have started.
	for i := range e.Fixtures {
		e.Fixtures[i].Started = true
	}
	finishGameweeks(e, 3)
	got, _, err = e.AssemblyBudget()
	if err == nil {
		t.Fatalf("an unpriceable in-season squad returned £%.1fm instead of an error; "+
			"planning with money that may not exist is the failure this prevents",
			float64(got)/10)
	}
	for _, want := range []string{"entry_id", "FPL API"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q, so it does not say what to check: %v",
				want, err)
		}
	}

	// Priced: the wildcard budget, which is not £100m and must not be rounded
	// to it. The config override must not touch it either — a real budget
	// overridden from a file is invented money.
	value, bank := 1023, 7
	e.SquadValue, e.Bank, e.HypotheticalBudget = &value, &bank, 1035
	got, source, err = e.AssemblyBudget()
	if err != nil {
		t.Fatal(err)
	}
	if got != 1030 {
		t.Errorf("budget %d, want 1030 — the squad's selling value plus its bank", got)
	}
	if !strings.Contains(source, "102.3") || !strings.Contains(source, "0.7") {
		t.Errorf("source %q does not show the split it was built from", source)
	}
}

// finishGameweeks marks exactly n of the season's gameweeks complete, so a test
// can put the engine either side of the pre-season boundary whatever the real
// date is. The alternative is a test that silently stops checking the branch it
// was written for, for nine months of the year.
func finishGameweeks(e *Engine, n int) {
	for i := range e.Boot.Events {
		e.Boot.Events[i].Finished = i < n
	}
}
