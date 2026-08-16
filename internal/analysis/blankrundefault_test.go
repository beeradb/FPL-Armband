package analysis

import "testing"

// TestBlankRunPenaltyHasOneDefault is the DefaultBenchWeight-versus-
// Weights.BenchWeight shape, caught before it cost anything.
//
// `blankRunFactor` reads `e.Weights.BlankRunPenalty` and falls back to the
// package-level `blankRunPenalty` when the configured value is <= 0. That makes
// it the only weight resolving "unset" at the READ site rather than in
// `config.Load`, and it has two consequences worth pinning:
//
//   - a config of 0 does NOT turn the term off. It is silently rewritten to
//     0.75, which is the opposite of off — the term multiplies expected minutes,
//     so 0 would zero them. `1.0` is the off switch. docs/configuration.md
//     listed this field among those where "zero is honoured … meaning turn this
//     term off", which was wrong in the most misleading direction: a reader
//     following it would believe they had disabled a live term.
//   - the two defaults must agree. This project's recorded failure is exactly
//     two implementations of one quantity drifting, with the measured one not
//     being the one that runs.
//
// If the fallback ever moves into config.Load — which would be tidier — this
// test should move with it rather than being deleted.
func TestBlankRunPenaltyHasOneDefault(t *testing.T) {
	e := &Engine{}

	// Unset (zero) resolves to the package default, not to zero.
	if got := e.blankRunFactor(1); got != blankRunPenalty {
		t.Errorf("an unset BlankRunPenalty resolved to %v, want the package default %v",
			got, blankRunPenalty)
	}

	// And 0 is therefore NOT an off switch. Pinned because the configuration
	// reference said it was.
	e.Weights.BlankRunPenalty = 0
	if got := e.blankRunFactor(1); got == 0 {
		t.Error("a configured 0 disabled the term; docs say 1.0 is the off switch " +
			"and the read site rewrites 0 to the default")
	}

	// 1.0 is the off switch, and must survive the <= 0 fallback untouched.
	e.Weights.BlankRunPenalty = 1.0
	if got := e.blankRunFactor(1); got != 1.0 {
		t.Errorf("BlankRunPenalty 1.0 gave %v, want 1.0 — the documented way to "+
			"disable the term no longer works", got)
	}

	// A real setting is honoured rather than overridden by the fallback.
	e.Weights.BlankRunPenalty = 0.5
	if got := e.blankRunFactor(1); got != 0.5 {
		t.Errorf("a configured 0.5 gave %v; the fallback is eating real settings", got)
	}

	// Outside the run window the term is inert whatever it is set to.
	e.Weights.BlankRunPenalty = 0.5
	if got := e.blankRunFactor(0); got != 1 {
		t.Errorf("a run of 0 gave %v, want 1 — there is nothing to correct", got)
	}
	if got := e.blankRunFactor(blankRunMax + 1); got != 1 {
		t.Errorf("a run past blankRunMax gave %v, want 1 — the exponential average "+
			"has caught up by then, which is the measured cliff", got)
	}
}
