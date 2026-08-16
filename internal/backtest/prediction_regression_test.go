package backtest

// Regression tests for the prediction benchmark. These run in the ordinary suite
// — no DIAG, no network, no archive — because each pins a property that fails
// silently.

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"armband/internal/fpl"
)

// TestReturnCategoriesMatchThePublishedDefinitions pins OpenFPL's boundaries.
//
// The whole point of borrowing them is that our figures sit beside published ones,
// and the trap is specific: **Zeros is no MINUTES, not no POINTS.** A player who
// came on for ten minutes and returned one point is a Blank. Getting that wrong
// would move thousands of observations between the two easiest categories and
// leave every number still looking plausible.
func TestReturnCategoriesMatchThePublishedDefinitions(t *testing.T) {
	cases := []struct {
		minutes, points int
		want            string
		why             string
	}{
		{0, 0, catZeros, "did not play"},
		{0, 1, catZeros, "no minutes wins even if FPL somehow paid a point"},
		{10, 1, catBlanks, "played, so not a Zero, however few points"},
		{90, 2, catBlanks, "two points is the top of Blanks"},
		{90, -1, catBlanks, "a red card is a Blank, not a Zero: he played"},
		{90, 3, catTickers, "three points is the bottom of Tickers"},
		{62, 4, catTickers, "four points is the top of Tickers"},
		{90, 5, catHaulers, "five points is the bottom of Haulers"},
		{90, 17, catHaulers, ""},
	}
	for _, c := range cases {
		if got := returnCategory(c.minutes, c.points); got != c.want {
			t.Errorf("returnCategory(%d minutes, %d points) = %q, want %q — %s",
				c.minutes, c.points, got, c.want, c.why)
		}
	}
}

// TestErrorDecompositionIsExact pins the identity the whole
// bias-reduction-versus-variance-trade verdict rests on.
//
// mean squared error = bias² + error-spread². If that stops holding, the verdict
// column becomes arbitrary while still printing a confident word, which is worse
// than printing nothing.
func TestErrorDecompositionIsExact(t *testing.T) {
	a := &errAcc{}
	for _, p := range [][2]float64{
		{5.1, 2}, {3.0, 9}, {1.2, 0}, {0.4, 0}, {6.8, 13}, {2.2, 2}, {4.4, 5},
	} {
		a.add(p[0], p[1])
	}
	mse := a.sumSq / float64(a.n)
	got := a.bias()*a.bias() + a.errorSD()*a.errorSD()
	if math.Abs(got-mse) > 1e-12 {
		t.Fatalf("bias² + spread² = %.15f but mean squared error = %.15f; the "+
			"candidate verdict is derived from this identity and is meaningless "+
			"without it", got, mse)
	}
	if math.Abs(a.rmse()*a.rmse()-mse) > 1e-12 {
		t.Fatalf("rmse² = %.15f, mean squared error = %.15f", a.rmse()*a.rmse(), mse)
	}
}

// TestSpearmanHandlesTiesAndDegenerateInput pins the rank correlation.
//
// Ties are not a technicality here: a gameweek's realised points are full of them
// — dozens of players on exactly 2 — and ranking tied values arbitrarily would
// inject an ordering the data does not contain, inflating the correlation for
// free.
func TestSpearmanHandlesTiesAndDegenerateInput(t *testing.T) {
	if rho, ok := spearman([]float64{1, 2, 3, 4}, []float64{10, 20, 30, 40}); !ok ||
		math.Abs(rho-1) > 1e-12 {
		t.Errorf("a perfectly agreeing ordering should be +1, got %v (ok=%v)", rho, ok)
	}
	if rho, ok := spearman([]float64{1, 2, 3, 4}, []float64{40, 30, 20, 10}); !ok ||
		math.Abs(rho+1) > 1e-12 {
		t.Errorf("a perfectly reversed ordering should be −1, got %v (ok=%v)", rho, ok)
	}
	// Monotone but not linear: Spearman must still be exactly +1 where Pearson
	// would not be. This is the reason for using ranks at all.
	if rho, ok := spearman([]float64{1, 2, 3, 4}, []float64{1, 4, 9, 16}); !ok ||
		math.Abs(rho-1) > 1e-12 {
		t.Errorf("a monotone non-linear relation should still be +1, got %v", rho)
	}
	// A constant column has no ordering to agree with, which is not the same as
	// agreeing zero.
	if _, ok := spearman([]float64{1, 1, 1, 1}, []float64{1, 2, 3, 4}); ok {
		t.Error("a constant input has no ordering; spearman should report ok=false")
	}
	if _, ok := spearman([]float64{1, 2}, []float64{1}); ok {
		t.Error("mismatched lengths should report ok=false")
	}
	// Tied ranks must average, and every cell's ranks must still total
	// n(n+1)/2 — the invariant that catches an off-by-one in the tie loop.
	got := tiedRanks([]float64{5, 5, 1, 9})
	want := []float64{2.5, 2.5, 1, 4}
	var sum float64
	for i := range got {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Errorf("tiedRanks position %d = %v, want %v", i, got[i], want[i])
		}
		sum += got[i]
	}
	if math.Abs(sum-10) > 1e-12 {
		t.Errorf("ranks of four values must total 10, got %v", sum)
	}
}

// TestTailSignedErrorReadsTheTopOfThePrediction pins the winner's-curse figure.
//
// It must select on the PREDICTION and not on the outcome — selecting on the
// outcome would make it a measure of nothing, since the highest-scoring players
// are always under-predicted by construction — and positive must mean the top of
// the predicted distribution is over-rated.
func TestTailSignedErrorReadsTheTopOfThePrediction(t *testing.T) {
	pred := []float64{9, 8, 1, 1, 1}
	act := []float64{2, 2, 9, 9, 9}
	got, ok := tailSignedError(pred, act, 2)
	if !ok {
		t.Fatal("tailSignedError refused a valid input")
	}
	// The two highest predictions were 9 and 8 against outcomes of 2 and 2, so
	// the tail is over-rated by (7 + 6) / 2.
	if math.Abs(got-6.5) > 1e-12 {
		t.Fatalf("tail signed error = %v, want +6.5. If it is negative the "+
			"selection is reading the outcome rather than the prediction, which "+
			"measures nothing.", got)
	}
	// Asking for more than exist is a legitimate small gameweek, not an error.
	if v, ok := tailSignedError(pred, act, 99); !ok || math.IsNaN(v) {
		t.Errorf("a tail wider than the population should fall back to all of it, got %v", v)
	}
	if _, ok := tailSignedError(nil, nil, 5); ok {
		t.Error("an empty gameweek should report ok=false rather than a figure")
	}
}

// TestPredictionPopulationCountsClubGameweeks pins the two filters that decide
// the sample, both of which fail silently.
//
// The club-fixture restriction is what makes a missing per-gameweek row an
// unambiguous zero rather than a blank gameweek in disguise. And the
// squad-relevance filter counts back over *club* gameweeks, so a club with a
// blank does not have its players quietly aged out of the population.
func TestPredictionPopulationCountsClubGameweeks(t *testing.T) {
	// A club that plays every gameweek except 4, and twice in 6.
	clubGWs := map[int]int{1: 1, 2: 1, 3: 1, 5: 1, 6: 2, 7: 1, 8: 1, 9: 1, 10: 1}

	// He played a full match in GW1 and nothing since. Counting back five *club*
	// gameweeks from GW8 reaches 7, 6, 5, 3, 2 — not GW1, because GW4 does not
	// count.
	p := &Player{GWs: map[int]GW{1: {Starts: 1, Minutes: 90, Fixtures: 1}}}
	if playedRecently(p, clubGWs, 8) {
		t.Error("a full match in GW1 should not make a player squad-relevant in GW8")
	}
	if !playedRecently(p, clubGWs, 6) {
		t.Error("counting back five club gameweeks from GW6 reaches GW1 (5,3,2,1 " +
			"after skipping 4), so he should still be relevant")
	}

	// The last-five baseline skips gameweeks the club did not play and counts
	// gameweeks it played in which he has no row as a genuine zero.
	q := &Player{GWs: map[int]GW{
		5: {Points: 6, Fixtures: 1},
		6: {Points: 4, Fixtures: 2},
		7: {Points: 2, Fixtures: 1},
	}}
	// From GW8, the last five club gameweeks are 7, 6, 5, 3, 2 — so (2+4+6+0+0)/5.
	if got := meanRecentClubGWs(q, clubGWs, 8, 5, gwPoints); math.Abs(got-2.4) > 1e-12 {
		t.Errorf("mean of the last five club gameweeks = %v, want 2.4 — a gameweek "+
			"the club played with no row for him is a zero, and GW4 must not be "+
			"counted at all", got)
	}
	// The season-to-date average uses the same convention, over GW1-7: five club
	// gameweeks with no row plus 6, 4, 2 across seven club gameweeks.
	if got := meanSeasonToDate(q, clubGWs, 8, gwPoints); math.Abs(got-12.0/6.0) > 1e-12 {
		t.Errorf("season-to-date mean = %v, want %v", got, 12.0/6.0)
	}
}

// TestSquadRelevanceDoesNotDependOnTheStartsColumn pins the fix for the bug this
// benchmark shipped with for one run.
//
// The archive's per-gameweek `starts` column is zero for the whole of 2021-22 and
// for 2022-23 up to GW15, verified directly against `gws/merged_gw.csv`. A
// squad-relevance filter reading it therefore admitted nobody in 2022-23 before
// GW20 and silently reduced a four-season figure to three, while every printed
// table stayed plausible. So the filter must read minutes, and this pins it with a
// player shaped exactly like a 2022-23 row: ninety minutes played and no start
// recorded.
func TestSquadRelevanceDoesNotDependOnTheStartsColumn(t *testing.T) {
	clubGWs := map[int]int{1: 1, 2: 1, 3: 1, 4: 1, 5: 1, 6: 1}
	ninetyNoStart := &Player{GWs: map[int]GW{
		5: {Minutes: 90, Starts: 0, Fixtures: 1},
	}}
	if !playedRecently(ninetyNoStart, clubGWs, 6) {
		t.Fatal("a player who played ninety minutes with no `starts` recorded must " +
			"still be squad-relevant. The archive leaves that column empty for all " +
			"of 2021-22 and for 2022-23 before GW16, so a starts-based filter " +
			"deletes a season and a half from the sample without changing how any " +
			"table looks.")
	}
	// A cameo is not squad relevance: sixty minutes is the bar, because that is
	// where FPL pays appearance points and the clean sheet.
	cameo := &Player{GWs: map[int]GW{5: {Minutes: 20, Fixtures: 1}}}
	if playedRecently(cameo, clubGWs, 6) {
		t.Error("a twenty-minute cameo should not count as squad relevance")
	}
}

// TestClubGameweeksCountsDoublesAndBlanks pins the fixture-count map the
// population filter is built from.
//
// A double gameweek must read 2 and a blank must be absent, because that
// distinction is what the whole population definition rests on. It is the same
// distinction the replay's own doubles bug turned on, arriving from a different
// direction: there, one row per fixture was being assigned instead of accumulated.
func TestClubGameweeksCountsDoublesAndBlanks(t *testing.T) {
	gw := func(n int) *int { return &n }
	s := &Season{Fixtures: []fpl.Fixture{
		{Event: gw(1), TeamH: 1, TeamA: 2},
		{Event: gw(2), TeamH: 2, TeamA: 3}, // club 1 blanks in GW2
		{Event: gw(3), TeamH: 1, TeamA: 3},
		{Event: gw(3), TeamH: 2, TeamA: 1}, // club 1 has a double in GW3
	}}
	got := clubGameweeks(s)
	if got[1][1] != 1 {
		t.Errorf("club 1 plays once in GW1, got %d", got[1][1])
	}
	if got[1][2] != 0 {
		t.Errorf("club 1 blanks in GW2 and must be absent, got %d — otherwise a "+
			"blank gameweek enters the sample as a false zero", got[1][2])
	}
	if got[1][3] != 2 {
		t.Errorf("club 1 has a double in GW3, got %d — a double must read 2 or the "+
			"per-gameweek unit is wrong for exactly the weeks chips are played",
			got[1][3])
	}
}

// TestPredictionCellsSumToTheReportedTotals pins the CSV against the printed
// tables.
//
// The sink accumulates per gameweek and the report accumulates over the whole
// grid. Those are not two implementations of one quantity — they are the same
// arithmetic at two granularities, and the coarse one must be exactly the sum of
// the fine ones. If they drift, the R layer's inference describes a different
// population from the tables a human reads, which is precisely the orphaned-figure
// failure this project keeps recording.
func TestPredictionCellsSumToTheReportedTotals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prediction.csv")
	sink, err := openPredictionSink(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := [][]playerGW{
		{
			{id: 1, relevant: true, actPoints: 2, actMinutes: 90, actXGI: 0.3,
				category: catBlanks, points: []float64{4.1, 3.0, 2.5},
				minutes: []float64{85, 80, 75}, xgi: []float64{0.4, 0.2, 0.25}},
			{id: 2, relevant: false, actPoints: 0, actMinutes: 0, actXGI: 0,
				category: catZeros, points: []float64{0.9, 0.2, 0.1},
				minutes: []float64{12, 5, 3}, xgi: []float64{0.02, 0, 0}},
		},
		{
			{id: 1, relevant: true, actPoints: 9, actMinutes: 90, actXGI: 1.1,
				category: catHaulers, points: []float64{4.4, 3.4, 2.7},
				minutes: []float64{86, 90, 82}, xgi: []float64{0.45, 0.3, 0.28}},
			{id: 3, relevant: true, actPoints: 3, actMinutes: 61, actXGI: 0.1,
				category: catTickers, points: []float64{2.2, 2.0, 1.8},
				minutes: []float64{60, 55, 58}, xgi: []float64{0.1, 0.08, 0.09}},
		},
	}
	run := newPredRun(armShipped)
	for i, r := range rows {
		foldPredictions(run, r)
		sink.emitGameweek(armShipped, "2024-25", "2023-24", 6+i, r)
	}
	sink.close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rd := csv.NewReader(f)
	head, err := rd.Read()
	if err != nil {
		t.Fatal(err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	recs, err := rd.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) == 0 {
		t.Fatal("the sink wrote no rows at all")
	}

	type total struct {
		n         int
		abs, sq   float64
		pred, act float64
	}
	sums := map[string]*total{}
	for _, rec := range recs {
		k := predKey(rec[col["population"]], rec[col["target"]],
			rec[col["predictor"]], rec[col["category"]])
		s := sums[k]
		if s == nil {
			s = &total{}
			sums[k] = s
		}
		n, _ := strconv.Atoi(rec[col["n"]])
		fl := func(name string) float64 {
			v, _ := strconv.ParseFloat(rec[col[name]], 64)
			return v
		}
		s.n += n
		s.abs += fl("sum_abs_err")
		s.sq += fl("sum_sq_err")
		s.pred += fl("sum_pred")
		s.act += fl("sum_act")
	}

	if len(sums) != len(run.err) {
		t.Fatalf("the CSV covers %d (population, target, predictor, category) cells "+
			"and the report accumulated %d; one of them is dropping a cell",
			len(sums), len(run.err))
	}
	for k, want := range run.err {
		got := sums[k]
		if got == nil {
			t.Errorf("%s is in the report and absent from the CSV", k)
			continue
		}
		if got.n != want.n {
			t.Errorf("%s: CSV n = %d, report n = %d", k, got.n, want.n)
		}
		for _, c := range []struct {
			name      string
			got, want float64
		}{
			{"sum_abs_err", got.abs, want.sumAbs},
			{"sum_sq_err", got.sq, want.sumSq},
			{"sum_pred", got.pred, want.sumPred},
			{"sum_act", got.act, want.sumAct},
		} {
			if math.Abs(c.got-c.want) > 1e-9 {
				t.Errorf("%s: CSV %s = %.12f, report = %.12f", k, c.name, c.got, c.want)
			}
		}
	}
}

// TestPredictionCellsCarryTheGameweekScalarsExactlyOnce pins the blank-versus-zero
// rule on the two columns that cannot be summed.
//
// The rank correlation and the tail signed error are gameweek-level scalars, so
// they belong on exactly one row per (gameweek, population, predictor) and must be
// BLANK elsewhere. A zero there would be averaged by R as though it were measured,
// and an unmeasured quantity and one measured at zero are different facts.
func TestPredictionCellsCarryTheGameweekScalarsExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prediction.csv")
	sink, err := openPredictionSink(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := []playerGW{
		{id: 1, relevant: true, actPoints: 2, category: catBlanks,
			points: []float64{4.1, 3.0, 2.5}, minutes: []float64{85, 80, 75},
			xgi: []float64{0.4, 0.2, 0.25}},
		{id: 2, relevant: true, actPoints: 9, category: catHaulers,
			points: []float64{5.4, 3.4, 2.7}, minutes: []float64{86, 90, 82},
			xgi: []float64{0.45, 0.3, 0.28}},
		{id: 3, relevant: true, actPoints: 0, category: catZeros,
			points: []float64{1.1, 0.5, 0.4}, minutes: []float64{20, 10, 12},
			xgi: []float64{0.03, 0.01, 0.01}},
	}
	sink.emitGameweek(armShipped, "2024-25", "2023-24", 6, rows)
	sink.close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rd := csv.NewReader(f)
	head, _ := rd.Read()
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	recs, err := rd.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	filled := map[string]int{}
	for _, rec := range recs {
		hasRank := rec[col["rank_corr"]] != ""
		hasTail := rec[col["tail_signed_err"]] != ""
		if hasRank != hasTail {
			t.Errorf("row %v has one gameweek scalar and not the other; they are "+
				"measured together and must be reported together", rec)
		}
		if !hasRank {
			continue
		}
		if rec[col["target"]] != "points" {
			t.Errorf("the gameweek scalars are about points and appeared on target %q",
				rec[col["target"]])
		}
		if rec[col["category"]] != catAll {
			t.Errorf("the gameweek scalars belong on the all-categories row and "+
				"appeared on %q — a per-category copy would be double-counted",
				rec[col["category"]])
		}
		filled[predKey(rec[col["population"]], rec[col["predictor"]])]++
	}
	if len(filled) == 0 {
		t.Fatal("no row carried the gameweek scalars at all")
	}
	for k, n := range filled {
		if n != 1 {
			t.Errorf("%s carries the gameweek scalars on %d rows; exactly one is "+
				"correct or R averages the same number several times", k, n)
		}
	}
}
