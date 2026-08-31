package backtest

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// A SYNTHETIC FIELD, SO A SQUAD HAS A RANK TO SIT AT
//
//	DIAG=1 go test ./internal/backtest -run TestDiagFieldModel -v -timeout 60m
//
// Every other diagnostic in this package asks whether the model beats a
// baseline squad, or itself, or a hindsight ceiling. None of them can answer
// "where would this squad have finished among real managers", because there is
// no population of real managers in the archive — only the players they held.
// This builds one: 5,000 synthetic managers per season, each a legal squad
// drawn the way an opening-week manager actually assembles one — by leaning on
// what everyone else is already doing, not by picking blind — so that a squad's
// season total can be located inside a distribution instead of reported alone.
//
// # What a "manager" is here
//
// A manager is fifteen player ids, drawn once before gameweek 1 with
// probability proportional to opening-week ownership within his position,
// legal under the same budget, squad-size and per-club rules a real entry
// obeys, and then played out for 38 gameweeks with a starting eleven and a
// captain chosen once, from that same ownership snapshot, and never touched
// again.
//
// # Why the squad is HELD rather than re-sampled every gameweek
//
// A fresh draw each week would answer a different question: "what does the
// ownership-weighted population look like this week", not "how would the
// squads people actually settled on in August have fared". Persistence is
// what makes a season total mean anything — it is the same fifteen names
// scoring in gameweek 38 that were bought in gameweek 1, exactly as a real
// entry (absent transfers, which this deliberately does not model) would be.
// Re-sampling per gameweek would also make the RNG sequence, and therefore
// the whole field, non-reproducible in the way a fresh coin flip every week
// always is: nothing would tie one run's gameweek-19 field to the next run's.
//
// # Why the starting eleven and captain are ALSO held, not recomputed weekly
//
// Ownership itself is captured once — opening week — and never updated as the
// season plays out, for the same reason the squad is held: this is a
// template manager who set his team in August and never adjusted to the
// market moving. Recomputing "highest owned" week by week from a snapshot
// that itself never changes would recover exactly the same eleven and
// captain every time, at 38x the cost, so it is computed once and reused.
// A model that DID refresh ownership weekly would need to say why a manager
// tracks the market's opinion but never acts on it by transferring — a
// harder position than the one this file takes, which is that he does
// neither.
//
// # Simplifications, stated rather than discovered by a reader
//
//   - No transfers: the fifteen bought in gameweek 1 are the fifteen held in
//     gameweek 38.
//   - No chips: no bench boost, triple captain, free hit or wildcard.
//   - No autosubs: a starter with no gameweek row scores zero, in place,
//     rather than being covered by the bench.
//
// Every one of these makes the synthetic field an UNDER-estimate of what real
// managers, who do transfer and do occasionally get an autosub for free,
// actually score. The validation gate below is read with that in mind.
//
// # Reuse rather than reimplementation
//
// Ownership comes from ownershipAt (ownershipseam.go) — the one place the
// archive's raw owner count is turned into the percentage the live bootstrap
// publishes — called exactly as the sampler is told to: ownershipAt(cur, 1,
// registeredBy(cur, 0)), the opening-week snapshot PreSeason itself measures
// against. Price comes from registeredBy(cur, 0).price(p, 1), the same call
// PreSeason makes, so a synthetic manager can never buy a player who was not
// actually in the game at that price. Squad size, per-club limit and budget
// are analysis.SquadSize, analysis.MaxPerClub and analysis.DefaultBudget —
// this file names none of those numbers itself. Legal-formation checking is
// analysis.LegalFormation, exported from the analysis package for exactly
// this reason ("the replay needs it too").
//
// The one number this file DOES own is the positional draft quota (2 GKP, 5
// DEF, 5 MID, 3 FWD): analysis.squadQuota carries the same map but is
// unexported, and RandomSquads (replay.go) already restates it locally for
// the same reason. A second restatement is the existing pattern, not a new
// one.
//
// # THE VALIDATION GATE
//
// The synthetic 2025-26 field is compared against the real one: 3,612 actual
// FPL managers' final totals, summarised as mean 1922, sd 307, p05 1404, p25
// 1823, p50 1989, p75 2108, p95 2258. Three bands are checked and each is
// logged PASS or FAIL. This is a DIAGNOSTIC, not a guard: the test never
// fails on a FAIL verdict, and the sampler is never adjusted to chase one.
// Tuning the generator against the very distribution that is supposed to
// validate it would prove nothing except that the generator can be made to
// match — the failure mode this whole exercise exists to avoid. If the gate
// fails, that is itself the finding, and it is reported as one: which band
// missed, by how much, and in which direction the simplifications above
// would be expected to push it.
const (
	fieldN           = 5000 // synthetic managers per season
	fieldSeed        = 7    // fixed, not time-derived — see DETERMINISM below
	fieldMaxRedraws  = 200  // per manager; exhausting this counts as a failed build
	fieldOutDir      = "/work/drop/field-model-2026-08-30"
	fieldGWFirst     = 1
	fieldGWLast      = 38
	fieldGateSeason  = "2025-26"
	fieldRefMean     = 1922.0
	fieldRefSD       = 307.0
	fieldRefP05      = 1404.0
	fieldRefP25      = 1823.0
	fieldRefP50      = 1989.0
	fieldRefP75      = 2108.0
	fieldRefP95      = 2258.0
	fieldMeanBandPct = 0.10
	fieldSDBandPct   = 0.25
	fieldTailBandPct = 0.05
)

// fieldQuota is the positional draft quota. See the reuse note above: this
// restates squadQuota (analysis/squad.go), which is unexported, exactly as
// RandomSquads (replay.go) already does.
var fieldQuota = map[int]int{1: 2, 2: 5, 3: 5, 4: 3}

// fieldCandidate is one player available to the sampler: enough to draw him,
// price him and place him in a squad, and nothing the scoring pass needs from
// the live archive beyond his id.
type fieldCandidate struct {
	ID    int
	Type  int
	Team  int
	Price int
	Own   float64 // opening-week ownership share within his position's pool
}

func TestDiagFieldModel(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	if err := os.MkdirAll(fieldOutDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seasonRows := [][]string{{"season", "manager", "total"}}
	gwRows := [][]string{{"season", "gw", "p10", "p25", "p50", "p75", "p90", "p99", "mean", "sd"}}

	// DETERMINISM: one RNG, seeded from a fixed constant, consumed in a fixed
	// order (seasons in sweepPairNames order, managers 1..fieldN, positions
	// GKP/DEF/MID/FWD). A field that changed between runs could not be
	// validated or reproduced, so nothing here may read the clock or iterate
	// a map in an order that affects which candidate consumes which draw —
	// every candidate pool below is sorted by id before it is used.
	rng := rand.New(rand.NewSource(fieldSeed))

	var gateStats *fieldSeasonStats

	for _, pair := range sweepPairNames() {
		season := pair[1]
		cur := loadSeason(t, cfg, season)

		reg := registeredBy(cur, 0)
		own := ownershipAt(cur, 1, reg)

		pools := buildOwnershipPools(cur, reg, own)
		for typ, need := range fieldQuota {
			if len(pools[typ]) < need {
				t.Fatalf("%s: only %d type-%d candidates with positive opening ownership, need %d",
					season, len(pools[typ]), typ, need)
			}
		}

		byID := map[int]*Player{}
		for _, p := range cur.Players {
			byID[p.ID] = p
		}

		gwScores := make([][]float64, fieldGWLast+1) // index by gameweek
		var seasonTotals []float64
		failed := 0

		for m := 1; m <= fieldN; m++ {
			squad, ok := drawFieldManager(rng, pools)
			if !ok {
				failed++
				continue
			}
			xi, captain := pickFieldXI(squad)

			total := 0.0
			for gw := fieldGWFirst; gw <= fieldGWLast; gw++ {
				pts := scoreFieldWeek(byID, xi, captain, gw)
				gwScores[gw] = append(gwScores[gw], pts)
				total += pts
			}
			seasonTotals = append(seasonTotals, total)
			seasonRows = append(seasonRows, []string{season, strconv.Itoa(m), f0(total)})
		}

		for gw := fieldGWFirst; gw <= fieldGWLast; gw++ {
			s := gwScores[gw]
			sorted := append([]float64(nil), s...)
			sort.Float64s(sorted)
			mean, sd := meanSD(sorted)
			gwRows = append(gwRows, []string{
				season, strconv.Itoa(gw),
				f2(percentileOf(sorted, 10)), f2(percentileOf(sorted, 25)),
				f2(percentileOf(sorted, 50)), f2(percentileOf(sorted, 75)),
				f2(percentileOf(sorted, 90)), f2(percentileOf(sorted, 99)),
				f2(mean), f2(sd),
			})
		}

		sortedTotals := append([]float64(nil), seasonTotals...)
		sort.Float64s(sortedTotals)
		stats := &fieldSeasonStats{
			n:    len(sortedTotals),
			mean: 0, sd: 0,
		}
		stats.mean, stats.sd = meanSD(sortedTotals)
		stats.p05 = percentileOf(sortedTotals, 5)
		stats.p25 = percentileOf(sortedTotals, 25)
		stats.p50 = percentileOf(sortedTotals, 50)
		stats.p75 = percentileOf(sortedTotals, 75)
		stats.p95 = percentileOf(sortedTotals, 95)

		t.Logf("%s: %d/%d managers built (%d failed to build after %d redraws)",
			season, stats.n, fieldN, failed, fieldMaxRedraws)
		t.Logf("%s season totals: mean=%.1f sd=%.1f p05=%.1f p25=%.1f p50=%.1f p75=%.1f p95=%.1f",
			season, stats.mean, stats.sd, stats.p05, stats.p25, stats.p50, stats.p75, stats.p95)

		if season == fieldGateSeason {
			gateStats = stats
		}
	}

	seasonPath := fieldOutDir + "/season_totals.csv"
	writeFieldCSV(t, seasonPath, seasonRows)
	t.Logf("wrote %s (%d rows)", seasonPath, len(seasonRows)-1)

	gwPath := fieldOutDir + "/gw_totals.csv"
	writeFieldCSV(t, gwPath, gwRows)
	t.Logf("wrote %s (%d rows)", gwPath, len(gwRows)-1)

	if gateStats == nil {
		t.Fatalf("%s never appeared as a sweepPairNames cur season — the gate has nothing to compare", fieldGateSeason)
	}
	reportFieldGate(t, gateStats)
}

// fieldSeasonStats is one season's synthetic-field summary, kept around long
// enough to feed the validation gate after the season loop that computed it
// has moved on to the next season.
type fieldSeasonStats struct {
	n                       int
	mean, sd                float64
	p05, p25, p50, p75, p95 float64
}

// buildOwnershipPools groups registered players by position into candidate
// pools, each sorted by id so the draw that follows consumes the RNG in an
// order that does not depend on Go's randomised map iteration.
//
// A candidate with zero opening-week ownership is dropped rather than kept
// at some floor weight. "Probability proportional to ownership" means a
// player nobody held has zero probability of being drawn — an honest
// consequence of the rule, not a data gap to paper over.
func buildOwnershipPools(cur *Season, reg registration, own ownershipShares) map[int][]fieldCandidate {
	pools := map[int][]fieldCandidate{}
	for _, p := range cur.Players {
		if !reg.has(p.ID) {
			continue
		}
		share := own.pct[p.ID]
		if share <= 0 {
			continue
		}
		price := reg.price(p, 1)
		if price <= 0 {
			continue
		}
		pools[p.Type] = append(pools[p.Type], fieldCandidate{
			ID: p.ID, Type: p.Type, Team: p.Team, Price: price, Own: share,
		})
	}
	for typ := range pools {
		sort.Slice(pools[typ], func(i, j int) bool { return pools[typ][i].ID < pools[typ][j].ID })
	}
	return pools
}

// drawFieldManager draws one legal fifteen: the positional quota by weighted
// sampling without replacement per position, then checked as a whole against
// budget and the per-club limit. A violation redraws the ENTIRE fifteen from
// scratch — not just the offending pick — up to fieldMaxRedraws times, and no
// constraint is ever loosened to make a draw succeed.
func drawFieldManager(rng *rand.Rand, pools map[int][]fieldCandidate) ([]fieldCandidate, bool) {
	for attempt := 0; attempt < fieldMaxRedraws; attempt++ {
		var squad []fieldCandidate
		for _, typ := range [4]int{1, 2, 3, 4} {
			squad = append(squad, weightedDrawNoReplace(rng, pools[typ], fieldQuota[typ])...)
		}

		spend, legal := 0, true
		club := map[int]int{}
		for _, c := range squad {
			spend += c.Price
			club[c.Team]++
			if club[c.Team] > analysis.MaxPerClub {
				legal = false
			}
		}
		if legal && spend <= analysis.DefaultBudget && len(squad) == analysis.SquadSize {
			return squad, true
		}
	}
	return nil, false
}

// weightedDrawNoReplace draws k of pool without replacement, each draw
// proportional to the ownership share of what remains — the textbook
// sequential definition of "sampled with probability proportional to weight",
// rather than an approximation of it. pool is never mutated; a private copy
// is drawn down instead.
func weightedDrawNoReplace(rng *rand.Rand, pool []fieldCandidate, k int) []fieldCandidate {
	remaining := append([]fieldCandidate(nil), pool...)
	out := make([]fieldCandidate, 0, k)
	for i := 0; i < k && len(remaining) > 0; i++ {
		total := 0.0
		for _, c := range remaining {
			total += c.Own
		}
		r := rng.Float64() * total
		idx, cum := len(remaining)-1, 0.0
		for j, c := range remaining {
			cum += c.Own
			if r < cum {
				idx = j
				break
			}
		}
		out = append(out, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}
	return out
}

// pickFieldXI chooses the eleven highest-owned of the fifteen subject to
// analysis.LegalFormation, and captains the highest-owned of THOSE eleven.
//
// The search enumerates every (GKP, DEF, MID, FWD) split analysis.LegalFormation
// allows and takes the split maximising summed ownership of the top-n players
// at each position — the same "which formation, then fill it with the best
// available" shape bestFormation (analysis/squad.go) uses for Score, run here
// on ownership instead because this file has no PlayerMetrics and no engine to
// build one from.
func pickFieldXI(squad []fieldCandidate) (xi []fieldCandidate, captain int) {
	byType := map[int][]fieldCandidate{}
	for _, c := range squad {
		byType[c.Type] = append(byType[c.Type], c)
	}
	for typ := range byType {
		sort.SliceStable(byType[typ], func(i, j int) bool {
			a, b := byType[typ][i], byType[typ][j]
			if a.Own != b.Own {
				return a.Own > b.Own
			}
			return a.ID < b.ID // deterministic tiebreak
		})
	}

	prefix := map[int][]float64{}
	for typ, cs := range byType {
		p := make([]float64, len(cs)+1)
		for i, c := range cs {
			p[i+1] = p[i] + c.Own
		}
		prefix[typ] = p
	}

	bestVal := -1.0
	var bestCounts map[string]int
	for g := 0; g <= len(byType[1]); g++ {
		for d := 0; d <= len(byType[2]); d++ {
			for mi := 0; mi <= len(byType[3]); mi++ {
				for fw := 0; fw <= len(byType[4]); fw++ {
					counts := map[string]int{"GKP": g, "DEF": d, "MID": mi, "FWD": fw}
					if !analysis.LegalFormation(counts) {
						continue
					}
					val := prefix[1][g] + prefix[2][d] + prefix[3][mi] + prefix[4][fw]
					if val > bestVal {
						bestVal = val
						bestCounts = counts
					}
				}
			}
		}
	}
	if bestCounts == nil {
		// Cannot happen for a squad built to quota (2/5/5/3 always admits
		// 1/4/4/2 minimum, which analysis.LegalFormation accepts), and is a
		// programming error rather than a data condition if it ever does.
		panic("pickFieldXI: no legal formation found for a quota-built squad")
	}

	typeOf := map[string]int{"GKP": 1, "DEF": 2, "MID": 3, "FWD": 4}
	for name, typ := range typeOf {
		n := bestCounts[name]
		xi = append(xi, byType[typ][:n]...)
	}

	captain = xi[0].ID
	best := xi[0].Own
	for _, c := range xi {
		if c.Own > best || (c.Own == best && c.ID < captain) {
			best, captain = c.Own, c.ID
		}
	}
	return xi, captain
}

// scoreFieldWeek is what the held eleven returned in one gameweek: each
// starter's points, captain doubled, zero for anyone with no row that week —
// there is no autosub to cover him.
func scoreFieldWeek(byID map[int]*Player, xi []fieldCandidate, captain, gw int) float64 {
	total := 0.0
	for _, c := range xi {
		p := byID[c.ID]
		pts := 0
		if p != nil {
			if g, ok := p.GWs[gw]; ok {
				pts = g.Points
			}
		}
		if c.ID == captain {
			pts *= 2
		}
		total += float64(pts)
	}
	return total
}

// meanSD returns the mean and population standard deviation of an ALREADY
// SORTED (or unsorted — order does not matter here) slice. Population rather
// than sample (n rather than n-1) purely for simplicity: at n on the order of
// a few thousand the two differ by well under a point, far inside the bands
// the validation gate checks.
func meanSD(vals []float64) (mean, sd float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(len(vals))
	var sq float64
	for _, v := range vals {
		d := v - mean
		sq += d * d
	}
	sd = math.Sqrt(sq / float64(len(vals)))
	return mean, sd
}

// percentileOf reads a percentile out of an ascending-sorted slice by linear
// interpolation between the two nearest ranks (the common "R type 7" /
// numpy-default convention) — chosen because it needs no separate rule for
// percentiles that land exactly on an element and ones that do not.
func percentileOf(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	rank := (p / 100) * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo < 0 {
		lo = 0
	}
	if hi >= n {
		hi = n - 1
	}
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// reportFieldGate is the explicit, reported PASS/FAIL comparison against the
// real 2025-26 distribution. It never fails the test — see the doc comment on
// TestDiagFieldModel for why a diagnostic reporting a measurement must be
// allowed to report a bad one.
func reportFieldGate(t *testing.T, s *fieldSeasonStats) {
	t.Helper()

	meanErr := math.Abs(s.mean-fieldRefMean) / fieldRefMean
	sdErr := math.Abs(s.sd-fieldRefSD) / fieldRefSD
	p50Err := math.Abs(s.p50-fieldRefP50) / fieldRefP50
	p95Err := math.Abs(s.p95-fieldRefP95) / fieldRefP95

	meanPass := meanErr <= fieldMeanBandPct
	sdPass := sdErr <= fieldSDBandPct
	tailPass := p50Err <= fieldTailBandPct && p95Err <= fieldTailBandPct

	verdict := func(ok bool) string {
		if ok {
			return "PASS"
		}
		return "FAIL"
	}

	t.Log("")
	t.Log("=== FIELD MODEL VALIDATION GATE (synthetic 2025-26 vs 3,612 real managers) ===")
	t.Logf("  mean: synthetic=%.1f real=%.1f (%.1f%% off, band ±%.0f%%) -> %s",
		s.mean, fieldRefMean, 100*meanErr, 100*fieldMeanBandPct, verdict(meanPass))
	t.Logf("  sd:   synthetic=%.1f real=%.1f (%.1f%% off, band ±%.0f%%) -> %s",
		s.sd, fieldRefSD, 100*sdErr, 100*fieldSDBandPct, verdict(sdPass))
	t.Logf("  p50:  synthetic=%.1f real=%.1f (%.1f%% off)", s.p50, fieldRefP50, 100*p50Err)
	t.Logf("  p95:  synthetic=%.1f real=%.1f (%.1f%% off, band ±%.0f%% on BOTH) -> %s",
		s.p95, fieldRefP95, 100*p95Err, 100*fieldTailBandPct, verdict(tailPass))
	t.Logf("  reference tails for context: p05=%.0f p25=%.0f p75=%.0f (not gated)",
		fieldRefP05, fieldRefP25, fieldRefP75)
	overall := "FAIL"
	if meanPass && sdPass && tailPass {
		overall = "PASS"
	}
	t.Logf("=== OVERALL: %s (this is a reported measurement, not a test failure) ===", overall)
	t.Log("")
}

func writeFieldCSV(t *testing.T, path string, rows [][]string) {
	t.Helper()
	fh, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	w := csv.NewWriter(fh)
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal(err)
	}
}

func f0(v float64) string { return fmt.Sprintf("%.0f", v) }
func f2(v float64) string { return fmt.Sprintf("%.2f", v) }
