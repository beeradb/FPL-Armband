package viewmodel

import "armband/internal/fpl"

// PlayerDetail is one footballer's playing history: last season's record, and this season's
// match-by-match log. It is a SEPARATE document from State, served at /api/player/{code}
// rather than as fields on Player — roughly 590 players times a full match log is a payload
// nobody reads on every page build, and a reader opens one card at a time.
//
// Keyed by permanent player CODE, never by the season-scoped element id everything else in
// this contract uses: an id comes back attached to a different footballer every August, and
// while a READ does not need that guarantee the way a write does, one spelling of "which
// player" is the rule this whole contract keeps.
//
// State and PlayerDetail cannot disagree, because they describe different things. State
// carries PROJECTIONS — two independent fetches really could see those differently, which is
// why State is one document. PlayerDetail carries HISTORY: finished football, which cannot
// disagree with a projection. So this struct carries no xp, no role, no price — every
// quantity still has exactly one source, and the sheet draws every projected figure from
// State and only the two bands below from here.
type PlayerDetail struct {
	// LastSeason is nil when FPL's history_past carries no season for this player at all — a
	// debutant, or a player promoted from a division this feed does not cover (it is
	// Premier League seasons only). The client must say so rather than draw an empty grid.
	LastSeason *SeasonSummary `json:"last_season,omitempty"`
	// Gameweeks is this season's played fixtures, oldest first — a passthrough of FPL's own
	// order. Empty at GW1 for every player in the game, and the client's empty state must say
	// so rather than rendering nothing.
	Gameweeks []PlayerGameweek `json:"gameweeks,omitempty"`
}

// SeasonSummary is one completed FPL season, from fpl.PastSeason.
//
// ⚠️ fpl.PastSeason carries no club, so this struct must not grow one from a second lookup —
// a player's previous club is not on this path.
type SeasonSummary struct {
	// Season is FPL's own name for it ("2025/26"), sent rather than assumed: history_past's
	// most recent entry is not always literally LAST season — a player who missed a whole
	// season through injury or a spell outside the Premier League still has one, and it is
	// older than a reader would guess.
	Season string `json:"season"`

	Points  int `json:"points"`
	Minutes int `json:"minutes"`
	Starts  int `json:"starts"`
	// PointsPer90 is arithmetic over two archive integers (points and minutes), computed here
	// rather than in the client for the same reason Player.ValueScore is: one way to do a
	// division, not one done server-side for some figures and client-side for others.
	PointsPer90 float64 `json:"points_per_90"`

	Goals   int `json:"goals"`
	Assists int `json:"assists"`
	// CleanSheets is nil for MID and FWD: a clean sheet is not their stat, and a bare 0 would
	// read as a claim about a quantity that does not apply to the position he plays now.
	CleanSheets *int `json:"clean_sheets,omitempty"`
	Bonus       int  `json:"bonus"`

	// XG and XA are FPL's own figures for the season, carried as reported.
	XG float64 `json:"xg"`
	XA float64 `json:"xa"`

	// PriceStart and PriceEnd are in millions, via fpl.TenthsToMillions -- the one
	// implementation of FPL's tenths-of-a-million convention, also used for a live price
	// (fpl.Element.PriceM) and for PlayerGameweek.Price below.
	PriceStart float64 `json:"price_start"`
	PriceEnd   float64 `json:"price_end"`
}

// PlayerGameweek is one played fixture, from fpl.HistoryEntry.
type PlayerGameweek struct {
	GW int `json:"gw"`
	// Opponent is the short club name — the server resolves fpl.HistoryEntry.OpponentTeam,
	// which is a team id, so the client never needs a second id-to-club table for it.
	Opponent string `json:"opponent"`
	Home     bool   `json:"home"`

	Minutes int  `json:"minutes"`
	Started bool `json:"started"`
	Points  int  `json:"points"`

	Goals      int `json:"goals"`
	Assists    int `json:"assists"`
	CleanSheet int `json:"clean_sheet"`
	Bonus      int `json:"bonus"`
	BPS        int `json:"bps"`

	XG float64 `json:"xg"`
	XA float64 `json:"xa"`

	// Price is what he cost THAT week, in millions — the only honest source for a
	// price-change line, since it is a fact rather than a start/end pair with everything
	// between interpolated. Carried in the contract; the sheet does not render it yet.
	Price float64 `json:"price"`
}

// BuildPlayerDetail translates one player's FPL history into the client contract.
//
// Pure, like the rest of this package: no I/O, no model quantity, nothing beyond arranging
// fields FPL already sent and one division of two archive integers.
//
// pos is his CURRENT position ("GKP", "DEF", "MID", "FWD"), read only to decide whether last
// season's clean-sheet count is worth sending. teams maps a team id to its short name, built
// once by the caller from fpl.Bootstrap.Teams so this function needs no bootstrap of its own.
func BuildPlayerDetail(es *fpl.ElementSummary, pos string, teams map[int]string) *PlayerDetail {
	d := &PlayerDetail{}
	if es == nil {
		return d
	}

	// history_past is returned oldest first, so the LAST entry is the most recent season on
	// record — never the first.
	if n := len(es.HistoryPast); n > 0 {
		s := es.HistoryPast[n-1]
		ls := &SeasonSummary{
			Season:      s.SeasonName,
			Points:      s.TotalPoints,
			Minutes:     s.Minutes,
			Starts:      s.Starts,
			PointsPer90: per90(s.TotalPoints, s.Minutes),
			Goals:       s.GoalsScored,
			Assists:     s.Assists,
			Bonus:       s.Bonus,
			XG:          s.ExpectedGoals.Float(),
			XA:          s.ExpectedAssists.Float(),
			PriceStart:  fpl.TenthsToMillions(s.StartCost),
			PriceEnd:    fpl.TenthsToMillions(s.EndCost),
		}
		if pos == "DEF" || pos == "GKP" {
			cs := s.CleanSheets
			ls.CleanSheets = &cs
		}
		d.LastSeason = ls
	}

	for _, h := range es.History {
		d.Gameweeks = append(d.Gameweeks, PlayerGameweek{
			GW:         h.Round,
			Opponent:   teams[h.OpponentTeam],
			Home:       h.WasHome,
			Minutes:    h.Minutes,
			Started:    h.Starts > 0,
			Points:     h.TotalPoints,
			Goals:      h.GoalsScored,
			Assists:    h.Assists,
			CleanSheet: h.CleanSheets,
			Bonus:      h.Bonus,
			BPS:        h.BPS,
			XG:         h.ExpectedGoals.Float(),
			XA:         h.ExpectedAssists.Float(),
			Price:      fpl.TenthsToMillions(h.Value),
		})
	}
	return d
}

// per90 is points scaled to a full match. Zero minutes returns zero rather than dividing by
// zero — encoding/json refuses to marshal the NaN that division would otherwise produce.
func per90(points, minutes int) float64 {
	if minutes <= 0 {
		return 0
	}
	return float64(points) / (float64(minutes) / 90.0)
}
