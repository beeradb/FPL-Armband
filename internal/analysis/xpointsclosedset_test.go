package analysis

import "testing"

// TestXPointsAccountsForEveryPaidScoringKey.
//
// # The gap this closes
//
// `XPointsResidual` replaces four channels and leaves the rest at their realised
// values, and its whole safety argument is that the second set is *known* — "a
// channel this file does not replace keeps whatever FPL paid for it". That argument
// is only as good as the file's account of which channels those are, and two
// independent rulebook audits found the account incomplete: the doc named five and
// FPL pays eight, with `penalties_missed`, `penalties_saved` and `own_goals` missing.
//
// Nothing was wrong in the arithmetic — those three sit inside `Points` and were
// handled correctly by construction. What was missing was any mechanism to notice a
// key nobody had classified, which is exactly the failure the engine's
// `TestFPLPaysNothingTheModelDoesNotPrice` exists to prevent one file away. That
// guard's own comment records the cost of the last one: defensive contribution
// arrived, nothing priced it, and a defender lost 0.84 points per 90 for a season
// while every test passed.
//
// So this is the same guard on the xPoints side, and its value is entirely about the
// NEXT rule change rather than this one.
//
// # Why "replaced" and "realised" are both listed rather than just the replaced set
//
// A key that is neither is the interesting case, and it can only be detected if both
// sets are written down. Listing only the replaced channels would let a new key fall
// into "realised" silently — which is the correct treatment nine times in ten, and
// catastrophic the tenth, when the new key is one the underlying could price.
// Defensive contribution is the worked example: left realised is right for it *today*
// because the archive carries one season of it, and that is a judgement someone made,
// not a default that happened.
// ⚠️ **It must FAIL rather than skip when FPL is unreachable**, which is why it does
// not use `scoringConfig`'s skipping helper alone. This guard's whole stated value is
// "entirely about the NEXT rule change", and the next rule change is exactly the
// moment somebody runs the suite on a machine that cannot reach `bootstrap-static`
// and reads `ok`. The sibling `TestTheXPointsScriptsShareTheScoringTable` makes the
// same choice and states the same reason: a guard that silently stops covering its
// subject is worse than none. Two conventions in one package was the defect.
func TestXPointsAccountsForEveryPaidScoringKey(t *testing.T) {
	gc, _ := scoringConfig(t)
	if len(gc.PaidScoringKeys()) == 0 {
		t.Fatal("FPL published no paid scoring keys, so this guard compared nothing.\n\n" +
			"That is a vacuous pass, not a clean one: the classification below would " +
			"be checked against an empty list and every unclassified key would go " +
			"unnoticed. If the API is genuinely unreachable, fix the fetch rather " +
			"than letting the closed-set check evaporate.")
	}

	// Priced off realised underlying by XPointsResidual.
	replaced := map[string]string{
		"goals_scored":   "XG against realised goals",
		"assists":        "XA against realised assists",
		"clean_sheets":   "per-fixture exp(-XGC/f), capped at eligible appearances",
		"goals_conceded": "Poisson E[floor(X/d)] against realised, single fixtures only",
	}

	// Deliberately left at their realised values, each with the reason. The reason
	// matters more than the membership: "conservative" is a claim about independence
	// from the replaced channels, and for bonus it is measurably false.
	realised := map[string]string{
		"long_play":  "appearance, not conversion variance",
		"short_play": "appearance, not conversion variance",
		"bonus": "⚠️ NOT independent of the replaced channels — BPS pays goals, " +
			"assists and clean sheets, so ~25% of an attacker's stripped " +
			"conversion luck returns here (slope 0.252). Under-smoothing, " +
			"measured, and not fixable without modelling expected bonus, which " +
			"is a closed line",
		"saves":                  "divisor unpublished; restructured for 2026/27",
		"defensive_contribution": "2025-26 only; 6 live cells in 36",
		"yellow_cards":           "not a conversion channel",
		"red_cards":              "not a conversion channel",
		"penalties_missed": "xG already includes the penalty at ~0.79, so the " +
			"expected value is credited and the realised −2 stays; the two " +
			"cancel over attempts, leaving dispersion rather than bias",
		"penalties_saved": "5 pts, ~14 occurrences a season league-wide",
		"own_goals": "−2 to the scorer, uncorrelated; its effect on his club's " +
			"conceded total is already inside the measured clean-sheet " +
			"over-prediction and must not be counted twice",
		"special_multiplier": "not a per-event scoring term; the identity at 1",
	}

	for _, k := range gc.PaidScoringKeys() {
		if _, ok := replaced[k]; ok {
			continue
		}
		if _, ok := realised[k]; ok {
			continue
		}
		t.Errorf("FPL pays for %q and xpoints.go classifies it as neither replaced "+
			"nor deliberately realised.\n\n"+
			"Leaving it realised is probably right — that is what the residual "+
			"construction does by default, and it is why this is a doc guard rather "+
			"than a scoring bug. But decide it rather than inherit it: if the new "+
			"key is something the underlying could price, silently realising it "+
			"wastes the instrument on exactly the channel it was built for.\n\n"+
			"Add %q to `replaced` or to `realised` with the reason and the size.",
			k, k)
	}

	// The other direction: a channel this file replaces that FPL has stopped paying
	// for. Then the residual subtracts a realised contribution against an expectation
	// for an event worth nothing, and every row carrying it is silently mis-scored.
	paid := map[string]bool{}
	for _, k := range gc.PaidScoringKeys() {
		paid[k] = true
	}
	for k, how := range replaced {
		if !paid[k] {
			t.Errorf("xpoints.go replaces %q (%s) and FPL no longer pays for it. "+
				"The residual would price an expectation for an event worth "+
				"nothing.", k, how)
		}
	}
}
