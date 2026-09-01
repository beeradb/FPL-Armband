package main

import (
	"math"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// A free single transfer worth more than a hit-taking pair must be RECOMMENDED,
// not merely reachable.
//
// Reproduced from a live run in September 2026, with the figures below chosen to
// match its shape: on one free transfer the command printed a two-move package
// whose second move cost a -4, while a one-move package with slightly less gross
// gain was free and worth more once each was charged for what it spends.
//
// BuildPlans sorts on gross GainPerGW and the display printed the head of that
// list, so the banking rule's own valuation -- which does charge per move -- was
// computed and then overruled.
func TestAFreeTransferBeatsAHitWorthLess(t *testing.T) {
	cfg := config.Default()
	cfg.Review.MinGainForHit = 0 // isolate the ranking; the gate has its own test
	pair := analysis.Plan{Transfers: 2, GainPerGW: 0.76}
	solo := analysis.Plan{Transfers: 1, GainPerGW: 0.67}

	got := rankPlansByNetValue(cfg, []analysis.Plan{pair, solo}, 1, 3, 5)
	if len(got) == 0 {
		t.Fatal("ranking dropped every plan")
	}
	if got[0].Transfers != 1 {
		t.Fatalf("recommended %d transfers on one free; the pair spends a -4 to gain "+
			"%.2f/gw where the free single gains %.2f/gw and is worth more net",
			got[0].Transfers, pair.GainPerGW, solo.GainPerGW)
	}
}

// min_net_gain_for_minus_4 is written into the config, printed to the reader in
// `brief` as "Min net gain across the horizon to justify a -4", and passed to the
// agent prompt, the page and the backtest. Before this it was read by neither this
// command nor internal/analysis -- displayed and never applied. A hit that does
// not clear it must not be offered at all.
func TestTheMinusFourGateIsActuallyApplied(t *testing.T) {
	cfg := config.Default()
	cfg.Review.MinGainForHit = 3.0
	// Over a 5-week horizon this gains 5.0 and pays 4.0 for the hit: net 1.0,
	// which is short of the 3.0 the config demands.
	thin := analysis.Plan{Transfers: 2, GainPerGW: 1.0}

	if got := rankPlansByNetValue(cfg, []analysis.Plan{thin}, 1, 3, 5); len(got) != 0 {
		t.Fatalf("offered a hit netting %.1f across the horizon against a stated floor "+
			"of %.1f", thin.GainPerGW*5-fpl.HitCost, cfg.Review.MinGainForHit)
	}
}

// Before the first deadline the squad is rebuilt freely, so no move is a hit and
// the order must not be disturbed.
func TestUnlimitedTransfersAreNeverCharged(t *testing.T) {
	cfg := config.Default()
	in := []analysis.Plan{{Transfers: 3, GainPerGW: 2.0}, {Transfers: 1, GainPerGW: 1.0}}
	got := rankPlansByNetValue(cfg, in, fpl.UnlimitedTransfers, 3, 5)
	if len(got) != 2 || got[0].Transfers != 3 {
		t.Fatalf("pre-deadline order disturbed: %+v", got)
	}
}

// The pool asked of BuildPlans must exceed what is printed, or the truncation it
// performs on GROSS gain evicts the better-net plan before anything prices it.
// That is the mechanism behind the defect above: at a two-move limit every plan in
// a five-deep pool can be a two-move plan, so the free single never survives to be
// priced.
func TestTheCandidatePoolExceedsWhatIsPrinted(t *testing.T) {
	const printed = 5
	if planCandidatePool <= printed {
		t.Fatalf("planCandidatePool is %d against %d printed: BuildPlans truncates on "+
			"gross gain, so a better-net plan can be evicted before ranking",
			planCandidatePool, printed)
	}
}

// A plan whose hit clears the gate is still charged for it, so the reported order
// reflects the -4 rather than merely surviving it.
func TestAClearingHitIsStillCharged(t *testing.T) {
	cfg := config.Default()
	cfg.Review.MinGainForHit = 0
	big := analysis.Plan{Transfers: 2, GainPerGW: 1.30}  // 6.50 - 4 - 1 = 1.50
	solo := analysis.Plan{Transfers: 1, GainPerGW: 0.60} // 3.00 - 1     = 2.00
	got := rankPlansByNetValue(cfg, []analysis.Plan{big, solo}, 1, 3, 5)
	if got[0].Transfers != 1 {
		t.Fatalf("the hit outranked a free move worth more once the -4 was paid: "+
			"%.2f vs %.2f net", math.Round(1.50*100)/100, 2.00)
	}
}

// equivalentTo used to assume its input was gain-ordered, which BuildPlans
// guaranteed. rankPlansByNetValue breaks that: a plan taking a -4 can sit above a
// higher-gain free one. Under the old prefix scan the run stopped at the first
// plan outside the band and silently dropped the equivalents behind it.
func TestEquivalentToDoesNotAssumeGainOrder(t *testing.T) {
	plans := []analysis.Plan{
		{Transfers: 1, GainPerGW: 0.60}, // ranked first on NET value
		{Transfers: 2, GainPerGW: 0.90}, // higher gain, sits behind it
		{Transfers: 2, GainPerGW: 0.88}, // within band of the 0.90
	}
	got := equivalentTo(plans, 0.05)
	if len(got) != 2 {
		t.Fatalf("kept %d plans; the two within 0.05 of the highest gain (0.90) are "+
			"equivalent regardless of where net-value ranking placed them", len(got))
	}
}
