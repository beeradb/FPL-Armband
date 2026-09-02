package backtest

import "armband/internal/fpl"

// A manager's money, which is not the same as his squad's market value.
//
// FPL shares price rises: sell a player and you get what you paid plus half of
// any rise since, rounded down to the nearest £0.1m. Falls are taken in full.
// So a squad whose prices have gone up is worth less than its headline value,
// and the gap widens all season.
//
// The replay priced every sale at market, which quietly hands the policy money
// no manager would have. That flatters exactly the decisions that matter — the
// expensive upgrades a real budget could not reach.
type wallet struct {
	bank   int         // tenths of a million
	bought map[int]int // element id -> what was paid, in tenths
}

func newWallet(bank int) *wallet {
	return &wallet{bank: bank, bought: map[int]int{}}
}

// clone is the wallet as it stands right now, detached from every later
// transaction.
//
// It exists for one caller: the frozen-squad observer, which prices a fifteen
// nobody ever sold against a bank nobody ever spent, alongside a policy that is
// doing both to the wallet this was copied from. Sharing the live wallet would
// have the frozen arm's budget move with the evolving arm's transfers, which is
// the confound the frozen arm exists to remove.
//
// The map is copied rather than aliased, because `buy` and `sellAt` write to it.
func (w *wallet) clone() *wallet {
	c := &wallet{bank: w.bank, bought: make(map[int]int, len(w.bought))}
	for id, paid := range w.bought {
		c.bought[id] = paid
	}
	return c
}

// buy records a purchase at the market price and takes it out of the bank.
func (w *wallet) buy(id, market int) {
	w.bought[id] = market
	w.bank -= market
}

// sell returns what the player raises and puts it in the bank, at a *market*
// price — so the half-of-any-rise rule is applied here, on the way out.
func (w *wallet) sell(id, market int) int {
	return w.sellAt(id, w.sellPrice(id, market))
}

// sellAt banks a sale at an amount that is **already final** and does the rest of
// the bookkeeping.
//
// It exists because a caller can legitimately hold a selling price rather than a
// market price, and handing one to sell would apply FPL's half-of-any-rise rule a
// second time — silently under-crediting every sale by half of whatever profit
// the first application had already halved away. The price oracle is exactly that
// caller: bestSellPrice is a maximum over a window of w.sellPrice, so its result
// is a selling price and passing it to sell would double-charge the rule.
//
// The two are one expression of the bookkeeping rather than two, so a change to
// what a sale does to the wallet cannot reach one path and miss the other.
func (w *wallet) sellAt(id, got int) int {
	w.bank += got
	delete(w.bought, id)
	return got
}

// sellPrice is what a player raises without moving any money.
func (w *wallet) sellPrice(id, market int) int {
	paid, ok := w.bought[id]
	if !ok {
		return market // untracked: no profit to share
	}
	// fpl.SellPrice is the one implementation of FPL's rounding rule -- see its own
	// doc comment. This used to restate "average of paid and market, rounded down"
	// here as well as in internal/fpl, which is exactly the kind of copy this
	// project's own standing rule warns drifts silently.
	return fpl.SellPrice(paid, market)
}

// sellPrices is the map SquadState wants, for the players held right now.
func (w *wallet) sellPrices(s *Season, held []int, gw int) map[int]int {
	out := make(map[int]int, len(held))
	for _, id := range held {
		if s.Players[id] != nil {
			out[id] = w.sellPrice(id, marketPrice(s, id, gw))
		}
	}
	return out
}

// value is the squad's selling value plus the bank — the money genuinely
// available, and what FPL shows as team value.
func (w *wallet) value(s *Season, held []int, gw int) int {
	total := w.bank
	for _, id := range held {
		if p := s.Players[id]; p != nil {
			total += w.sellPrice(id, marketPrice(s, id, gw))
		}
	}
	return total
}
