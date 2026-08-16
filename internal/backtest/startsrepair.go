package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Recorded starting elevens, for the seasons whose archive lost them.
//
// # What this replaces, and why replacing it is worth doing
//
// `merged_gw.csv` carries `starts` as zero for all of 2021-22 and for 2022-23 through
// GW15, and does not carry the column at all before 2022-23. `reconstructStarts` fills
// that by rank — within a club-gameweek the eleven with the most minutes started —
// which is exact in *count* by construction and misclassifies **2.36% of starter
// slots**.
//
// Its residual is not noise. A nailed starter withdrawn at half time and a fringe
// player eased back on at half time both record 45 minutes, they tie, and the tie-break
// promotes the wrong one: in the 0.85+ start-share band **24.5% of players lose starts
// against 7.7% gaining them**, a 3:1 asymmetry that flatters exactly the player whose
// start share is least certain. That is boundary three in `reconstructstarts.go`, and
// it is why that file forbids using a reconstructed row as evidence about a rotation or
// returning player.
//
// Recorded starts have no tie to break. They also settle the case the rank rule
// deliberately declines — a double gameweek, where a gameweek total cannot say whether
// one start was made or two, and `reconstructStarts` leaves the recorded zero alone.
// The harvest is per match, so two starts in one gameweek are counted directly.
//
// # Where the data comes from, and why it was not already here
//
// Understat's `getPlayerData` returns a `matches` array whose rows carry `position`,
// with **`Sub` as an explicit value**; a start is exactly `position != "Sub"`.
//
// This project already downloaded that field and threw it away.
// `stats/understat_xg_backfill.py` has fetched the same endpoint per player since the
// expected-goals backfill, caches the entire response, and reads `time` out of the very
// rows that carry `position` — but its output schema is
// `element,GW,expected_goals,expected_assists`, so the discard happened at the write.
// The 2026-08-12 archive audit did not catch it because that audit was a
// column-by-column pass over the archive's own files, which can only ever answer
// "reconstruct it"; it never asked whether the truth was recoverable. Second instance,
// after expected goals conceded. See `stats/understat_starts_backfill.py`.
//
// # The order this runs in is load-bearing, and it is PRECEDENCE rather than sequence
//
// ⚠️ **This paragraph said "it must run **before** `reconstructStarts`" until 2026-08-15.**
// That was true when it was written and stopped being true the same day: `c287862` moved
// the repair out of `fetch` into `repaired`, because a repair applied before the cache
// write is baked into the cache and `FPL_NO_STARTS_REPAIR` then reads a repaired cache
// and reports both arms identical. So the reconstruction runs **first**
// (`season.go`, end of `fetch`) and this overwrites it (`season.go`, in `repaired`) —
// `Replaced` counts exactly that, and `Conflict` counts the harvest declining to
// overwrite a start the archive already recorded.
//
// The invariant is unchanged, which is why the error survived: **recorded beats
// inferred**, so the reconstruction ends up filling only what the harvest could not
// reach either way. A repaired row keeps `StartsReconstructed = false`, because
// it is recorded rather than inferred; a row the harvest misses keeps the
// reconstruction and its flag, so an honest gap stays visible to any consumer that
// checks. Coverage is never uniform — Understat carries only players in its match
// rosters, and fringe players with few minutes are both the likeliest gap and the
// population the rank rule gets wrong — so the two must stay distinguishable.
func noStartsRepair() bool { return os.Getenv("FPL_NO_STARTS_REPAIR") != "" }

// StartsRepairResult reports what the repair reached, so an absent repair and an
// applied one never look the same downstream.
type StartsRepairResult struct {
	Rows     int // rows read from the harvest
	Applied  int // player-gameweeks whose starts came from the harvest
	Replaced int // of those, ones that had been inferred by reconstructStarts
	Starts   int // starts written
	Conflict int // rows the ARCHIVE recorded a start for, refused and left alone
}

// applyStartsRepair fills recorded starts from the harvest, returning what it did.
//
// A missing repair file is not an error. The harvest is a separate, network-bound step
// and the replay must still run without it — that is the same contract the expected
// -goals repair keeps.
func applyStartsRepair(s *Season) (StartsRepairResult, error) {
	var res StartsRepairResult
	if noStartsRepair() {
		return res, nil
	}

	f, err := repairData.Open("repairdata/" + s.Name + "-starts.csv")
	if err != nil {
		return res, nil
	}
	defer f.Close()

	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		return res, fmt.Errorf("starts repair %s: reading header: %w", s.Name, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, want := range []string{"element", "GW", "starts"} {
		if _, ok := col[want]; !ok {
			return res, fmt.Errorf("starts repair %s: no %q column", s.Name, want)
		}
	}

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, fmt.Errorf("starts repair %s: %w", s.Name, err)
		}
		res.Rows++

		el, err1 := strconv.Atoi(strings.TrimSpace(rec[col["element"]]))
		gw, err2 := strconv.Atoi(strings.TrimSpace(rec[col["GW"]]))
		st, err3 := strconv.Atoi(strings.TrimSpace(rec[col["starts"]]))
		if err1 != nil || err2 != nil || err3 != nil {
			return res, fmt.Errorf("starts repair %s: unparseable row %v", s.Name, rec)
		}

		p := s.Players[el]
		if p == nil {
			continue
		}
		g, ok := p.GWs[gw]
		if !ok {
			// The harvest found a match the archive has no row for. That is a real
			// disagreement rather than something to paper over — a gameweek the
			// archive is missing entirely, like 2022-23's GW7 — so it is skipped
			// and left to the reconstruction, which is equally blind to it.
			continue
		}
		// Overwrite an INFERRED start; never overwrite a recorded one.
		//
		// The distinction is what makes this safe to run after the cache. By then
		// `reconstructStarts` has already filled every club-gameweek the archive
		// left empty, so `Starts > 0` no longer means "the archive said so" — it
		// usually means "the rank rule guessed". Replacing that guess with the
		// recorded value is the entire point. A row the archive recorded itself has
		// `StartsReconstructed` false and is refused, which should never fire
		// because the harvest windows exclude those gameweeks; it is counted rather
		// than assumed, because "the window is right" is exactly the kind of claim
		// this project has been bitten by, and `Season.StartsRepair` now carries the
		// count somewhere a caller can read it.
		if g.Starts > 0 && !g.StartsReconstructed {
			res.Conflict++
			continue
		}
		if g.StartsReconstructed {
			res.Replaced++
		}
		g.Starts = st
		g.StartsReconstructed = false
		p.GWs[gw] = g
		res.Applied++
		res.Starts += st
	}
	return res, nil
}
