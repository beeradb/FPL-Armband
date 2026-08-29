package backtest

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// DOES A TEMPLATE EXIST IN OUR PROJECTIONS, AND IS IT THE RIGHT ONE?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTemplateCore -v -count=1 -timeout 120m
//
// # The claim under test, which is the owner's and is about FOOTBALL
//
// **"Usually a group of 5-6 very strong players emerges. You build around that
// core and keep 2-3 people in fixture runs — high-upside non-premium — or defcon
// magnets if you want steady points."** Refined by the owner while this was being
// built: **"the core will evolve over the season, just more slowly than the
// edges."**
//
// ⚠️ **That refinement makes the test far more robust, and it is why the RANK
// table below is the primary reading.** A claim that the core is FIXED is
// absolute, and every artifact this harness has — estimator convergence early,
// budget constraints, argmax churn — inflates turnover and would refute it
// spuriously. A claim that the core turns over MORE SLOWLY THAN the edges is
// relative, measured within one optimum on one run in one unit, and none of those
// artifacts predicts that churn should concentrate in the cheap slots. **The
// confounds largely cancel in the comparison and do not cancel in the level.**
//
// That is a falsifiable statement about the shape of a season, and it makes this
// a test of the PROJECTIONS rather than of any policy. If a persistent core
// really does emerge every season and our point-in-time optima cannot find it,
// **the projections are churning noise** — and that one fact would explain the
// drift curve, the failed triggers and the whole record of unresolved chip arms
// without needing any of them to be about chips at all.
//
// # What is measured
//
//  1. **Persistence.** How often each player appears in the point-in-time optimal
//     fifteen across the season. A template predicts a BIMODAL distribution: a
//     handful near 100%, a rotating band, and a long tail of one-week wonders. A
//     flat unimodal spread means we do not reproduce the structure.
//  2. **Breadth.** How many distinct players ever appear. A template implies few;
//     churn implies hundreds.
//  3. **Whether the core is RIGHT.** The model's persistent core against the
//     players who actually finished the season highest-scoring. This is the part
//     that cannot be gamed by a model that is merely self-consistent: a stable
//     core of the wrong players is worse than no core at all, because it is
//     confidently wrong.
//
// ⚠️ **The optimum is budget-constrained and the realised ranking is not.** A
// player can be top-scoring and correctly absent from every optimum because he
// costs too much for what he returns. So (3) is reported as overlap and rank
// correlation, never as an accuracy score, and a low overlap is a prompt to look
// rather than a verdict on its own.
//
// # ⚠️ CONCENTRATION, not a per-player threshold — the owner's exact claim
//
// **"I predict around 12-15 players will make up over half your team most
// weeks."** That is a statement about how SMALL the supplying pool is, and it is
// the right question. An earlier version of this test asked which INDIVIDUALS
// appear in >=80% of weeks, found 0-1 a season, and read that as "no template" —
// but a pool can be perfectly stable while individuals rotate within it, which is
// exactly what "the core evolves, just more slowly than the edges" describes. A
// per-player threshold cannot see that and the concentration measure can.
//
// So `N50` is the headline: how many distinct players it takes to fill HALF of
// all starting-eleven slots in a season. 38 weeks x 11 slots = 418 slots, and the
// prediction is that 12-15 players supply 209 of them. `top15` is the share the
// fifteen most-used players supply, which is the same claim read the other way.
//
// ⚠️ **Appearing in the OPTIMUM is not the same as being good.** The fifteen
// includes a cheap bench the optimiser fills with the least-bad filler it can
// afford, and those slots churn for reasons that have nothing to do with a
// template. Persistence is therefore reported for the STARTING eleven as well.
func TestDiagTemplateCore(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	fmt.Printf("\n=== IS THERE A TEMPLATE IN OUR OPTIMA?\n")
	fmt.Printf("Point-in-time optimal squad recomputed every gameweek from GW1.\n")
	fmt.Printf("`core` counts players in >=80%% of weeks' STARTING ELEVEN — the owner's\n")
	fmt.Printf("claim is that 5-6 emerge. `breadth` is how many distinct players ever\n")
	fmt.Printf("start. A model reproducing a template has a small core and low breadth.\n\n")
	fmt.Printf("%-9s %6s %6s %7s %8s %8s %9s %9s  %s\n",
		"season", "weeks", "N50", "top15", "core>=80", "cond>=60", "breadth", "top-8 hit", "most-used")
	fmt.Printf("%-9s %6s %6s %6s   the ladder in SLOTS: the core, the squad, the pool\n",
		"", "N50", "N75", "N90")

	// Banked so the tables below are re-derivable rather than only
	// re-measurable. This diagnostic printed and banked nothing until
	// 2026-08-28, which left its own result impossible to check off the
	// machine that ran it.
	var conc, turn [][]string

	for _, pr := range loadPairsOrSkip(t, cfg) {
		sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
		// ⚠️ **The SHIPPED horizon, not 1.** A template is a season-long quality
		// claim, and a horizon-1 optimum chases this Saturday's fixture — it
		// picks whoever has the best single game and churns by construction. The
		// first run of this test used horizon 1 and reported 0-1 core players out
		// of a claimed 5-6; that number was partly manufactured by the lens.
		// FPL_TEMPLATE_HORIZON=1 takes the contrast reading.
		if os.Getenv("FPL_TEMPLATE_HORIZON") == "1" {
			sc.Weights.Horizon = 1
		}
		inXI := map[int]int{}
		// ⚠️ **Weeks the player was AVAILABLE**, so selection can be measured
		// without injury in the way. 80% of 38 gameweeks means missing fewer than
		// eight — including injuries, suspensions and rotation — which almost
		// nobody sustains. A raw >=80% rate therefore tests DURABILITY and
		// selection at once, and a genuine core player who missed eight weeks
		// injured but was picked in every week he was fit fails it for the wrong
		// reason. Conditional selection separates the two.
		avail := map[int]int{}
		weeks := 0
		for gw := 1; gw <= 38; gw++ {
			ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
			sq, ok := repairSquad(ew, nil, 1000, 0, sc)
			if !ok {
				continue
			}
			xi, _, _, _ := pickXI(ew, sq)
			for _, id := range xi {
				inXI[id]++
			}
			for id, p := range pr.Cur.Players {
				if g, ok := p.GWs[gw]; ok && g.Minutes > 0 {
					avail[id]++
				}
			}
			weeks++
		}
		if weeks < 20 {
			continue
		}
		// N50: how many players supply half of all starting slots — the owner's
		// "12-15 make up over half the team", which is a claim about the POOL
		// rather than about any individual holding a place.
		var counts []int
		total := 0
		for _, n := range inXI {
			counts = append(counts, n)
			total += n
		}
		sort.Sort(sort.Reverse(sort.IntSlice(counts)))
		// nAt is how many distinct players supply the given share of all
		// starting slots.
		//
		// ⚠️ There is NO NULL for any rung. Nothing here establishes what a
		// churning projection would score, and an over-concentrated
		// deterministic argmax reads low too — this is a census of what the
		// optimum did, not a result against noise.
		//
		// Slots are the currency the season actually spends —
		// eleven a week, no more — so the ladder is denominated in them and not
		// in points. Points are what the slots are spent FOR, and that is a
		// different quantity measured elsewhere.
		//
		// ⚠️ The three rungs answer different questions and only the first is the
		// owner's original claim. N50 is the CORE. N90 is closer to "everyone who
		// could reasonably feature" — the squad you would actually have to rate
		// well — and the gap between them is the size of the rotating edge.
		nAt := func(pct int) int {
			k, run := 0, 0
			for _, n := range counts {
				run += n
				k++
				if run*100 >= total*pct {
					break
				}
			}
			return k
		}
		n50, n75, n90 := nAt(50), nAt(75), nAt(90)
		var top15 int
		for i, n := range counts {
			if i >= 15 {
				break
			}
			top15 += n
		}

		// Conditional on availability: of the weeks he FEATURED, how often was he
		// in the optimal eleven? Requires a real sample of appearances so a
		// three-game cameo cannot read as 100%.
		var condCore []int
		for id, n := range inXI {
			if a := avail[id]; a >= weeks/2 && float64(n)/float64(a) >= 0.6 {
				condCore = append(condCore, id)
			}
		}

		var core80, core60 []int
		for id, n := range inXI {
			if float64(n)/float64(weeks) >= 0.8 {
				core80 = append(core80, id)
			}
			if float64(n)/float64(weeks) >= 0.6 {
				core60 = append(core60, id)
			}
		}
		sort.Slice(core80, func(a, b int) bool { return inXI[core80[a]] > inXI[core80[b]] })

		// Who actually finished top by realised points, budget ignored.
		type sc2 struct {
			id, pts int
		}
		var real []sc2
		for id, p := range pr.Cur.Players {
			var tot int
			for _, g := range p.GWs {
				tot += g.Points
			}
			real = append(real, sc2{id, tot})
		}
		sort.Slice(real, func(a, b int) bool { return real[a].pts > real[b].pts })
		top8 := map[int]bool{}
		for i := 0; i < 8 && i < len(real); i++ {
			top8[real[i].id] = true
		}
		var hit int
		for _, id := range core80 {
			if top8[id] {
				hit++
			}
		}
		var names []string
		for i, id := range core80 {
			if i >= 6 {
				break
			}
			n := "?"
			if p := pr.Cur.Players[id]; p != nil {
				n = p.WebName
			}
			mark := ""
			if top8[id] {
				mark = "*"
			}
			names = append(names, fmt.Sprintf("%s%s(%d%%)", n, mark, 100*inXI[id]/weeks))
		}
		fmt.Printf("%-9s %6d %6d %6d\n", pr.Name, n50, n75, n90)
		fmt.Printf("%-9s %6d %6d %6.0f%% %8d %8d %9d %9s  %v\n",
			pr.Name, weeks, n50, 100*float64(top15)/float64(total), len(core80),
			len(condCore), len(inXI), fmt.Sprintf("%d/%d", hit, len(core80)), names)
		conc = append(conc, []string{pr.Name, strconv.Itoa(weeks), strconv.Itoa(n50),
			strconv.Itoa(n75), strconv.Itoa(n90),
			strconv.FormatFloat(100*float64(top15)/float64(total), 'f', 1, 64),
			strconv.Itoa(len(core80)), strconv.Itoa(len(condCore)),
			strconv.Itoa(len(inXI)), strconv.Itoa(hit)})
	}
	// The primary reading: turnover by squad rank. If the core evolves more
	// slowly than the edges, persistence FALLS monotonically with rank.
	fmt.Printf("\n=== DOES TURNOVER RISE WITH RANK? (the owner's refined claim)\n")
	fmt.Printf("Each week the starting eleven is ranked by projected points. A slot's\n")
	fmt.Printf("`held` is how often the SAME player occupies it the following week.\n")
	fmt.Printf("The claim predicts held FALLS from rank 1 to rank 11 — the core evolving\n")
	fmt.Printf("more slowly than the edges rather than not evolving at all.\n\n")
	fmt.Printf("%-9s", "season")
	for r := 1; r <= 11; r++ {
		fmt.Printf(" %5s", fmt.Sprintf("#%d", r))
	}
	fmt.Printf("   %8s\n", "1-4 vs 8-11")
	for _, pr := range loadPairsOrSkip(t, cfg) {
		sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
		if os.Getenv("FPL_TEMPLATE_HORIZON") == "1" {
			sc.Weights.Horizon = 1
		}
		var prev []int
		held := make([]int, 12)
		seen := make([]int, 12)
		for gw := 1; gw <= 38; gw++ {
			ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
			sq, ok := repairSquad(ew, nil, 1000, 0, sc)
			if !ok {
				continue
			}
			xi, _, _, _ := pickXI(ew, sq)
			ranked := append([]int(nil), xi...)
			sort.Slice(ranked, func(a, b int) bool {
				return scoreOf(ew, ranked[a]) > scoreOf(ew, ranked[b])
			})
			if prev != nil {
				in := map[int]bool{}
				for _, id := range ranked {
					in[id] = true
				}
				for r, id := range prev {
					if r >= 11 {
						break
					}
					seen[r+1]++
					if in[id] {
						held[r+1]++
					}
				}
			}
			prev = ranked
		}
		if seen[1] == 0 {
			continue
		}
		fmt.Printf("%-9s", pr.Name)
		var top, bot, topN, botN int
		for r := 1; r <= 11; r++ {
			if seen[r] == 0 {
				fmt.Printf(" %5s", "-")
				continue
			}
			turn = append(turn, []string{pr.Name, strconv.Itoa(r),
				strconv.Itoa(held[r]), strconv.Itoa(seen[r])})
			fmt.Printf(" %4.0f%%", 100*float64(held[r])/float64(seen[r]))
			if r <= 4 {
				top += held[r]
				topN += seen[r]
			}
			if r >= 8 {
				bot += held[r]
				botN += seen[r]
			}
		}
		if topN > 0 && botN > 0 {
			fmt.Printf("   %+7.1f\n",
				100*float64(top)/float64(topN)-100*float64(bot)/float64(botN))
		} else {
			fmt.Printf("   %8s\n", "-")
		}
	}
	fmt.Printf("\nLast column is (rank 1-4 held%%) minus (rank 8-11 held%%). POSITIVE\n")
	fmt.Printf("supports the claim: the core turns over more slowly than the edges.\n")
	fmt.Printf("⚠️ Rank is by PROJECTED points, so a slot's identity is the model's own\n")
	fmt.Printf("ordering — this measures whether OUR core is stabler than OUR edges, not\n")
	fmt.Printf("whether football has a template. A negative or flat column would say the\n")
	fmt.Printf("projections churn their best picks as readily as their worst, which is\n")
	fmt.Printf("the diagnostic that matters regardless of what football does.\n")

	fmt.Printf("\n`cond>=60` counts players picked in >=60%% of the weeks they were FIT,\n")
	fmt.Printf("among those available at least half the season. That separates selection\n")
	fmt.Printf("from durability: >=80%% of ALL weeks means missing under eight gameweeks\n")
	fmt.Printf("including injuries, which is a test of availability more than of whether\n")
	fmt.Printf("the model rates a player.\n")
	fmt.Printf("\nN50 is how many players fill HALF the starting slots; top15 is the\n")
	fmt.Printf("share the fifteen most-used supply. **The prediction is N50 of 12-15.**\n")
	fmt.Printf("A much larger N50 means the projections spread their picks far wider\n")
	fmt.Printf("than a template would; a much smaller one means they concentrate harder.\n")
	fmt.Printf("\n* marks a core player who also finished in the season's realised top 8.\n")
	fmt.Printf("⚠️ Absence of a star is NOT an error: the optimum is budget-constrained\n")
	fmt.Printf("and the realised ranking is not, so a correctly-rejected expensive player\n")
	fmt.Printf("looks like a miss. Read the overlap as a prompt, not a score.\n")
	fmt.Printf("\n⚠️ **Read N50, not core>=80.** A per-player threshold near zero is\n")
	fmt.Printf("compatible with a perfectly stable pool whose members rotate within it,\n")
	fmt.Printf("which is what the claim actually says. The threshold column is kept only\n")
	fmt.Printf("because it is what an earlier version of this test wrongly led with.\n")

	// One file per table, so either can be differenced without parsing the other.
	// Written only when asked, so an ordinary run stays read-only.
	if dir := os.Getenv("FPL_CELLS_DIR"); dir != "" {
		write := func(name string, head []string, rows [][]string) {
			f, err := os.Create(dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			w := csv.NewWriter(f)
			if err := w.Write(head); err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				if err := w.Write(r); err != nil {
					t.Fatal(err)
				}
			}
			w.Flush()
			if err := w.Error(); err != nil {
				t.Fatal(err)
			}
		}
		write("concentration.csv", []string{"season", "weeks", "n50", "n75", "n90", "top15_pct",
			"core_ge80", "cond_ge60", "breadth", "top8_hit"}, conc)
		// held and seen rather than a percentage: a ratio cannot be re-aggregated
		// across seasons and the counts can.
		write("turnover.csv", []string{"season", "rank", "held", "seen"}, turn)
		fmt.Printf("\n  cells written to %s\n", dir)
	}
}

// scoreOf is one player's projected score on this engine, for ordering only.
func scoreOf(e *analysis.Engine, id int) float64 {
	el := e.Boot.ElementByID(id)
	if el == nil {
		return 0
	}
	return e.Metrics(el).Score
}
