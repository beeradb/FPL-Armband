package analysis

import (
	"strings"
	"testing"
)

// An unverified budget must never render as silence. It is the one condition
// where every number downstream still looks right and the arithmetic underneath
// is wrong, so the absence of a warning has to mean the budget is real.
func TestUnverifiedBudgetIsAlwaysAudible(t *testing.T) {
	ok := VerifiedBudget()
	if ok.Warning() != "" {
		t.Errorf("a verified budget warns: %q", ok.Warning())
	}
	if !strings.Contains(ok.Label(), "verified") {
		t.Errorf("verified label %q does not say so", ok.Label())
	}

	bad := AssumedBudget("No session.")
	if bad.Warning() == "" {
		t.Fatal("an assumed budget is silent — a run that lost its session would " +
			"produce a report indistinguishable from a good one")
	}
	for _, want := range []string{"NOT VERIFIED", "market", "No session."} {
		if !strings.Contains(bad.Warning(), want) {
			t.Errorf("warning %q omits %q", bad.Warning(), want)
		}
	}
	if !strings.Contains(bad.Label(), "ASSUMED") {
		t.Errorf("assumed label %q does not say so", bad.Label())
	}
	// The label goes into tool JSON that is replayed on every API call, so it
	// has to stay short.
	if len(bad.Label()) > 80 {
		t.Errorf("label is %d characters; it is replayed on every call", len(bad.Label()))
	}
}

// The zero value is the dangerous one: a caller that forgets to set Budget must
// get the loud state, not the quiet one.
func TestZeroBudgetTrustIsUnverified(t *testing.T) {
	var b BudgetTrust
	if b.Verified {
		t.Fatal("the zero value claims to be verified, so forgetting to set it " +
			"silently asserts a budget nobody checked")
	}
	if b.Warning() == "" {
		t.Error("the zero value is silent")
	}
}

// TestSellPriceSourceNamesAllFourStates pins the label a squad's own selling
// prices get, across every BudgetTrust a caller can actually construct.
// Verified alone is ambiguous between "nothing bought yet" and "a real
// reconstruction" -- hasSell is what tells the two apart, and both must stay
// equally silent on Warning() since neither is an assumption.
func TestSellPriceSourceNamesAllFourStates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trust   BudgetTrust
		hasSell bool
		want    string
	}{
		{"pre-season, nothing bought", VerifiedBudget(), false, "market"},
		{"an exact reconstruction", VerifiedBudget(), true, "reconstructed"},
		{"a reconstruction that missed FPL's own team value", DriftingBudget(5), true, "estimated"},
		{"no entry to reconstruct from at all", AssumedBudget("no session"), false, "market"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.trust.SellPriceSource(tc.hasSell); got != tc.want {
				t.Errorf("SellPriceSource(%v) = %q, want %q", tc.hasSell, got, tc.want)
			}
		})
	}
}
