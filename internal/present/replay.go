package present

import (
	"fmt"
	"io"
	"sort"
)

// ReplayWeek is one gameweek of a finished season, as it was actually played.
//
// # Why this is not analysis.WeekView
//
// `WeekView` is a *forecast*: its Score is what the model expects a player to
// return over a horizon it cannot see the end of. A replay is the opposite — every
// number in it already happened. Reusing WeekView would put a modelled score in a
// column a reader will read as a result, which is this project's signature failure
// (one quantity, two meanings, and the label describing the wrong one).
//
// So the shapes stay separate and the *renderer* is shared. That is the part worth
// reusing: the position-ordered squad table, the CSS-only week tabs, the chip
// badges, the opponent labels and the html/template escaping all behave identically
// whether the numbers are predictions or results.
type ReplayWeek struct {
	GW int
	// Points is what the eleven actually scored, net of any hit.
	Points  int
	HitCost int
	// Captain is who wore the armband, and CaptainPts what that returned doubled.
	Captain    string
	CaptainPts int
	// Value and Bank are in tenths of a million, as FPL carries them.
	Value, Bank int
	Squad       []ReplayPlayer
	// Moves are the transfers made going INTO this gameweek.
	Moves []ReplayMove
	Chip  string
	// Transfers is the week's count as the REPLAY counted it, which is not always
	// len(Moves).
	//
	// A wildcard rebuilds the fifteen in one act: the replay adds its swaps to the
	// transfer total but records no individual Move for them, because there is no
	// "out for in" pair to record — the whole squad is replaced at once. Counting
	// the rendered rows instead gave a page reporting 30 transfers for a season the
	// terminal called 37, which is this project's signature failure (one quantity,
	// two implementations) reappearing in the renderer.
	//
	// So the count is carried rather than derived, and the page says how many are
	// unlisted instead of quietly showing a smaller number.
	Transfers int
	// Rebuilt marks a week whose squad was replaced wholesale rather than
	// transferred into, which is what makes the gap above legible.
	Rebuilt bool
	// Caveat says what the squad table on this week actually shows, when that is
	// not what a reader would assume. A free-hit week is the case it exists for:
	// the table is the permanent fifteen, and the points in the header came from
	// a borrowed one this page never sees.
	Caveat string
}

// ReplayPlayer is one player in one gameweek of the replay.
type ReplayPlayer struct {
	Name, Team, Position string
	// Points is what he actually returned that week.
	Points int
	// Started says he was in the eleven; a false here is a bench week, which is
	// the difference between a blank that cost points and one that did not.
	Started bool
	Captain bool
	// New says he was bought going into this gameweek, which is what makes the
	// page answer "what changed" rather than only "what was held".
	New bool
	// Opponent is who his club played that week, e.g. "BOU (H)", and is EMPTY only
	// for a genuine blank.
	//
	// It is a field rather than something the renderer derives because the renderer
	// has no fixture list. The first version of this left it unset, so every card in
	// every week read "no fixture" — which does not look like a missing field, it
	// looks like a blank gameweek, and a blank is real information. A hole that
	// renders as a confident wrong answer is worse than one that renders as nothing.
	Opponent string
}

// ReplayMove is one transfer, with what the model believed against what happened.
type ReplayMove struct {
	Out, In string
	// Gain is the modelled gain per gameweek that justified the move.
	Gain float64
	// OutGot and InGot are what the two players actually returned over the
	// horizon the move was judged on. The gain is what the policy believed; only
	// these say whether it was right.
	OutGot, InGot int
	Hit           bool
}

// HTMLReplay writes a finished season's opening gameweeks as a self-contained page.
//
// It renders through the same template as HTMLFull, so the replay inherits every
// convention that page already established rather than inventing a second look for
// the same information: the squad ordered GKP/DEF/MID/FWD with the bench in the same
// table, one tab per gameweek with no JavaScript, chip badges, and names escaped
// because they come from FPL.
//
// The one thing it adds is the transfer strip, because a replay is read for what
// CHANGED. A squad page answers "who is in the team"; this has to answer "what did
// it do, and was it right", and the second needs both sides of every move and what
// each went on to return.
func HTMLReplay(w io.Writer, weeks []ReplayWeek, title, subtitle string) error {
	if len(weeks) == 0 {
		return fmt.Errorf("replay: no gameweeks to render")
	}

	var hw []htmlWeek
	total := 0
	for i, wk := range weeks {
		total += wk.Points

		// Position order, starters before bench — the order a team sheet is read
		// in, and the same order the brief's squad table uses.
		ps := append([]ReplayPlayer(nil), wk.Squad...)
		rank := map[string]int{"GKP": 0, "DEF": 1, "MID": 2, "FWD": 3}
		sort.SliceStable(ps, func(a, b int) bool {
			if ps[a].Started != ps[b].Started {
				return ps[a].Started
			}
			if rank[ps[a].Position] != rank[ps[b].Position] {
				return rank[ps[a].Position] < rank[ps[b].Position]
			}
			return ps[a].Points > ps[b].Points
		})

		byPos := map[string][]htmlCard{}
		var bench []htmlCard
		for _, p := range ps {
			c := htmlCard{
				Name: p.Name, Team: p.Team, Position: p.Position,
				Score: float64(p.Points), IsCaptain: p.Captain,
				Opponent: p.Opponent,
			}
			if p.New {
				// Reuses the risk slot, which the template already renders as a
				// small marker on the card. A second mechanism for one badge is
				// how two renderers start disagreeing.
				c.Risk = "in"
			}
			if p.Started {
				byPos[p.Position] = append(byPos[p.Position], c)
			} else {
				bench = append(bench, c)
			}
		}
		var rows []htmlRow
		for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
			if len(byPos[pos]) > 0 {
				rows = append(rows, htmlRow{Position: pos, Players: byPos[pos]})
			}
		}
		if len(bench) > 0 {
			rows = append(rows, htmlRow{Position: "BENCH", Players: bench})
		}

		var moves []htmlMove
		for _, m := range wk.Moves {
			moves = append(moves, htmlMove{
				Out:   htmlCard{Name: m.Out, Score: float64(m.OutGot)},
				In:    htmlCard{Name: m.In, Score: float64(m.InGot)},
				Delta: m.Gain,
				GW:    wk.GW, Hit: m.Hit,
			})
		}

		hw = append(hw, htmlWeek{
			Event: wk.GW, Rows: rows, First: i == 0,
			Formation: fmt.Sprintf("£%.1fm squad · £%.1fm bank",
				float64(wk.Value)/10, float64(wk.Bank)/10),
			Captain:  fmt.Sprintf("%s (%d)", wk.Captain, wk.CaptainPts),
			Expected: float64(wk.Points),
			Chip:     wk.Chip,
			Actual:   true,
			Moves:    moves,
			Running:  total,
			Hit:      wk.HitCost,
			Rebuilt:  wk.Rebuilt,
			Caveat:   wk.Caveat,
		})
	}

	sum := &replaySummary{Points: total}
	best, worst := weeks[0], weeks[0]
	hi := 0
	for i, wk := range weeks {
		// The replay's own count, falling back to the rendered rows for a caller
		// that does not carry one. Never max() of the two: a silent disagreement
		// is what this field exists to surface.
		if wk.Transfers > 0 {
			sum.Transfers += wk.Transfers
		} else {
			sum.Transfers += len(wk.Moves)
		}
		sum.Hits += wk.HitCost
		// The season's transfers, in gameweek order. Taken from the already-built
		// per-week rows rather than re-derived from wk.Moves, so the two views
		// cannot disagree — one quantity with two implementations is this
		// project's signature failure and a page is no exception.
		sum.Moves = append(sum.Moves, hw[i].Moves...)
		for _, m := range wk.Moves {
			if m.Hit {
				sum.HitMoves++
			}
		}
		// What was actually played, read off the weeks themselves. Deriving it
		// from the plan that was *configured* would let the page claim a chip the
		// replay never reached — a mid-season entry starts after GW6.
		if wk.Chip != "" {
			sum.Chips = append(sum.Chips, fmt.Sprintf("%s GW%d", wk.Chip, wk.GW))
		}
		if wk.Points > best.Points {
			best = wk
		}
		if wk.Points < worst.Points {
			worst = wk
		}
		if wk.Points > hi {
			hi = wk.Points
		}
	}
	// What the transfer list cannot show, stated rather than left as a gap between
	// two numbers on the same page.
	sum.Unlisted = sum.Transfers - len(sum.Moves)
	if sum.Unlisted < 0 {
		sum.Unlisted = 0
	}
	sum.Best, sum.BestPts = best.GW, best.Points
	sum.Worst, sum.WorstPts = worst.GW, worst.Points
	for _, wk := range weeks {
		// Scaled against the best week rather than against zero: the question a
		// reader asks of this strip is which weeks carried the season, and a bar
		// chart anchored at zero flattens exactly that.
		h := 4
		if hi > 0 {
			h = 4 + wk.Points*96/hi
		}
		cls := ""
		switch wk.GW {
		case best.GW:
			cls = "best"
		case worst.GW:
			cls = "worst"
		}
		sum.Bars = append(sum.Bars, summaryBar{GW: wk.GW, Points: wk.Points,
			Height: h, Class: cls})
	}

	sub := subtitle
	if sub == "" {
		sub = fmt.Sprintf("%d gameweeks · %d points", len(weeks), total)
	}
	// NoPlan is the empty case only. It used to carry the "already happened"
	// caveat unconditionally, which meant the season's Transfers section printed
	// that sentence and none of the transfers — the caveat now sits under the
	// summary where it belongs, and this fires only when there is genuinely
	// nothing to list.
	noPlan := ""
	if len(sum.Moves) == 0 {
		noPlan = "No transfers. The gate found nothing worth a free move all season, " +
			"so this is the opening fifteen held to the end with the eleven re-picked weekly."
	}
	return pageTmpl.Execute(w, withPalette(pageData{Title: title, Subtitle: sub, Weeks: hw,
		Replay: true, Summary: sum, NoPlan: noPlan}))
}
