package backtest

// HOW EXPENSIVE MUST THE REPAIR BE BEFORE YOU WILDCARD?
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest \
//	    -run TestDiagWildcardReservation -v -timeout 90m
//
// # Three numbers for one decision, and the shipped one is asserted
//
// `config.DefaultWildcardReservation` is **12.0 — three hits — and its own
// comment marks it "Asserted"**, never measured. That comment then cites a
// community rule of thumb that is a DIFFERENT number: *"if it takes more than two
// hits to fix, wildcard it"*, which is 8. And the user's own practice is a third:
// *"4 or more hits is usually a wildcard"*, which is 16.
//
// | source | hits | points |
// |---|---|---|
// | the shipped comment's rule | >2 | >8 |
// | what actually ships | 3 | 12 |
// | the user's practice | 4 | 16 |
//
// Nothing has ever measured which is right.
//
// # Why this is the right shape for the question, where a drift bar was not
//
// The user's framing is that the wildcard is not a special decision at all:
// *"it's the same decision you make on all transfers. The first half wildcard
// just gets spent when the hits get too expensive and you are too far from
// optimal."* A reservation priced IN HITS says exactly that, and it is already
// the quantity the shipped rule reads.
//
// It is also the quantity the transfer path weighs, so the chip rule and the
// weekly rule are commensurate rather than two decisions about one thing in
// different units.
//
// ⚠️ **The bar is a BASE, not a threshold.** `analysis.ChipBarAt` decays it
// through the option window, so a reservation of 12 is not "fires at 12 points
// all season" — it falls toward expiry, which is what stops an unplayed chip
// scoring nothing. Read the ladder as a base, and read the fire weeks beside it.
//
// # ⚠️ What this still does not price
//
// The value of WAITING for information — injuries revealed, rotations observed —
// which the user names and which nothing here models. `ChipBarAt`'s decay is the
// stand-in and is a generic curve, not a fitted one. **A bar that measures well
// here is well-fitted to a replay that is as blind to that value as the rule is**,
// so a low winning bar should be read with that in mind rather than adopted.
//
// ⚠️ And `changes` is still `changesBetween` — a raw count over all fifteen, in
// which a £4.0m bench swap scores like a lost captain. That is the objection
// `xidrift.go` exists for, and it means "four hits" here counts four SWAPS rather
// than four swaps worth paying for. Fixing that changes the input to this ladder,
// not the ladder's shape.

import (
	"fmt"
	"os"
	"testing"
)

// wildcardReservations bracket the argued range and sample inside it, in points.
//
// The three stated rules are 8, 12 and 16 — two, three and four hits — and the
// user's read is that *"2-3 hits could be argued; the truth is likely between"*.
// So the ladder spans exactly 2 to 4 hits and puts a rung between each pair,
// rather than spending an arm on 20 (five hits), which nobody argues for and
// which would only tell us the top falls off.
//
// ⚠️ **The half-hit rungs are legitimate bases even though half a hit is not a
// move.** `ChipBarAt` decays the base continuously toward expiry, so the firing
// threshold is a curve rather than a step and 10 is a real position on it — it is
// not "2.5 transfers".
//
// ⚠️ **Five rungs is more argmax exposure than four**, and picking the best of
// five overstates it further. The trade is deliberate: closer spacing is what
// makes a PLATEAU legible, and this record accepts a plateau with a cliff as
// evidence for a knob where it refuses a lone spike. Read the shape.
var wildcardReservations = []float64{8, 10, 12, 14, 16}

func TestDiagWildcardReservation(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== how expensive must the repair be before the wildcard fires?\n")
	fmt.Printf("Reservations in POINTS: 8 to 16 is 2 to 4 hits, sampled every half\n")
	fmt.Printf("hit because the argued range is 2-3 and the truth is likely between.\n")
	fmt.Printf("The shipped\n")
	fmt.Printf("default is 12 and is ASSERTED. Confined to GW1-%d, where there is\n", ChipResetGW-1)
	fmt.Printf("no double to anchor to and the rule is a squad condition.\n")
	fmt.Printf("⚠️ These are BASES: ChipBarAt decays each one toward expiry.\n")

	arms := []policyVariant{
		{label: "no wildcard trigger (control)",
			apply: func(sc *SimConfig) { sc.WildcardTrigger = false }},
	}
	for _, r := range wildcardReservations {
		res := r
		label := fmt.Sprintf("reservation %.0f pts (%.0f hits)", res, res/HitCost)
		if res == 12 {
			label += " — SHIPPED"
		}
		arms = append(arms, policyVariant{
			label: label,
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger = true
				sc.WildcardTriggerFirstHalfOnly = true
				sc.WildcardReservation = res
			},
		})
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ Read the SHAPE, not the best arm: four bars and picking the top\n")
	fmt.Printf("is winner's curse, and this project accepts a plateau with a cliff\n")
	fmt.Printf("as evidence for a knob where a lone spike is not.\n")
	fmt.Printf("⚠️ Read `--scale=per_path`, and read wc_trig_gw beside every arm — a\n")
	fmt.Printf("bar that never fires is inert, not neutral, and reads as the control.\n")
}
