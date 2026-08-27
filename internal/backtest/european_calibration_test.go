package backtest

// Calibration for the European half of the congestion block: UCLPenalty 0.93,
// UELPenalty 0.95, UECLPenalty 0.97.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagEuropean -v
//
// # What the shipped term actually does
//
// DefaultEuropeanCampaigns gives each club a start date and no MatchDates, and
// CoversGameweek returns true for every gameweek after it. So this is not a
// midweek-fixture model at all: it is a flat season-long discount applied to
// every player at a European club from September onward — 7% for a Champions
// League side. That is a large structural claim about five to nine clubs, and
// it has never been checked.
//
// # Why it cannot be measured within-player
//
// The rest constants were measurable within-player because congested weeks sit
// alongside uncongested ones in the same season. A blanket season-long discount
// has no such contrast: the window covers nearly every gameweek, so a player's
// "treated" average is his season average and the comparison is empty.
//
// So the design is the one the rest and new-manager tests already use — baseline
// each player on the season *before* the treatment, which is fixed before
// anything about the treated season is known — plus a control group of players
// at clubs not in Europe, which turns it into a difference in differences. The
// control matters: minutes drift downward with age and squad churn for
// everyone, and without it that drift would read as a European effect.
//
// Players who changed clubs over the summer are excluded. A move confounds the
// treatment with a new role at a new club, and it is exactly the population most
// likely to move.

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

// europeanClubs is which Premier League clubs played in Europe each season, by
// FPL short name.
//
// Hand-maintained, like DefaultEuropeanCampaigns, DefaultNewCoachClubs and the
// tournament lists, because participation is not in the FPL API or the archive.
// A wrong entry biases the result silently in both directions at once — it
// removes a treated player and adds a control — so the test cross-checks every
// entry against the previous season's final table, computed from the archive,
// and prints it. Qualification comes from that table plus the two domestic cups,
// so a club listed here that finished 14th is either a cup winner or a mistake.
var europeanClubs = map[string]map[string]string{
	"2022-23": {
		"MCI": "UCL", "LIV": "UCL", "CHE": "UCL", "TOT": "UCL",
		"ARS": "UEL", "MUN": "UEL",
		"WHU": "UECL",
	},
	"2023-24": {
		"MCI": "UCL", "ARS": "UCL", "MUN": "UCL", "NEW": "UCL",
		"LIV": "UEL", "WHU": "UEL", "BHA": "UEL",
		"AVL": "UECL",
	},
	"2024-25": {
		"MCI": "UCL", "ARS": "UCL", "LIV": "UCL", "AVL": "UCL",
		"TOT": "UEL", "MUN": "UEL",
		"CHE": "UECL",
	},
	"2025-26": {
		"LIV": "UCL", "ARS": "UCL", "MCI": "UCL", "CHE": "UCL",
		"NEW": "UCL", "TOT": "UCL",
		"AVL": "UEL", "NFO": "UEL",
		"CRY": "UECL",
	},
}

func TestDiagEuropeanPenalty(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	prevOf := map[string]string{
		"2022-23": "2021-22", "2023-24": "2022-23",
		"2024-25": "2023-24", "2025-26": "2024-25",
	}
	seasons := []string{"2022-23", "2023-24", "2024-25", "2025-26"}

	get := func(sn string) *Season { return loadSeason(t, cfg, sn) }

	// Cross-check the hand-maintained lists before using them.
	fmt.Printf("\nQualification check — where each listed club finished the season before.\n")
	fmt.Printf("A club outside the top seven is a cup winner or an error in the list.\n\n")
	for _, sn := range seasons {
		table := finalTable(get(prevOf[sn]))
		var names []string
		for club := range europeanClubs[sn] {
			names = append(names, club)
		}
		sort.Strings(names)
		fmt.Printf("  %s: ", sn)
		for _, club := range names {
			pos, ok := table[club]
			switch {
			case !ok:
				fmt.Printf("%s(promoted/absent) ", club)
			default:
				fmt.Printf("%s(%s,%d) ", club, europeanClubs[sn][club], pos)
			}
		}
		fmt.Println()
	}

	type sample struct{ mins, pts, per90 []float64 }
	groups := map[string]*sample{
		"UCL": {}, "UEL": {}, "UECL": {}, "no European football": {},
	}

	for _, sn := range seasons {
		cur, prev := get(sn), get(prevOf[sn])
		prevBy := prev.ByCode()
		curName := shortNames(cur)
		prevName := shortNames(prev)

		for _, p := range cur.Players {
			was, ok := prevBy[p.Code]
			if !ok || was.Minutes < 1500 {
				continue // no baseline, or not established enough to have one
			}
			club := curName[p.Team]
			if club == "" || club != prevName[was.Team] {
				continue // moved clubs: a new role confounded with the treatment
			}
			basePer := float64(was.Minutes) / 38
			basePts := float64(was.TotalPoints) / 38
			base90 := float64(was.TotalPoints) / (float64(was.Minutes) / 90)
			if basePer <= 0 || basePts <= 0 || base90 <= 0 {
				continue
			}

			g := groups["no European football"]
			if comp, in := europeanClubs[sn][club]; in {
				g = groups[comp]
			}
			g.mins = append(g.mins, (float64(p.Minutes)/38)/basePer)
			g.pts = append(g.pts, (float64(p.TotalPoints)/38)/basePts)
			if p.Minutes > 600 {
				g.per90 = append(g.per90, (float64(p.TotalPoints)/(float64(p.Minutes)/90))/base90)
			}
		}
	}

	fmt.Printf("\nSeason after, as a share of the season before. Established players\n")
	fmt.Printf("(1500+ prior minutes) who stayed at the same club.\n\n")
	fmt.Printf("%-22s %6s %9s %9s %9s\n", "group", "n", "minutes", "points", "pts/90")
	order := []string{"UCL", "UEL", "UECL", "no European football"}
	for _, k := range order {
		g := groups[k]
		if len(g.mins) == 0 {
			continue
		}
		fmt.Printf("%-22s %6d %9.3f %9.3f %9.3f\n",
			k, len(g.mins), meanOf(g.mins), meanOf(g.pts), meanOf(g.per90))
	}

	ctrl := groups["no European football"]
	fmt.Printf("\nAgainst the control, which is what the penalty should be:\n\n")
	fmt.Printf("%-22s %9s %9s %9s %10s\n",
		"group", "minutes", "points", "pts/90", "shipped")
	shipped := map[string]float64{
		"UCL": cfg.Congestion.UCLPenalty, "UEL": cfg.Congestion.UELPenalty,
		"UECL": cfg.Congestion.UECLPenalty,
	}
	for _, k := range []string{"UCL", "UEL", "UECL"} {
		g := groups[k]
		if len(g.mins) == 0 {
			continue
		}
		fmt.Printf("%-22s %9.3f %9.3f %9.3f %10.2f\n", k,
			meanOf(g.mins)/meanOf(ctrl.mins),
			meanOf(g.pts)/meanOf(ctrl.pts),
			meanOf(g.per90)/meanOf(ctrl.per90),
			shipped[k])
	}

	fmt.Printf("\n95%% intervals on the minutes difference against the control:\n")
	for _, k := range []string{"UCL", "UEL", "UECL"} {
		g := groups[k]
		if len(g.mins) < 30 {
			continue
		}
		d := meanOf(g.mins) - meanOf(ctrl.mins)
		se := seDiff(g.mins, ctrl.mins)
		fmt.Printf("  %-20s %+.3f ± %.3f  [%+.3f, %+.3f]\n",
			k, d, 1.96*se, d-1.96*se, d+1.96*se)
	}
	fmt.Printf("\nA penalty of p corresponds to a ratio of p against the control, so\n")
	fmt.Printf("UCL 0.93 predicts -0.07 on the minutes line.\n")

	stratifiedPer90(t, seasons, prevOf, get)
}

// stratifiedPer90 repeats the per-90 comparison within bands of prior output.
//
// The raw comparison cannot be believed on its own. Champions League clubs are
// where the best players are, and per-90 output reverts to the mean hardest at
// the top, so a treated group drawn from the highest band will fall further than
// a control drawn from everywhere regardless of what Europe does. Comparing
// like with like is the only way to tell the two apart — and if the gap survives
// it is worth something, because a term that moves per-90 output is a Score
// multiplier, which is what this one already is.
func stratifiedPer90(t *testing.T, seasons []string, prevOf map[string]string,
	get func(string) *Season) {

	type obs struct {
		prior90 float64
		ratio   float64
		comp    string
	}
	var all []obs

	for _, sn := range seasons {
		cur, prev := get(sn), get(prevOf[sn])
		prevBy := prev.ByCode()
		curName, prevName := shortNames(cur), shortNames(prev)
		for _, p := range cur.Players {
			was, ok := prevBy[p.Code]
			if !ok || was.Minutes < 1500 || p.Minutes < 600 {
				continue
			}
			club := curName[p.Team]
			if club == "" || club != prevName[was.Team] {
				continue
			}
			base90 := float64(was.TotalPoints) / (float64(was.Minutes) / 90)
			if base90 <= 0 {
				continue
			}
			comp := europeanClubs[sn][club]
			if comp == "" {
				comp = "none"
			}
			all = append(all, obs{
				prior90: base90,
				ratio:   (float64(p.TotalPoints) / (float64(p.Minutes) / 90)) / base90,
				comp:    comp,
			})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].prior90 < all[j].prior90 })

	fmt.Printf("\nPer-90 output, within bands of prior output. Mean reversion is\n")
	fmt.Printf("strongest at the top and that is where the European clubs are, so\n")
	fmt.Printf("the unstratified gap above is not evidence on its own.\n\n")
	fmt.Printf("%-18s %6s %10s %8s %10s %8s %9s\n",
		"prior pts/90", "n(eur)", "eur ratio", "n(ctrl)", "ctrl ratio", "diff", "band")

	const bands = 4
	per := len(all) / bands
	var wDiff, wN float64
	for b := 0; b < bands; b++ {
		lo := b * per
		hi := lo + per
		if b == bands-1 {
			hi = len(all)
		}
		var eur, ctl []float64
		for _, o := range all[lo:hi] {
			if o.comp == "none" {
				ctl = append(ctl, o.ratio)
			} else {
				eur = append(eur, o.ratio)
			}
		}
		if len(eur) < 10 || len(ctl) < 10 {
			continue
		}
		d := meanOf(eur) - meanOf(ctl)
		wDiff += d * float64(len(eur))
		wN += float64(len(eur))
		fmt.Printf("%5.2f - %-10.2f %6d %10.3f %8d %10.3f %8.3f %9.3f\n",
			all[lo].prior90, all[hi-1].prior90, len(eur), meanOf(eur), len(ctl), meanOf(ctl),
			d, 1+d)
	}
	if wN > 0 {
		fmt.Printf("\nPooled across bands, weighted by European n: %+.3f, i.e. a Score\n",
			wDiff/wN)
		fmt.Printf("multiplier of %.3f against the shipped 0.93 to 0.97.\n", 1+wDiff/wN)
	}
}

// finalTable returns each club's finishing position from the archive's results.
// Qualification for Europe is decided by it, so it is the check on the
// hand-maintained list above.
func finalTable(s *Season) map[string]int {
	name := shortNames(s)
	type rec struct{ pts, gd int }
	tab := map[string]*rec{}
	at := func(id int) *rec {
		n := name[id]
		if tab[n] == nil {
			tab[n] = &rec{}
		}
		return tab[n]
	}
	for _, f := range s.Fixtures {
		if f.TeamHScore == nil || f.TeamAScore == nil {
			continue
		}
		h, a := at(f.TeamH), at(f.TeamA)
		hs, as := *f.TeamHScore, *f.TeamAScore
		h.gd += hs - as
		a.gd += as - hs
		switch {
		case hs > as:
			h.pts += 3
		case as > hs:
			a.pts += 3
		default:
			h.pts++
			a.pts++
		}
	}
	type row struct {
		club string
		rec
	}
	var rows []row
	for club, r := range tab {
		if club == "" {
			continue
		}
		rows = append(rows, row{club, *r})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].pts != rows[j].pts {
			return rows[i].pts > rows[j].pts
		}
		return rows[i].gd > rows[j].gd
	})
	out := make(map[string]int, len(rows))
	for i, r := range rows {
		out[r.club] = i + 1
	}
	return out
}

func shortNames(s *Season) map[int]string {
	out := make(map[int]string, len(s.Teams))
	for _, t := range s.Teams {
		out[t.ID] = t.ShortName
	}
	return out
}

func mustFinite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
