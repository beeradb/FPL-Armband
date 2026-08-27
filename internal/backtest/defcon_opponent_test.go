package backtest

// Do some opponents hand out more defensive contributions than others?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDefconOpponent -v
//
// Defensive contribution is the one scoring term the model treats as entirely
// fixture-insensitive: fixtureAdjustedXP90 lists it in the "remainder" with
// saves and cards, while goals, assists and the clean sheet all take an
// opponent multiplier. That is an assumption nobody has checked, and it is
// suspicious on its face — a side facing a possession team spends the match
// clearing, blocking and intercepting, which is precisely what the category
// counts.
//
// Measured within-player, each appearance against a player's own average, which
// is the design the fixture work settled on: comparing raw totals by opponent
// would mostly measure which teams the defensive players happen to play for.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
)

func TestDiagDefconOpponent(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	s, err := Load(ctx, cfg.CacheDir, "2025-26")
	if err != nil {
		t.Fatal(err)
	}
	name := shortNames(s)

	// Who each club played, by gameweek, and the opponent's identity.
	type opp struct{ opponent int }
	sched := map[int]map[int]opp{}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		for _, side := range [][2]int{{f.TeamH, f.TeamA}, {f.TeamA, f.TeamH}} {
			if sched[side[0]] == nil {
				sched[side[0]] = map[int]opp{}
			}
			sched[side[0]][*f.Event] = opp{opponent: side[1]}
		}
	}

	// Within-player: each 60+ minute appearance as a ratio to his own mean rate.
	ratios := map[int][]float64{}
	var players int
	for _, p := range s.Players {
		if p.Type == 1 || p.Minutes < 1200 {
			continue // keepers earn none
		}
		var tot, mins float64
		type app struct {
			opponent int
			per90    float64
		}
		var apps []app
		for gw, g := range p.GWs {
			if g.Minutes < 60 {
				continue
			}
			tot += float64(g.DefCon)
			mins += float64(g.Minutes)
			o, ok := sched[p.Team][gw]
			if !ok {
				continue
			}
			apps = append(apps, app{o.opponent, float64(g.DefCon) / (float64(g.Minutes) / 90)})
		}
		if mins < 900 || tot == 0 || len(apps) < 10 {
			continue
		}
		own := tot / (mins / 90)
		if own <= 0 {
			continue
		}
		players++
		for _, a := range apps {
			ratios[a.opponent] = append(ratios[a.opponent], a.per90/own)
		}
	}

	type res struct {
		club  string
		n     int
		ratio float64
	}
	var out []res
	for team, rs := range ratios {
		if len(rs) < 40 {
			continue
		}
		var sum float64
		for _, r := range rs {
			sum += r
		}
		out = append(out, res{name[team], len(rs), sum / float64(len(rs))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ratio > out[j].ratio })

	fmt.Printf("\nDefensive contribution against each opponent, within-player.\n")
	fmt.Printf("2025-26, %d outfielders with 1200+ minutes. 1.00 is a player's own average.\n\n",
		players)
	fmt.Printf("%-6s %6s %9s\n", "vs", "n", "dc ratio")
	for _, r := range out {
		fmt.Printf("%-6s %6d %9.3f\n", r.club, r.n, r.ratio)
	}
	if len(out) >= 2 {
		hi, lo := out[0], out[len(out)-1]
		var sum, sq float64
		for _, r := range out {
			sum += r.ratio
		}
		mean := sum / float64(len(out))
		for _, r := range out {
			sq += (r.ratio - mean) * (r.ratio - mean)
		}
		fmt.Printf("\nspread %.3f (%s) to %.3f (%s), sd %.3f across %d clubs\n",
			hi.ratio, hi.club, lo.ratio, lo.club, math.Sqrt(sq/float64(len(out))), len(out))
		fmt.Printf("For comparison the attacking fixture ladder spans 1.30 to 0.72, and the\n")
		fmt.Printf("model applies *nothing* here.\n")
	}
}

// TestDiagSavesAndCardsByOpponent — the other two terms in the
// "fixture-insensitive remainder".
//
//	DIAG=1 go test ./internal/backtest -run TestDiagSavesAndCards -v
//
// Saves are the interesting one, because the model already has half of the
// trade-off and not the other half. Facing a strong attack it raises expected
// goals conceded, so the keeper correctly loses clean-sheet value — and it
// credits him nothing extra for the saves that same attack forces him into.
// If saves are fixture-sensitive at all, keepers facing hard fixtures are
// systematically underrated.
func TestDiagSavesAndCardsByOpponent(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	s, err := Load(context.Background(), cfg.CacheDir, "2025-26")
	if err != nil {
		t.Fatal(err)
	}
	name := shortNames(s)
	sched := map[int]map[int]int{}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		if sched[f.TeamH] == nil {
			sched[f.TeamH] = map[int]int{}
		}
		if sched[f.TeamA] == nil {
			sched[f.TeamA] = map[int]int{}
		}
		sched[f.TeamH][*f.Event] = f.TeamA
		sched[f.TeamA][*f.Event] = f.TeamH
	}

	run := func(label string, keepers bool, val func(GW) float64) {
		ratios := map[int][]float64{}
		n := 0
		for _, p := range s.Players {
			isGK := p.Type == 1
			if isGK != keepers || p.Minutes < 1200 {
				continue
			}
			var tot, mins float64
			for _, g := range p.GWs {
				if g.Minutes >= 60 {
					tot += val(g)
					mins += float64(g.Minutes)
				}
			}
			if mins < 900 || tot <= 0 {
				continue
			}
			own := tot / (mins / 90)
			n++
			for gw, g := range p.GWs {
				if g.Minutes < 60 {
					continue
				}
				o, ok := sched[p.Team][gw]
				if !ok {
					continue
				}
				ratios[o] = append(ratios[o], (val(g)/(float64(g.Minutes)/90))/own)
			}
		}
		type res struct {
			club  string
			n     int
			ratio float64
		}
		var out []res
		for team, rs := range ratios {
			if len(rs) < 15 {
				continue
			}
			var sum float64
			for _, r := range rs {
				sum += r
			}
			out = append(out, res{name[team], len(rs), sum / float64(len(rs))})
		}
		if len(out) < 5 {
			fmt.Printf("\n%s: too few observations\n", label)
			return
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ratio > out[j].ratio })
		var sum, sq float64
		for _, r := range out {
			sum += r.ratio
		}
		mean := sum / float64(len(out))
		for _, r := range out {
			sq += (r.ratio - mean) * (r.ratio - mean)
		}
		fmt.Printf("\n%s — %d players, %d clubs, sd %.3f\n", label, n, len(out),
			math.Sqrt(sq/float64(len(out))))
		fmt.Printf("  most:  ")
		for _, r := range out[:4] {
			fmt.Printf("%s %.2f  ", r.club, r.ratio)
		}
		fmt.Printf("\n  least: ")
		for _, r := range out[len(out)-4:] {
			fmt.Printf("%s %.2f  ", r.club, r.ratio)
		}
		fmt.Printf("\n  spread %.3f to %.3f\n", out[0].ratio, out[len(out)-1].ratio)
	}

	run("saves, keepers", true, func(g GW) float64 { return float64(g.Saves) })
	run("yellow cards, outfielders", false, func(g GW) float64 { return float64(g.Yellow) })
	fmt.Printf("\nFor comparison: defcon spans 1.069 to 0.901 and the attacking ladder\n")
	fmt.Printf("1.30 to 0.72. The model applies nothing to any of these three.\n")
}
