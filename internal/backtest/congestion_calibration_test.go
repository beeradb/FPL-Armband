package backtest

// Calibration for the fixture-congestion block: ShortRestPenalty,
// VeryShortRest and PostBreakPenalty.
//
// These eight constants (the five here and abroad, plus the European ones) were
// set by feel and never measured. That is a gap worth closing on precedent: the
// two hand-set constants that *were* measured came out opposite ways, one right
// in size and wrong in channel, the other worth nothing at all.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCongestion -v
//
// # Both channels, because the code and its comment disagree
//
// metrics.go says the congestion factor "scales expected minutes, not per-90
// quality" and then multiplies Score by it. That is exactly the mismatch that
// made new_coach_penalty wrong: 0.93 was a true statement about minutes applied
// to points, where the effect turned out to be zero because the survivors'
// per-90 output rose by as much as the group's minutes fell.
//
// So this reports minutes, points and points-per-90 separately. A term that
// only moves minutes belongs on the minutes channel like rest_minutes_factor; a
// term that moves points-per-90 too is a Score multiplier; one that moves
// neither should ship at 1.0.
//
// # Within-player, against his own normal weeks
//
// Comparing congested weeks across players would mostly measure which clubs
// play midweek — the good ones — so every ratio here is a player against his own
// uncongested average in the same season. That is the design the fixture-
// difficulty work in AGENTS.md settled on for the same reason.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"armband/internal/analysis"
)

// restBucket labels a gameweek for one club, using congestion.go's own
// thresholds. Calibrating against different boundaries than the code applies
// would measure a term the model does not have.
const (
	bucketVeryShort = "under 3 days' rest"
	bucketShort     = "3 to 4 days' rest"
	bucketPostBreak = "after an international break"
	bucketNormal    = "normal"
)

// TestDiagCongestionRest measures the rest and international-break penalties
// against every season the archive holds.
func TestDiagCongestionRest(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	seasons := []string{"2021-22", "2022-23", "2023-24", "2024-25", "2025-26"}

	// Per-bucket accumulators, pooled across seasons. Each entry is one
	// player-gameweek expressed as a ratio to that player's own normal-week
	// average, so the mean is directly the multiplier the model should carry.
	mins := map[string][]float64{}
	pts := map[string][]float64{}
	per90 := map[string][]float64{}
	// blanks counts the share of congested weeks where a regular starter
	// dropped under half his usual minutes. A mean multiplier hides that;
	// rotation is the risk a squad actually cares about.
	blankN := map[string]float64{}
	blankHit := map[string]float64{}
	weeks := map[string]float64{}

	for _, sn := range seasons {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatalf("%s: %v", sn, err)
		}
		bucket, covered := restBuckets(s)
		if covered == 0 {
			t.Fatalf("%s: no kickoff times in the archive, so nothing can be measured", sn)
		}
		for b, n := range countBuckets(bucket) {
			weeks[b] += float64(n)
		}

		for _, p := range s.Players {
			// A regular starter, or the comparison is dominated by players
			// whose minutes vary for reasons that have nothing to do with rest.
			if p.Minutes < 1500 || len(p.GWs) == 0 {
				continue
			}
			// The player's own baseline: his normal-rest weeks in this season.
			var baseMins, basePts, baseN float64
			for gw, g := range p.GWs {
				if bucket[p.Team][gw] != bucketNormal {
					continue
				}
				baseMins += float64(g.Minutes)
				basePts += float64(g.Points)
				baseN++
			}
			if baseN < 8 || baseMins <= 0 {
				continue
			}
			bm, bp := baseMins/baseN, basePts/baseN
			base90 := basePts / (baseMins / 90)
			if bm < 45 {
				continue // not a starter in his own normal weeks either
			}

			for gw, g := range p.GWs {
				b := bucket[p.Team][gw]
				if b == "" {
					continue
				}
				if b == bucketNormal {
					// The control for the rotation column. "23% of congested
					// weeks saw him drop under half his minutes" says nothing
					// without the rate in the weeks that were not congested —
					// squad players get rested in ordinary weeks too.
					blankN[b]++
					if float64(g.Minutes) < 0.5*bm {
						blankHit[b]++
					}
					continue
				}
				mins[b] = append(mins[b], float64(g.Minutes)/bm)
				if bp > 0 {
					pts[b] = append(pts[b], float64(g.Points)/bp)
				}
				// Per-90 only where he actually played enough for the rate to
				// mean anything — otherwise a 5-minute cameo dominates.
				if g.Minutes >= 60 && base90 > 0 {
					per90[b] = append(per90[b], (float64(g.Points)/(float64(g.Minutes)/90))/base90)
				}
				blankN[b]++
				if float64(g.Minutes) < 0.5*bm {
					blankHit[b]++
				}
			}
		}
	}

	fmt.Printf("\nFixture congestion, measured within-player against his own normal weeks.\n")
	fmt.Printf("Seasons %v, starters with 1500+ minutes and 8+ normal weeks.\n\n", seasons)
	fmt.Printf("%-30s %7s %8s %8s %8s %8s %9s\n",
		"bucket", "club-gws", "n", "minutes", "points", "pts/90", "sub-half")
	for _, b := range []string{bucketVeryShort, bucketShort, bucketPostBreak} {
		if len(mins[b]) == 0 {
			fmt.Printf("%-30s %7.0f  (no player-gameweeks)\n", b, weeks[b])
			continue
		}
		fmt.Printf("%-30s %7.0f %8d %8.3f %8.3f %8.3f %8.1f%%\n",
			b, weeks[b], len(mins[b]),
			meanOf(mins[b]), meanOf(pts[b]), meanOf(per90[b]),
			100*blankHit[b]/math.Max(blankN[b], 1))
	}
	fmt.Printf("%-30s %7.0f %8.0f %8s %8s %8s %8.1f%%   <- control\n",
		bucketNormal, weeks[bucketNormal], blankN[bucketNormal], "1.000", "1.000", "1.000",
		100*blankHit[bucketNormal]/math.Max(blankN[bucketNormal], 1))

	// A ratio of 1.00 is "no effect". Whether an estimate is distinguishable
	// from that is the whole question, so print the interval rather than
	// inviting a read of the third decimal place.
	fmt.Printf("\n95%% intervals on the minutes ratio (the channel the comment claims):\n")
	for _, b := range []string{bucketVeryShort, bucketShort, bucketPostBreak} {
		if len(mins[b]) < 30 {
			continue
		}
		m, se := meanOf(mins[b]), sd(mins[b])/math.Sqrt(float64(len(mins[b])))
		fmt.Printf("  %-30s %.3f ± %.3f  [%.3f, %.3f]\n",
			b, m, 1.96*se, m-1.96*se, m+1.96*se)
	}

	fmt.Printf("\nShipped: VeryShortRest %.2f, ShortRestPenalty %.2f, PostBreakPenalty %.2f\n",
		cfg.Congestion.VeryShortRest, cfg.Congestion.ShortRestPenalty,
		cfg.Congestion.PostBreakPenalty)
	fmt.Printf("Note PostBreakPenalty only applies to players whose region is in\n")
	fmt.Printf("regular_intl_regions, which ships empty — so it is currently inert\n")
	fmt.Printf("for everyone regardless of what this measures.\n")
}

// restBuckets labels every (team, gameweek) by how congested it was, using the
// same rules congestion.go applies: rest days are the gap between a club's
// consecutive league kickoffs, and an international break is a gap of at least
// IntlBreakThresholdDays in the fixture calendar.
//
// It returns the number of fixtures that carried a kickoff time, so a season
// whose archive lacks them fails loudly rather than reporting a clean null.
func restBuckets(s *Season) (map[int]map[int]string, int) {
	type stamped struct {
		event int
		when  time.Time
	}
	byTeam := map[int][]stamped{}
	var earliest, latest map[int]time.Time
	earliest, latest = map[int]time.Time{}, map[int]time.Time{}
	covered := 0
	for _, f := range s.Fixtures {
		if f.KickoffTime == nil || f.Event == nil {
			continue
		}
		covered++
		byTeam[f.TeamH] = append(byTeam[f.TeamH], stamped{*f.Event, *f.KickoffTime})
		byTeam[f.TeamA] = append(byTeam[f.TeamA], stamped{*f.Event, *f.KickoffTime})
		if t, ok := earliest[*f.Event]; !ok || f.KickoffTime.Before(t) {
			earliest[*f.Event] = *f.KickoffTime
		}
		if t, ok := latest[*f.Event]; !ok || f.KickoffTime.After(t) {
			latest[*f.Event] = *f.KickoffTime
		}
	}

	// Gameweeks that follow a break in the calendar. congestion.go reads
	// deadlines; the archive has kickoffs, so this uses the gap between one
	// round's last match and the next round's first.
	postBreak := map[int]bool{}
	var events []int
	for ev := range earliest {
		events = append(events, ev)
	}
	sort.Ints(events)
	for i := 1; i < len(events); i++ {
		gap := earliest[events[i]].Sub(latest[events[i-1]]).Hours() / 24
		if gap >= analysis.IntlBreakThresholdDays {
			postBreak[events[i]] = true
		}
	}

	out := map[int]map[int]string{}
	for team, ss := range byTeam {
		sort.Slice(ss, func(i, j int) bool { return ss[i].when.Before(ss[j].when) })
		out[team] = map[int]string{}
		for i, cur := range ss {
			b := bucketNormal
			if i > 0 {
				switch rd := cur.when.Sub(ss[i-1].when).Hours() / 24; {
				case rd < 3:
					b = bucketVeryShort
				case rd < 4:
					b = bucketShort
				}
			}
			// A post-break week is never also a short-rest week — the gap that
			// defines one excludes the other — so the labels cannot collide.
			if b == bucketNormal && postBreak[cur.event] {
				b = bucketPostBreak
			}
			out[team][cur.event] = b
		}
	}
	return out, covered
}

func countBuckets(b map[int]map[int]string) map[string]int {
	out := map[string]int{}
	for _, byGW := range b {
		for _, label := range byGW {
			out[label]++
		}
	}
	return out
}
