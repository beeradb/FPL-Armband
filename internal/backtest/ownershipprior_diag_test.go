package backtest

import (
	"fmt"
	"os"
	"testing"
)

// DOES THE MARKET KNOW WHAT THE PRIOR CANNOT?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagOwnershipPredictsMinutes -v -count=1 -timeout 60m
//
// # The question
//
// The opening fifteen is the worst squad the model builds — it carries several
// times the gap from ideal that any in-season rebuild does — and a reachability
// probe found that almost nothing in the configuration can touch it: seven
// constants move ZERO of fifteen players at GW1. Weighting last season's closing
// minutes was the one live lever and it loses.
//
// The reason is stated in the shipped config itself. It carries **thirteen
// hand-written roster overrides**, and they are overwhelmingly about players the
// prior cannot describe: a £34m summer signing whose only history is five years
// old, two promoted-club regulars with no Premier League minutes at all, a keeper
// just named first choice, a defender stepping into an injured man's place.
// Several cite the market in their own reasoning — *"19% of managers already hold
// him, the market is right and the model cannot see why"*.
//
// **So a human is already supplying this information, by hand, from public
// sources, thirteen times a season.** That is not irreducible uncertainty; it is
// unmodelled information. Ownership is the obvious channel: a consensus forecast
// of ROLE, formed before a ball is kicked, and the archive has published it per
// gameweek all along.
//
// # What this measures, and what it deliberately does not
//
// **Ordering, not points.** Does GW1 ownership rank players by their coming
// minutes better than the model's own pre-season expectation does? An ordering is
// far cheaper to establish than a magnitude, and minutes are the channel the
// overrides act on.
//
// ⚠️ **It does NOT test whether ownership should enter `Score`.** A better
// predictor can make a worse policy here — the transfer search is an argmax and
// lives in the tail — so a win in this table is a candidate worth spending replay
// time on and nothing more.
//
// # ⚠️ The circularity, and how the split answers it
//
// Ownership is partly an echo of past points: people own players who scored.
// Against the general population it might beat the model by re-encoding the same
// history with extra noise, which would be no discovery at all.
//
// **The split is the whole design.** For a player with NO prior-season minutes
// there is no history for ownership to echo — the market's opinion of him is
// formed from transfers, pre-season and team news, which is exactly the
// information the prior lacks. If ownership wins on that stratum and not on the
// other, the mechanism is what it claims to be.
//
// # PRE-REGISTERED, before running
//
//   - **If the market carries role information the prior lacks**, ownership beats
//     the model's expected minutes on the NO-HISTORY stratum, and the gap is
//     smaller or reversed on the has-history stratum where the prior is informative.
//   - **If ownership is only an echo of past points**, it does no better on the
//     no-history stratum than the model does — there is nothing for it to echo
//     there.
//   - **If it wins on BOTH strata equally**, that is the uncomfortable result:
//     consistent with ownership simply being a better minutes model than ours,
//     which would be a bigger finding and a less specific one.
//   - ⚠️ **Uninterpretable if the no-history stratum is thin.** The count is
//     printed per season and a stratum under ~30 players says nothing.
//
// ownRow is one player's three predictions and what he actually played.
//
// Package level rather than inside the loop so the within-position helpers can
// name it — an anonymous struct repeated in three signatures is a shape that
// drifts the moment a field is added.
type ownRow struct {
	own, price, model, tilted, actual float64
	pos                               int
}

// priceTiltUnderTest is the lever value the ON column is scored at. Not the
// shipped value — nothing ships — just a setting large enough that a failure to
// move would mean the wiring is dead rather than the signal weak.
const priceTiltUnderTest = 0.5

func TestDiagOwnershipPredictsMinutes(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	// The window the opening squad actually has to survive. A season total would
	// let a January arrival's minutes count against a GW1 forecast nobody could
	// have made.
	const window = 10

	fmt.Printf("\n=== DOES GW1 OWNERSHIP RANK COMING MINUTES BETTER THAN THE PRIOR?\n")
	fmt.Printf("Spearman against minutes actually played in GW1-%d. Higher is better.\n", window)
	fmt.Printf("⚠️ Split on whether the PRIOR season has any minutes for the player.\n")
	fmt.Printf("For a player with none there is no history for ownership to echo, so\n")
	fmt.Printf("that stratum is where the mechanism can be told from the circularity.\n")
	fmt.Printf("⚠️ Ordering only. This does not say ownership belongs in Score — a\n")
	fmt.Printf("better predictor can make a worse policy, and the replay decides that.\n\n")
	fmt.Printf("%38s %-33s %s\n", "", "-- POOLED --", "-- WITHIN POSITION --")
	fmt.Printf("  %-9s %-12s %5s %8s %8s %8s %8s %8s %8s %8s %8s\n",
		"season", "stratum", "n",
		"own", "price", "mdl OFF", "mdl ON",
		"own", "price", "mdl OFF", "mdl ON")

	// ⚠️ Go prints rhos and no standard error, on purpose: the inference is
	// R's, on the season-clustered per-season rhos this file writes.
	var csv *os.File
	if path := os.Getenv("FPL_RANKS_CSV"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("FPL_RANKS_CSV=%s: %v", path, err)
		}
		defer f.Close()
		csv = f
		// ⚠️ The provenance is COLUMNS, not a `#` header. A comment line needs a
		// reader that skips it, and the sanctioned reader (`read_sidecar` in
		// stats/cells_common.R) does not — a raw read.csv here trips the
		// one-implementation guard. Columns are the better answer anyway: they
		// survive stacking two runs made at different settings, where a header
		// comment silently describes only the first file.
		fmt.Fprintln(csv, "season,stratum,n,scope,predictor,rho,rankable,window,price_tilt")
	}

	// The level half, in its own file because it is a different shape: a rho per
	// arm against a mean per position. One file with two schemas would be read
	// wrongly by whichever reader met it second.
	var csv2 *os.File
	if path := os.Getenv("FPL_LEVELS_CSV"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("FPL_LEVELS_CSV=%s: %v", path, err)
		}
		defer f.Close()
		csv2 = f
		fmt.Fprintln(csv2, "season,stratum,pos,n,pred_off,pred_on,actual,window,price_tilt")
	}

	sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
	for _, pr := range loadPairsOrSkip(t, cfg) {
		// The model's view before a ball is kicked, which is what builds the
		// opening fifteen.
		e, _ := EngineAt(pr.Cur, pr.Prior, 0, sc)

		// ⚠️ The SAME players scored twice, once with the price tilt off and
		// once on, so the comparison is within-population and the stratum's own
		// spread cancels. A second engine rather than a second run: two runs
		// would differ in the parse cache and the season load as well as in the
		// lever, and only one of those is the declared variable.
		tilted := sc
		tilted.Weights.PriceMinutesPrior = priceTiltUnderTest
		et, _ := EngineAt(pr.Cur, pr.Prior, 0, tilted)

		// Prior-season minutes by permanent player code: element ids are
		// reassigned every summer, so matching on them would compare strangers.
		priorMins := map[int]int{}
		if pr.Prior != nil {
			for _, q := range pr.Prior.Players {
				if q.Code != 0 {
					priorMins[q.Code] += q.Minutes
				}
			}
		}

		// ⚠️ Price is carried beside ownership because the two are different
		// claims that are easy to conflate. Ownership is what managers DID;
		// price is what FPL's own algorithm thinks a player is worth, set before
		// a ball is kicked and revised on transfer activity. They correlate —
		// expensive players are owned more — so a stratum where both beat the
		// model says less than one where they diverge.
		strata := map[string][]ownRow{}

		for id, p := range pr.Cur.Players {
			g1, has := p.GWs[1]
			if !has || g1.Selected <= 0 {
				continue // not in the game at GW1, or an archive with no ownership
			}
			el := e.Boot.ElementByID(id)
			if el == nil {
				continue
			}
			var actual float64
			for gw := 1; gw <= window; gw++ {
				if g, ok := p.GWs[gw]; ok {
					actual += float64(g.Minutes)
				}
			}
			key := "has history"
			if priorMins[p.Code] == 0 {
				key = "NO history"
			}
			strata[key] = append(strata[key], ownRow{
				own:    float64(g1.Selected),
				price:  float64(g1.Value),
				model:  e.Metrics(el).ExpectedMinutes,
				tilted: et.Metrics(el).ExpectedMinutes,
				actual: actual,
				pos:    el.ElementType,
			})
		}

		for _, key := range []string{"NO history", "has history"} {
			rs := strata[key]
			if len(rs) < 8 {
				fmt.Printf("  %-9s %-12s %5d %s\n", pr.Name, key, len(rs), "too thin")
				continue
			}
			// The four predictors, named once. Ordering the same list pooled
			// and within position is the whole comparison, so they may not be
			// spelled twice.
			preds := []struct {
				name string
				of   func(ownRow) float64
			}{
				{"own", func(r ownRow) float64 { return r.own }},
				{"price", func(r ownRow) float64 { return r.price }},
				{"model_off", func(r ownRow) float64 { return r.model }},
				{"model_on", func(r ownRow) float64 { return r.tilted }},
			}
			act := make([]float64, len(rs))
			for i, r := range rs {
				act[i] = r.actual
			}
			// ⚠️ Ownership is a COUNT and the totals differ between seasons, so
			// only its ranking is comparable — which is why this is Spearman and
			// not a correlation on levels.
			// ⚠️ Reported SEPARATELY, and a flat predictor is named rather than
			// collapsed into "constant". `spearman` returns ok=false when one
			// input has no variation, and an earlier version printed a single
			// "constant" for either case — which hid the finding, because WHICH
			// predictor is flat is the whole point. A model that assigns every
			// player it has no history for the same expected minutes cannot rank
			// them at all, and that is a stronger statement than losing a
			// comparison.
			cell := func(r float64, ok bool) string {
				if !ok {
					return "FLAT"
				}
				return fmt.Sprintf("%.3f", r)
			}

			// ⚠️ **WITHIN POSITION as well as pooled, and the pair is the
			// point.** The league fallback is flat inside a position but differs
			// BETWEEN them — a goalkeeper's average minutes are far above a
			// rotating forward's — so a pooled rank over unknowns is largely a
			// rank by POSITION. Unknown goalkeepers are usually backups, because
			// a first-choice keeper generally has history, so pooling can read
			// negative for a reason that says nothing about the ordering inside
			// a position, which is the only ordering the optimiser uses when it
			// fills a quota.
			//
			// Within position is reported as the n-weighted mean of the
			// per-position rhos, so it is one number comparable with the pooled
			// one beside it.
			cells := make([]string, 0, 2*len(preds))
			for _, scope := range []string{"pooled", "within_pos"} {
				for _, p := range preds {
					var r float64
					var ok bool
					if scope == "pooled" {
						pv := make([]float64, len(rs))
						for i := range rs {
							pv[i] = p.of(rs[i])
						}
						r, ok = spearman(pv, act)
					} else {
						r, ok = withinPos(rs, p.of)
					}
					cells = append(cells, cell(r, ok))
					if csv != nil {
						// ⚠️ ok is carried, not dropped. R must be able to tell
						// "could not rank" from "ranked at zero" — they are
						// opposite findings and a bare rho column conflates them.
						fmt.Fprintf(csv, "%s,%s,%d,%s,%s,%.6f,%t,%d,%.3f\n",
							pr.Name, key, len(rs), scope, p.name, r, ok,
							window, priceTiltUnderTest)
					}
				}
			}
			fmt.Printf("  %-9s %-12s %5d %8s %8s %8s %8s %8s %8s %8s %8s\n",
				pr.Name, key, len(rs),
				cells[0], cells[1], cells[2], cells[3],
				cells[4], cells[5], cells[6], cells[7])

			// ⚠️ **A rho cannot see a LEVEL, and the level is the other half of
			// the fix.** Spearman is invariant to any monotone rescaling, so a
			// fallback that hands every unknown goalkeeper a first-choice
			// workload would score identically to one that hands him a backup's.
			// It matters because an unknown goalkeeper is USUALLY a backup — a
			// first-choice keeper generally has history — so the position where
			// the fallback is most likely to over-state is exactly the one whose
			// league average is highest.
			//
			// Per position, per season, so R clusters on the season as
			// everywhere else. Predicted is per match and actual is a window
			// total, so only the RATIO of the two is comparable, and R forms it.
			if csv2 != nil {
				byPos := map[int][]ownRow{}
				for _, r := range rs {
					byPos[r.pos] = append(byPos[r.pos], r)
				}
				for _, pos := range []int{1, 2, 3, 4} {
					pr2 := byPos[pos]
					if len(pr2) < 4 {
						continue
					}
					var off, on, actual float64
					for _, r := range pr2 {
						off += r.model
						on += r.tilted
						actual += r.actual
					}
					n := float64(len(pr2))
					fmt.Fprintf(csv2, "%s,%s,%s,%d,%.4f,%.4f,%.4f,%d,%.3f\n",
						pr.Name, key, positionNames[pos], len(pr2),
						off/n, on/n, actual/n, window, priceTiltUnderTest)
				}
			}
		}
	}

	fmt.Printf("\n⚠️ Read the WITHIN-POSITION columns, not the pooled ones, for anything\n")
	fmt.Printf("about ordering. `Optimize` fills a positional quota, so it compares\n")
	fmt.Printf("goalkeepers with goalkeepers; a pooled rank over unknowns is very\n")
	fmt.Printf("largely a rank by position and can carry either sign for that reason.\n")
	fmt.Printf("⚠️ FLAT is not a rho of zero. It means the predictor is constant\n")
	fmt.Printf("inside every position, so no ordering exists there to score at all.\n")
	fmt.Printf("⚠️ A win on the NO-history stratum is the finding. A win on both is\n")
	fmt.Printf("consistent with ownership simply being a better minutes model, which\n")
	fmt.Printf("is a larger claim needing a larger test.\n")
	fmt.Printf("⚠️ No standard error here. Season is the clustering axis and the\n")
	fmt.Printf("inference belongs in R: stats/unknown_prior_ranks.R.\n")
}

// positionNames spells FPL's element_type, so the CSV names a position rather
// than an integer nothing downstream can decode.
var positionNames = map[int]string{1: "GK", 2: "DEF", 3: "MID", 4: "FWD"}

// withinPos is the n-weighted mean of the per-position Spearman rhos for one
// predictor, and whether any position could supply one at all.
//
// ⚠️ **The ok return is the whole reason this is not a bare float.** A position
// with fewer than four players, or one where the predictor is CONSTANT,
// contributes nothing — and if that is every position the answer is "this
// predictor cannot rank these players", which is a far stronger statement than
// the 0.000 an unguarded mean would print. The two are opposites and they look
// identical in a table.
func withinPos(rs []ownRow, of func(ownRow) float64) (float64, bool) {
	byPos := map[int][]int{}
	for i, r := range rs {
		byPos[r.pos] = append(byPos[r.pos], i)
	}
	var num, den float64
	for _, idx := range byPos {
		if len(idx) < 4 {
			continue
		}
		pred := make([]float64, 0, len(idx))
		act := make([]float64, 0, len(idx))
		for _, i := range idx {
			pred = append(pred, of(rs[i]))
			act = append(act, rs[i].actual)
		}
		r, ok := spearman(pred, act)
		if !ok {
			continue // flat inside this position, or tied throughout
		}
		num += r * float64(len(idx))
		den += float64(len(idx))
	}
	if den == 0 {
		return 0, false
	}
	return num / den, true
}
