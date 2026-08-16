package backtest

// Does tracking a club's attack faster earn points?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTeamFormSweep -v -timeout 4h
//
// # What is being arbitrated, and why it needs the replay
//
// `TestDiagTeamGoalShare` established two things about the club-level error. The
// **static** anchor is closed: a club's error correlates with its own next season at
// −0.232, so an offset fitted on history would carry noise forward rather than
// correct anything. And a **short clock** is not closed: a half-and-half blend of
// the model with the club's own preceding nine gameweeks predicts its next stretch
// 13.1% better pooled, with an interior optimum, though at t = 0.82 across four
// seasons it does not resolve.
//
// Everything in that result is club-level expected-goals *prediction*. This project
// has a recorded case of a 2% better predictor costing about 49 points a season,
// because a transfer policy is an argmax living in the tail of the estimate
// distribution and a change that improves the average player can still inflate
// whatever the search reaches for. Prediction cannot settle it. The replay can.
//
// # Why this one is more dangerous than most, stated before the result
//
// This file's standing rule is that a bias shared by every player in a position is
// harmless, because the optimiser consumes an ordering and FPL forces five defenders
// regardless. **A club-level shift does not have that property.** Nothing forces you
// to own players from any particular club — only a maximum of three — so moving one
// club's whole attack reorders the entire pool. It is an ordering change of the
// dangerous kind, which is exactly why it ships off and why the arms below include
// the shipped setting as arm zero rather than as an afterthought.
//
// # The grid, and what to read
//
// `HOLD` is the quiet metric and the one a scoring change belongs on: it holds the
// opening fifteen and re-picks the eleven and the captain weekly, so it excludes the
// noisy transfer decision. `POLICY` is reported beside it because a between-club
// correction plausibly acts *through* transfers — it changes which clubs look worth
// buying — and a change that moves POLICY while leaving HOLD alone would be saying
// something specific about that channel.
//
// Weights are 0 (shipped), 0.25, 0.50 and 0.75. The measured optimum is 0.50 and the
// two neighbours bracket it, so this is a shape check rather than an argmax hunt: a
// monotone ladder or a clean interior peak means something, and a single winner
// surrounded by noise means the surface is rough and nothing should move.
//
// **The prediction work's ordering, committed here in advance: best 0.50, then 0.75,
// then 0.25** — rms log errors 0.2390, 0.2457 and 0.2495. An ordering is cheaper to
// establish than a gap on this harness, which is why it is written down before the
// run rather than noticed after it.
//
// ⚠️ An earlier version of this sentence said "0.25 worse than 0.50 worse than 0.75",
// which describes a different ordering from the table it claimed to be quoting — the
// prediction run has 0.75 *better* than 0.25, not worse. The transcription was wrong,
// not the data. Anyone weighing the ordering agreement should treat 0.25-is-worst as
// genuinely pre-registered and the 0.50-against-0.75 pair as having been stated
// correctly only after the fact.
//
// Judge on paired differences per gameweek played, never on totals — a GW1 entry
// banks 38 gameweeks and a GW26 entry 13. The per-cell rows go to
// `stats/sweep_inference.R`, which is the only thing in this project that turns them
// into a standard error, a t or a verdict word.

import (
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagTeamFormSweep(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	// Restore whatever was live rather than assuming it was off, so running this
	// with FPL_TEAM_FORM already set does not leave the process on a different
	// setting from the one it started with.
	was := analysis.TeamFormWeight()
	defer analysis.SetTeamFormWeight(was)

	arm := func(label string, w float64) policyVariant {
		return policyVariant{
			label: label,
			apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				// The weight is a package variable rather than a SimConfig field
				// because it is read deep inside scoring, by an engine the sweep
				// does not construct. Set here, in apply, so each arm's setting is
				// in force for exactly its own cells.
				analysis.SetTeamFormWeight(w)
			},
		}
	}

	runPolicySweep(t, []policyVariant{
		arm("shipped, no club-form blend", 0),
		arm("club form, weight 0.25", 0.25),
		arm("club form, weight 0.50", 0.50),
		arm("club form, weight 0.75", 0.75),
	}, sweepStarts())
}
