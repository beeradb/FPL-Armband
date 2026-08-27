package backtest

// Does the defensive-contribution term double-count the bonus term?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDefconBias -v
//
// The suspicion is concrete. Now that the replay can see defensive contribution
// at all, the term measures as *net harmful* on the only season that has it —
// 2136 blind against 2041 sighted, so a weight of zero beats a weight of one.
// BPS is driven by tackles, recoveries, clearances, blocks and interceptions
// among other things, so `Bonus90` may already price the very actions
// `defConPoints` is paying for a second time. That is the signature that
// condemned the set-piece bonus, where first-choice penalty takers were
// over-predicted by 0.400 points while their set-piece term was worth 0.393 —
// the entire bonus was redundant.
//
// Points cannot settle it: one season is one season, and sweeping a weight on
// it would be argmax-mining on n=1. A bias decomposition needs no points at
// all. Group players by the size of their defcon term, and ask whether the
// model over-predicts them by roughly that much. If it does, the term is
// redundant; if the bias is flat across groups, it is carrying its weight.
//
// Split within 2025-26 rather than across seasons, because it is the only
// season with the category: the model is built from the first half and scored
// against the second.

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagDefconBias(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	// Pinned by requirement, not by hand: defensive contribution was a scoring
	// category in exactly one season, so this grid is one pair and will stay one
	// until another is played. gridFor says that; a literal could not.
	pair := gridFor(needsDefcon)[0]
	cur, err := Load(ctx, cfg.CacheDir, pair[1])
	if err != nil {
		t.Fatal(err)
	}
	prior, err := Load(ctx, cfg.CacheDir, pair[0])
	if err != nil {
		t.Fatal(err)
	}

	const cut = 19
	e, boot := EngineAt(cur, prior, cut, SimConfig{Weights: cfg.Weights})

	type row struct {
		name            string
		pos             int
		term, predicted float64
		actual, bias    float64
		bonus, defcon90 float64
	}
	var rows []row

	for i := range boot.Elements {
		el := &boot.Elements[i]
		// Outfielders only: keepers earn no defensive contribution.
		if el.ElementType == 1 || el.Minutes < 600 {
			continue
		}
		m := e.Metrics(el)
		p := cur.Players[el.ID]
		if p == nil {
			continue
		}
		// What actually happened after the cutoff, per 90.
		var mins, pts float64
		for gw := cut + 1; gw <= 38; gw++ {
			if g, ok := p.GWs[gw]; ok {
				mins += float64(g.Minutes)
				pts += float64(g.Points)
			}
		}
		if mins < 450 {
			continue // too little football after the split to rate him on
		}
		actual := pts / (mins / 90)

		// The model's per-90 estimate, and the slice of it the defcon term is
		// responsible for.
		predicted := m.BaseXP90
		term := analysis.DefconTermFor(el.ElementType, m.DefCon90)
		rows = append(rows, row{
			name: m.Name, pos: el.ElementType, term: term,
			predicted: predicted, actual: actual, bias: actual - predicted,
			bonus: m.Bonus90, defcon90: m.DefCon90,
		})
	}
	if len(rows) < 40 {
		t.Skipf("only %d players clear the filters", len(rows))
	}

	fmt.Printf("\nDefensive contribution: is the term already in the bonus term?\n")
	fmt.Printf("2025-26, model built through GW%d and scored on GW%d-38, %d outfielders.\n",
		cut, cut+1, len(rows))
	fmt.Printf("\nWithin position, because defcon and position are nearly the same variable:\n")
	fmt.Printf("a low-defcon player is an attacker, and the model's bias by position would\n")
	fmt.Printf("otherwise be read as a defcon effect.\n")

	report := func(label string, in []row) {
		if len(in) < 24 {
			fmt.Printf("\n%s: only %d players, skipped\n", label, len(in))
			return
		}
		sort.Slice(in, func(i, j int) bool { return in[i].term < in[j].term })
		fmt.Printf("\n%s (%d)\n", label, len(in))
		fmt.Printf("%-18s %5s %9s %10s %10s %11s %11s\n",
			"defcon term", "n", "dc/90", "term", "bias", "bias+term", "bonus/90")
		q := len(in) / 3
		var firstBias, lastBias, firstTerm, lastTerm float64
		for b := 0; b < 3; b++ {
			lo, hi := b*q, (b+1)*q
			if b == 2 {
				hi = len(in)
			}
			var term, bias, dc, bonus float64
			for _, r := range in[lo:hi] {
				term += r.term
				bias += r.bias
				dc += r.defcon90
				bonus += r.bonus
			}
			n := float64(hi - lo)
			name := []string{"lowest third", "middle", "highest third"}[b]
			fmt.Printf("%-18s %5.0f %9.2f %10.3f %+10.3f %+11.3f %11.3f\n",
				name, n, dc/n, term/n, bias/n, (bias+term)/n, bonus/n)
			// The term beside the bias, then their sum: a flat sum is the redundancy
			// signature, and the bonus rate last because its job is to rule out the
			// obvious rival explanation rather than to be read first.
			sink.emitAll("defcon_bias",
				fmt.Sprintf("2025-26, model through GW%d, scored GW%d-38", cut, cut+1),
				label+", "+name+" by defcon rate", int(n),
				measure{"defcon actions per 90", dc / n},
				measure{"term", term / n},
				measure{"bias", bias / n},
				measure{"bias plus term", (bias + term) / n},
				measure{"bonus per 90", bonus / n})
			if b == 0 {
				firstBias, firstTerm = bias/n, term/n
			}
			if b == 2 {
				lastBias, lastTerm = bias/n, term/n
			}
		}
		if d := lastTerm - firstTerm; d > 0 {
			fmt.Printf("  bias falls %+.3f as the term grows %+.3f -> %.0f%% of the term is "+
				"already priced elsewhere\n", lastBias-firstBias, d, 100*(firstBias-lastBias)/d)
		}
	}

	var def, mid, fwd []row
	for _, r := range rows {
		switch r.pos {
		case 2:
			def = append(def, r)
		case 3:
			mid = append(mid, r)
		case 4:
			fwd = append(fwd, r)
		}
	}
	report("defenders", def)
	report("midfielders", mid)
	report("forwards", fwd)

	fmt.Printf("\nbias is actual minus predicted points per 90; negative means over-prediction.\n")
	fmt.Printf("If the term were fully earned, bias would be flat across the groups. If it were\n")
	fmt.Printf("entirely redundant, bias would fall by exactly the term's growth and bias+term\n")
	fmt.Printf("would be flat instead.\n")
}
