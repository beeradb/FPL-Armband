package backtest

import (
	"fmt"
	"sort"
	"testing"
)

// What is the engine's OWN ownership profile?
//
// PRE-REGISTERED and DESCRIPTIVE — see the vault note "prereg what is the
// engine's own differential rate". There is no policy arm and no counterfactual;
// this reports what maximising Score under a budget actually picks.
//
// Answerable only because #147 landed the ownership seam. Before it every
// replayed player read 0% owned, which is also the first void check below.
func TestDiagEngineDifferentialRate(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	const diffCut = 10.0 // % owned, the cohort's threshold

	type seasonRow struct {
		name                       string
		xiSlots, xiDiff            int
		openSlots, openDiff        int
		distinctDiff               map[int]bool
		deciles                    [10]int
		ownedPlayers, totalPlayers int
		diffPts, tmplPts           int
		diffN, tmplN               int
		diffMins, tmplMins         int
		diffHaul, tmplHaul         int
		diffScore, tmplScore       float64
	}
	var out []seasonRow

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)
		sc := sweepConfig(cfg, 1, false)

		res, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatalf("%s: %v", season, err)
		}

		row := seasonRow{name: season, distinctDiff: map[int]bool{}}
		byID := map[int]*Player{}
		for _, q := range cur.Players {
			byID[q.ID] = q
		}

		// Void check: is the seam actually populated for this season?
		reg := registeredBy(cur, 10)
		own10 := ownershipAt(cur, 10, reg)
		row.totalPlayers = len(reg.in)
		row.ownedPlayers = len(own10.pct)

		// The opening fifteen, which is one decision and the directly comparable
		// one: the cohort figure is a GW1 measurement.
		openOwn := ownershipAt(cur, 1, registeredBy(cur, 0))
		for _, id := range res.OpeningSquad {
			row.openSlots++
			if openOwn.pct[id] < diffCut {
				row.openDiff++
			}
		}

		// The fielded eleven, every week, on that week's own ownership.
		for _, w := range res.Weeks {
			if len(w.XI) == 0 {
				continue
			}
			o := ownershipAt(cur, w.GW-1, registeredBy(cur, w.GW-1))
			// ⚠️ THE CONTROL. If the engine's differential picks simply carry a
			// lower Score, a lower return is the budget constraint doing its job
			// and there is no defect to report. Only an EQUAL-Score gap is a
			// statement about the model.
			eng, _ := EngineAt(cur, prior, w.GW-1, sc)
			score := map[int]float64{}
			if eng != nil {
				for _, m := range eng.AllMetrics() {
					score[m.ID] = m.Score
				}
			}
			for _, id := range w.XI {
				row.xiSlots++
				p := o.pct[id]
				if p < diffCut {
					row.xiDiff++
					row.distinctDiff[id] = true
				}
				d := int(p / 10)
				if d > 9 {
					d = 9
				}
				row.deciles[d]++

				// What the pick actually returned. The cohort measured exactly
				// this on managers; measuring it on the engine puts both on one
				// scale instead of comparing a rate to a return.
				var pts, mins int
				if q, ok := byID[id]; ok {
					if g, ok := q.GWs[w.GW]; ok {
						pts, mins = g.Points, g.Minutes
					}
				}
				if p < diffCut {
					row.diffN++
					row.diffScore += score[id]
					row.diffPts += pts
					row.diffMins += mins
					if pts >= 9 {
						row.diffHaul++
					}
				} else {
					row.tmplN++
					row.tmplScore += score[id]
					row.tmplPts += pts
					row.tmplMins += mins
					if pts >= 9 {
						row.tmplHaul++
					}
				}
			}
		}
		out = append(out, row)
	}

	t.Log("=== VOID CHECK: is the ownership seam populated? ===")
	for _, r := range out {
		t.Logf("  %-8s %d/%d registered players carry an ownership share",
			r.name, r.ownedPlayers, r.totalPlayers)
	}

	t.Logf("=== DIFFERENTIAL SHARE (<%.0f%% owned) ===", diffCut)
	t.Log("  season    opening 15        fielded XI (all 38)   distinct diff players")
	var tx, td, ox, od int
	for _, r := range out {
		t.Logf("  %-8s  %2d/15 = %5.1f%%   %4d/%4d = %5.1f%%       %d",
			r.name, r.openDiff, 100*float64(r.openDiff)/float64(r.openSlots),
			r.xiDiff, r.xiSlots, 100*float64(r.xiDiff)/float64(r.xiSlots),
			len(r.distinctDiff))
		tx += r.xiSlots
		td += r.xiDiff
		ox += r.openSlots
		od += r.openDiff
	}
	t.Logf("  POOLED    %2d/%d = %5.1f%%   %4d/%4d = %5.1f%%",
		od, ox, 100*float64(od)/float64(ox), td, tx, 100*float64(td)/float64(tx))
	t.Log("  comparators (GW1 2026-27, NOT like-for-like): elite 11.4%, ordinary 29.8%")

	t.Log("=== WHAT THE ENGINE'S PICKS ACTUALLY RETURNED, per start ===")
	t.Log("  season    differential (<10%)              template (>=10%)")
	var dN, dP, dM, dH, tN, tP, tM, tH int
	for _, r := range out {
		t.Logf("  %-8s  n=%4d %5.2f pts %5.1f min %5.1f%% haul   n=%4d %5.2f pts %5.1f min %5.1f%% haul",
			r.name, r.diffN, fdiv(r.diffPts, r.diffN), fdiv(r.diffMins, r.diffN),
			100*fdiv(r.diffHaul, r.diffN),
			r.tmplN, fdiv(r.tmplPts, r.tmplN), fdiv(r.tmplMins, r.tmplN),
			100*fdiv(r.tmplHaul, r.tmplN))
		dN += r.diffN
		dP += r.diffPts
		dM += r.diffMins
		dH += r.diffHaul
		tN += r.tmplN
		tP += r.tmplPts
		tM += r.tmplMins
		tH += r.tmplHaul
	}
	t.Logf("  POOLED    n=%4d %5.2f pts %5.1f min %5.1f%% haul   n=%4d %5.2f pts %5.1f min %5.1f%% haul",
		dN, fdiv(dP, dN), fdiv(dM, dN), 100*fdiv(dH, dN),
		tN, fdiv(tP, tN), fdiv(tM, tN), 100*fdiv(tH, tN))
	t.Log("  cohort GW1 2026-27 differential pts/start (NOT like-for-like): elite 6.02, ordinary 3.40-4.22")

	t.Log("=== ⚠️ THE CONTROL: does Score already explain the gap? ===")
	t.Log("  season    diff Score  tmpl Score  |  diff pts  tmpl pts  |  residual (pts gap - Score gap)")
	var dS, tS float64
	for _, r := range out {
		ds := r.diffScore / float64(atLeast1(r.diffN))
		ts := r.tmplScore / float64(atLeast1(r.tmplN))
		dp := fdiv(r.diffPts, r.diffN)
		tp := fdiv(r.tmplPts, r.tmplN)
		t.Logf("  %-8s   %6.3f     %6.3f    |  %6.3f    %6.3f  |  %+6.3f",
			r.name, ds, ts, dp, tp, (dp-tp)-(ds-ts))
		dS += r.diffScore
		tS += r.tmplScore
	}
	ds := dS / float64(atLeast1(dN))
	ts := tS / float64(atLeast1(tN))
	dp := fdiv(dP, dN)
	tp := fdiv(tP, tN)
	t.Logf("  POOLED     %6.3f     %6.3f    |  %6.3f    %6.3f  |  %+6.3f",
		ds, ts, dp, tp, (dp-tp)-(ds-ts))
	t.Logf("  Score gap %+.3f, realised gap %+.3f. If these match, the budget explains it.",
		ds-ts, dp-tp)

	t.Log("=== OWNERSHIP DECILE PROFILE OF THE FIELDED XI (pooled) ===")
	var pooled [10]int
	for _, r := range out {
		for i, n := range r.deciles {
			pooled[i] += n
		}
	}
	total := 0
	for _, n := range pooled {
		total += n
	}
	for i, n := range pooled {
		bar := ""
		if total > 0 {
			bar = bars(60 * n / total)
		}
		t.Logf("  %2d-%3d%% owned  %5d  %5.1f%%  %s", i*10, i*10+10, n,
			100*float64(n)/float64(total), bar)
	}
}

func atLeast1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func fdiv(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func bars(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = '#'
	}
	return string(b)
}

var _ = sort.Ints
var _ = fmt.Sprintf
