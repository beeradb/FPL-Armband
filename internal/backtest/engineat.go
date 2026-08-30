package backtest

import (
	"armband/internal/analysis"
	"armband/internal/fpl"
)

// EngineAt builds the engine the replay itself would use at a cutoff, wired the
// same way `Simulate` wires it.
//
// # Why this is exported, and why it had to be
//
// `PointInTime` plus `analysis.NewEngineFull` looks like the whole of the seam and
// is not. `Simulate` attaches three things afterwards — the prior-season blend, the
// recency-weighted minutes index and the team-form source — and two of them change
// `ExpectedMinutes`, which is the input half the model's terms are built from:
//
//   - with `Engine.Recent` nil, `blendFor` never reaches the recency branch, so
//     `MinutesPerMatch` stays at the flat season-to-date mean and `blankRunFactor`
//     — the discount for a player one to three gameweeks into an absence — never
//     fires at all;
//   - with `Engine.Priors` nil, `blendRates` returns early: no prior blend, no
//     `BlendMinutesK`, no `shrinkToLeague`.
//
// So an unwired engine returns the *field* the model reads carrying its *fallback*
// value. It looks right, it is populated, and it is a different number.
//
// `ExpectedMinutes` is the headline but not the whole of it, and reading it as the
// whole of it has already produced one wrong diagnosis. `Engine.Priors` moves every
// **per-90 rate** as well, through the same `blendRates` — and one of those rates,
// `DefCon90`, the replay's prior index supplies only as a zero on every archived
// season — the field exists since 2026-08-14 but no archived pair has a
// defcon-carrying prior — so wiring it there deflates rather than informs. `Engine.Recent`, by contrast, reaches a rate only through the `form`
// branch, gated on `RateHalfLife > 0`, which ships at zero: at shipped weights it
// moves minutes and nothing else.
//
// That is not hypothetical. `cmd/flagfit` measured 48,803 observations against an
// unwired `ExpectedMinutes` and reported them as "the denominator the model uses";
// the audit that caught it noted the mechanism the write-up named — the recency
// index carrying old absences — was not even switched on in the run. This function
// exists so the next caller cannot make that mistake by omission.
//
// # Why the guard test could not catch it
//
// `TestEveryScoringEngineGetsRecency` counts `NewEngineFull` calls against
// `.Recent =` assignments **inside `simulate.go`**. An engine built anywhere else is
// invisible to it by construction, which is exactly where this one was built. The
// file's own rule — "every engine that scores players needs the index" — held; the
// enforcement had a seam.
//
// `through` is the last completed gameweek: everything up to and including it is
// known, and gameweek `through+1` is not. That is the same convention `PointInTime`
// takes and the same one `Simulate` uses when it passes `start-1`.
func EngineAt(cur, prior *Season, through int, cfg SimConfig) (*analysis.Engine, *fpl.Bootstrap) {
	boot, fx := PointInTime(cur, prior, through)
	e := analysis.NewEngineFull(boot, fx, cfg.Weights, analysis.Congestion{}, analysis.RoleRisk{})
	e.Priors = cfg.priors(cur, prior)
	e.Recent = cfg.recentIndex(cur, through)
	e.TeamForm = newTeamFormIndex(cur, through)
	// The arm under test. Off by default, so a replay that does not set it is
	// byte-identical to one from before this existed.
	e.Tiebreak = cfg.Tiebreak
	return e, boot
}
