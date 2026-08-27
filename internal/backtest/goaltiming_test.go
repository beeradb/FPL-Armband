package backtest

// When in a match are goals conceded?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagGoalTiming -v
//
// The archive carries no minute-of-goal, and FPL's fixtures endpoint lists
// scorers without timings. The defensive side is recoverable anyway, without
// either.
//
// Two players who STARTED the same match for the same club have different
// minutes and their own goals_conceded. A starter withdrawn at 70 minutes has
// goals_conceded covering minutes 0-70; an ever-present team-mate's covers 0-90.
// The difference is goals conceded in minutes 70-90. Aggregated over many matches
// with withdrawals at varied times, that is an empirical hazard curve by minute
// from data already parsed.
//
// Restricted to starters, because the window has to be a PREFIX of the match —
// a substitute's minutes are a suffix and the subtraction would be meaningless.
// Restricted to single-fixture gameweeks, because per-gameweek values are totals
// across a club's fixtures that week.
//
// # The built-in validation
//
// Every starter who played the full 90 for the same club in the same match must
// have an IDENTICAL goals_conceded. If they do not, the field is not what this
// measurement assumes and every number below is meaningless. That check runs
// first and is reported, because a reconstruction resting on an assumption about
// a field's meaning should verify the assumption rather than hope.

import (
	"fmt"
	"testing"
)

func TestDiagGoalTiming(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type starter struct {
		mins, gc int
	}
	// Buckets of exit minute; the tail is (exit, 90].
	bounds := []int{45, 60, 70, 80, 85}
	labels := []string{"45-59", "60-69", "70-79", "80-84", "85-89"}

	fmt.Printf("\n=== Goals conceded per minute, by when in the match\n")
	fmt.Printf("Reconstructed from team-mates' differing minutes: a starter withdrawn at\n")
	fmt.Printf("minute m has goals_conceded for [0,m], an ever-present team-mate for [0,90],\n")
	fmt.Printf("so the difference is goals in (m,90]. Starters only, single-fixture weeks.\n\n")

	var totalRefs, badRefs int

	type agg struct {
		n         int
		tailGoals float64
		tailMins  float64
	}
	pooled := map[string]*agg{}
	var wholeGoals, wholeMins float64

	for _, name := range []string{"2022-23", "2023-24", "2024-25", "2025-26"} {
		cur := loadSeason(t, cfg, name)

		byTeamGW := map[[2]int][]starter{}
		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			for gw, g := range p.GWs {
				if g.Fixtures != 1 || g.Starts != 1 || g.Minutes == 0 {
					continue
				}
				byTeamGW[[2]int{p.Team, gw}] = append(byTeamGW[[2]int{p.Team, gw}],
					starter{mins: g.Minutes, gc: g.GoalsConceded})
			}
		}

		for _, ss := range byTeamGW {
			// The reference: starters who played the whole match.
			ref, refN, consistent := -1, 0, true
			for _, s := range ss {
				if s.mins < 90 {
					continue
				}
				refN++
				if ref < 0 {
					ref = s.gc
				} else if s.gc != ref {
					consistent = false
				}
			}
			if refN == 0 {
				continue
			}
			totalRefs++
			if !consistent {
				badRefs++
				continue
			}
			wholeGoals += float64(ref)
			wholeMins += 90

			for _, s := range ss {
				if s.mins >= 90 || s.mins < 45 {
					continue
				}
				// Goals conceded after he left.
				tail := ref - s.gc
				if tail < 0 {
					continue // cannot happen if the field means what we assume
				}
				lab := ""
				for i := len(bounds) - 1; i >= 0; i-- {
					if s.mins >= bounds[i] {
						lab = labels[i]
						break
					}
				}
				if lab == "" {
					continue
				}
				if pooled[lab] == nil {
					pooled[lab] = &agg{}
				}
				a := pooled[lab]
				a.n++
				a.tailGoals += float64(tail)
				a.tailMins += float64(90 - s.mins)
			}
		}
	}

	fmt.Printf("Validation: %d team-matches with a full-90 reference, %d where full-90\n",
		totalRefs, badRefs)
	fmt.Printf("team-mates DISAGREED on goals conceded (those are discarded).\n")
	if totalRefs > 0 && float64(badRefs)/float64(totalRefs) > 0.02 {
		fmt.Printf("*** More than 2%% disagree. The field does not mean what this assumes;\n")
		fmt.Printf("*** treat every number below as unreliable.\n")
	}

	whole := wholeGoals / wholeMins
	fmt.Printf("\nWhole-match baseline: %.5f goals conceded per minute (%.3f per 90).\n\n",
		whole, whole*90)
	fmt.Printf("%-10s %8s %11s %11s %10s %9s\n",
		"left at", "n", "tailGoals", "tailMins", "perMin", "vs whole")

	for i, lab := range labels {
		a := pooled[lab]
		if a == nil || a.n < 30 {
			continue
		}
		rate := a.tailGoals / a.tailMins
		fmt.Printf("%-10s %8d %11.0f %11.0f %10.5f %8.2fx\n",
			lab, a.n, a.tailGoals, a.tailMins, rate, rate/whole)
		_ = i
	}

	// ------------------------------------------------------------- the confound
	//
	// FPL records at most 90 minutes. A player withdrawn at 87 is off for the rest
	// of a match that actually runs to about 90+5, so his true unexposed window is
	// ~8 minutes where this counts 3 — and goals in stoppage time land in the
	// difference regardless. Every tail denominator is therefore too small, and
	// worse the later the withdrawal, which inflates precisely the late rates.
	//
	// The archive carries no match length, so the fix is not available. What IS
	// available is the sensitivity: recompute under several stoppage assumptions
	// and see whether the finding survives any of them.
	fmt.Printf("\n=== Sensitivity to second-half stoppage time, which the archive lacks\n")
	fmt.Printf("Each column adds that many real minutes to every tail window AND to the\n")
	fmt.Printf("whole-match baseline. 0 is the table above.\n\n")
	stoppages := []float64{0, 3, 5, 8}
	fmt.Printf("%-10s", "left at")
	for _, st := range stoppages {
		fmt.Printf(" %9s", fmt.Sprintf("+%.0fmin", st))
	}
	fmt.Printf("\n")
	for _, lab := range labels {
		a := pooled[lab]
		if a == nil || a.n < 30 {
			continue
		}
		fmt.Printf("%-10s", lab)
		for _, st := range stoppages {
			base := wholeGoals / (wholeMins + st*wholeMins/90)
			rate := a.tailGoals / (a.tailMins + st*float64(a.n))
			fmt.Printf(" %8.2fx", rate/base)
		}
		fmt.Printf("\n")
	}
	fmt.Printf("\nIf the ratios collapse toward 1.0 as stoppage rises, the apparent\n")
	fmt.Printf("late-loading is an artifact of the 90-minute cap and this method cannot\n")
	fmt.Printf("settle the question without match lengths.\n")

	fmt.Printf("\nRead the last column. Above 1.0 means the closing minutes concede FASTER\n")
	fmt.Printf("than the match average, so a player withdrawn early is protected by more\n")
	fmt.Printf("than his lost minutes suggest — and, by the same token, late attacking\n")
	fmt.Printf("minutes are worth more than a linear prorating of a per-90 rate implies.\n")
	fmt.Printf("Those two errors point in OPPOSITE directions in the current model.\n")
}
