package main

import (
	"os"
	"strings"
	"testing"
)

// The transfer search's minutes floor must be SCALED to the window FPL's aggregates
// cover, never compared as a raw season total.
//
// # The bug this pins, which shipped
//
// buildTransferBoard filtered candidates on `c.Minutes < 600`. That 600 is a season
// total; FPL's aggregates reset when GW1 completes and then accumulate. Measured on the
// live data that exposed it: 0 of 609 players had 600+ minutes and the most anyone had
// played was 90, so the candidate pool was EMPTY. BuildPlans had nothing to rank and
// every transfer surface — `armband transfers`, the agent's suggest_transfers, and the
// page's Suggest-transfers button — answered "nothing would improve this squad" for
// every user, for roughly the first seven gameweeks of a season.
//
// It is the same defect ScaledMinMinutesFor was written for, whose own doc comment says
// "every player in the game failed it against a fresh-season minutes count". That was
// fixed for Optimize and not here, so the two searches disagreed about who is even a
// candidate.
//
// # Why a source scan rather than a behavioural test
//
// The scaling itself is already pinned behaviourally, in
// internal/analysis/minutesfloorwindow_test.go. What was missing was not that the
// scaling worked — it did — but that this caller never asked for it. A source scan is
// what catches that, and it is the same instrument this repository already uses for
// TestTheHitCeilingIsReadByTheFundedPairBranch.
//
// ⚠️ This does NOT prove the pool is non-empty on live data. It proves the floor is
// scaled before it is compared. The emptiness was the symptom; the unscaled compare was
// the cause.
func TestTheTransferPoolScalesItsMinutesFloor(t *testing.T) {
	// ⚠️ EVERY file that builds a transfer candidate pool, not just this package's.
	//
	// The first version of this guard read transfers.go alone, and a THIRD live copy of
	// the same defect sat in internal/agent/tools.go the whole time it was green: the
	// agent's suggest_transfers built its own pool with a bare `c.Minutes < minMins` and
	// went on returning nothing while the CLI was fixed and shipped. A guard scoped to
	// the file whose bug prompted it certifies that one file and nothing else, which on
	// a defect that exists in three places is worse than no guard, because it reads as
	// coverage.
	for _, f := range []string{"transfers.go", "../../internal/agent/tools.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		checkScaledFloor(t, f, string(src))
	}
}

func checkScaledFloor(t *testing.T, name, body string) {
	t.Helper()

	if !strings.Contains(body, "ScaledMinMinutesFor(") {
		t.Errorf("%s no longer scales its minutes floor. A season-total floor compared "+
			"against fresh-season aggregates empties the pool and every transfer surface "+
			"silently answers \"nothing would improve this squad\".", name)
	}

	// Any comparison of a player's Minutes against a bare number is the defect returning.
	// The scaled floor arrives through floorFor(), never as a literal.
	//
	// ⚠️ Comments are stripped first. The first draft of this guard scanned the raw file
	// and failed on the sentence explaining the bug, which names `c.Minutes < 600` as
	// the thing not to write — a guard that cannot coexist with its own rationale is one
	// somebody deletes rather than fixes.
	// Checked per LINE, and a line is clean only if its bound is the scaler itself.
	//
	// Two shapes are the defect: a literal (`Minutes < 600`) and an unscaled variable
	// (`Minutes < minMins`). The third copy was the second kind, so a literal-only scan
	// would have stayed green through it. But a naive "no identifier after <" rule also
	// rejects the CORRECT form, `Minutes < t.Engine.ScaledMinMinutesFor(...)`, which is
	// what the sibling tool in the same file already does — so the test asks whether the
	// bound is the scaled one rather than what it looks like.
	for i, line := range strings.Split(stripComments(body), "\n") {
		if !strings.Contains(line, ".Minutes <") {
			continue
		}
		if strings.Contains(line, "ScaledMinMinutesFor") || strings.Contains(line, "floorFor") {
			continue
		}
		t.Errorf("%s:%d: minutes compared against an unscaled bound:\n\t%s\nThe floor is "+
			"a SEASON TOTAL and must go through ScaledMinMinutesFor first — see this "+
			"test's comment for what an unscaled compare did on live GW1 data.",
			name, i+1, strings.TrimSpace(line))
	}
}

// stripComments removes // comment text so a source scan matches code rather than the
// prose describing it. Crude on purpose: it does not understand strings or block
// comments, and does not need to — this file's scan only asks about one operator.
//
// ⚠️ It is load-bearing. The first draft of this guard scanned the raw file and failed on
// the sentence explaining the bug, which names the thing not to write — a guard that
// cannot coexist with its own rationale is one somebody deletes rather than fixes.
func stripComments(src string) string {
	var out strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}
