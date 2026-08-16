package backtest

import "testing"

// FPL's selling rule, pinned. Halving a rise is the part that costs money, and
// it is easy to implement as "average of paid and market", which is the same
// thing only when the difference is even.
func TestWalletSharesRisesAndEatsFalls(t *testing.T) {
	w := newWallet(0)
	w.bought = map[int]int{1: 50, 2: 50, 3: 50, 4: 50}

	for _, tc := range []struct {
		id, market, want int
		why              string
	}{
		{1, 56, 53, "a 0.6 rise is halved to 0.3"},
		{2, 55, 52, "a 0.5 rise halves to 0.25 and rounds down to 0.2"},
		{3, 44, 44, "a fall is taken in full"},
		{4, 50, 50, "no move, no change"},
	} {
		if got := w.sellPrice(tc.id, tc.market); got != tc.want {
			t.Errorf("paid 50, market %d: sells for %d, want %d — %s",
				tc.market, got, tc.want, tc.why)
		}
	}

	// Untracked players sell at market, which is the pre-season case.
	if got := w.sellPrice(99, 61); got != 61 {
		t.Errorf("untracked player sells for %d, want market 61", got)
	}
}

// Buying and selling must conserve money exactly, or the replay's budget drifts
// over a season and every late-season decision is made with a fictional bank.
func TestWalletConservesMoney(t *testing.T) {
	w := newWallet(100)
	w.buy(1, 60)
	if w.bank != 40 {
		t.Fatalf("bank %d after a 6.0 purchase from 10.0, want 40", w.bank)
	}
	// He rises to 7.0; selling returns 6.5, not 7.0.
	if got := w.sell(1, 70); got != 65 {
		t.Errorf("sold for %d, want 65", got)
	}
	if w.bank != 105 {
		t.Errorf("bank %d, want 105 — the half-rise is the only profit", w.bank)
	}
	if _, still := w.bought[1]; still {
		t.Error("sold player still tracked as owned")
	}
}

// TestSellingPriceIsNotHalvedTwice pins the distinction between the two entry
// points, which is the whole reason sellAt exists.
//
// The trap is that bestSellPrice already returns a *selling* price — it is a
// maximum over a window of w.sellPrice, so the half-of-any-rise rule has been
// applied inside it. Handing that to sell applies it again, and the arithmetic is
// quiet about it: the number is still plausible, still below market, still
// monotone in the rise. Nothing errors and every sale is short.
func TestSellingPriceIsNotHalvedTwice(t *testing.T) {
	w := newWallet(0)
	w.buy(1, 60)
	w.bank = 0

	final := w.sellPrice(1, 80) // paid 6.0, market 8.0: 6.0 + 1.0 = 7.0
	if final != 70 {
		t.Fatalf("selling price %d, want 70", final)
	}
	// The wrong call, stated so the test documents the failure rather than only
	// the fix: sell would treat 7.0 as a market price and halve the rise again.
	if again := w.sellPrice(1, final); again != 65 {
		t.Fatalf("re-applying the rule gives %d, want 65 — if this ever equals "+
			"the selling price the trap has gone away and so may this guard", again)
	}
	if got := w.sellAt(1, final); got != 70 || w.bank != 70 {
		t.Errorf("sellAt credited %d and banked %d, want 70 and 70 — a caller "+
			"holding a final amount must be able to bank it without the rule "+
			"being charged a second time", got, w.bank)
	}
	if _, still := w.bought[1]; still {
		t.Error("sellAt left the player tracked as owned, so his purchase price " +
			"would survive into a squad that no longer holds him")
	}

	// And sell is defined in terms of sellAt rather than beside it, so the
	// bookkeeping cannot reach one path and miss the other.
	v := newWallet(0)
	v.buy(2, 60)
	v.bank = 0
	if got := v.sell(2, 80); got != 70 || v.bank != 70 {
		t.Errorf("sell credited %d and banked %d, want 70 and 70", got, v.bank)
	}
}

// TestASaleSettlesAtThePriceTheSearchWasQuoted drives the seam the price oracle
// used to leak through.
//
// The defect was invisible on the baseline and only on the baseline: un-oracled,
// the quote handed to the transfer search *is* w.sellPrice of the market price
// settle would recompute, so both forms bank the same number and no recorded
// figure could distinguish them. With the oracle on, the search is quoted the
// window maximum and settle recomputed from the window minimum — the arm bought
// low and sold low, so the money the gate was promised never arrived and every
// price-timing figure came from an arm that was not an upper bound on anything.
func TestASaleSettlesAtThePriceTheSearchWasQuoted(t *testing.T) {
	// Paid 6.0. Around the decision his market price ranged 5.5 to 8.0, so
	// bestBuyPrice is 55 and bestSellPrice is 60 + (80-60)/2 = 70.
	const paid, windowLow, quoted = 60, 55, 70

	w := newWallet(0)
	w.buy(1, paid)
	w.bank = 0
	if got := settleSale(w, map[int]int{1: quoted}, 1, windowLow); got != quoted {
		t.Errorf("the sale raised %d against a quote of %d — the oracled arm is "+
			"promising the gate money the wallet does not receive, which makes its "+
			"figure a mixture rather than an upper bound", got, quoted)
	}
	if w.bank != quoted {
		t.Errorf("bank %d, want %d", w.bank, quoted)
	}
	if _, still := w.bought[1]; still {
		t.Error("the sold player is still tracked as owned")
	}

	// The old arithmetic, stated rather than only avoided: it is quiet, plausible
	// and 20% short. If this ever equals the quote the defect has gone away on its
	// own and this guard is measuring nothing.
	v := newWallet(0)
	v.buy(1, paid)
	v.bank = 0
	if wrong := v.sell(1, windowLow); wrong >= quoted {
		t.Fatalf("settling through the window minimum raises %d, which is no worse "+
			"than the quote %d", wrong, quoted)
	}

	// The fallback is the un-oracled behaviour exactly, for the one case that has
	// no quote: a player bought earlier in the same week, after `sell` was built.
	u := newWallet(0)
	u.buy(2, paid)
	u.bank = 0
	if got := settleSale(u, map[int]int{}, 2, 80); got != 70 || u.bank != 70 {
		t.Errorf("with no quote the sale raised %d and banked %d, want 70 and 70 — "+
			"the honest computation, since settling at the map's zero would be a "+
			"silent change to the path every recorded figure was measured on",
			got, u.bank)
	}
}
