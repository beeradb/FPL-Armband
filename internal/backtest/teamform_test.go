package backtest

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// TestEveryScoringEngineGetsTeamForm is TestEveryScoringEngineGetsRecency for the
// club-form blend, and it guards the same bug for the same reason.
//
// Simulate builds engines to decide transfers, to pick the eleven, and inside Hold.
// A patch wired recency into two of them and missed the transfer decision, so the
// reported gain came entirely from better captaincy while transfers still ran on
// flat season minutes — plausible scores, moving totals, nothing failing. A
// club-level correction is *more* exposed to that failure than recency was, because
// it is a between-club shift: wired into the eleven but not into transfers, it would
// field a squad chosen on one view of the league and rebuild it on another.
func TestEveryScoringEngineGetsTeamForm(t *testing.T) {
	src, err := os.ReadFile("simulate.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	engines := strings.Count(body, "analysis.NewEngineFull(")
	// Any assignment counts. The free-hit engine reuses the neighbouring one's
	// index rather than rebuilding an identical thing, exactly as it does for
	// Recent, and a prefix match on the builder would call that unwired.
	wired := strings.Count(body, ".TeamForm = ")
	if !strings.Contains(body, ".TeamForm = newTeamFormIndex") {
		t.Error("no engine builds a club-form index from newTeamFormIndex; if the " +
			"builder was renamed, update this test — if it was dropped, FPL_TEAM_FORM " +
			"is silently inert and a sweep of it would measure nothing while looking " +
			"like it measured a null")
	}
	if wired != engines {
		t.Errorf("%d engines built, %d given a club-form index. An engine scoring "+
			"without one uses a different view of the league from its neighbours.",
			engines, wired)
	}
}

// TestTeamFormIsPointInTime is the leak check.
//
// The window is trailing, so a source built at gameweek G must not change when the
// rest of the season exists. This is the one place a club-level feature could read
// the future: `playedFixtures` strips scorelines from unplayed matches, but nothing
// stops a careless window from being centred rather than trailing, and the archive
// holds every gameweek for the whole season.
func TestTeamFormIsPointInTime(t *testing.T) {
	analysis.SetTeamFormWeight(0.5)
	defer analysis.SetTeamFormWeight(0)

	cfg := loadConfig(t)
	full, err := Load(context.Background(), cfg.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	// The same season with everything after the cutoff removed — **player rows and
	// fixtures both**. The fixture list is the second input and it is easy to miss:
	// the archive holds every fixture for the whole season, and the club-form index
	// reads their difficulty ratings to put each window on neutral terms. Omitting
	// them from this copy is how the first version of this test passed a version of
	// the code that had no adjustment and then failed the one that did, for a
	// reason that was the test's fault rather than the code's.
	//
	// Difficulty ratings are published before a ball is kicked, so reading a future
	// fixture's rating would not be hindsight in the way reading its scoreline is.
	// It is still checked, because "not hindsight" is a judgement and "identical
	// without the future" is a fact.
	const cutoff = 19
	truncated := &Season{Name: full.Name, Teams: full.Teams, Players: map[int]*Player{}}
	for _, f := range full.Fixtures {
		if f.Event != nil && *f.Event <= cutoff {
			truncated.Fixtures = append(truncated.Fixtures, f)
		}
	}
	for id, p := range full.Players {
		cp := *p
		cp.GWs = map[int]GW{}
		for gw, g := range p.GWs {
			if gw <= cutoff {
				cp.GWs[gw] = g
			}
		}
		truncated.Players[id] = &cp
	}

	a := newTeamFormIndex(full, cutoff)
	b := newTeamFormIndex(truncated, cutoff)
	if a == nil || b == nil {
		t.Fatal("index is nil with the weight set")
	}
	for _, tm := range full.Teams {
		ra, sa, oka := a.TeamForm(tm.ID)
		rb, sb, okb := b.TeamForm(tm.ID)
		if oka != okb || ra != rb || sa != sb {
			t.Errorf("%s: with the full season %v/%.6f/%.6f, truncated at GW%d "+
				"%v/%.6f/%.6f. The trailing window is reading past the cutoff.",
				tm.ShortName, oka, ra, sa, cutoff, okb, rb, sb)
		}
	}
}

// TestTeamFormIsInertWhenOff is the invariant that lets this ship switched off.
//
// The feature adds a field to every engine and a branch to the hottest path in
// scoring. With the weight at zero none of it may move a number — not by a rounding
// bit, because AGENTS.md records a 2% nudge to one exponent moving four-season
// points by 67, so a response surface this rough turns "almost identical" into a
// different answer somewhere.
func TestTeamFormIsInertWhenOff(t *testing.T) {
	analysis.SetTeamFormWeight(0)
	e := teamFormEngine(t, 19)
	if e == nil {
		t.Skip("archive unavailable")
	}
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if m := e.Metrics(el); m.TeamFormFactor != 0 {
			t.Fatalf("%s reports a club-form factor of %v with the weight off; the "+
				"field must stay absent so the JSON the agent reads does not grow a "+
				"term that is doing nothing", m.Name, m.TeamFormFactor)
		}
	}
}

// TestTeamFormClampIsNotDoingTheWork checks that the guard rails are guard rails.
//
// The factor is clamped to [0.60, 1.60] to stop a ratio of two thin samples moving
// every player at a club at once. A clamp that binds often is not a safety net, it
// **is** the effect — the measured thing would then be "cap the correction" rather
// than "apply the correction", and the sweep would be reporting the cap's value.
// The observed club ratios run about 0.48 to 1.84 before the power is applied, and
// at w = 0.5 the square root pulls those well inside the bounds, so binding should
// be rare.
func TestTeamFormClampIsNotDoingTheWork(t *testing.T) {
	analysis.SetTeamFormWeight(0.5)
	defer analysis.SetTeamFormWeight(0)

	e := teamFormEngine(t, 19)
	if e == nil {
		t.Skip("archive unavailable")
	}
	var n, atBound int
	seen := map[int]bool{}
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if seen[el.Team] {
			continue
		}
		seen[el.Team] = true
		f := e.Metrics(el).TeamFormFactor
		if f == 0 {
			continue // no correction for this club
		}
		n++
		if math.Abs(f-0.60) < 1e-9 || math.Abs(f-1.60) < 1e-9 {
			atBound++
		}
	}
	if n == 0 {
		t.Skip("no club carried a correction")
	}
	if share := float64(atBound) / float64(n); share > 0.10 {
		t.Errorf("%d of %d clubs sit exactly on a clamp bound (%.0f%%). The clamp is "+
			"meant to catch arithmetic accidents, and at this rate it is the effect "+
			"being measured rather than a rail around it.", atBound, n, 100*share)
	}
}

// teamFormEngine builds a point-in-time engine at a cutoff with the club-form
// source wired, or nil when the archive is unavailable.
func teamFormEngine(t *testing.T, cutoff int) *analysis.Engine {
	t.Helper()
	cfg := loadConfig(t)
	ctx := context.Background()
	prior, err := Load(ctx, cfg.CacheDir, "2023-24")
	if err != nil {
		return nil
	}
	cur, err := Load(ctx, cfg.CacheDir, "2024-25")
	if err != nil {
		return nil
	}
	e, _ := EngineAt(cur, prior, cutoff, SimConfig{Weights: cfg.Weights})
	e.Priors = newPriorIndex(prior)
	e.TeamForm = newTeamFormIndex(cur, cutoff)
	return e
}
