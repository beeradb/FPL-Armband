package main

import (
	"os"
	"regexp"
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
	src, err := os.ReadFile("transfers.go")
	if err != nil {
		t.Fatalf("reading transfers.go: %v", err)
	}
	body := string(src)

	if !strings.Contains(body, "ScaledMinMinutesFor(") {
		t.Error("the transfer pool no longer scales its minutes floor. A season-total " +
			"floor compared against fresh-season aggregates empties the pool and every " +
			"transfer surface silently answers \"nothing would improve this squad\".")
	}

	// Any comparison of a player's Minutes against a bare number is the defect returning.
	// The scaled floor arrives through floorFor(), never as a literal.
	//
	// ⚠️ Comments are stripped first. The first draft of this guard scanned the raw file
	// and failed on the sentence explaining the bug, which names `c.Minutes < 600` as
	// the thing not to write — a guard that cannot coexist with its own rationale is one
	// somebody deletes rather than fixes.
	bare := regexp.MustCompile(`\.Minutes\s*[<>]=?\s*\d`)
	if m := bare.FindString(stripComments(body)); m != "" {
		t.Errorf("minutes compared against a literal (%q). The floor is a SEASON TOTAL "+
			"and must go through ScaledMinMinutesFor first — see this test's comment for "+
			"what an unscaled compare did on live GW1 data.", m)
	}
}

// stripComments removes // comment text so a source scan matches code rather than the
// prose describing it. Crude on purpose: it does not understand strings or block
// comments, and does not need to — this file's scan only asks about one operator.
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
