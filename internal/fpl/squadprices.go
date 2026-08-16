package fpl

import (
	"context"
	"fmt"
)

// Selling prices without a login.
//
// FPL pays what you paid plus half of any rise, so the model needs each owned
// player's purchase price. my-team reports it directly but is private. All of it
// can be reconstructed from public endpoints instead:
//
//   - entry/{id}/event/{gw}/picks/ lists who was owned that week, and its
//     entry_history carries bank and value;
//   - walking those weeks in order shows when each current player first
//     appeared, which is when he was bought;
//   - element-summary/{id}/ carries that player's price in every gameweek, so
//     his price on arrival is a lookup.
//
// # The reported team value is a checksum, and that is the point
//
// FPL's entry_history.value IS the squad's selling value — it is what the site
// calls team value. So the reconstruction can be checked against the real
// answer: if the per-player selling prices sum to it, they are right.
//
// That is a stronger guarantee than a session provides. my-team hands over
// numbers that have to be trusted; this produces numbers that can be verified.
type SquadPrices struct {
	// Sell is element id -> selling price in tenths.
	Sell map[int]int
	// Bank is money not in the squad, in tenths.
	Bank int
	// Value is FPL's own team value: the squad's selling value, in tenths.
	Value int
	// Reconstructed is what the per-player selling prices actually sum to.
	Reconstructed int
	// Held is the fifteen owned at the gameweek asked about.
	Held []int
}

// Exact reports whether the reconstruction reproduces FPL's team value. When it
// does not, the per-player numbers are approximations and must not be presented
// as fact — the usual cause is a player bought mid-window at a price that moved
// the same night.
func (s *SquadPrices) Exact() bool { return s.Reconstructed == s.Value }

// Drift is how far off the reconstruction is, in tenths. Signed: positive means
// the model believes the squad raises more than it does.
func (s *SquadPrices) Drift() int { return s.Reconstructed - s.Value }

// SquadPrices reconstructs selling prices for an entry's current squad from
// public data alone.
//
// through is the last completed gameweek. Picks for a completed gameweek never
// change, so every request but the most recent is permanently cacheable.
func (c *Client) SquadPrices(ctx context.Context, entryID, through int) (*SquadPrices, error) {
	if entryID <= 0 {
		return nil, fmt.Errorf("no entry id")
	}
	if through < 1 {
		return nil, fmt.Errorf("no gameweeks played yet, so nothing has been bought")
	}

	latest, err := c.Picks(ctx, entryID, through)
	if err != nil {
		return nil, fmt.Errorf("picks for GW%d: %w", through, err)
	}
	out := &SquadPrices{
		Sell:  map[int]int{},
		Bank:  latest.EntryHistory.Bank,
		Value: latest.EntryHistory.Value,
	}
	owned := map[int]bool{}
	for _, p := range latest.Picks {
		owned[p.Element] = true
		out.Held = append(out.Held, p.Element)
	}

	// The earliest gameweek each current player was owned. Walking forwards and
	// keeping the first hit gives the gameweek he arrived — with the caveat that
	// a player sold and later re-bought reads as bought the first time, which
	// the checksum will catch.
	arrived := map[int]int{}
	for gw := 1; gw <= through; gw++ {
		pk, err := c.Picks(ctx, entryID, gw)
		if err != nil {
			continue // a missed week costs precision, not correctness: checksum catches it
		}
		for _, p := range pk.Picks {
			if owned[p.Element] {
				if _, seen := arrived[p.Element]; !seen {
					arrived[p.Element] = gw
				}
			}
		}
	}

	boot, err := c.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	now := map[int]int{}
	for i := range boot.Elements {
		now[boot.Elements[i].ID] = boot.Elements[i].NowCost
	}

	for _, id := range out.Held {
		market := now[id]
		paid := market
		if gw, ok := arrived[id]; ok {
			if v, err := c.priceAtGW(ctx, id, gw); err == nil && v > 0 {
				paid = v
			}
		}
		sell := market
		if market > paid {
			sell = paid + (market-paid)/2 // FPL keeps half the rise, rounded down
		}
		out.Sell[id] = sell
		out.Reconstructed += sell
	}
	return out, nil
}

// priceAtGW is a player's price in a given gameweek, from his own history.
func (c *Client) priceAtGW(ctx context.Context, element, gw int) (int, error) {
	sum, err := c.ElementSummary(ctx, element)
	if err != nil {
		return 0, err
	}
	best := 0
	for _, h := range sum.History {
		if h.Round <= gw && h.Value > 0 {
			best = h.Value // last entry at or before the gameweek
		}
	}
	if best == 0 {
		return 0, fmt.Errorf("no price history for element %d at GW%d", element, gw)
	}
	return best, nil
}
