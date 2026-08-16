package backtest

// Real per-gameweek team news, as an information oracle.
//
// # The question this exists to answer
//
// `statusAt` reconstructs availability from an **end-of-season snapshot**: the
// archive carries each player's final `status` and the date its news was posted, so
// the only thing that can honestly be carried backwards is a flag the player was
// still carrying in May. It therefore emits `a` and `u`/`i` and never `d`, and it is
// blind by construction to every injury that resolved — a player out from September
// to November finishes the season fit and reads as fit throughout.
//
// `OracleAvailability` is narrower still: it fires on a **season total** of zero
// minutes, so it sees only players who never appeared at all. The record calls its ≈14
// points a season a **design average** off a degenerate population — inert in 13 of 24
// held cells, nearer ≈32 conditional on firing — rather than an answer. Not a "floor":
// that word is a term of art elsewhere in the record, and ≈14 is a dilution of a larger
// conditional effect rather than a lower bound on the capability.
//
// The captures under `data/captures/<season>/GW*/` are FPL's own `bootstrap-static`
// payloads, crawled before each deadline and verified point-in-time honest. They
// carry the thing the reconstruction cannot: `status` as it stood **at that
// deadline**, including the doubtful flags and the absences that later resolved, plus
// `chance_of_playing_next_round`, a published percentage the replay has never once
// seen.
//
// # Two oracles, because they answer two questions
//
// `OracleTeamNews` replaces the reconstruction with the real flag. That is a *data*
// change on the scale of the expected-goals backfill, and it is the one that bounds
// "what is knowing the team news worth at all".
//
// `OracleTeamNewsChance` additionally hands `availabilityFactor` the percentage.
// The percentage **overrides** the flag inside that function
// (`metrics.go:availabilityFactor`), so the second arm is strictly the first plus
// granularity: a doubtful player priced at 0.5 by the flag is priced at his published
// 0.25 / 0.50 / 0.75 instead. Run as two arms against a common un-oracled baseline,
// the difference between them is what the percentage is worth on its own. Merged into
// one arm the figure would bound neither, which is why `Validate` refuses the
// percentage without the flag underneath it.
//
// # Why the source is injected rather than imported
//
// `internal/backfill`'s `TestTheScoringPathCannotSeeRecoveredTeamNews` forbids
// `internal/backtest` importing `internal/capture`, and it is right to: every figure
// in the research record was measured against the reconstruction, and improving the
// input underneath them silently would make half the record incomparable with the
// other half.
//
// So the replay declares what it needs — `TeamNews`, below — and something outside
// this package supplies it. The interface deals only in strings, ints and a
// `*int`, so an implementation needs no import of this package either; see
// `cmd/teamnewsexport`, which is the producer, and `newsFile`, which is the reader
// the sweep wires. That is the same shape as `analysis.RecentForm` and
// `Engine.Recent`: the consumer declares the seam and the fetcher stays out of the
// scoring path.

import (
	"armband/internal/fpl"
)

// TeamNews is what FPL was publishing about availability at one deadline.
//
// # The contract, and the one part of it that is a judgement call
//
// `Covers` is authoritative and `FlagAt` is a lookup within it. A covered gameweek
// means the whole payload was recovered, so a player the source has nothing to say
// about is a player FPL was **not** flagging — available, no percentage — and not a
// player about whom nothing is known. That is what makes this a *replacement* for
// `statusAt` rather than a patch on top of it: with a covered gameweek the
// reconstruction is not consulted at all, in either direction.
//
// The alternative reading — "unflagged means fall back to the reconstruction" —
// would mix two availability models inside one arm and give the oracle a
// silent second opinion in exactly the cases the reconstruction gets wrong. An
// uncovered gameweek falls back to the reconstruction wholesale, which is the honest
// treatment of a genuine gap: `capture`'s own store refuses to invent one, and
// coverage is genuinely patchy in the earliest backfilled seasons.
//
// Implementations must be **comparable** — a pointer, in practice. `runPolicySweep`
// compares two `Oracles` values with `!=` to check that an arm's hindsight does not
// vary by cell, and a non-comparable dynamic type would panic there rather than fail.
type TeamNews interface {
	// Covers reports whether the payload for this gameweek's deadline was
	// recovered at all, after any staleness filter the source applies.
	Covers(season string, gw int) bool

	// FlagAt returns FPL's one-letter availability flag and its published chance
	// of playing next round for one player, keyed on **permanent player code**.
	//
	// Element ids are reassigned every summer, so an availability record keyed on
	// one comes back next season attached to a different footballer. This project
	// has already paid for that once, in the standing overrides.
	//
	// `ok` is false when the gameweek is not covered. Within a covered gameweek an
	// unflagged player returns ("a", nil, true) — see the type comment.
	FlagAt(season string, gw int, code int) (status string, chance *int, ok bool)
}

// applyTeamNews overwrites what the model believes about one player's availability
// with what FPL was actually publishing.
//
// `gw` is the gameweek whose deadline the model is standing at — `through+1` for an
// in-season view and 1 for the pre-season one — because that is the deadline a squad
// is being chosen for and the deadline `chance_of_playing_next_round` speaks about.
//
// It is a no-op with the oracle off, with no source attached, or on a gameweek the
// source does not cover. The middle case is refused by `Validate` before a
// simulation runs, so it cannot reach a measurement; it is tolerated here because
// `PointInTimeWith` is called from diagnostics that do not validate.
func applyTeamNews(el *fpl.Element, season string, gw int, o Oracles) {
	if !o.Has(OracleTeamNews) || o.News == nil {
		return
	}
	if !o.News.Covers(season, gw) {
		return
	}
	// No permanent key, nothing safe to join on. `capture.Store` refuses to index
	// such a player for the same reason, and the consumer has to agree: a lookup on
	// code 0 finds no flag and would therefore mark him **available**, which is a
	// decision made from the absence of an identifier.
	if el.Code == 0 {
		return
	}
	status, chance, ok := o.News.FlagAt(season, gw, el.Code)
	if !ok {
		return
	}
	el.Status = status
	// The percentage is the second arm and only the second arm. Without this gate
	// the two questions collapse into one figure that bounds neither.
	if o.Has(OracleTeamNewsChance) {
		el.ChanceOfPlayingNextRound = chance
	}
}
