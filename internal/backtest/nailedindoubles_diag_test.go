package backtest

// Is a nailed player still nailed in a double gameweek?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagNailednessInDoubles -v -timeout 30m
//
// # The three mechanisms this is built to separate
//
//   - **Amplification** — nailedness is worth the same per match, so a double
//     simply doubles the stake. Predicts the per-match start and 60+ rates for
//     nailed players are the SAME in doubles as in singles.
//   - **Diversification** — in a double a rotation risk gets two lotteries, so his
//     chance of clearing the hour at least once in the week rises. Predicts the
//     nailed-to-fringe gap NARROWS when read per gameweek rather than per match.
//   - **Congestion** — doubles are caused by fixture congestion, and congestion is
//     when managers rotate, so nailed players are least reliable exactly when the
//     double arrives. Predicts their per-match start rate DROPS in doubles.
//
// # The denominator is the CLUB's double, never the player's appearance
//
// `merged_gw.csv` lists only players who were in a matchday squad, so conditioning
// on a row existing drops precisely the rotated player the congestion mechanism
// predicts. Every population below is enumerated from (player, gameweek) pairs the
// player was *eligible* for — he has enough recent history at a club for a prior
// nailedness to be computable — and a gameweek with no row is a zero rather than
// an absence.
//
// # Nailedness is prior information
//
// A player's nailedness entering gameweek g is his minutes over his club's last
// `nailedWindow` matches BEFORE g, divided by ninety times that many matches. It
// never reads gameweek g itself, so nothing here splits a population on the
// outcome it then measures. The archive's own end-of-season totals would be
// hindsight, and this record has been bitten by an end-stamped field before.
//
// # Why the headline is "both legs" and not "played in the double week"
//
// Only sixty minutes in BOTH matches turns an appearance floor of 2 into 4. A
// talisman rested for the second leg banks 3, not 4, and every framing short of
// this one counts him as a success.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"armband/internal/stats"
)

// nailedWindow is how many of his club's most recent matches a player's
// nailedness is measured over, and nailedMinHistory is the fewest that will do.
//
// **Asserted, both of them.** Six matches is about six weeks of football, which is
// the horizon over which "he is in the team at the moment" is a statement anyone
// would make; four is the fewest that keeps the ratio from being one substitute
// appearance. Neither was swept and neither was chosen against an outcome — a
// nailedness cut is a population definition here, not a fitted constant. The
// season-to-date alternative was rejected on mechanism rather than on a number: a
// player back from a ten-week injury is nailed today and reads as fringe on the
// season, which is the exact case the congestion mechanism is about.
const (
	nailedWindow     = 6
	nailedMinHistory = 4
)

// nailedBuckets partition prior minutes share.
var nailedBuckets = []struct {
	Lo, Hi float64
	Label  string
}{
	{0.00, 0.25, "fringe <0.25"},
	{0.25, 0.60, "rotation .25-.60"},
	{0.60, 0.85, "regular .60-.85"},
	{0.85, 1.01, "nailed >=0.85"},
}

func nailedBucket(x float64) string {
	for _, b := range nailedBuckets {
		if x >= b.Lo && x < b.Hi {
			return b.Label
		}
	}
	return "?"
}

// seasonWeeks is one season's match rows paired with the eligible player-weeks
// derived from them. Named rather than anonymous because seven report functions
// take it.
type seasonWeeks struct {
	sm    *seasonMatches
	weeks []eligibleWeek
}

// eligibleWeek is one player's eligible gameweek: what his club faced and what he
// did about it.
type eligibleWeek struct {
	Element int
	Code    int
	Club    int
	Pos     int
	GW      int
	Value   int

	ClubFixtures int // 1 single, 2+ double; blanks are not eligible weeks
	Nailed       float64
	Legs         []matchRow

	// Available is FPL's own point-in-time flag at this gameweek's deadline:
	// true where the published status was "a". NewsKnown says whether the
	// captures cover this gameweek at all — an uncovered week is unknown, never
	// available.
	Available bool
	NewsKnown bool
}

func (w eligibleWeek) sixtyLegs() int {
	n := 0
	for _, l := range w.Legs {
		if l.Sixty() {
			n++
		}
	}
	return n
}

func (w eligibleWeek) appearedLegs() int {
	n := 0
	for _, l := range w.Legs {
		if l.Appeared() {
			n++
		}
	}
	return n
}

// startedLegs counts recorded starts, and returns ok=false where the archive
// records no start in this gameweek at all.
//
// The reconstruction in `reconstructStarts` is deliberately not consulted. Its own
// boundary two forbids using a reconstructed start for a per-gameweek start
// classification, and boundary three forbids it as evidence about an individual
// rotation case — which is precisely what every row here would be.
func (w eligibleWeek) startedLegs() (int, bool) {
	n := 0
	for _, l := range w.Legs {
		if l.Started < 0 {
			return 0, false
		}
		n += l.Started
	}
	return n, true
}

// appearancePointsBanked is what the week's appearance floor actually paid.
func (w eligibleWeek) appearancePointsBanked() float64 {
	var p float64
	for _, l := range w.Legs {
		p += l.Comp.Appearance
	}
	return p
}

// eligibleWeeks builds every (player, gameweek) the season can speak about.
func eligibleWeeks(sm *seasonMatches, news *TeamNewsTable) []eligibleWeek {
	if !sm.HasCalendar {
		return nil
	}
	// Per player: his rows and his club, by gameweek.
	legs := map[int]map[int][]matchRow{}
	club := map[int]map[int]int{}
	for _, r := range sm.Rows {
		if legs[r.Element] == nil {
			legs[r.Element] = map[int][]matchRow{}
			club[r.Element] = map[int]int{}
		}
		legs[r.Element][r.GW] = append(legs[r.Element][r.GW], r)
		club[r.Element][r.GW] = r.Club
	}

	elements := make([]int, 0, len(legs))
	for el := range legs {
		elements = append(elements, el)
	}
	sort.Ints(elements)

	var out []eligibleWeek
	for _, el := range elements {
		p := sm.Players[el]
		if p == nil || !sm.Rules.Prices(p.Type) {
			continue
		}
		// His first gameweek in a matchday squad. Nothing before it is evidence
		// about him: a January signing has no rows in December and would otherwise
		// read as a fringe player for his first six weeks at a club he was not at.
		first := 39
		for gw := 1; gw <= 38; gw++ {
			if len(legs[el][gw]) > 0 {
				first = gw
				break
			}
		}
		// Forward-filled club, so a gameweek he did not play still has one.
		lastClub := 0
		clubAt := map[int]int{}
		for gw := 1; gw <= 38; gw++ {
			if c, ok := club[el][gw]; ok && c > 0 {
				lastClub = c
			}
			clubAt[gw] = lastClub
		}
		lastValue := 0
		for gw := 1; gw <= 38; gw++ {
			for _, l := range legs[el][gw] {
				lastValue = l.Value
			}
			c := clubAt[gw]
			if c == 0 {
				continue // nothing known about him yet this season
			}
			n := sm.clubGWFixtures(c, gw)
			if n < 1 {
				continue // a blank is not a week this measurement is about
			}
			nailed, ok := priorMinutesShare(sm, el, c, gw, first, legs[el])
			if !ok {
				continue
			}
			w := eligibleWeek{
				Element: el, Code: p.Code, Club: c, Pos: p.Type, GW: gw,
				Value: lastValue, ClubFixtures: n, Nailed: nailed,
				Legs: legs[el][gw],
			}
			if news != nil && news.Covers(sm.Name, gw) {
				st, _, ok := news.FlagAt(sm.Name, gw, p.Code)
				if ok {
					w.NewsKnown = true
					w.Available = st == "a"
				}
			}
			out = append(out, w)
		}
	}
	return out
}

// priorMinutesShare is a player's minutes over his club's last nailedWindow
// matches before gameweek gw, as a share of ninety per match.
//
// Club MATCHES rather than gameweeks, so a double in the window counts twice and a
// blank counts for nothing — which is what makes the denominator the football he
// could have played rather than the calendar.
func priorMinutesShare(sm *seasonMatches, el, club, gw, first int,
	legs map[int][]matchRow) (float64, bool) {
	matches, minutes := 0, 0
	for g := gw - 1; g >= first && matches < nailedWindow; g-- {
		n := sm.clubGWFixtures(club, g)
		if n == 0 {
			continue
		}
		// A gameweek can carry more of the club's matches than the window has
		// room for; take it whole rather than splitting a gameweek's minutes,
		// which the archive cannot do anyway.
		matches += n
		for _, l := range legs[g] {
			minutes += l.Minutes
		}
	}
	// Too little tenure to say anything. A zero here would be a *claim* that he is
	// a fringe player, which is the byte-identical-null trap in miniature: absent
	// evidence read as evidence of absence.
	if matches < nailedMinHistory {
		return 0, false
	}
	return math.Min(1.0, float64(minutes)/(90*float64(matches))), true
}

// TestDiagNailednessInDoubles is measurement two.
func TestDiagNailednessInDoubles(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	mirrorArchive(t, cfg)

	// The point-in-time availability source, under its strict default filter: a
	// gameweek counts only where the capture was taken inside its own run-up.
	// Nil rather than fatal if it cannot be read — every table below reports its
	// own coverage, and the injury split simply does not run.
	news, err := LoadTeamNews(TeamNewsFilter{})
	if err != nil {
		t.Logf("team news unavailable, so the injury split will not run: %v", err)
		news = nil
	}

	var all []seasonWeeks
	for _, name := range appearanceFloorSeasons {
		sm := loadMatches(t, cfg, name)
		if !sm.HasCalendar {
			t.Logf("%s: the archive publishes no fixtures.csv, so this season has no "+
				"calendar and cannot say what a double gameweek is. Excluded.", name)
			continue
		}
		all = append(all, seasonWeeks{sm, eligibleWeeks(sm, news)})
	}

	reportCalendarCensus(t, func() []*seasonMatches {
		var out []*seasonMatches
		for _, l := range all {
			out = append(out, l.sm)
		}
		return out
	}())

	reportPerMatchRates(t, all)
	reportPerGameweekRates(t, all)
	reportBothLegs(t, all, news != nil)
	reportEliteInDoubles(t, all)
	reportClubSplits(t, all)
	reportWithinPlayer(t, all)
	reportPostDoubleLoad(t, all)
	reportBothLegsBySeason(t, all)
}

// reportBothLegsBySeason is the concentration check the headline needs.
//
// Doubles are not spread evenly: the census above runs 22/6/43/61/42/23/10/10
// club-gameweeks. A pooled rate over eight seasons can therefore be two seasons
// wearing a large denominator, and this record's own rule is to say when one cell
// carries an arm rather than to quote the pool alone.
func reportBothLegsBySeason(t *testing.T, all []seasonWeeks) {
	t.Log("== the headline, per season, so concentration is visible ==")
	t.Logf("%-9s   %8s %8s %9s   %8s %8s %9s   %9s %8s %9s",
		"season", "nail wks", "both60", "mean app", "t30 wks", "both60", "mean app",
		"n&t30 wks", "both60", "mean app")
	for _, l := range all {
		top30 := topByPoints(l.sm, 30)
		type acc struct {
			n, both int
			app     float64
		}
		var nailed, elite, both acc
		bump := func(a *acc, w eligibleWeek) {
			a.n++
			if w.sixtyLegs() >= 2 {
				a.both++
			}
			a.app += w.appearancePointsBanked()
		}
		for _, w := range l.weeks {
			if w.ClubFixtures < 2 {
				continue
			}
			if w.Nailed >= 0.85 {
				bump(&nailed, w)
			}
			if top30[w.Element] {
				bump(&elite, w)
			}
			if w.Nailed >= 0.85 && top30[w.Element] {
				bump(&both, w)
			}
		}
		t.Logf("%-9s   %8d %8.4f %9.3f   %8d %8.4f %9.3f   %9d %8.4f %9.3f",
			l.sm.Name,
			nailed.n, rate(nailed.both, nailed.n), nailed.app/math.Max(1, float64(nailed.n)),
			elite.n, rate(elite.both, elite.n), elite.app/math.Max(1, float64(elite.n)),
			both.n, rate(both.both, both.n), both.app/math.Max(1, float64(both.n)))
	}
}

// reportCalendarCensus prints what each season's calendar actually contains,
// because every table after it is conditional on that.
func reportCalendarCensus(t *testing.T, seasons []*seasonMatches) {
	t.Log("== calendar census, from fixtures.csv ==")
	t.Log("A club-gameweek is blank at 0 fixtures, single at 1, double at 2+.")
	t.Logf("%-9s %8s %8s %8s   %-9s %-9s %s",
		"season", "blank", "single", "double", "clubs>=1", "clubs>=2", "starts recorded in gws")
	for _, sm := range seasons {
		var blank, single, double int
		blanksPerClub := map[int]int{}
		for club := range sm.Fixtures {
			for gw := 1; gw <= 38; gw++ {
				switch n := sm.clubGWFixtures(club, gw); {
				case n == 0:
					blank++
					blanksPerClub[club]++
				case n == 1:
					single++
				default:
					double++
				}
			}
		}
		var one, many int
		for _, b := range blanksPerClub {
			if b == 1 {
				one++
			} else if b >= 2 {
				many++
			}
		}
		var gws int
		for gw := 1; gw <= 38; gw++ {
			if sm.StartsRecorded[gw] {
				gws++
			}
		}
		t.Logf("%-9s %8d %8d %8d   %-9d %-9d %d of 38",
			sm.Name, blank, single, double, one, many, gws)
	}
	t.Log("`clubs>=1` is clubs blanking exactly once — the dragged-in heuristic; " +
		"`clubs>=2` is repeated blankers, the cup-runner heuristic.")
}

// rateAcc counts one bucket.
type rateAcc struct {
	Weeks    int
	Matches  int
	Appeared int
	Sixty    int
	Started  int
	// StartsKnown counts matches in gameweeks where the archive records starts.
	StartsKnown int
	// WeekSixty counts weeks with at least one 60+ leg.
	WeekSixty int
	// AppearPts is the total appearance points banked.
	AppearPts float64
}

func (a *rateAcc) add(w eligibleWeek) {
	a.Weeks++
	a.Matches += w.ClubFixtures
	a.Appeared += w.appearedLegs()
	a.Sixty += w.sixtyLegs()
	a.AppearPts += w.appearancePointsBanked()
	if w.sixtyLegs() > 0 {
		a.WeekSixty++
	}
	if s, ok := w.startedLegs(); ok {
		a.Started += s
		a.StartsKnown += w.ClubFixtures
	}
}

// reportPerMatchRates is the amplification-versus-congestion table.
func reportPerMatchRates(t *testing.T, all []seasonWeeks) {
	single := map[string]*rateAcc{}
	double := map[string]*rateAcc{}
	touch := func(m map[string]*rateAcc, k string) *rateAcc {
		if m[k] == nil {
			m[k] = &rateAcc{}
		}
		return m[k]
	}
	for _, l := range all {
		for _, w := range l.weeks {
			m := single
			if w.ClubFixtures >= 2 {
				m = double
			}
			touch(m, nailedBucket(w.Nailed)).add(w)
		}
	}
	t.Log("== per-MATCH rates, doubles against singles, by prior nailedness ==")
	t.Log("Amplification predicts the two rate pairs are equal; congestion predicts the")
	t.Log("double column is lower for the nailed bucket.")
	t.Logf("%-18s %9s %8s %8s %8s %7s   %9s %8s %8s %8s %7s   %8s %8s",
		"nailedness", "S matches", "S appear", "S 60+", "S start", "S n(st)",
		"D matches", "D appear", "D 60+", "D start", "D n(st)", "d(60+)", "d(start)")
	for _, b := range nailedBuckets {
		s, d := single[b.Label], double[b.Label]
		if s == nil || d == nil {
			continue
		}
		ds := math.NaN()
		if s.StartsKnown > 0 && d.StartsKnown > 0 {
			ds = rate(d.Started, d.StartsKnown) - rate(s.Started, s.StartsKnown)
		}
		t.Logf("%-18s %9d %8.4f %8.4f %8.4f %7d   %9d %8.4f %8.4f %8.4f %7d   %+8.4f %+8.4f",
			b.Label,
			s.Matches, rate(s.Appeared, s.Matches), rate(s.Sixty, s.Matches),
			rate(s.Started, s.StartsKnown), s.StartsKnown,
			d.Matches, rate(d.Appeared, d.Matches), rate(d.Sixty, d.Matches),
			rate(d.Started, d.StartsKnown), d.StartsKnown,
			rate(d.Sixty, d.Matches)-rate(s.Sixty, s.Matches), ds)
	}
	t.Log("`n(st)` is the matches the start columns are computed over: gameweeks where the")
	t.Log("archive records starts at all, which is 2022-23 from GW16 onward and nothing before.")
}

// reportPerGameweekRates is the diversification table.
func reportPerGameweekRates(t *testing.T, all []seasonWeeks) {
	single := map[string]*rateAcc{}
	double := map[string]*rateAcc{}
	touch := func(m map[string]*rateAcc, k string) *rateAcc {
		if m[k] == nil {
			m[k] = &rateAcc{}
		}
		return m[k]
	}
	for _, l := range all {
		for _, w := range l.weeks {
			m := single
			if w.ClubFixtures >= 2 {
				m = double
			}
			touch(m, nailedBucket(w.Nailed)).add(w)
		}
	}
	t.Log("== per-GAMEWEEK 'cleared 60 at least once', doubles against singles ==")
	t.Log("Diversification predicts the gap between nailed and not narrows in the double")
	t.Log("column. Read the rotation reference, not the fringe one — see the note below the table.")
	t.Logf("%-18s %8s %9s   %8s %9s   %8s", "nailedness",
		"S weeks", "S >=1x60", "D weeks", "D >=1x60", "diff")
	sRate := map[string]float64{}
	dRate := map[string]float64{}
	for _, b := range nailedBuckets {
		s, d := single[b.Label], double[b.Label]
		if s == nil || d == nil {
			continue
		}
		sr, dr := rate(s.WeekSixty, s.Weeks), rate(d.WeekSixty, d.Weeks)
		sRate[b.Label], dRate[b.Label] = sr, dr
		t.Logf("%-18s %8d %9.4f   %8d %9.4f   %+8.4f",
			b.Label, s.Weeks, sr, d.Weeks, dr, dr-sr)
	}

	// Two reference buckets, because the choice of reference decides the answer
	// and only one of the two is a fair test.
	//
	// The fringe bucket sits at 0.05 in a single week, so it is close to a floor
	// and cannot gain much from a second lottery — a gap measured against it moves
	// mostly with what happens at the top. The rotation bucket is where a second
	// chance can actually help, which is what the diversification mechanism is
	// about, so nailed-minus-rotation is the contrast that discriminates and
	// nailed-minus-fringe is reported beside it rather than instead of it.
	top := nailedBuckets[len(nailedBuckets)-1].Label
	for _, ref := range []string{nailedBuckets[0].Label, nailedBuckets[1].Label} {
		sg, dg := sRate[top]-sRate[ref], dRate[top]-dRate[ref]
		word := "WIDENS"
		if dg < sg {
			word = "narrows"
		}
		t.Logf("gap nailed minus %-18s singles %.4f, doubles %.4f — %s by %+0.4f",
			ref, sg, dg, word, dg-sg)
	}
}

// reportBothLegs is the headline: given the club doubled, how often did he do the
// whole week?
func reportBothLegs(t *testing.T, all []seasonWeeks, haveNews bool) {
	type acc struct {
		weeks                  int
		both, one, none        int
		startBoth, startKnown  int
		startOne, startNoneAcc int
		appearPts              float64
	}
	raw := map[string]*acc{}
	avail := map[string]*acc{}
	touch := func(m map[string]*acc, k string) *acc {
		if m[k] == nil {
			m[k] = &acc{}
		}
		return m[k]
	}
	add := func(a *acc, w eligibleWeek) {
		a.weeks++
		switch w.sixtyLegs() {
		case 0:
			a.none++
		case 1:
			a.one++
		default:
			a.both++
		}
		a.appearPts += w.appearancePointsBanked()
		if s, ok := w.startedLegs(); ok {
			a.startKnown++
			switch s {
			case 0:
				a.startNoneAcc++
			case 1:
				a.startOne++
			default:
				a.startBoth++
			}
		}
	}
	var newsCovered, newsTotal int
	for _, l := range all {
		top30 := topByPoints(l.sm, 30)
		top100 := topByPoints(l.sm, 100)
		for _, w := range l.weeks {
			if w.ClubFixtures < 2 {
				continue
			}
			newsTotal++
			if w.NewsKnown {
				newsCovered++
			}
			keys := []string{nailedBucket(w.Nailed)}
			if top100[w.Element] {
				keys = append(keys, "top100 by points")
			}
			if top30[w.Element] {
				keys = append(keys, "top30 by points")
			}
			if w.Value >= 100 {
				keys = append(keys, "price 10.0m+")
			}
			if w.Nailed >= 0.85 && top30[w.Element] {
				keys = append(keys, "nailed AND top30")
			}
			for _, k := range keys {
				add(touch(raw, k), w)
				if w.NewsKnown && w.Available {
					add(touch(avail, k), w)
				}
			}
		}
	}

	order := []string{}
	for _, b := range nailedBuckets {
		order = append(order, b.Label)
	}
	order = append(order, "top100 by points", "top30 by points", "price 10.0m+",
		"nailed AND top30")

	print := func(title string, m map[string]*acc) {
		t.Log(title)
		t.Logf("%-18s %8s %8s %8s %8s   %9s   %8s %8s %8s %8s",
			"bucket", "dbl weeks", "both60", "one60", "none",
			"mean appPts", "n starts", "startBoth", "startOne", "startNone")
		for _, k := range order {
			a := m[k]
			if a == nil || a.weeks == 0 {
				continue
			}
			t.Logf("%-18s %8d %8.4f %8.4f %8.4f   %9.3f   %8d %8.4f %8.4f %8.4f",
				k, a.weeks, rate(a.both, a.weeks), rate(a.one, a.weeks),
				rate(a.none, a.weeks), a.appearPts/float64(a.weeks),
				a.startKnown, rate(a.startBoth, a.startKnown),
				rate(a.startOne, a.startKnown), rate(a.startNoneAcc, a.startKnown))
		}
	}

	t.Log("== the headline: given his CLUB doubled, did he do both legs? ==")
	t.Log("`mean appPts` is the appearance floor actually banked that week. Four is both")
	t.Log("legs at sixty minutes; two is one full leg; the claim under test is that a nailed")
	t.Log("elite player banks close to four.")
	print("-- raw, injuries included. This is the deployable figure: at the moment of "+
		"a transfer you do not know he will stay fit. --", raw)
	if haveNews {
		t.Logf("point-in-time team news covers %d of %d double-gameweek player-weeks",
			newsCovered, newsTotal)
		print("-- restricted to weeks FPL was publishing status 'a' at the deadline. "+
			"This answers 'does congestion cause rotation' and is NOT the deployable figure. --",
			avail)
	}
}

// reportEliteInDoubles answers the elite question in one line per population.
func reportEliteInDoubles(t *testing.T, all []seasonWeeks) {
	type pair struct{ s, d rateAcc }
	m := map[string]*pair{}
	touch := func(k string) *pair {
		if m[k] == nil {
			m[k] = &pair{}
		}
		return m[k]
	}
	for _, l := range all {
		top30 := topByPoints(l.sm, 30)
		top100 := topByPoints(l.sm, 100)
		for _, w := range l.weeks {
			var keys []string
			if top100[w.Element] {
				keys = append(keys, "top100 by points")
			}
			if top30[w.Element] {
				keys = append(keys, "top30 by points")
			}
			if w.Value >= 100 {
				keys = append(keys, "price 10.0m+ (prior)")
			}
			for _, k := range keys {
				p := touch(k)
				if w.ClubFixtures >= 2 {
					p.d.add(w)
				} else {
					p.s.add(w)
				}
			}
		}
	}
	t.Log("== elite populations, per-match rates in doubles against singles ==")
	t.Log("`top N by points` is END-STAMPED — selected on the season's outcome, and a")
	t.Log("player's doubles feed his own total, so it selects mildly toward having played")
	t.Log("them. `price 10.0m+` is the prior-information counterpart: a price is set before")
	t.Log("the deadline.")
	t.Logf("%-22s %9s %8s %8s   %9s %8s %8s   %8s",
		"population", "S matches", "S appear", "S 60+", "D matches", "D appear", "D 60+", "d(60+)")
	for _, k := range []string{"top100 by points", "top30 by points", "price 10.0m+ (prior)"} {
		p := m[k]
		if p == nil {
			continue
		}
		t.Logf("%-22s %9d %8.4f %8.4f   %9d %8.4f %8.4f   %+8.4f",
			k, p.s.Matches, rate(p.s.Appeared, p.s.Matches), rate(p.s.Sixty, p.s.Matches),
			p.d.Matches, rate(p.d.Appeared, p.d.Matches), rate(p.d.Sixty, p.d.Matches),
			rate(p.d.Sixty, p.d.Matches)-rate(p.s.Sixty, p.s.Matches))
	}
}

// clubBlanks counts a club's blank gameweeks in a season.
func clubBlanks(sm *seasonMatches, club int) int {
	n := 0
	for gw := 1; gw <= 38; gw++ {
		if sm.clubGWFixtures(club, gw) == 0 {
			n++
		}
	}
	return n
}

// clubRotation is how much a club changes the set of players it keeps on for an
// hour, between consecutive matches.
//
// Measured on 60+ minutes rather than on starts, because starts are absent from
// half the archive and 60+ is recoverable everywhere. It is a **club-season**
// property computed over the whole season, so it is end-stamped — acceptable
// because it is only ever used to SPLIT clubs, never as a predictor.
func clubRotation(sm *seasonMatches, club int) (float64, int) {
	// The club's matches in order, each with its set of hour-clearing players.
	// `order` sorts on (gameweek, fixture id) so a double's two matches are
	// consecutive and in a stable order, which is why the key is composite rather
	// than the gameweek alone.
	type m struct {
		key int
		set map[int]bool
	}
	// Keyed on the FIXTURE, never the gameweek. A double gameweek merges two
	// elevens into one set of up to twenty-two, which reads as a club that changed
	// nothing — and doubling clubs are exactly the population this measure is used
	// to split, so the error would run straight into the answer.
	byFixture := map[int]map[int]bool{}
	when := map[int]int{}
	for _, r := range sm.Rows {
		if r.Club != club || !r.Sixty() {
			continue
		}
		if byFixture[r.Fixture] == nil {
			byFixture[r.Fixture] = map[int]bool{}
		}
		byFixture[r.Fixture][r.Element] = true
		when[r.Fixture] = r.GW
	}
	var order []m
	for fid, set := range byFixture {
		order = append(order, m{when[fid]*100000 + fid, set})
	}
	sort.Slice(order, func(i, j int) bool { return order[i].key < order[j].key })
	if len(order) < 2 {
		return math.NaN(), 0
	}
	var churn float64
	for i := 1; i < len(order); i++ {
		diff := 0
		for el := range order[i].set {
			if !order[i-1].set[el] {
				diff++
			}
		}
		churn += float64(diff)
	}
	return churn / float64(len(order)-1), len(order)
}

// reportClubSplits crosses the double against club depth and against the
// cup-runner heuristic.
func reportClubSplits(t *testing.T, all []seasonWeeks) {
	type pair struct{ s, d rateAcc }
	m := map[string]*pair{}
	order := []string{}
	touch := func(k string) *pair {
		if m[k] == nil {
			m[k] = &pair{}
			order = append(order, k)
		}
		return m[k]
	}

	for _, l := range all {
		top30 := topByPoints(l.sm, 30)
		// Rotation, split at the season's own median so the label means "relative
		// to this season's league" rather than to an absolute churn number.
		rot := map[int]float64{}
		var vals []float64
		for club := range l.sm.Fixtures {
			if r, n := clubRotation(l.sm, club); n >= 2 && !math.IsNaN(r) {
				rot[club] = r
				vals = append(vals, r)
			}
		}
		med := stats.Median(vals)
		for _, w := range l.weeks {
			r, ok := rot[w.Club]
			depth := "settled club"
			if ok && r > med {
				depth = "rotating club"
			} else if !ok {
				depth = "club unknown"
			}
			blanks := clubBlanks(l.sm, w.Club)
			cup := "never blanked"
			switch {
			case blanks == 1:
				cup = "dragged-in (1 blank)"
			case blanks >= 2:
				cup = "cup runner (2+ blanks)"
			}
			put := func(k string) {
				p := touch(k)
				if w.ClubFixtures >= 2 {
					p.d.add(w)
				} else {
					p.s.add(w)
				}
			}
			if w.Nailed >= 0.85 {
				put("nailed | " + depth)
				put("nailed | " + cup)
				if top30[w.Element] {
					put("top30 | " + depth)
					put("top30 | " + cup)
				}
			}
		}
	}
	sort.Strings(order)
	t.Log("== nailed and elite players, split by club depth and by the cup-runner heuristic ==")
	t.Log("Club depth is the season's own median churn in the hour-clearing eleven — end-stamped,")
	t.Log("used only to split. The cup-runner split is a HEURISTIC: a club that blanks repeatedly")
	t.Log("is progressing in a cup, a club that blanks once is usually somebody else's collateral.")
	t.Log("It fails where a blank has another cause (European scheduling, weather, a title decider),")
	t.Log("and deep cup runs and deep squads correlate anyway.")
	t.Logf("%-30s %9s %8s   %9s %8s   %8s",
		"bucket", "S matches", "S 60+", "D matches", "D 60+", "d(60+)")
	for _, k := range order {
		p := m[k]
		if p.d.Matches == 0 {
			t.Logf("%-30s %9d %8.4f   %9d %8s   %8s",
				k, p.s.Matches, rate(p.s.Sixty, p.s.Matches), 0, "-", "no doubles")
			continue
		}
		t.Logf("%-30s %9d %8.4f   %9d %8.4f   %+8.4f",
			k, p.s.Matches, rate(p.s.Sixty, p.s.Matches),
			p.d.Matches, rate(p.d.Sixty, p.d.Matches),
			rate(p.d.Sixty, p.d.Matches)-rate(p.s.Sixty, p.s.Matches))
	}
}

// minWeeksEitherSide is how many single weeks and double weeks one player must
// have before his own within-player contrast is used.
//
// **Asserted.** One double week against thirty singles is a difference with a
// denominator of one on the treated side; two is the fewest that is not a single
// coin toss. It is a floor on a paired estimator, not a fitted value.
const minWeeksEitherSide = 2

// reportWithinPlayer is the same player's doubles against his own singles.
//
// This removes every player-level and club-level confound that does not vary
// within a season — quality, position, club depth, whether his club is a cup
// runner — because both arms are the same footballer at the same club. What it
// does NOT remove is *when* in the season the double fell.
func reportWithinPlayer(t *testing.T, all []seasonWeeks) {
	type armPair struct{ s, d rateAcc }
	buckets := map[string][]float64{}
	order := []string{}
	push := func(k string, v float64) {
		if _, ok := buckets[k]; !ok {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], v)
	}
	for _, l := range all {
		top30 := topByPoints(l.sm, 30)
		per := map[int]*armPair{}
		nailedWeeks := map[int]int{}
		for _, w := range l.weeks {
			if per[w.Element] == nil {
				per[w.Element] = &armPair{}
			}
			if w.ClubFixtures >= 2 {
				per[w.Element].d.add(w)
			} else {
				per[w.Element].s.add(w)
			}
			if w.Nailed >= 0.85 {
				nailedWeeks[w.Element]++
			}

		}
		for el, p := range per {
			if p.d.Weeks < minWeeksEitherSide || p.s.Weeks < minWeeksEitherSide {
				continue
			}
			diff := rate(p.d.Sixty, p.d.Matches) - rate(p.s.Sixty, p.s.Matches)
			push("everyone", diff)
			if 2*nailedWeeks[el] >= p.s.Weeks+p.d.Weeks {
				push("mostly nailed", diff)
			}
			if top30[el] {
				push("top30 by points", diff)
			}
		}
	}
	sort.Strings(order)
	t.Log("== within-player: the same footballer's 60+ rate per match, doubles minus singles ==")
	t.Logf("Players need at least %d weeks of each kind in one season.", minWeeksEitherSide)
	t.Logf("%-20s %8s %9s %9s %9s %9s", "population", "players", "mean", "median", "P(<0)", "sd")
	for _, k := range order {
		xs := buckets[k]
		neg := 0
		for _, x := range xs {
			if x < 0 {
				neg++
			}
		}
		_, med, _ := quartiles(xs)
		t.Logf("%-20s %8d %+9.4f %+9.4f %9.4f %9.4f",
			k, len(xs), meanOf(xs), med, rate(neg, len(xs)), sd(xs))
	}
}

// reportPostDoubleLoad asks whether playing twice in a week costs matches later.
//
// The treated event is "cleared sixty minutes in both legs of a double". The
// control is the same players' weeks where they cleared sixty in a single
// gameweek. Both are followed for three gameweeks.
//
// ⚠️ The confound is named rather than removed: doubling clubs are cup-progressing
// clubs, which also play midweek cup ties, so a post-double absence mixes
// double-induced load with cup load. The dragged-in split below is the closest this
// archive gets to an exogenous dose of "played twice this week", because those
// clubs took the double without the cup run.
func reportPostDoubleLoad(t *testing.T, all []seasonWeeks) {
	type follow struct {
		n       int
		absent  [3]int // no row at all in gw+k
		appear  [3]int
		sixty   [3]int
		minutes [3]int
		// horizon counts how many of the three follow-up weeks existed as
		// non-blank club weeks, per offset, so a rate has an honest denominator.
		horizon [3]int
	}
	buckets := map[string]*follow{}
	order := []string{}
	touch := func(k string) *follow {
		if buckets[k] == nil {
			buckets[k] = &follow{}
			order = append(order, k)
		}
		return buckets[k]
	}

	for _, l := range all {
		// Index the season's eligible weeks by (element, gw) so a follow-up can be
		// looked up, and by element so a player's absence is distinguishable from
		// a week his club did not play.
		byKey := map[[2]int]eligibleWeek{}
		for _, w := range l.weeks {
			byKey[[2]int{w.Element, w.GW}] = w
		}
		for _, w := range l.weeks {
			// The third arm is the matched control, and it is the one to read.
			//
			// "after a full double" against "after a full single" is not a fair
			// comparison: the treated arm conditions on clearing the hour TWICE,
			// which selects harder on being fit and in favour than clearing it
			// once does, and the selection runs in the direction that makes the
			// double look harmless. Two consecutive full singles is the same two
			// matches of load over two weeks instead of one, so the fitness
			// selection matches and what differs is the compression.
			prev, hadPrev := byKey[[2]int{w.Element, w.GW - 1}]
			var arm string
			switch {
			case w.ClubFixtures >= 2 && w.sixtyLegs() >= 2:
				arm = "after a full double"
			case w.ClubFixtures == 1 && w.sixtyLegs() == 1 &&
				hadPrev && prev.ClubFixtures == 1 && prev.sixtyLegs() == 1:
				arm = "after two full singles"
			case w.ClubFixtures == 1 && w.sixtyLegs() == 1:
				arm = "after a full single"
			default:
				continue
			}
			blanks := clubBlanks(l.sm, w.Club)
			labels := []string{arm}
			if blanks == 1 {
				labels = append(labels, arm+" | dragged-in")
			} else if blanks >= 2 {
				labels = append(labels, arm+" | cup runner")
			}
			for _, lab := range labels {
				f := touch(lab)
				f.n++
				for k := 0; k < 3; k++ {
					nx, ok := byKey[[2]int{w.Element, w.GW + k + 1}]
					if !ok {
						continue // blank week, season over, or no longer eligible
					}
					f.horizon[k]++
					if nx.appearedLegs() == 0 {
						f.absent[k]++
					} else {
						f.appear[k]++
					}
					if nx.sixtyLegs() > 0 {
						f.sixty[k]++
					}
					for _, leg := range nx.Legs {
						f.minutes[k] += leg.Minutes
					}
				}
			}
		}
	}
	sort.Strings(order)
	t.Log("== the second-order cost: what happens in the three gameweeks after ==")
	t.Log("`absent` is no matchday-squad row at all in that gameweek — the archive's only")
	t.Log("proxy for unavailability, and it also catches a straight omission.")
	t.Logf("%-38s %7s   %s", "arm", "events",
		"gw+1 absent/60+/mins   gw+2 absent/60+/mins   gw+3 absent/60+/mins")
	for _, k := range order {
		f := buckets[k]
		var parts []string
		for i := 0; i < 3; i++ {
			parts = append(parts, fmt.Sprintf("%6.4f/%6.4f/%5.1f",
				rate(f.absent[i], f.horizon[i]), rate(f.sixty[i], f.horizon[i]),
				float64(f.minutes[i])/math.Max(1, float64(f.horizon[i]))))
		}
		t.Logf("%-38s %7d   %s  %s  %s", k, f.n, parts[0], parts[1], parts[2])
	}
}
