package analysis

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"armband/internal/fpl"
)

func roleEngine(t *testing.T, w Weights, rr RoleRisk) *Engine {
	t.Helper()
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	ctx := context.Background()
	boot, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	fx, err := c.Fixtures(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	return NewEngineFull(boot, fx, w, DefaultCongestion(), rr)
}

func TestNewSigningsArePenalisedAndExemptible(t *testing.T) {
	off := DefaultRoleRisk()
	off.NewSigningPenalty = 1 // disabled
	base := roleEngine(t, DefaultWeights(), off)

	var signing string
	for _, m := range base.AllMetrics() {
		if m.NewSigning && m.ExpectedMinutes > 60 {
			signing = m.Name
			break
		}
	}
	if signing == "" {
		t.Skip("no new signings with real minutes in this dataset")
	}

	find := func(e *Engine, name string) PlayerMetrics {
		for _, m := range e.AllMetrics() {
			if m.Name == name {
				return m
			}
		}
		t.Fatalf("player %s not found", name)
		return PlayerMetrics{}
	}

	on := DefaultRoleRisk()
	penalised := find(roleEngine(t, DefaultWeights(), on), signing)
	unpenalised := find(base, signing)

	if penalised.Score >= unpenalised.Score {
		t.Errorf("new signing %s not penalised: %.3f vs %.3f",
			signing, penalised.Score, unpenalised.Score)
	}
	if len(penalised.RoleNotes) == 0 {
		t.Errorf("new signing %s penalised with no stated reason", signing)
	}

	// A confirmed starter must be exempt.
	exempt := on
	exempt.ConfirmedStarters = []string{signing}
	confirmed := find(roleEngine(t, DefaultWeights(), exempt), signing)
	if confirmed.Score <= penalised.Score {
		t.Errorf("confirmed starter exemption had no effect for %s: %.3f vs %.3f",
			signing, confirmed.Score, penalised.Score)
	}
	t.Logf("%s — no penalty %.3f, penalised %.3f, confirmed starter %.3f",
		signing, unpenalised.Score, penalised.Score, confirmed.Score)
}

// The managerial-change penalty ships disabled — it measured as a variance
// effect rather than a discount, see DefaultRoleRisk — so this exercises the
// mechanism at an explicit value rather than at the default. What it locks is
// that setting the penalty reaches every player at the club, which is the part
// that would break silently.
func TestNewCoachPenaltyAppliesToWholeClub(t *testing.T) {
	base := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	club := base.Boot.Teams[0].ShortName

	rr := DefaultRoleRisk()
	rr.NewSigningPenalty = 1 // isolate the coach effect
	rr.NewCoachPenalty = 0.93
	rr.NewCoachClubs = []string{club}
	withCoach := roleEngine(t, DefaultWeights(), rr)

	rrOff := rr
	rrOff.NewCoachClubs = nil
	without := roleEngine(t, DefaultWeights(), rrOff)

	var checked int
	byName := map[string]PlayerMetrics{}
	for _, m := range without.AllMetrics() {
		byName[m.Name] = m
	}
	for _, m := range withCoach.AllMetrics() {
		if m.Team != club || m.Score == 0 {
			continue
		}
		if m.Score >= byName[m.Name].Score {
			t.Errorf("%s at %s not penalised for the managerial change", m.Name, club)
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no scoring players at the chosen club")
	}
	t.Logf("managerial-change penalty applied to %d players at %s", checked, club)
}

// Midfielders should be penalised less harshly for the same expected minutes.
func TestMidfieldMinutesWeightIsRelaxed(t *testing.T) {
	w := DefaultWeights()
	e := roleEngine(t, w, DefaultRoleRisk())

	midExp := e.minutesExponent(3) // MID
	defExp := e.minutesExponent(2) // DEF

	// Midfielders carry three quarters of the severity. This nearly went to
	// neutral as an unmeasured assertion, on a sweep spanning 13 points — taken
	// against the old reliability mix, which the same session then changed.
	// Re-run against minutes-only reliability it spans 226, and 0.9 and above
	// fall off a cliff. The two are coupled: the old mix credited a substituted
	// starter for having started, and midfielders are who that was propping up.
	if !(midExp < defExp) {
		t.Errorf("midfield exponent %.4f should be below defence %.4f", midExp, defExp)
	}
	want := 1 + (w.MinutesWeight-1)*0.75
	if diff := midExp - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("midfield exponent %.4f, want %.4f", midExp, want)
	}
	t.Logf("exponents — MID %.4f, DEF %.4f, global %.2f", midExp, defExp, w.MinutesWeight)
}

// The rest list and the new-coach list are hand-maintained strings matched
// against live FPL data, which is a silent failure mode: a misspelt name, a
// dropped accent or a club short name that does not exist simply matches
// nothing, and the penalty quietly stops applying to a player the model was
// meant to discount.
//
// These two tests make that loud. They do not assert *who* is on the lists —
// that changes every season — only that every entry still resolves.
func TestDefaultRestPlayersAllResolveToDistinctPlayers(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	seen := map[int]string{}
	for _, name := range DefaultRestPlayers() {
		matches := e.Boot.FindPlayers(name)
		if len(matches) == 0 {
			t.Errorf("%q matches no FPL player — check spelling and accents", name)
			continue
		}
		el := matches[0]
		full := el.FirstName + " " + el.SecondName
		if !strings.EqualFold(full, name) && !strings.EqualFold(el.WebName, name) {
			t.Errorf("%q resolved only fuzzily, to %q — ambiguous entries pick the wrong player", name, full)
		}
		if prev, dup := seen[el.ID]; dup {
			t.Errorf("%q and %q both resolve to element %d (%s)", prev, name, el.ID, el.WebName)
		}
		seen[el.ID] = name
	}
}

func TestDefaultNewCoachClubsAreRealClubs(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	valid := map[string]bool{}
	for _, tm := range e.Boot.Teams {
		valid[tm.ShortName] = true
	}
	for _, short := range DefaultNewCoachClubs() {
		if !valid[short] {
			t.Errorf("new_coach_clubs contains %q, which is not a club in this season's Premier League", short)
		}
	}
}

// The post-tournament factor feeds minutes that are already averaged over the
// fixture horizon, so it must be prorated by the share of that horizon falling
// inside the rest window. Applying it flat asserts the player is eased in during
// every gameweek of the horizon, including ones long after he is fresh — the
// same mistake the European penalty made before it was date-gated.
func TestRestFactorIsProratedAcrossTheHorizon(t *testing.T) {
	w := DefaultWeights()
	w.Horizon = 5
	w.RestGameweeks = 2
	w.RestMinutesFactor = 0.83
	e := roleEngine(t, w, DefaultRoleRisk())

	next := e.Boot.NextEvent()
	if next == nil || next.ID > w.RestGameweeks {
		t.Skipf("rest window has passed (next gameweek %v)", next)
	}
	affected := w.RestGameweeks - next.ID + 1
	want := (float64(affected)*w.RestMinutesFactor + float64(w.Horizon-affected)) / float64(w.Horizon)

	var checked int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		reason, got := e.restFactor(el)
		if reason == "" {
			continue
		}
		checked++
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("%s: rest factor %.4f, want %.4f (%d of %d horizon gameweeks affected)",
				el.WebName, got, want, affected, w.Horizon)
		}
		if got <= w.RestMinutesFactor {
			t.Fatalf("%s: prorated factor %.4f is no gentler than the raw factor %.2f",
				el.WebName, got, w.RestMinutesFactor)
		}
	}
	if checked == 0 {
		t.Fatal("no player matched the rest list — proration was never exercised")
	}
}

// The rest factor was originally a Score multiplier and was moved onto minutes,
// because minutes is the channel it was measured in. That move is invisible in
// a season total — Score falls either way — so it would revert silently.
//
// What must hold is that the term reaches ExpectedMinutes (and therefore the
// rotation_risk band the agent is told to lead with), and that it leaves the
// per-90 rates alone: a player back from a tournament plays less, not worse.
func TestRestFactorMovesMinutesAndNotRates(t *testing.T) {
	off := DefaultWeights()
	off.RestMinutesFactor = 1 // disabled
	base := roleEngine(t, off, DefaultRoleRisk())

	on := DefaultWeights()
	on.RestMinutesFactor = 0.83
	rested := roleEngine(t, on, DefaultRoleRisk())

	if next := rested.Boot.NextEvent(); next == nil || next.ID > on.RestGameweeks {
		t.Skipf("rest window has passed (next gameweek %v)", next)
	}

	// Keyed by element id, not name: the rest list contains two Martínezes and
	// two Jameses, and a name-keyed map compares one against the other.
	was := map[int]PlayerMetrics{}
	for _, m := range base.AllMetrics() {
		was[m.ID] = m
	}

	var checked int
	for _, m := range rested.AllMetrics() {
		if m.RestRisk == "" {
			continue
		}
		b := was[m.ID]
		if b.ExpectedMinutes == 0 {
			continue
		}
		checked++
		if m.ExpectedMinutes >= b.ExpectedMinutes {
			t.Errorf("%s: rest flag did not reduce expected minutes (%.1f vs %.1f) — "+
				"the term is back on Score instead of minutes",
				m.Name, m.ExpectedMinutes, b.ExpectedMinutes)
		}
		if m.BaseXP90 != b.BaseXP90 {
			t.Errorf("%s: rest flag changed per-90 output (%.4f vs %.4f) — "+
				"rest is an availability effect, rates must be untouched",
				m.Name, m.BaseXP90, b.BaseXP90)
		}
	}
	if checked == 0 {
		t.Fatal("no player carried a rest flag — the term was never exercised")
	}
	t.Logf("rest factor verified on %d players", checked)
}
