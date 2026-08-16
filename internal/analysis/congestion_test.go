package analysis

import (
	"context"
	"testing"
	"time"

	"armband/internal/fpl"
)

func congestionEngine(t *testing.T, cg Congestion) *Engine {
	t.Helper()
	c := fpl.New(t.TempDir(), 24*time.Hour)
	ctx := context.Background()
	boot, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	fx, err := c.Fixtures(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	return NewEngineWithCongestion(boot, fx, DefaultWeights(), cg)
}

// International breaks must be derived from the real calendar, not configured.
func TestPostBreakGameweeksDerivedFromCalendar(t *testing.T) {
	e := congestionEngine(t, DefaultCongestion())
	breaks := e.PostBreakGameweeks()
	if len(breaks) == 0 {
		t.Fatal("no international breaks detected; a 38-gameweek season always has some")
	}
	for gw, days := range breaks {
		if gw <= 1 {
			t.Errorf("gameweek %d cannot follow a break", gw)
		}
		if days < IntlBreakThresholdDays {
			t.Errorf("GW%d recorded a %.1f-day break, below the %d-day threshold",
				gw, days, IntlBreakThresholdDays)
		}
	}
}

// With nothing configured, congestion must be a no-op rather than a silent penalty.
func TestCongestionIsNeutralWhenUnconfigured(t *testing.T) {
	e := congestionEngine(t, DefaultCongestion())
	for _, m := range e.AllMetrics() {
		if m.Congestion < 1 && len(m.CongestionNotes) == 0 {
			t.Fatalf("%s penalised to %.3f with no stated reason", m.Name, m.Congestion)
		}
	}
}

// MarkCompetitionsVerified is the free CLI path to recording a check against
// the web — it must stamp LastVerified without disturbing any club's windows,
// or a manual "nothing to correct" check would silently erase real data.
func TestMarkCompetitionsVerifiedLeavesWindowsAlone(t *testing.T) {
	cg := DefaultCongestion()
	cg.European = map[string][]CompetitionWindow{
		"ARS": {{Competition: "UCL", Start: "2026-09-08"}},
	}
	e := congestionEngine(t, cg)
	if _, ok := e.StatusAge(); ok {
		t.Fatal("StatusAge reported verified before anything was stamped")
	}

	updated := e.MarkCompetitionsVerified("2020-01-01")
	if len(updated.European["ARS"]) != 1 || updated.European["ARS"][0].Competition != "UCL" {
		t.Fatalf("windows changed: %+v", updated.European["ARS"])
	}
	days, ok := e.StatusAge()
	if !ok {
		t.Fatal("StatusAge still unverified after MarkCompetitionsVerified")
	}
	if days < 1000 { // 2020-01-01 is years in the past
		t.Errorf("StatusAge %d days, want a large figure for a 2020 stamp", days)
	}
}

// Each competition must read its own penalty, in the order configured.
//
// This used to assert the ordering of the *shipped* values — UCL harsher than
// UEL harsher than UECL — which was the belief the constants encoded. It is not
// true: measured against players at clubs not in Europe, all three come out at
// 1.0, and they now ship that way. What is still worth pinning is that the
// lookup works, so a future measurement that does find an effect is applied to
// the right competition. The penalties are therefore set here.
func TestEuropeanCompetitionOrdersPenalties(t *testing.T) {
	base := congestionEngine(t, DefaultCongestion())
	var club string
	for i := range base.Boot.Teams {
		if len(base.TeamFixtures(base.Boot.Teams[i].ID, 5)) == 5 {
			club = base.Boot.Teams[i].ShortName
			break
		}
	}
	if club == "" {
		t.Skip("no club with a full horizon")
	}

	factor := func(comp string) float64 {
		cg := DefaultCongestion()
		cg.UCLPenalty, cg.UELPenalty, cg.UECLPenalty = 0.90, 0.94, 0.98
		cg.European = map[string][]CompetitionWindow{}
		cg.DomesticCups = map[string][]CompetitionWindow{}
		if comp != "" {
			cg.European[club] = []CompetitionWindow{{Competition: comp, Start: "2000-01-01"}}
		}
		e := congestionEngine(t, cg)
		for _, m := range e.AllMetrics() {
			if m.Team == club {
				return m.Congestion
			}
		}
		t.Fatalf("no players found for %s", club)
		return 0
	}

	none, uecl, uel, ucl := factor(""), factor("UECL"), factor("UEL"), factor("UCL")
	if !(ucl < uel && uel < uecl && uecl < none) {
		t.Errorf("penalties out of order: UCL %.3f UEL %.3f UECL %.3f none %.3f",
			ucl, uel, uecl, none)
	}
	t.Logf("%s congestion — none %.3f, UECL %.3f, UEL %.3f, UCL %.3f", club, none, uecl, uel, ucl)
}

// European football starts weeks after the Premier League does. Gameweeks before
// that date must carry no European penalty at all.
func TestEuropeanPenaltyIsDateGated(t *testing.T) {
	probe := congestionEngine(t, DefaultCongestion())
	var club string
	for i := range probe.Boot.Teams {
		if len(probe.TeamFixtures(probe.Boot.Teams[i].ID, 5)) == 5 {
			club = probe.Boot.Teams[i].ShortName
			break
		}
	}
	if club == "" {
		t.Skip("no club with a full horizon")
	}

	// Start date far in the future: no gameweek in the horizon should be hit.
	// The penalty is set here rather than taken from the defaults, because this
	// tests date gating and the shipped European penalties measure as 1.0.
	late := DefaultCongestion()
	late.UCLPenalty = 0.9
	late.DomesticCups = map[string][]CompetitionWindow{}
	late.European = map[string][]CompetitionWindow{
		club: {{Competition: "UCL", Start: "2099-01-01"}},
	}
	eLate := congestionEngine(t, late)

	// Start date in the past: every gameweek should be hit.
	early := late
	early.European = map[string][]CompetitionWindow{
		club: {{Competition: "UCL", Start: "2000-01-01"}},
	}
	eEarly := congestionEngine(t, early)

	var gatedFactor, ungatedFactor float64
	for _, m := range eLate.AllMetrics() {
		if m.Team == club {
			gatedFactor = m.Congestion
			break
		}
	}
	for _, m := range eEarly.AllMetrics() {
		if m.Team == club {
			ungatedFactor = m.Congestion
			break
		}
	}

	if gatedFactor != 1.0 {
		t.Errorf("with a future start date the factor should be 1.0, got %.3f", gatedFactor)
	}
	if ungatedFactor >= 1.0 {
		t.Errorf("with a past start date the factor should be penalised, got %.3f", ungatedFactor)
	}

	// The real configured campaign should land strictly between the two,
	// because only some horizon gameweeks fall after the European start.
	real := late
	real.European = map[string][]CompetitionWindow{
		club: DefaultEuropeanCampaigns()["ARS"],
	}
	eReal := congestionEngine(t, real)
	var realFactor float64
	for _, m := range eReal.AllMetrics() {
		if m.Team == club {
			realFactor = m.Congestion
			break
		}
	}
	if !(realFactor > ungatedFactor && realFactor <= gatedFactor) {
		t.Errorf("partially-gated factor %.3f should sit between ungated %.3f and gated %.3f",
			realFactor, ungatedFactor, gatedFactor)
	}
	t.Logf("%s: ungated %.3f, real start %.3f, fully gated %.3f",
		club, ungatedFactor, realFactor, gatedFactor)
}

// A club knocked out must stop carrying the penalty from its end date, and a
// club with no window must never carry one.
func TestCompetitionEndDateStopsPenalty(t *testing.T) {
	probe := congestionEngine(t, DefaultCongestion())
	var club string
	for i := range probe.Boot.Teams {
		if len(probe.TeamFixtures(probe.Boot.Teams[i].ID, 5)) == 5 {
			club = probe.Boot.Teams[i].ShortName
			break
		}
	}
	if club == "" {
		t.Skip("no club with a full horizon")
	}

	factorFor := func(w []CompetitionWindow) float64 {
		cg := DefaultCongestion()
		// See TestMatchDatesNarrowThePenalty: this is about whether an End date
		// stops a window applying, not about the size of the penalty, and the
		// shipped European penalties measure as 1.0.
		cg.UCLPenalty = 0.9
		cg.European = map[string][]CompetitionWindow{}
		cg.DomesticCups = map[string][]CompetitionWindow{}
		if w != nil {
			cg.European[club] = w
		}
		e := congestionEngine(t, cg)
		for _, m := range e.AllMetrics() {
			if m.Team == club {
				return m.Congestion
			}
		}
		t.Fatalf("no players for %s", club)
		return 0
	}

	none := factorFor(nil)
	stillIn := factorFor([]CompetitionWindow{{Competition: "UCL", Start: "2000-01-01"}})
	knockedOut := factorFor([]CompetitionWindow{
		{Competition: "UCL", Start: "2000-01-01", End: "2000-06-01"},
	})

	if none != 1.0 {
		t.Errorf("a club in no competition should have factor 1.0, got %.3f", none)
	}
	if stillIn >= 1.0 {
		t.Errorf("an active campaign should penalise, got %.3f", stillIn)
	}
	if knockedOut != 1.0 {
		t.Errorf("a campaign ended in the past should not penalise, got %.3f", knockedOut)
	}
	t.Logf("%s — none %.3f, still in %.3f, knocked out %.3f", club, none, stillIn, knockedOut)
}

// MatchDates should confine the penalty to gameweeks near an actual fixture,
// rather than every week in the window.
func TestMatchDatesNarrowThePenalty(t *testing.T) {
	probe := congestionEngine(t, DefaultCongestion())
	var club string
	var teamID int
	for i := range probe.Boot.Teams {
		if len(probe.TeamFixtures(probe.Boot.Teams[i].ID, 5)) == 5 {
			club = probe.Boot.Teams[i].ShortName
			teamID = probe.Boot.Teams[i].ID
			break
		}
	}
	if club == "" {
		t.Skip("no club with a full horizon")
	}
	// Anchor one match date to a real gameweek deadline in the horizon.
	fx := probe.TeamFixtures(teamID, 5)
	deadline, ok := probe.GameweekDeadline(fx[0].Event)
	if !ok {
		t.Skip("no deadline for the first horizon gameweek")
	}

	factorFor := func(w CompetitionWindow) float64 {
		cg := DefaultCongestion()
		// A penalty of its own, because this is about the *windowing* — whether
		// match dates narrow which gameweeks are affected — and the shipped
		// European penalties now measure as 1.0, which would make every
		// comparison here trivially equal without testing anything.
		cg.UCLPenalty = 0.9
		cg.European = map[string][]CompetitionWindow{club: {w}}
		cg.DomesticCups = map[string][]CompetitionWindow{}
		e := congestionEngine(t, cg)
		for _, m := range e.AllMetrics() {
			if m.Team == club {
				return m.Congestion
			}
		}
		t.Fatalf("no players for %s", club)
		return 0
	}

	everyWeek := factorFor(CompetitionWindow{Competition: "UCL", Start: "2000-01-01"})
	oneWeek := factorFor(CompetitionWindow{
		Competition: "UCL", Start: "2000-01-01",
		MatchDates: []string{deadline.Format("2006-01-02")},
	})

	if !(oneWeek > everyWeek) {
		t.Errorf("match dates should narrow the penalty: one fixture %.3f vs every week %.3f",
			oneWeek, everyWeek)
	}
	if oneWeek >= 1.0 {
		t.Errorf("a fixture inside the horizon should still penalise, got %.3f", oneWeek)
	}
	t.Logf("%s — every week %.3f, single match date %.3f", club, everyWeek, oneWeek)
}
