package backtest

import (
	"os"
	"sort"
)

// Who was in the game, and what he cost.
//
// These two questions are one question, and this file is the only place either is
// answered. Before it there were five implementations of "what did this player
// cost" — `openingPrice`, `priceAt`, and three inline copies in `PreSeason`,
// `Score` and `RandomSquads` — every one of them ending in the same fallback to
// `Player.NowCost`, which is the *closing* price of the season. That fallback is
// how a replay came to buy players who were not in the game yet.
//
// # The leak
//
// `players_raw.csv` lists everyone who was registered at any point in the season;
// `gws/merged_gw.csv` lists, per gameweek, everyone registered *at that gameweek*.
// A January signing therefore appears in the first file with a full record and in
// the second only from January. `PreSeason` and `PointInTime` iterated the first,
// found no row at the cutoff, and fell back to the closing price — so the player
// went into the GW1 pool, priced at what he was worth in May.
//
// Measured on 2025-26: 151 of 841 players (18%) have no GW1 row and every one of
// them sat in the GW1 pool. Five were priced below FPL's £4.0m minimum, which is
// not a price that existed. The replayed opening squad contained four of them, two
// of whom went on to return 168 points — 12% of that squad's total. It is worse in
// the other three seasons by count: 205, 207 and 188 unregistered players.
//
// The leak paid twice. It granted **free budget**, because a player who joined
// later and drifted down is offered below the £4.0m floor — Mané entered the game
// at £4.5m in GW11 and the replay bought him at £4.2m in GW1. And it granted **free
// hindsight**, because the model reads his *prior* season by code and so rates a
// footballer nobody could pick.
//
// # Why excluding him is safe
//
// The worry is the opposite error: dropping somebody who genuinely was buyable,
// because `merged_gw` only lists players who were in a matchday squad. It does
// not. 57% of 2025-26's 690 GW1 rows carry **zero minutes**, and 690 rows across
// 20 clubs is 34.5 players a club — far more than a matchday squad. The file holds
// the whole *registered* squad, so a missing row means not registered, and no
// pickable player is lost.
//
// # The fallback that would have hidden all of this
//
// `now_cost - cost_change_start` reproduces the GW1 price exactly — 690 of 690 on
// 2025-26 — and is the obvious replacement for the `NowCost` fallback. **Do not use
// it.** For an unregistered player it returns his price on the day *he* entered the
// game: at or above the £4.0m floor for all 151 of them, so it passes the very
// check that caught the bug, and Mané comes back a plausible £4.5m rather than an
// impossible £4.2m. It would have repaired every symptom, made every squad look
// legal, and left the replay picking players who did not exist. It belongs in an
// assertion, which is where TestDiagUnregisteredPool puts it, and never in a
// fallback.

// leakRestored reports whether FPL_UNREGISTERED_POOL is set, which puts the
// unregistered players back in the pool at their closing price.
//
// It exists so the two arms can be compared, which is the only way to say what the
// leak was worth — every figure in AGENTS.md predates the fix, and "the corrected
// squad scored more" is not interpretable from one cell of a non-deterministic
// replay. Read on every call rather than cached at process start, on the
// FPL_NO_AVAILABILITY precedent: a cached copy cannot be toggled between cells of
// one sweep, so the arms cannot be paired and the whole measurement is lost.
func leakRestored() bool { return os.Getenv("FPL_UNREGISTERED_POOL") != "" }

// registration is which players the archive shows in the game at a cutoff, and
// what to charge for them.
//
// Both halves live on one type because the second is only answerable given the
// first. A player kept by the blind-club guard has no row to price him from, and
// the honest zero `priceAt` returns would put him in the pool **free** — a worse
// bug than the leak, since the optimiser would take fifteen of him.
type registration struct {
	in map[int]bool
	// blindClub are clubs the archive shows no rows for at all by the cutoff, and
	// about whose players it therefore says nothing. Their players are kept.
	blindClub map[int]bool
	blind     []int
}

func (r registration) has(id int) bool { return r.in[id] }

// price is what the pool charges for a player, and the only place the closing
// price is ever reachable.
//
// Three cases, in the order they are asked. A row at or before the cutoff is the
// real answer. The escape hatch reproduces the old closing-price fallback exactly,
// so the two arms differ in the leak and nothing else. And a player kept by the
// blind-club guard is priced from his *earliest* row — a forward walk, the mirror
// of priceAt's backward one — because his club simply had not played yet, and the
// price a week later is a far better estimate of what he cost than either zero or
// what he was worth in May.
func (r registration) price(p *Player, through int) int {
	if v := priceAt(p, through); v > 0 {
		return v
	}
	if leakRestored() {
		return p.NowCost
	}
	if r.blindClub[p.Team] {
		for gw := 1; gw <= 38; gw++ {
			if g, ok := p.GWs[gw]; ok && g.Value > 0 {
				return g.Value
			}
		}
	}
	return 0
}

// registeredBy reports which element ids the archive shows in the game by
// gameweek `through`. Pre-season (`through` of 0) asks about gameweek 1.
//
// Reading gameweek 1 to answer a pre-season question is not hindsight. What is
// read is that a row *exists* and the price on it, and FPL publishes both — the
// player list and the opening prices — weeks before the season starts. Nothing
// about how he performed is touched.
//
// # The blind-club guard
//
// A club with no rows at all by the cutoff has told us nothing, and excluding its
// whole squad — five defenders and a keeper — would be a far larger distortion than
// the leak. In practice that means a club blanking the opening gameweek, since any
// later cutoff has earlier gameweeks to fall back on.
//
// It does not arise anywhere in the shipped grid: all 20 clubs have GW1 rows in all
// four seasons, and TestDiagUnregisteredPool fails if that ever stops being true,
// because a firing guard silently re-opens the leak for that club. It is a safety
// net for a future season rather than live behaviour — but whole-gameweek absences
// are real in this archive, 2022-23 GW7 having been cancelled outright after the
// Queen's death with no rows for anybody.
func registeredBy(s *Season, through int) registration {
	cutoff := through
	if cutoff < 1 {
		cutoff = 1
	}
	r := registration{in: map[int]bool{}, blindClub: map[int]bool{}}
	if leakRestored() {
		for _, p := range s.Players {
			r.in[p.ID] = true
		}
		return r
	}
	clubs, seen := map[int]bool{}, map[int]bool{}
	for _, p := range s.Players {
		clubs[p.Team] = true
		for gw := 1; gw <= cutoff; gw++ {
			if _, ok := p.GWs[gw]; ok {
				r.in[p.ID] = true
				seen[p.Team] = true
				break
			}
		}
	}
	for t := range clubs {
		if !seen[t] {
			r.blindClub[t] = true
			r.blind = append(r.blind, t)
		}
	}
	sort.Ints(r.blind)
	for _, p := range s.Players {
		if r.blindClub[p.Team] {
			r.in[p.ID] = true
		}
	}
	return r
}

// priceAt is what a player cost at gameweek `through`, and the only implementation
// of that quantity. Pre-season (`through` of 0) means the opening price.
//
// **Zero means the archive cannot price him**, which for a player with no row at or
// before the cutoff is the honest answer: he was not in the game, so he had no
// price. It deliberately does not fall back to `Player.NowCost` — that is the
// closing price, and returning it is the leak this file exists to close. Every
// gameweek row in all four archived seasons carries a price (verified: 0 unpriced
// rows in 110,268), so for a registered player the walk always succeeds and a zero
// return is a clean signal rather than a data quirk.
func priceAt(p *Player, through int) int {
	if through < 1 {
		through = 1
	}
	for gw := through; gw >= 1; gw-- {
		if g, ok := p.GWs[gw]; ok && g.Value > 0 {
			return g.Value
		}
	}
	return 0
}

// marketPrice is what a player costs to buy, by element id, or zero if the id is
// not in the season or the archive cannot price him then.
//
// Shared rather than re-derived at each call site. The same three-line lookup —
// find the player, price him at a gameweek, cope with a missing id — was written
// out in decide, in squadValue and in the wallet, and one of those forgot the nil
// guard and would have panicked on an id the season does not carry. That is the
// same argument that put RankSwaps and RankPairs behind one implementation: the
// duplication is cheap, and the divergence is not.
func marketPrice(s *Season, id, gw int) int {
	if p := s.Players[id]; p != nil {
		return priceAt(p, gw)
	}
	return 0
}
