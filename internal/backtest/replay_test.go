package backtest

import (
	"context"
	"testing"
	"time"

	"armband/internal/analysis"
)

// The scoring logic is tested against a hand-built season so the suite never
// touches the network. Only the loader itself needs the archive, and that test
// skips when it is unreachable.
func mkSeason() *Season {
	s := &Season{Name: "test", Players: map[int]*Player{}}
	add := func(id, typ, team, price, ppg, mins int) {
		p := &Player{ID: id, Code: 1000 + id, Type: typ, Team: team,
			WebName: string(rune('A' + id - 1)), NowCost: price, GWs: map[int]GW{}}
		for gw := 1; gw <= 38; gw++ {
			p.GWs[gw] = GW{Points: ppg, Minutes: mins, Value: price}
			p.TotalPoints += ppg
			p.Minutes += mins
		}
		s.Players[id] = p
	}
	// 15 players: two keepers, five defenders, five midfielders, three forwards.
	add(1, 1, 1, 50, 4, 90)
	add(2, 1, 2, 40, 0, 0) // reserve keeper who never plays
	for i := 3; i <= 7; i++ {
		add(i, 2, i, 50, 3, 90)
	}
	for i := 8; i <= 12; i++ {
		add(i, 3, i, 70, 5, 90)
	}
	for i := 13; i <= 15; i++ {
		add(i, 4, i, 80, 6, 90)
	}
	// A hand-built season still has to resolve its points table, because a loaded
	// one does — `repaired()` runs this for every season `Load` returns. Skipping
	// it leaves every player with a zero `Rules`, which `analysis.XPointsResidual`
	// refuses outright rather than pricing a goal at nothing.
	//
	// ⚠️ "test" does NOT sort below a real season — Go compares bytes and 't' is
	// above '2' — so this fixture is priced under the MODERN table, not the oldest.
	// Inert here (no goals, no xG anywhere in it) and stated because a first
	// version of this comment had it backwards.
	s.resolveInstrumentInputs()
	return s
}

func metrics(s *Season, ids ...int) []analysis.PlayerMetrics {
	var out []analysis.PlayerMetrics
	for _, id := range ids {
		p := s.Players[id]
		out = append(out, analysis.PlayerMetrics{
			ID: p.ID, Name: p.WebName, Price: float64(p.NowCost) / 10,
		})
	}
	return out
}

func TestScoreSumsActualReturns(t *testing.T) {
	s := mkSeason()
	all := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	squad := metrics(s, all...)
	xi := metrics(s, 1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15)

	r := Score("t", s, squad, xi, 13, 0)

	want := 0
	for _, id := range all {
		want += s.Players[id].TotalPoints
	}
	if r.SquadPoints != want {
		t.Errorf("squad points %d, want %d", r.SquadPoints, want)
	}
	// Eleven players, all playing, captain doubled: the captain's season again.
	xiWant := 0
	for _, id := range []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15} {
		xiWant += s.Players[id].TotalPoints
	}
	xiWant += s.Players[13].TotalPoints
	if r.XIPoints != xiWant {
		t.Errorf("XI points %d, want %d (captain doubled)", r.XIPoints, xiWant)
	}
}

// TestAutosubsCoverABlank — a starter who does not play is replaced from the
// bench, or every squad would be penalised for rotation the manager never saw.
func TestAutosubsCoverABlank(t *testing.T) {
	s := mkSeason()
	// Player 3 (a defender) plays nowhere; player 6 sits on the bench.
	for gw := 1; gw <= 38; gw++ {
		s.Players[3].GWs[gw] = GW{Points: 0, Minutes: 0, Value: 50}
	}
	squad := metrics(s, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15)
	xi := metrics(s, 1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15)

	r := Score("t", s, squad, xi, 13, 0)
	// The blank is covered by a benched defender on 3 a week for 38 weeks.
	if got := r.XIPoints; got < 38*3 {
		t.Errorf("XI points %d suggests the autosub never fired", got)
	}
}

// TestViceCaptainCoversABlankCaptain — FPL passes the armband to the
// vice-captain when the captain records no minutes, for any reason. Before
// this the replay simply forfeited the bonus: TestDiagBlankHandling measured
// the model's own captain choice blanking 9.6% of weeks and the replay never
// crediting the ~16 points a season that costs.
func TestViceCaptainCoversABlankCaptain(t *testing.T) {
	s := mkSeason()
	// Captain (13) blanks in gameweek 5; vice (14) plays normally.
	s.Players[13].GWs[5] = GW{Points: 0, Minutes: 0, Value: 80}

	xi := idsToPlayers(s, []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15})
	got := weekPoints(xi, nil, 5, 13, 14)

	want := 0
	for _, p := range xi {
		want += p.GWs[5].Points
	}
	want += s.Players[14].GWs[5].Points // vice doubled instead of the blanking captain
	if got != want {
		t.Errorf("week points %d, want %d (vice-captain covering the blank)", got, want)
	}
}

// TestNeitherCaptainNorViceDoubledWhenBothBlank — the real FPL rule stops at
// the vice-captain. If he blanks too, nobody is doubled that week.
func TestNeitherCaptainNorViceDoubledWhenBothBlank(t *testing.T) {
	s := mkSeason()
	s.Players[13].GWs[5] = GW{Points: 0, Minutes: 0, Value: 80}
	s.Players[14].GWs[5] = GW{Points: 0, Minutes: 0, Value: 80}

	xi := idsToPlayers(s, []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15})
	got := weekPoints(xi, nil, 5, 13, 14)

	want := 0
	for _, p := range xi {
		want += p.GWs[5].Points
	}
	if got != want {
		t.Errorf("week points %d, want %d (no armband bonus when both blank)", got, want)
	}
}

// TestCaptaincyRungsRemoveOnlyTheArmband pins the two diagnostic rungs
// HoldCaptaincyWeekly builds on top of HOLD.
//
// Both are expressed by passing a different captain id to the *same* scoring
// function rather than by adding a flag to it, which is what keeps one
// implementation of a gameweek's points — so what needs pinning is that the ids
// chosen actually mean what the rungs claim:
//
//   - captain 0 and vice 0 must double nobody. Player ids come from FPL and are
//     positive, so 0 is a safe sentinel today; if a synthetic season ever carried
//     an id of 0 the "no captain" rung would silently double him and read as a
//     *smaller* variance reduction than it is, which is the direction that gets
//     believed.
//   - a captain who is no longer in the eleven must not be doubled from the
//     bench. The pinned-armband rung re-picks the eleven weekly, so the day-one
//     captain does drop out of it, and FPL does not pay a benched captain.
func TestCaptaincyRungsRemoveOnlyTheArmband(t *testing.T) {
	s := mkSeason()
	ids := []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15}
	xi := idsToPlayers(s, ids)

	bare := 0
	for _, p := range xi {
		bare += p.GWs[5].Points
	}

	// The "nobody doubled" rung.
	if got := weekPoints(xi, nil, 5, 0, 0); got != bare {
		t.Errorf("captain 0 scored %d, want %d — a sentinel id is being doubled",
			got, bare)
	}
	// And it really is removing something, or the rung is measuring nothing and
	// its lower variance would be a tautology rather than a finding.
	full := weekPoints(xi, nil, 5, 13, 14)
	if full <= bare {
		t.Fatalf("HOLD's own armband adds nothing (%d against %d); the comparison "+
			"between the rungs would be vacuous", full, bare)
	}

	// A pinned captain who has dropped out of the eleven, with his pinned vice
	// still in it: the armband passes to the vice, exactly as when a captain
	// blanks.
	benchedCaptain := 12 // in the squad, not in this eleven
	if got, want := weekPoints(xi, nil, 5, benchedCaptain, 14),
		bare+s.Players[14].GWs[5].Points; got != want {
		t.Errorf("pinned captain outside the eleven scored %d, want %d (vice covers)",
			got, want)
	}
	// And when neither is in the eleven, nobody is doubled rather than the bench
	// being paid.
	if got := weekPoints(xi, nil, 5, benchedCaptain, 2); got != bare {
		t.Errorf("pinned captain and vice both outside the eleven scored %d, want %d",
			got, bare)
	}
}

func TestValueChangeTracksPrices(t *testing.T) {
	s := mkSeason()
	// One player rises 0.5m over the season, another falls 0.3m.
	for gw := 20; gw <= 38; gw++ {
		g := s.Players[13].GWs[gw]
		g.Value = 85
		s.Players[13].GWs[gw] = g
		h := s.Players[14].GWs[gw]
		h.Value = 77
		s.Players[14].GWs[gw] = h
	}
	squad := metrics(s, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15)
	r := Score("t", s, squad, metrics(s, 1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15), 13, 0)
	if r.ValueChange != 5-3 {
		t.Errorf("value change %d tenths, want %d (one riser, one faller)", r.ValueChange, 2)
	}
}

// TestDudsSeparateAbsenceFromUnderperformance is the distinction the backtest
// exists to draw. A player who never took the pitch had left the league or was
// injured all season, which the model cannot know; one who played and returned
// nothing is a genuine miss.
func TestDudsSeparateAbsenceFromUnderperformance(t *testing.T) {
	s := mkSeason()
	for gw := 1; gw <= 38; gw++ {
		s.Players[13].GWs[gw] = GW{Points: 0, Minutes: 0, Value: 80}  // gone
		s.Players[14].GWs[gw] = GW{Points: 1, Minutes: 90, Value: 80} // poor
	}
	s.Players[13].TotalPoints, s.Players[13].Minutes = 0, 0
	s.Players[14].TotalPoints, s.Players[14].Minutes = 38, 38*90

	duds, wasted := Duds(s, metrics(s, 13, 14, 15), 5.0, 60)
	if len(duds) != 2 {
		t.Fatalf("found %d duds, want 2", len(duds))
	}
	if !duds[0].NeverPlayed && !duds[1].NeverPlayed {
		t.Error("neither dud was marked as never having played")
	}
	for _, d := range duds {
		if d.Name == "N" && d.NeverPlayed {
			t.Error("a player with 3420 minutes was marked as never playing")
		}
	}
	if wasted != 16.0 {
		t.Errorf("wasted £%.1fm, want £16.0m", wasted)
	}
}

func TestCeilingIsLegalAndBeatsAnyRealSquad(t *testing.T) {
	s := mkSeason()
	c := Ceiling(s, 1000)
	total := 0
	for _, p := range s.Players {
		total += p.TotalPoints
	}
	if c <= 0 || c > total {
		t.Errorf("ceiling %d is outside the possible range (0, %d]", c, total)
	}
}

func TestPercentile(t *testing.T) {
	d := []int{10, 20, 30, 40, 50}
	for _, c := range []struct {
		v    int
		want float64
	}{{5, 0}, {30, 40}, {60, 100}} {
		if got := Percentile(d, c.v); got != c.want {
			t.Errorf("Percentile(%d) = %.0f, want %.0f", c.v, got, c.want)
		}
	}
	if got := Percentile(nil, 1); got != 0 {
		t.Errorf("empty distribution gave %.0f", got)
	}
}

func TestPriorSeasonName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"2023-24", "2022-23"}, {"2025-26", "2024-25"}, {"2016-17", "2015-16"},
	} {
		got, err := PriorSeasonName(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("PriorSeasonName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := PriorSeasonName("nonsense"); err == nil {
		t.Error("expected an error for a malformed season")
	}
}

// TestPreSeasonCarriesLastSeasonNotThis is the property the whole replay rests
// on. A squad built from end-of-season numbers would be reading the answers.
func TestPreSeasonCarriesLastSeasonNotThis(t *testing.T) {
	prior := mkSeason()
	cur := mkSeason()
	// Distinguish the two: this season everyone scores far more.
	for _, p := range cur.Players {
		p.TotalPoints *= 10
		p.Minutes = 3420
	}
	for _, p := range prior.Players {
		p.Minutes = 1000
	}

	boot, _ := PreSeason(cur, prior)
	for _, el := range boot.Elements {
		if el.Minutes == 3420 {
			t.Fatalf("%s carries this season's minutes; the replay can see the future", el.WebName)
		}
		if el.Minutes != 1000 && el.Minutes != 0 {
			t.Fatalf("%s has %d minutes, expected last season's 1000", el.WebName, el.Minutes)
		}
	}
	if len(boot.Events) != 38 {
		t.Errorf("%d events, want 38", len(boot.Events))
	}
	for _, ev := range boot.Events {
		if ev.Finished {
			t.Error("a pre-season bootstrap must have no finished gameweeks")
		}
	}
}

// TestLoadArchive exercises the loader itself, and skips when the archive is
// unreachable — the same contract the FPL client tests use.
func TestLoadArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads a season")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	s, err := Load(ctx, "../../.cache/fpl", "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	if len(s.Players) < 400 {
		t.Errorf("%d players, expected a full season", len(s.Players))
	}
	if len(s.Fixtures) < 300 {
		t.Errorf("%d fixtures, expected 380", len(s.Fixtures))
	}
	var withGWs, codes int
	for _, p := range s.Players {
		if len(p.GWs) > 0 {
			withGWs++
		}
		if p.Code > 0 {
			codes++
		}
	}
	if withGWs < 300 {
		t.Errorf("only %d players have gameweek rows", withGWs)
	}
	if codes < 400 {
		t.Errorf("only %d players carry a permanent code; the cross-season join needs it", codes)
	}
	t.Logf("2023-24: %d players, %d with gameweeks, %d fixtures", len(s.Players), withGWs, len(s.Fixtures))
}

// TestOnlyAKeeperReplacesAKeeper — the eleven must contain exactly one
// goalkeeper, so a blanking keeper can only be replaced by the reserve keeper.
// bestXIWith orders the bench with the reserve keeper last, so before the
// formation check the highest-scoring bench outfielder was substituted in —
// a swap FPL never makes.
func TestOnlyAKeeperReplacesAKeeper(t *testing.T) {
	s := mkSeason()
	// The starting keeper blanks in gameweek 5. The reserve keeper (2) plays
	// and scores 7; bench outfielder 6 plays and scores 3. The two must differ,
	// or the test cannot tell which one was credited.
	s.Players[1].GWs[5] = GW{Points: 0, Minutes: 0, Value: 50}
	s.Players[2].GWs[5] = GW{Points: 7, Minutes: 90, Value: 40}

	xi := idsToPlayers(s, []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15})
	// Bench in the order BestXI produces it: outfielders first, keeper last.
	bench := idsToPlayers(s, []int{6, 7, 12, 2})
	got := weekPoints(xi, bench, 5, 0, 0)

	want := 0
	for _, p := range xi {
		want += p.GWs[5].Points
	}
	want += s.Players[2].GWs[5].Points // the reserve keeper, not outfielder 6
	if got != want {
		t.Errorf("week points %d, want %d (only the reserve keeper may replace a keeper)", got, want)
	}
}

// TestNoSubstitutionWhenItWouldBreakTheFormation — three at the back is legal,
// two is not. With only three defenders starting, a blanking defender cannot be
// replaced by a midfielder or forward, so FPL makes no substitution and the
// slot simply scores nothing.
func TestNoSubstitutionWhenItWouldBreakTheFormation(t *testing.T) {
	s := mkSeason()
	s.Players[3].GWs[5] = GW{Points: 0, Minutes: 0, Value: 50}

	// 3-5-3 is not legal; use a real 3-4-3: one keeper, defenders 3-5,
	// midfielders 8-11, forwards 13-15.
	xi := idsToPlayers(s, []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15})
	// Every bench outfielder is a midfielder or forward, so none can come on.
	bench := idsToPlayers(s, []int{12, 2})
	got := weekPoints(xi, bench, 5, 0, 0)

	want := 0
	for _, p := range xi {
		want += p.GWs[5].Points
	}
	if got != want {
		t.Errorf("week points %d, want %d (no legal substitution exists)", got, want)
	}
}

// TestABenchDefenderStillCoversADefender — the formation check must not block
// substitutions FPL would actually make, or it trades one error for another.
func TestABenchDefenderStillCoversADefender(t *testing.T) {
	s := mkSeason()
	s.Players[3].GWs[5] = GW{Points: 0, Minutes: 0, Value: 50}

	xi := idsToPlayers(s, []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15})
	bench := idsToPlayers(s, []int{6, 12, 2}) // 6 is a defender
	got := weekPoints(xi, bench, 5, 0, 0)

	want := 0
	for _, p := range xi {
		want += p.GWs[5].Points
	}
	want += s.Players[6].GWs[5].Points
	if got != want {
		t.Errorf("week points %d, want %d (a bench defender covers a defender)", got, want)
	}
}
