package backtest

import (
	"sort"
	"strings"
	"testing"
)

// TestDiagExtendedSeasons validates the three seasons the Understat backfill adds,
// without running a replay.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagExtendedSeasons -v
//
// # Why this is a diagnostic and not an ordinary test
//
// It fetches four archive files per season over the network. That is cached after the
// first run, but paying it on every `go test ./...` for seasons nothing replays by
// default is the wrong trade — and it is the same reason every other archive-reading
// check in this package is DIAG-gated. The parts that are cheap and always-on live in
// xgrepair_test.go: the renumber arithmetic, the sidecar agreement, and idempotence.
//
// # What it checks, and why each one is a thing that fails silently
//
//   - **38 gameweeks with football.** 2019-20's rounds are labelled 1-29 then 39-47, and
//     `loadGameweeks` drops anything outside 1..38 — so before `renumberGW` the season
//     parsed as 29 gameweeks and every figure from it would have been plausible and a
//     quarter short.
//   - **Fixtures on the same numbering as the gameweek rows.** A fixture list on 39-47
//     beside player rows on 30-38 makes `TeamFixtures` find nothing for the restart, so
//     every player at every club reads as blanking for nine gameweeks while still
//     scoring points. Two views of one quantity, only one of them shifted.
//   - **League minutes near 749,000.** The invariant that says no football went missing:
//     20 clubs x 38 matches x 11 players x 90 minutes, plus substitutions, lands there
//     in every season the archive holds.
//   - **xG present in every gameweek after the repair.** The whole point. A repair that
//     silently applied nothing is the failure mode this package has hit twice, and it
//     looks exactly like a season that had no xG to begin with.
//   - **Starts reconstructed.** `starts` is absent as a column before 2022-23, so
//     `StartShare` would read zero for everyone and `blankRate` would report the whole
//     league as blanking 62% of the time.
//   - **The repair is off when asked.** The paired-arm switch has to work on these
//     seasons too, or a sweep measuring the backfill's effect would measure one arm
//     twice.
func TestDiagExtendedSeasons(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	// Every season either grid names, so this reports the new ones beside the four the
	// record is measured on and a reader can see they are the same shape.
	seen := map[string]bool{}
	var seasons []string
	for _, p := range scoringPairNames() {
		for _, n := range p {
			if !seen[n] {
				seen[n] = true
				seasons = append(seasons, n)
			}
		}
	}
	sort.Strings(seasons)

	t.Log("season    gws  minutes  fixtures  events   xG gws   xG total  " +
		"reconstructed starts  offset     metric")
	for _, name := range seasons {
		s := loadSeason(t, cfg, name)

		var minutes, players int
		var xg, xa float64
		gws := map[int]bool{}
		xgGWs := map[int]bool{}
		recon := 0
		for _, p := range s.Players {
			if len(p.GWs) > 0 {
				players++
			}
			for gw, g := range p.GWs {
				if g.Minutes > 0 || g.Fixtures > 0 {
					gws[gw] = true
				}
				minutes += g.Minutes
				xg += g.XG
				xa += g.XA
				if g.XG > 0 {
					xgGWs[gw] = true
				}
				if g.StartsReconstructed {
					recon++
				}
			}
		}
		events := map[int]bool{}
		for _, f := range s.Fixtures {
			if f.Event != nil {
				events[*f.Event] = true
			}
		}

		rep := "none"
		if s.XGRepair.Rows > 0 {
			rep = s.XGRepair.Meta.OffsetSource
		}
		metric := "both"
		if !TransferPathComparable(name) {
			metric = "HOLD only"
		}
		if s.PriorOnly() {
			metric = "prior only (no " + strings.Join(s.Absent, ", ") + ")"
		}
		t.Logf("%-9s %3d %8d %9d %7d %8d %10.1f %21d  %-10s %s",
			name, len(gws), minutes, len(s.Fixtures), len(events), len(xgGWs), xg,
			recon, rep, metric)

		// A prior-only season must refuse to be played, and every other season must
		// agree to be. Both directions, because the failure this marker exists to
		// prevent is a *partial* load reading as a whole one: a season that quietly
		// became prior-only would drop out of every grid it appears in while every
		// number still printed.
		err := s.PlayableAsCurrent()
		if s.PriorOnly() != (err != nil) {
			t.Errorf("%s: PriorOnly()=%v but PlayableAsCurrent()=%v — the marker and "+
				"the guard disagree", name, s.PriorOnly(), err)
		}
		if name == "2018-19" && !s.PriorOnly() {
			t.Errorf("2018-19 loaded as playable; the archive publishes no teams.csv " +
				"for it, so either the loader invented clubs or the archive gained a " +
				"file and this grid can be widened")
		}
		if name != "2018-19" && s.PriorOnly() {
			t.Errorf("%s loaded as prior-only, missing %v. Every season in this grid "+
				"bar 2018-19 is meant to be playable, and a transient network fault "+
				"must be a hard error rather than absence — check errNoSuchFile",
				name, s.Absent)
		}

		// Nothing below is season-specific. A grid whose cells are different shapes is
		// the thing that makes a paired difference meaningless, so every season in it
		// gets the same assertions.
		// 2022-23 has 37, and it is real football rather than a hole: GW7 was
		// cancelled outright after the Queen's death in September 2022 and several
		// GW8 fixtures were pushed back for policing. A backfill there would be
		// inventing matches, so the exception is named with its cause instead of the
		// bound being loosened to 37 for everyone — which would stop the check
		// catching 2019-20's nine missing restart rounds, the thing it is for.
		wantGWs := 38
		if name == "2022-23" {
			wantGWs = 37
		}
		if len(gws) != wantGWs {
			t.Errorf("%s: %d gameweeks carry football, want %d. For 2019-20 that means "+
				"renumberGW is not reaching loadGameweeks and the nine restart rounds "+
				"labelled 39-47 are being dropped", name, len(gws), wantGWs)
		}
		if len(events) != wantGWs {
			t.Errorf("%s: fixtures span %d events, want %d — the fixture list and the "+
				"gameweek rows must be on one numbering or TeamFixtures finds nothing",
				name, len(events), wantGWs)
		}
		if len(s.Fixtures) != 380 {
			t.Errorf("%s: %d fixtures, want 380", name, len(s.Fixtures))
		}
		if minutes < 740000 || minutes > 760000 {
			t.Errorf("%s: %d league minutes, want about 749,000 — a quarter of a "+
				"season going missing shows up here and almost nowhere else",
				name, minutes)
		}
		if len(xgGWs) < wantGWs {
			t.Errorf("%s: only %d of %d gameweeks carry any expected goals. A repair "+
				"that applied nothing looks identical to a season that never had the "+
				"statistic, which is why this is asserted rather than eyeballed",
				name, len(xgGWs), wantGWs)
		}
		if xg < 500 || xg > 1600 {
			t.Errorf("%s: total weekly xG is %.1f, which is not a season's worth "+
				"(the four seasons FPL publishes it for run 1068 to 1198)", name, xg)
		}
	}

	// Exactly one season in the grid must be HOLD-only, and it must be 2019-20.
	// Asserted rather than trusted, because a grid gaining a POLICY cell governed by a
	// rule with no analogue is the failure BankLimitFor exists to prevent, one season
	// further back than BankLimitFor can reach.
	var holdOnly []string
	for _, n := range seasons {
		if !TransferPathComparable(n) {
			holdOnly = append(holdOnly, n)
		}
	}
	if len(holdOnly) != 1 || holdOnly[0] != "2019-20" {
		t.Errorf("HOLD-only seasons in the grid are %v, want exactly [2019-20] — FPL "+
			"granted unlimited free transfers before the GW30+ deadline that season "+
			"and froze prices for three months, so its transfer path is not a sample "+
			"of the same process", holdOnly)
	}

	// And the arithmetic the seventh season is for, stated as a count rather than left
	// to be inferred from the grid. The pair 2018-19 unlocks *plays* 2019-20, which
	// POLICY has to exclude anyway — so this widens the season axis for scoring
	// constants and not for transfer ones, and saying so here stops the seven being
	// quoted at a transfer constant.
	played := map[string]bool{}
	for _, p := range scoringPairNames() {
		played[p[1]] = true
	}
	var forHold, forPolicy int
	for n := range played {
		forHold++
		if TransferPathComparable(n) {
			forPolicy++
		}
	}
	t.Logf("playable seasons: %d for HOLD (df %d), %d for POLICY (df %d)",
		forHold, forHold-1, forPolicy, forPolicy-1)
	if forHold != 7 || forPolicy != 6 {
		t.Errorf("the grid plays %d seasons for HOLD and %d for POLICY, want 7 and 6",
			forHold, forPolicy)
	}

	// The escape hatch, on the seasons the repair actually reaches. A sweep comparing
	// backfilled against blind needs both arms to be real, and this package's recorded
	// failure is a hatch that reads a repaired cache and reports the two as identical.
	t.Setenv("FPL_NO_XG_REPAIR", "1")
	for _, name := range repairedSeasons() {
		// Load, not loadSeason: the process-global cache would hand back the arm that
		// won the race, and the whole point is to parse this one with the switch set.
		s, err := Load(t.Context(), cfg.CacheDir, name)
		if err != nil {
			t.Fatal(err)
		}
		if s.XGRepair.Rows != 0 {
			t.Errorf("%s: FPL_NO_XG_REPAIR set and the repair still read %d rows",
				name, s.XGRepair.Rows)
		}
		var xg float64
		for _, p := range s.Players {
			for _, g := range p.GWs {
				xg += g.XG
			}
		}
		t.Logf("%-9s with the repair off: %.1f xG", name, xg)
		if name != "2022-23" && xg != 0 {
			t.Errorf("%s: %.1f xG with the repair off, want 0 — this season has no "+
				"expected_goals column in the archive at all, so anything here came "+
				"from the repair and the switch did not reach it", name, xg)
		}
	}
}
