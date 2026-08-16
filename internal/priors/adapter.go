package priors

import "armband/internal/analysis"

// Adapter presents a loaded season through the narrow interface
// internal/analysis needs, so the scoring package stays free of fetching,
// caching and CSV parsing.
type Adapter struct{ S *Season }

// Get returns last season's totals for a player, by FPL's permanent code.
func (a Adapter) Get(code int) (*analysis.PriorPlayer, bool) {
	p, ok := a.S.Get(code)
	if !ok {
		return nil, false
	}
	q := priorFrom(p)
	return &q, true
}

// priorFrom projects one player of this source — the CSV/JSON season mirror —
// into the shape internal/analysis blends.
//
// # Why it is a function and not two composite literals
//
// This source had two of them: here, and inside [LoadBlended], which built the
// same ten fields off the same [Player] to feed [analysis.BlendPriors]. They
// agreed, and that is the whole problem — a copy is correct on the day it is
// written and diverges later, silently, because nothing compares them. The
// archive side of the same quantity has already paid for this: all four of its
// constructions omitted `DefCon` and three of the four omitted the capability
// flags as well, and `cmd/priorblend` carried two more that omitted `DefCon` too —
// so an experiment's ordering statistics were computed against a prior differing
// from the live path's by a whole statistic, for reasons nobody had chosen.
//
// One projection per SOURCE is the rule, not one in the whole repository: the
// archive (`backtest.PriorFrom`), FPL's `history_past` (`internal/recent`) and
// this mirror read three different structs with three different field spellings —
// `YellowCards` here against `Yellow` there — so a single shared function would
// need a converter per source anyway and would buy nothing.
func priorFrom(p *Player) analysis.PriorPlayer {
	return analysis.PriorPlayer{
		Minutes: p.Minutes, Starts: p.Starts,
		XG: p.XG, XA: p.XA, XGC: p.XGC, DefCon: p.DefCon,
		Bonus: p.Bonus, Saves: p.Saves,
		Yellow: p.YellowCards, Red: p.RedCards,
	}
}
