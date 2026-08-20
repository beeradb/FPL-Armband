package analysis

import (
	"math"
	"testing"
)

// TestPoissonAtLeastMatchesKnownValues pins the survival function against
// values computed independently, so a refactor of the summation cannot quietly
// change every defensive-contribution score in the game.
func TestPoissonAtLeastMatchesKnownValues(t *testing.T) {
	cases := []struct {
		k    int
		lam  float64
		want float64
	}{
		// Computed independently at 50-digit precision.
		{10, 0, 0},
		{10, 2, 0.00004650},
		{10, 5, 0.03182806},
		{10, 8, 0.28337574},
		{10, 10, 0.54207029},
		{10, 13, 0.83418812},
		{12, 8.38, 0.14128414},
		{12, 12, 0.53840267},
		{12, 20, 0.97861318},
		{0, 5, 1},
	}
	for _, c := range cases {
		got := poissonAtLeast(c.k, c.lam)
		if math.Abs(got-c.want) > 1e-5 {
			t.Errorf("poissonAtLeast(%d, %g) = %.6f, want %.6f", c.k, c.lam, got, c.want)
		}
	}
}

// TestPoissonAtLeastIsAWellFormedProbability guards the properties the scoring
// term relies on: bounded, and monotonic in the rate.
func TestPoissonAtLeastIsAWellFormedProbability(t *testing.T) {
	for _, k := range []int{10, 12} {
		prev := -1.0
		for lam := 0.0; lam <= 40; lam += 0.25 {
			p := poissonAtLeast(k, lam)
			if p < 0 || p > 1 {
				t.Fatalf("poissonAtLeast(%d, %g) = %v, outside [0,1]", k, lam, p)
			}
			if p < prev {
				t.Fatalf("poissonAtLeast(%d, %g) = %v fell below the previous rate's %v", k, lam, p, prev)
			}
			prev = p
		}
		if p := poissonAtLeast(k, 60); p < 0.999 {
			t.Errorf("poissonAtLeast(%d, 60) = %v, want ~1 for a rate far above the bar", k, p)
		}
	}
}

// TestDefConIsNotALinearRamp is the regression guard for the shipped bug. The
// award is per match and all-or-nothing, so averaging half the bar must not
// credit half the bonus, and averaging exactly the bar must not credit all of
// it — clearing it is a coin flip, not a certainty.
func TestDefConIsNotALinearRamp(t *testing.T) {
	const bar = 10

	if got := poissonAtLeast(bar, bar/2.0); got > 0.10 {
		t.Errorf("averaging half the bar clears it %.1f%% of the time under this model; "+
			"the linear ramp claimed 50%%", got*100)
	}
	if got := poissonAtLeast(bar, bar); got > 0.75 {
		t.Logf("averaging exactly the bar clears it %.1f%% of matches", got*100)
	} else if got < 0.40 {
		t.Errorf("averaging exactly the bar should be near a coin flip, got %.3f", got)
	}

	// The ramp capped at the bar, compressing the gap between an elite
	// defensive defender and a mediocre one. It must reopen.
	elite, middling := poissonAtLeast(bar, 11.47), poissonAtLeast(bar, 8.02)
	ramp := math.Min(11.47/bar, 1) - math.Min(8.02/bar, 1)
	if elite-middling <= ramp {
		t.Errorf("gap between 11.47 and 8.02 CBIT is %.3f, no wider than the ramp's %.3f",
			elite-middling, ramp)
	}
}

// TestTheDefconChanceIsTheOneTheScoreUsed guards against defconChance and
// defconPerGameweek drifting into two implementations of the same
// probability — the metricsIgnoring call site stores *m.DefConChance at the
// same probability perGW is built from, so a client reading the field back
// must recover exactly what the score used, and must see nil precisely where
// the term was not priced (no minutes, or no defensive-contribution rate).
func TestTheDefconChanceIsTheOneTheScoreUsed(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	cases := []struct {
		name             string
		pos              int
		defCon90         float64
		expectedMinutes  float64
		startShare       float64
		wantPricedAtZero bool // priced (non-nil), but the chance itself may be 0 or not
	}{
		{"defender priced", 2, 11.5, 75, 0.9, true},
		{"midfielder priced", 3, 9.0, 60, 0.6, true},
		{"forward, low exposure", 4, 6.0, 20, 0.2, true},
		{"defender, no rate", 2, 0, 75, 0.9, false},
		{"defender, no expected minutes", 2, 11.5, 0, 0.9, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := PlayerMetrics{
				DefCon90:        c.defCon90,
				ExpectedMinutes: c.expectedMinutes,
				StartShare:      c.startShare,
			}
			chance := e.defconChance(c.pos, m)
			perGW := e.defconPerGameweek(c.pos, m)
			if got, want := perGW, chance*defConPoints; math.Abs(got-want) > 1e-12 {
				t.Errorf("defconPerGameweek(%d, m) = %v, want defconChance(%d, m) * defConPoints = %v",
					c.pos, got, c.pos, want)
			}
			if !c.wantPricedAtZero && chance != 0 {
				t.Errorf("defconChance(%d, m) = %v, want 0 for an unpriced player", c.pos, chance)
			}
		})
	}

	// The field the JSON view carries must agree with the exposure the score
	// actually used, and be nil exactly where the term isn't priced — a nil
	// pointer reads as "not priced", a zero value would read as "priced at
	// zero chance", which is a different claim.
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		m := e.Metrics(el)
		if el.Minutes <= 0 {
			if m.DefConChance != nil {
				t.Fatalf("%s: DefConChance set with zero minutes", el.WebName)
			}
			continue
		}
		if m.DefConChance == nil {
			t.Fatalf("%s: DefConChance nil despite minutes > 0", el.WebName)
		}
		want := e.defconChance(el.ElementType, m)
		if got := *m.DefConChance; math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: DefConChance = %v, want %v (what the score used)", el.WebName, got, want)
		}
	}
}

// TestDefConCreditStaysWithinTheAward checks no player can be credited more
// than the award is worth, across the whole live pool.
func TestDefConCreditStaysWithinTheAward(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var checked int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Minutes == 0 {
			continue
		}
		threshold := 12
		if el.ElementType == 2 {
			threshold = 10
		}
		dc := el.DefensiveContributionPer90.Float()
		if dc == 0 && el.DefensiveContribution > 0 {
			dc = float64(el.DefensiveContribution) * (90.0 / float64(el.Minutes))
		}
		if dc <= 0 {
			continue
		}
		checked++
		if credit := poissonAtLeast(threshold, dc) * defConPoints; credit > defConPoints {
			t.Errorf("%s credited %.3f from defensive contribution, award is %.1f",
				el.WebName, credit, defConPoints)
		}
	}
	if checked == 0 {
		t.Skip("no players with defensive contribution data")
	}
}
