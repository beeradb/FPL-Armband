package backtest

import (
	"fmt"
	"testing"
)

// DOES TRANSFERRING TOWARD THE CURRENT POPULAR CORE EACH WEEK BEAT SHIPPED POLICY?
//
//	DIAG=1 EXP=TEMPLATECORETRANSFER FPL_CELLS=/tmp/tct.csv scripts/replay \
//	    -run TestDiagTemplateCoreTransfer -v -timeout 4h
//
// # What this measures
//
// The weekly transfer search gains a default-off overlay: TemplateCoreTransferK
// re-reads the k highest-owned feasible players at each deadline, refuses to
// sell any already owned, and gives RankSwaps/RankPairs the first look at
// buying a missing one. The shipped acceptTransfer gate still decides. If a
// core-buy fails the gate, decide retries the shipped search with retention
// still on.
//
// Opening TemplateCoreK stays 0 in every arm — that lever was measured on HOLD
// and did not clear its threshold. This is not the closed separable-band
// ownership tiebreak; ownership does not enter Score.
//
// # Arms (family size 1)
//
//	A0  TemplateCoreTransferK = 0   (shipped)
//	A1  TemplateCoreTransferK = 4   (the only arm; k=3 is not in the family)
//	    TemplateCoreK = 0 in both
//
// POLICY is the metric. HOLD is printed and is not in the family. Do not ship
// a default of 4 from this file — the shipped default stays 0 until a POLICY
// comparison resolves past its own threshold.
//
// # VOID, not null
//
// runPolicySweep already emits moves and policy_points. VOID if A1's moves and
// POLICY are identical to A0 in every cell — the overlay never changed a
// transfer. Same shape as an unwired ownership table.
func TestDiagTemplateCoreTransfer(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== DOES WEEKLY TRANSFER TOWARD THE FORMING POPULAR CORE BEAT SHIPPED POLICY?\n")
	fmt.Printf("A0: TemplateCoreTransferK=0 (shipped). A1: TemplateCoreTransferK=4.\n")
	fmt.Printf("Opening TemplateCoreK=0 in both. Family size 1. POLICY is the metric; HOLD is descriptive.\n")
	fmt.Printf("⚠️ VOID if every cell's moves and POLICY match A0 — the overlay never\n")
	fmt.Printf("changed a transfer. Read moves before any points figure.\n")
	fmt.Printf("⚠️ Do not ship TemplateCoreTransferK=4 from this diagnostic; default stays 0.\n")

	arms := []policyVariant{
		{label: "A0: weekly template core off (k=0)",
			apply: func(sc *SimConfig) {
				sc.Weights.TemplateCoreK = 0
				sc.Weights.TemplateCoreTransferK = 0
			}},
		{label: "A1: weekly template core k=4",
			apply: func(sc *SimConfig) {
				sc.Weights.TemplateCoreK = 0
				sc.Weights.TemplateCoreTransferK = 4
			}},
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ Decision rule: A1 resolves positive on POLICY iff the mean paired\n")
	fmt.Printf("difference clears this comparison's own season-clustered threshold.\n")
	fmt.Printf("Unresolved or negative ⇒ default stays off (k=0). Write \"ruled out for\n")
	fmt.Printf("shipping,\" not \"weekly template transfers do not pay.\" k=4 is the only\n")
	fmt.Printf("arm; a second k is a new comparison.\n")
}
