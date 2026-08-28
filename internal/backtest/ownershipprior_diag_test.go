package backtest

import (
	"fmt"
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
	fmt.Printf("  %-9s %-12s %6s %9s %9s %9s %9s\n",
		"season", "stratum", "n", "own rho", "price rho", "model OFF", "model ON")

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
		type row struct{ own, price, model, tilted, actual float64 }
		strata := map[string][]row{}

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
			strata[key] = append(strata[key], row{
				own:    float64(g1.Selected),
				price:  float64(g1.Value),
				model:  e.Metrics(el).ExpectedMinutes,
				tilted: et.Metrics(el).ExpectedMinutes,
				actual: actual,
			})
		}

		for _, key := range []string{"NO history", "has history"} {
			rs := strata[key]
			if len(rs) < 8 {
				fmt.Printf("  %-9s %-12s %6d %9s %9s %9s %9s\n",
					pr.Name, key, len(rs), "-", "-", "-", "too thin")
				continue
			}
			own, price := make([]float64, len(rs)), make([]float64, len(rs))
			model, act := make([]float64, len(rs)), make([]float64, len(rs))
			tilt := make([]float64, len(rs))
			for i, r := range rs {
				own[i], price[i] = r.own, r.price
				model[i], act[i] = r.model, r.actual
				tilt[i] = r.tilted
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
			ro, ok1 := spearman(own, act)
			rp, okp := spearman(price, act)
			rm, ok2 := spearman(model, act)
			rt, okt := spearman(tilt, act)
			cell := func(r float64, ok bool) string {
				if !ok {
					return "FLAT"
				}
				return fmt.Sprintf("%.3f", r)
			}
			gap := "-"
			if ok1 && ok2 {
				gap = fmt.Sprintf("%+.3f", ro-rm)
			} else if ok1 && !ok2 {
				gap = "no rival"
			}
			_ = gap
			fmt.Printf("  %-9s %-12s %6d %9s %9s %9s %9s\n",
				pr.Name, key, len(rs), cell(ro, ok1), cell(rp, okp),
				cell(rm, ok2), cell(rt, okt))
		}
	}

	fmt.Printf("\n⚠️ `gap` is ownership's rho minus the model's ON THE SAME PLAYERS, so\n")
	fmt.Printf("the population's own spread cancels. Read the gap, not either rho.\n")
	fmt.Printf("⚠️ A win on the NO-history stratum is the finding. A win on both is\n")
	fmt.Printf("consistent with ownership simply being a better minutes model, which\n")
	fmt.Printf("is a larger claim needing a larger test.\n")
	fmt.Printf("⚠️ No standard error here. Season is the clustering axis and the\n")
	fmt.Printf("inference belongs in R, on the per-season gaps.\n")
}
