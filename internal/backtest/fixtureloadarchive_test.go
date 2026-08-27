package backtest

// Does the fixture-load multiplier agree with the archive about how many matches
// a club actually plays?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagFixtureLoadMatchesTheArchive -v -timeout 30m
//
// `PlayerMetrics.FixtureLoad` at a horizon of 1 is a claim with a right answer:
// the number of fixtures the club plays in the imminent gameweek. The archive
// holds every fixture of every season, so the claim can be checked exactly
// rather than argued about — which is what found the defect this file now
// guards against. Anchored on the club's next FIXTURE rather than the next
// GAMEWEEK, the window slid past a blank and the load read >= 1 by construction.
// This probe against the pre-fix anchor, on the six-season grid:
//
//	2020-21  agree 716 | blank scored as playing 44 | other 0
//	2021-22  agree 699 | blank scored as playing 61 | other 0
//	2022-23  agree 718 | blank scored as playing 22 | other 0 | 1 round nobody plays
//	2023-24  agree 737 | blank scored as playing 23 | other 0
//	2024-25  agree 750 | blank scored as playing 10 | other 0
//	2025-26  agree 750 | blank scored as playing 10 | other 0
//	TOTAL   agree 4370 | blank scored as playing 170 | other 0
//
// Every blank missed and no double ever missed, over 4,540 comparisons — that
// is `agree + blank`, not the agree column. The doubles half is therefore what a
// fix must not break, and the "other" column is what says so.
//
// ⚠️ **2022-23 reads 22 here and 42 in the audit that first found this**, and
// both are right about different populations: the 20 extra are its wholly
// postponed GW7, which the `played[gw] == 0` guard below now counts separately
// because a round nobody plays is not twenty blanks.
//
// It reads FixtureLoad off `Metrics`, not off the unexported counter, because
// the quantity that matters is the one the scoring path puts on a footballer.
//
// ⚠️ **This measures how often the defect bit, and not what it was worth.** The
// replay's fixture list is final, so it knows from GW1 which rounds a club will
// blank, where FPL announces them as cup rounds resolve — the same hindsight
// caveat the `+33` doubles figure already carries, and blanks are if anything
// more exposed to it. No points figure derived from this fix escapes it.

import (
	"fmt"
	"testing"

	"armband/internal/config"
	"armband/internal/fpl"
)

func TestDiagFixtureLoadMatchesTheArchive(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	totals := struct{ agree, blank, other, dead int }{}
	for _, pair := range sweepPairNames() {
		cur := loadSeason(t, cfg, pair[1])
		prior := loadSeason(t, cfg, pair[0])

		// The archive's truth: how many matches each club plays in each round.
		truth := map[[2]int]int{}
		played := map[int]int{}
		for _, f := range cur.Fixtures {
			if f.Event == nil {
				continue
			}
			truth[[2]int{f.TeamH, *f.Event}]++
			truth[[2]int{f.TeamA, *f.Event}]++
			played[*f.Event]++
		}

		agree, blank, other, dead := 0, 0, 0, 0
		for gw := 1; gw <= 38; gw++ {
			// A round nobody plays is not twenty blanks, it is a round that does
			// not exist — 2022-23 GW7 was postponed whole and its fixtures were
			// redistributed. `fixtureLoadFor` anchors on the rounds the fixture
			// list still holds, so it prices the next real one, which is what a
			// squad decision taken at that deadline is about. Counted rather than
			// skipped silently: if this column grew, "all zeros" below would be
			// the probe averting its eyes.
			if played[gw] == 0 {
				dead++
				continue
			}
			// The deadline for gameweek `gw` stands after `gw-1` is played, which
			// is where the shipped weekly engine is built. Through EngineAt, so
			// the priors, the recency index and the team-form source are attached
			// the way Simulate attaches them — an unwired engine reads flat season
			// minutes and reports the fallback as the model's own number.
			e, vb := EngineAt(cur, prior, gw-1, weeklyXIConfig(cfg, gw))
			if !e.FixtureLoadInScore() {
				t.Fatal("FixtureLoadInScore is false on a horizon-1 engine, so this " +
					"probe would be reading a field nothing multiplies. Check " +
					"FPL_NO_FIXTURE_LOAD.")
			}

			// One representative element per club is enough: FixtureLoad is a
			// property of the club's calendar and nothing else.
			seen := map[int]bool{}
			for i := range vb.Elements {
				el := &vb.Elements[i]
				if seen[el.Team] {
					continue
				}
				seen[el.Team] = true
				got := e.Metrics(el).FixtureLoad
				want := float64(truth[[2]int{el.Team, gw}])
				switch {
				case got == want:
					agree++
				case want == 0:
					blank++
				default:
					other++
				}
			}
		}
		fmt.Printf("%-8s agree %4d | blank scored as playing %3d | other %d | rounds nobody plays %d\n",
			cur.Name, agree, blank, other, dead)
		totals.agree += agree
		totals.blank += blank
		totals.other += other
		totals.dead += dead
	}
	fmt.Printf("%-8s agree %4d | blank scored as playing %3d | other %d | rounds nobody plays %d\n",
		"TOTAL", totals.agree, totals.blank, totals.other, totals.dead)

	if totals.blank != 0 || totals.other != 0 {
		t.Errorf("%d club-gameweeks disagree with the archive (%d of them blanks scored "+
			"as playing). FixtureLoad at horizon 1 is a count of matches, so any "+
			"disagreement is the multiplier mis-pricing a real week",
			totals.blank+totals.other, totals.blank)
	}
	if totals.agree == 0 {
		t.Fatal("no club-gameweek was compared at all, so the zeros above are the " +
			"probe not running rather than the model agreeing")
	}
}

// TestFixtureLoadMatchesTheArchiveOnOneSeason is the cheap always-on half of the
// probe above: one season, six deadlines, the same comparison.
//
// The DIAG version is 190 point-in-time rebuilds and belongs in a sweep. This
// one is seconds, and exists because a regression on the scoring path should not
// be waiting on somebody choosing to run a diagnostic. It skips when the archive
// is not present, in the same way the API-backed tests skip when the API is.
func TestFixtureLoadMatchesTheArchiveOnOneSeason(t *testing.T) {
	cfg := loadConfig(t)
	pairs := sweepPairNames()
	pair := pairs[len(pairs)-1]
	cur, prior := loadSeason(t, cfg, pair[1]), loadSeason(t, cfg, pair[0])

	truth := map[[2]int]int{}
	clubs := map[int]bool{}
	blanks, doubles := 0, 0
	for _, f := range cur.Fixtures {
		if f.Event == nil {
			continue
		}
		truth[[2]int{f.TeamH, *f.Event}]++
		truth[[2]int{f.TeamA, *f.Event}]++
		clubs[f.TeamH], clubs[f.TeamA] = true, true
	}

	// The weeks are chosen from the archive rather than written down. Which
	// gameweeks blank and which double is a fact about one season's cup draws,
	// so a hardcoded list silently stops exercising one of the two directions
	// the moment the grid moves — which is how a test comes to pin only the half
	// that was never broken.
	for _, gw := range sampledFixtureWeeks(truth, clubs) {
		e, vb := EngineAt(cur, prior, gw-1, weeklyXIConfig(cfg, gw))

		seen := map[int]bool{}
		for i := range vb.Elements {
			el := &vb.Elements[i]
			if seen[el.Team] {
				continue
			}
			seen[el.Team] = true
			want := float64(truth[[2]int{el.Team, gw}])
			switch {
			case want == 0:
				blanks++
			case want > 1:
				doubles++
			}
			if got := e.Metrics(el).FixtureLoad; got != want {
				t.Errorf("GW%d %s: FixtureLoad %.3f against %d fixtures in the archive",
					gw, clubName(vb, el.Team), got, int(want))
			}
		}
	}
	// Both directions have to be exercised or the test pins only the half that
	// was never broken. A season that stops carrying a blank in these weeks
	// should fail here rather than pass quietly on doubles alone.
	if blanks == 0 || doubles == 0 {
		t.Errorf("the sampled gameweeks hold %d blanks and %d doubles; this test "+
			"pins nothing it does not exercise", blanks, doubles)
	}
}

// weeklyXIConfig is the replay's own config with the horizon shortened to the
// single imminent gameweek — the view `Simulate` builds under `WeeklyXI`, and
// the only one on which `FixtureLoadInScore()` is true.
//
// It goes through `sweepConfig` rather than a `SimConfig` literal so that every
// other field is whatever the replay uses, and it is fed to `EngineAt` so the
// priors, the recency index and the team-form source are attached. An engine
// built by hand carries the *fallback* value of `ExpectedMinutes` in a field the
// model reads, which `TestEveryScoringEngineGetsRecency` exists to refuse.
func weeklyXIConfig(cfg config.Config, gw int) SimConfig {
	sc := sweepConfig(cfg, gw, true)
	sc.Weights.Horizon = 1
	return sc
}

// sampledFixtureWeeks picks an ordinary week, the last week of the season, and
// up to two each of the weeks that carry a blank and a double.
func sampledFixtureWeeks(truth map[[2]int]int, clubs map[int]bool) []int {
	pick := map[int]bool{2: true, 38: true}
	blank, double := 0, 0
	for gw := 1; gw <= 38; gw++ {
		b, d := false, false
		for club := range clubs {
			switch n := truth[[2]int{club, gw}]; {
			case n == 0:
				b = true
			case n > 1:
				d = true
			}
		}
		if b && blank < 2 {
			pick[gw], blank = true, blank+1
		}
		if d && double < 2 {
			pick[gw], double = true, double+1
		}
	}
	var out []int
	for gw := 1; gw <= 38; gw++ {
		if pick[gw] {
			out = append(out, gw)
		}
	}
	return out
}

func clubName(b *fpl.Bootstrap, id int) string {
	if t := b.TeamByID(id); t != nil {
		return t.ShortName
	}
	return fmt.Sprintf("team %d", id)
}
