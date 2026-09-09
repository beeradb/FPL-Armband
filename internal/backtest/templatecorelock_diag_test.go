package backtest

import (
	"fmt"
	"testing"
)

// DOES LOCKING THE POPULAR CORE INTO THE OPENING FIFTEEN BEAT UNCONSTRAINED
// OPTIMIZE ON HOLD?
//
//	DIAG=1 EXP=TEMPLATECORELOCK FPL_CELLS=/tmp/tcl.csv scripts/replay \
//	    -run TestDiagTemplateCoreLock -v -timeout 4h
//
// # What this measures
//
// Optimize gains a default-off LockIDs overlay: TemplateCoreK appends the k
// highest-owned feasible players (position- and club-capped) before the
// shipped Score search fills the rest. Ownership does not enter Score. This
// is not the closed separable-band ownership tiebreak — that line stays
// closed; price shipped there.
//
// # Arms (family size 1)
//
//	A0  TemplateCoreK = 0   (shipped; unconstrained)
//	A1  TemplateCoreK = 4   (the only arm; k=3 is not in the family)
//
// HOLD is the metric. POLICY is printed and is not in the family. Do not
// ship a default of 4 from this file — the shipped default stays 0 until a
// HOLD comparison resolves past its own threshold.
//
// Ownership is archive ownershipAt already on PlayerMetrics.Ownership in the
// replay (GW.Selected → percent). The core is re-read at each cell's entry
// deadline because EngineAt rebuilds the bootstrap there.
//
// # VOID, not null
//
// runPolicySweep already emits SquadHash (opening fifteen identity) and
// moves. VOID if A1's HOLD is identical to A0 in every cell AND opening
// squad hashes never differ — the lock never bound. Same shape as the
// ownership-tiebreak run that came back byte-identical because every player
// read 0% owned. Read the squad-hash / moves columns before any points
// figure.
func TestDiagTemplateCoreLock(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== DOES A TEMPLATE-CORE LOCK BEAT UNCONSTRAINED OPTIMIZE ON HOLD?\n")
	fmt.Printf("A0: TemplateCoreK=0 (shipped). A1: TemplateCoreK=4.\n")
	fmt.Printf("Family size 1. HOLD is the metric; POLICY is descriptive.\n")
	fmt.Printf("⚠️ VOID if every cell's opening SquadHash matches A0 and HOLD is\n")
	fmt.Printf("byte-identical — the lock never changed a fifteen. Read squad\n")
	fmt.Printf("identity / moves before any points figure.\n")
	fmt.Printf("⚠️ Do not ship TemplateCoreK=4 from this diagnostic; default stays 0.\n")

	arms := []policyVariant{
		{label: "A0: template core off (k=0)",
			apply: func(sc *SimConfig) {
				sc.Weights.TemplateCoreK = 0
			}},
		{label: "A1: template core k=4",
			apply: func(sc *SimConfig) {
				sc.Weights.TemplateCoreK = 4
			}},
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ Decision rule: A1 resolves positive on HOLD iff the mean paired\n")
	fmt.Printf("difference clears this comparison's own season-clustered threshold.\n")
	fmt.Printf("Unresolved or negative ⇒ default stays off (k=0). k=4 is the only\n")
	fmt.Printf("arm; a second k is a new comparison.\n")
}
