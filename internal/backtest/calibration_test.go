package backtest

// Calibration diagnostics for the two hand-set model constants that the FPL API
// cannot supply evidence for on its own: the post-tournament rest discount and
// the new-manager penalty. Both shipped as guesses. These measure them.
//
// They print rather than assert, walk five seasons of the archive, and are
// research tools rather than regression tests, so they are gated behind DIAG=1:
//
//	DIAG=1 go test ./internal/backtest -run TestDiag -v
//
// The treatment lists are by hand because the API publishes neither managers
// nor tournament squads. Re-deriving a constant means editing the list, not
// just re-running the test.

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"testing"
	"time"
)

// The summer of 2021 as the model would have flagged it. Euro 2020's
// semi-finals were 6-7 July 2021 and the final 11 July; the Copa América final
// was 10 July. The 2021-22 season began on 13 August — the same four-and-a-half
// week gap as 2024.
//
// Same selection rule as summer2024: on the pitch for the majority of his
// semi-final. Web names here carry accents that the 2024-25 spellings drop,
// so the lists cannot be shared.
var summer2021 = []string{
	// Italy — semi-final 6 Jul, final 11 Jul (winners).
	"Jorginho",
	// Spain — semi-final 6 Jul.
	"Azpilicueta", "Laporte",
	// England — semi-final 7 Jul, final 11 Jul (runners-up).
	"Pickford", "Walker", "Stones", "Maguire", "Shaw", "Rice", "Phillips",
	"Saka", "Mount", "Sterling", "Kane",
	// Denmark — semi-final 7 Jul.
	"Schmeichel", "Christensen", "Højbjerg",
	// Argentina — Copa final 10 Jul (winners).
	"Martínez",
	// Brazil — Copa final 10 Jul (runners-up).
	"Alisson", "Fabinho", "Thiago Silva", "Jesus",
}

// The summer of 2024 as the model would have flagged it. Euro 2024's
// semi-finals were 9-10 July and the final 14 July; Copa América 2024 ran to a
// final on the same day. The 2024-25 season began on 16 August.
//
// The selection rule is DefaultRestPlayers': on the pitch for the majority of
// his semi-final. That is the rule the discount actually ships with, so it is
// the rule this has to be calibrated against — measuring a looser group
// measures a different term. It excludes Eze, Konsa, Watkins, Bowen,
// Alexander-Arnold, Gordon and Konaté, who were unused or late substitutes and
// had an easy summer, and who are exactly the players a squad-membership list
// would have wrongly swept in.
//
// Names are FPL web_names, which are unaccented for some players and a forename
// for Virgil van Dijk. Both Martínezes went deep and share a web_name.
var summer2024 = []string{
	// Spain — semi-final 9 Jul, final 14 Jul (winners).
	"Rodri", "Cucurella",
	// France — semi-final 9 Jul.
	"Saliba",
	// England — semi-final 10 Jul, final 14 Jul (runners-up).
	"Pickford", "Walker", "Stones", "Guéhi", "Rice", "Foden", "Saka", "Mainoo",
	// Netherlands — semi-final 10 Jul.
	"Verbruggen", "Virgil", "Gakpo", "Aké",
	// Argentina — semi-final 9 Jul, final 14 Jul (winners).
	"Martinez", "Romero", "Mac Allister", "Enzo",
	// Colombia — semi-final 10 Jul, final 14 Jul (runners-up).
	"Luis Díaz", "Muñoz",
}

// cohort is one tournament summer: the season that follows it, the players who
// went deep, and the seasons in which those same players had an ordinary
// summer and so serve as their own control.
type cohort struct {
	name     string
	post     string
	names    []string
	controls []string
}

// TestDiagRestPooled pools both tournament summers the archive can reach.
// A single cohort of fifteen-odd players cannot separate a 25% discount from
// zero — the first attempt produced estimates from 0.69 to 1.23 depending on
// which control season was used. Two cohorts and a fixed pre-treatment
// baseline is the most the data supports.
func TestDiagRestPooled(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	cohorts := []cohort{
		{"Euro 2020 / Copa 2021", "2021-22", summer2021, []string{"2022-23"}},
		{"Euro 2024 / Copa 2024", "2024-25", summer2024, []string{"2023-24", "2025-26"}},
	}
	prevOf := map[string]string{
		"2021-22": "2020-21", "2022-23": "2021-22", "2023-24": "2022-23",
		"2024-25": "2023-24", "2025-26": "2024-25",
	}

	seasons := map[string]*Season{}
	get := func(sn string) *Season {
		if s, ok := seasons[sn]; ok {
			return s
		}
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		seasons[sn] = s
		return s
	}

	fmt.Printf("\nOpening two gameweeks as a share of the previous season's rate.\n")
	fmt.Printf("The baseline is fixed before the summer in question, so nothing\n")
	fmt.Printf("conditions on how the player's season turned out.\n")

	var pooledM, pooledP, pooledN float64
	var perPlayerM, perPlayerP []float64
	var pp90DiD, lostDiD, cohortN []float64
	for _, ch := range cohorts {
		// Fix cohort membership from the post-tournament season, once.
		treated := map[int]bool{}
		for _, p := range get(ch.post).Players {
			for _, want := range ch.names {
				if p.WebName == want {
					treated[p.Code] = true
				}
			}
		}

		// Per-player ratios, so the difference-in-differences can be decomposed
		// back to individuals and given a standard error. The group means alone
		// have no variance to report.
		ratio := map[string]map[int][2]float64{}

		type cell struct{ mins, pts, n, pp90, pp90n, lost float64 }
		measure := func(sn string) (t8, other cell) {
			cur, prev := get(sn), get(prevOf[sn])
			prevBy := prev.ByCode()
			for _, p := range cur.Players {
				was, ok := prevBy[p.Code]
				if !ok || was.Minutes < 1500 {
					continue
				}
				pm, pp := float64(was.Minutes)/38, float64(was.TotalPoints)/38
				if pm < 45 || pp <= 0 {
					continue
				}
				var mins, pts, gws float64
				for gw := 1; gw <= 2; gw++ {
					if g, ok := p.GWs[gw]; ok {
						mins += float64(g.Minutes)
						pts += float64(g.Points)
						gws++
					}
				}
				if gws < 2 {
					continue
				}
				c := &other
				if treated[p.Code] {
					c = &t8
				}
				// Separate the two channels. If the discount is only a minutes
				// story it belongs on expected minutes, where it would also move
				// the rotation_risk band the agent reads; if per-90 output drops
				// too, a Score multiplier is the right instrument.
				prior90 := float64(was.TotalPoints) / (float64(was.Minutes) / 90)
				if mins > 60 && prior90 > 0 {
					c.pp90 += (pts / (mins / 90)) / prior90
					c.pp90n++
				}
				if (mins/gws)/pm < 0.5 {
					c.lost++
				}
				c.mins += (mins / gws) / pm
				c.pts += (pts / gws) / pp
				c.n++
				if ratio[sn] == nil {
					ratio[sn] = map[int][2]float64{}
				}
				ratio[sn][p.Code] = [2]float64{(mins / gws) / pm, (pts / gws) / pp}
			}
			return
		}

		fmt.Printf("\n%s — season %s\n", ch.name, ch.post)
		fmt.Printf("  %-10s %-22s %5s %9s %9s %9s %8s\n",
			"season", "group", "n", "minutes", "points", "pts/90", "<half")
		gapOf := func(sn string) (gaps [4]float64, n float64) {
			a, b := measure(sn)
			if a.n == 0 || b.n == 0 {
				return gaps, 0
			}
			label := "control"
			if sn == ch.post {
				label = "post-tournament"
			}
			fmt.Printf("  %-10s %-22s %5.0f %9.3f %9.3f %9.3f %7.0f%%  (%s)\n",
				sn, "went deep", a.n, a.mins/a.n, a.pts/a.n, a.pp90/a.pp90n,
				100*a.lost/a.n, label)
			fmt.Printf("  %-10s %-22s %5.0f %9.3f %9.3f %9.3f %7.0f%%\n",
				sn, "everyone else", b.n, b.mins/b.n, b.pts/b.n, b.pp90/b.pp90n,
				100*b.lost/b.n)
			gaps = [4]float64{
				a.mins/a.n - b.mins/b.n,
				a.pts/a.n - b.pts/b.n,
				a.pp90/a.pp90n - b.pp90/b.pp90n,
				a.lost/a.n - b.lost/b.n,
			}
			return gaps, a.n
		}
		post, n := gapOf(ch.post)
		var ctrl [4]float64
		var k float64
		for _, sn := range ch.controls {
			g, _ := gapOf(sn)
			for i := range g {
				ctrl[i] += g[i]
			}
			k++
		}
		var did [4]float64
		for i := range did {
			did[i] = post[i] - ctrl[i]/k
		}
		fmt.Printf("  difference-in-differences  minutes %+.3f  points %+.3f"+
			"  pts/90 %+.3f  <half %+.0fpp  (n=%.0f)\n",
			did[0], did[1], did[2], 100*did[3], n)
		pp90DiD = append(pp90DiD, did[2])
		lostDiD = append(lostDiD, did[3])
		cohortN = append(cohortN, n)
		pooledM += did[0] * n
		pooledP += did[1] * n
		pooledN += n

		// Decompose to per-player contributions: each treated player's excess
		// over his season's untreated mean, post-tournament minus control.
		base := map[string][2]float64{}
		for _, sn := range append([]string{ch.post}, ch.controls...) {
			var s [2]float64
			var k float64
			for code, r := range ratio[sn] {
				if treated[code] {
					continue
				}
				s[0] += r[0]
				s[1] += r[1]
				k++
			}
			base[sn] = [2]float64{s[0] / k, s[1] / k}
		}
		for code := range treated {
			post, ok := ratio[ch.post][code]
			if !ok {
				continue
			}
			var cm, cp, k float64
			for _, sn := range ch.controls {
				if r, ok := ratio[sn][code]; ok {
					cm += r[0] - base[sn][0]
					cp += r[1] - base[sn][1]
					k++
				}
			}
			if k == 0 {
				continue
			}
			perPlayerM = append(perPlayerM, (post[0]-base[ch.post][0])-cm/k)
			perPlayerP = append(perPlayerP, (post[1]-base[ch.post][1])-cp/k)
		}
	}

	fmt.Printf("\npooled across both tournament summers (n=%.0f treated player-seasons)\n", pooledN)
	fmt.Printf("  minutes %+.3f  ->  rest_discount %.2f\n", pooledM/pooledN, 1+pooledM/pooledN)
	fmt.Printf("  points  %+.3f  ->  rest_discount %.2f\n", pooledP/pooledN, 1+pooledP/pooledN)

	mm, mp := meanOf(perPlayerM), meanOf(perPlayerP)
	sm, sp := seOf(perPlayerM), seOf(perPlayerP)
	fmt.Printf("\nper-player decomposition (n=%d treated men seen in both a post\n", len(perPlayerM))
	fmt.Printf("and a control season), with standard errors:\n")
	fmt.Printf("  minutes %+.3f ± %.3f   95%% CI [%+.3f, %+.3f]  ->  %.2f\n",
		mm, sm, mm-1.96*sm, mm+1.96*sm, 1+mm)
	fmt.Printf("  points  %+.3f ± %.3f   95%% CI [%+.3f, %+.3f]  ->  %.2f\n",
		mp, sp, mp-1.96*sp, mp+1.96*sp, 1+mp)
}

// Calibrating new_coach_penalty.
//
// The premise of the term is that a manager change invalidates last season's
// record: the numbers were produced under someone else, so they say less about
// what happens next. The shipped code default is 0.93 and the shipped config is
// 1.0 (off). Neither was measured.
//
// The FPL API carries no manager data at all, so the treatment lists are by
// hand. The test the project applies is not "is the manager new" but "was last
// season's data produced under him", which is why a November appointment does
// not count the following summer — Amorim's Manchester United and Moyes'
// Everton are excluded for 2025-26 for exactly that reason.
//
// Promoted clubs are excluded throughout: they have no prior Premier League
// record for a coach change to invalidate, and the new-signing term already
// covers their squads.
var newCoachClubs = map[string]map[string]bool{
	// Pochettino to Chelsea, Postecoglou to Tottenham, Iraola to Bournemouth,
	// O'Neil to Wolves.
	"2023-24": {"CHE": true, "TOT": true, "BOU": true, "WOL": true},
	// Slot to Liverpool, Maresca to Chelsea, Hürzeler to Brighton,
	// Lopetegui to West Ham.
	"2024-25": {"LIV": true, "CHE": true, "BHA": true, "WHU": true},
	// Frank to Tottenham, Andrews to Brentford.
	"2025-26": {"TOT": true, "BRE": true},
}

func TestDiagNewCoachPenalty(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type acc struct{ mins, pp90, pts, n float64 }
	tot := map[bool]*acc{true: {}, false: {}}
	var tMins, cMins []float64
	perClub := map[string][]float64{}
	lost := map[bool]int{}

	fmt.Printf("\nhow much of last season's output an established player delivers\n")
	fmt.Printf("in the opening 6 gameweeks, under a new manager and under a\n")
	fmt.Printf("continuing one. Same club both seasons, 1500+ minutes last season.\n\n")

	// The xG grid, not the sweep grid: this needs last season's output as the
	// baseline, and 2021-22 carries no expected goals.
	for _, pair := range xgPairNames() {
		prev := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		short := func(s *Season) map[int]string {
			m := map[int]string{}
			for _, tm := range s.Teams {
				m[tm.ID] = tm.ShortName
			}
			return m
		}
		pShort, cShort := short(prev), short(cur)
		prevBy := prev.ByCode()

		for _, p := range cur.Players {
			was, ok := prevBy[p.Code]
			if !ok || was.Minutes < 1500 {
				continue
			}
			club := cShort[p.Team]
			// Same club both seasons: a mover is a new-signing case, not a
			// new-coach one, and conflating them measures neither.
			if club == "" || club != pShort[was.Team] {
				continue
			}
			// Last season's per-gameweek rates, over the 38 games it ran to.
			priorMins := float64(was.Minutes) / 38
			priorPP90 := float64(was.TotalPoints) / (float64(was.Minutes) / 90)
			if priorMins < 45 || priorPP90 <= 0 {
				continue
			}
			var mins, pts, gws float64
			for gw := 1; gw <= 6; gw++ {
				if g, ok := p.GWs[gw]; ok {
					mins += float64(g.Minutes)
					pts += float64(g.Points)
					gws++
				}
			}
			if gws < 5 {
				continue
			}
			isNew := newCoachClubs[pair[1]][club]
			a := tot[isNew]
			mRatio := (mins / gws) / priorMins
			a.mins += mRatio
			a.pts += (pts / gws) / (priorPP90 * priorMins / 90)
			if mins > 180 {
				a.pp90 += (pts / (mins / 90)) / priorPP90
			} else {
				a.pp90 += 1 // too few minutes to rate; neutral
			}
			a.n++
			if isNew {
				tMins = append(tMins, mRatio)
				perClub[pair[1]+" "+club] = append(perClub[pair[1]+" "+club], (pts/gws)/(priorPP90*priorMins/90))
			} else {
				cMins = append(cMins, mRatio)
			}
			// A player who lost his place outright is the risk the term is
			// meant to price. Count them rather than only averaging.
			if mRatio < 0.5 {
				lost[isNew]++
			}
		}
	}

	fmt.Printf("  %-22s %5s %9s %9s %9s\n", "group", "n", "minutes", "pts/90", "points")
	for _, isNew := range []bool{true, false} {
		a := tot[isNew]
		label := "manager continued"
		if isNew {
			label = "new manager"
		}
		fmt.Printf("  %-22s %5.0f %9.3f %9.3f %9.3f\n",
			label, a.n, a.mins/a.n, a.pp90/a.n, a.pts/a.n)
	}
	nw, st := tot[true], tot[false]
	dm := nw.mins/nw.n - st.mins/st.n
	dp := nw.pts/nw.n - st.pts/st.n
	se := seDiff(tMins, cMins)
	fmt.Printf("\n  difference   minutes %+.3f (± %.3f)   points %+.3f\n", dm, se, dp)
	fmt.Printf("  implied new_coach_penalty  %.2f on minutes, %.2f on points\n",
		1+dm, 1+dp)
	fmt.Printf("  0.93 is %.1f SE from the measured minutes effect\n", (dm+0.07)/se)

	// Both terms multiply Score, which is expected points, so the points row is
	// the one that governs. But a mean is the wrong summary if what a new
	// manager really does is widen the distribution.
	fmt.Printf("\n  dispersion of the minutes ratio (SD) and share who lost their place:\n")
	fmt.Printf("    new manager        SD %.3f   below half their minutes %d/%d (%.0f%%)\n",
		sd(tMins), lost[true], len(tMins), 100*float64(lost[true])/float64(len(tMins)))
	fmt.Printf("    manager continued  SD %.3f   below half their minutes %d/%d (%.0f%%)\n",
		sd(cMins), lost[false], len(cMins), 100*float64(lost[false])/float64(len(cMins)))

	fmt.Printf("\n  per club, points delivered against last season's rate:\n")
	var keys []string
	for k := range perClub {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var s float64
		for _, v := range perClub[k] {
			s += v
		}
		fmt.Printf("    %-16s n=%-3d %.2f\n", k, len(perClub[k]), s/float64(len(perClub[k])))
	}
}

// TestDiagRestDiscount asks the rest question of whole nationalities rather
// than of the men who actually played, and is the evidence for RestRegions
// shipping empty.
//
// The design is the same difference-in-differences as TestDiagRestPooled: Euro
// 2024's semi-finalists were Spain, England, France and the Netherlands, and no
// major tournament preceded 2023-24 or 2025-26, so the nation-versus-nation gap
// that is specific to the tournament season is the effect.
//
// It finds nothing — +0.031 on minutes, against roughly -0.21 for the named
// squads. That is the expected result rather than a contradiction: most
// England-eligible players in the league never left, so flagging the
// nationality averages twenty men who had a normal summer in with the four who
// did not. It is here to keep RestRegions honest. Anyone tempted to populate it
// should run this first.
func TestDiagRestDiscount(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	// Euro 2024 semi-finalists, by FPL region code.
	deep := map[int]bool{200: true, 241: true, 73: true, 152: true}

	// Nationality is a permanent attribute but the archive only publishes it
	// from 2024-25 on, so read it once and key it by player_code, which is
	// permanent too. Element ids are reassigned annually and cannot carry it.
	byCode := map[int]int{}
	for _, sn := range []string{"2025-26", "2024-25"} {
		m, err := regionsFor(ctx, sn)
		if err != nil {
			t.Fatal(err)
		}
		for code, reg := range m {
			if _, seen := byCode[code]; !seen {
				byCode[code] = reg
			}
		}
	}

	fmt.Printf("opening-gameweek minutes as a share of the player's GW3-12 rate\n\n")
	fmt.Printf("%-10s %-22s %6s %9s %9s\n", "season", "group", "n", "GW1-2", "GW1 only")

	type res struct{ two, one float64 }
	out := map[string]map[bool]res{}

	for _, sn := range []string{"2023-24", "2024-25", "2025-26"} {
		cur, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		sums := map[bool]*struct{ two, one, n float64 }{
			true:  {},
			false: {},
		}
		for _, p := range cur.Players {
			r, ok := byCode[p.Code]
			if !ok {
				continue
			}
			// Established starters only: the question is whether a regular is
			// eased in, not whether a squad player broke through.
			var base, m float64
			for gw := 3; gw <= 12; gw++ {
				if g, ok := p.GWs[gw]; ok {
					base += float64(g.Minutes)
					m++
				}
			}
			if m < 8 || base/m < 60 {
				continue
			}
			rate := base / m
			var early float64
			for gw := 1; gw <= 2; gw++ {
				early += float64(p.GWs[gw].Minutes)
			}
			s := sums[deep[r]]
			s.two += (early / 2) / rate
			s.one += float64(p.GWs[1].Minutes) / rate
			s.n++
		}
		out[sn] = map[bool]res{}
		for _, isDeep := range []bool{true, false} {
			s := sums[isDeep]
			if s.n == 0 {
				continue
			}
			label := "other nations"
			if isDeep {
				label = "Euro 2024 semi-finalists"
			}
			fmt.Printf("%-10s %-22s %6.0f %9.3f %9.3f\n",
				sn, label, s.n, s.two/s.n, s.one/s.n)
			out[sn][isDeep] = res{s.two / s.n, s.one / s.n}
		}
		fmt.Println()
	}

	gap := func(sn string) (two, one float64) {
		return out[sn][true].two - out[sn][false].two,
			out[sn][true].one - out[sn][false].one
	}
	t2, t1 := gap("2024-25")
	c2, c1 := gap("2023-24")
	d2, d1 := gap("2025-26")
	fmt.Printf("nation gap (semi-finalists minus others):\n")
	fmt.Printf("  2024-25, after Euro 2024   GW1-2 %+.3f   GW1 %+.3f\n", t2, t1)
	fmt.Printf("  2023-24, no tournament     GW1-2 %+.3f   GW1 %+.3f\n", c2, c1)
	fmt.Printf("  2025-26, no tournament     GW1-2 %+.3f   GW1 %+.3f\n", d2, d1)
	fmt.Printf("\ndifference-in-differences against 2023-24: GW1-2 %+.3f, GW1 %+.3f\n", t2-c2, t1-c1)
	fmt.Printf("implied rest_discount over two gameweeks: %.2f\n", 1+(t2-c2))
}

// seDiff is the standard error of the difference between two sample means.
func seDiff(a, b []float64) float64 {
	v := func(x []float64) (mean, varr float64) {
		for _, f := range x {
			mean += f
		}
		mean /= float64(len(x))
		for _, f := range x {
			varr += (f - mean) * (f - mean)
		}
		return mean, varr / float64(len(x)-1)
	}
	_, va := v(a)
	_, vb := v(b)
	return math.Sqrt(va/float64(len(a)) + vb/float64(len(b)))
}

// regionsFor reads FPL region codes for a season straight from the archive,
// which carries them even though backtest.Player does not. Keyed by the
// permanent player_code, not the season's element id.
func regionsFor(ctx context.Context, season string) (map[int]int, error) {
	url := fmt.Sprintf("%s/%s/players_raw.csv", archiveURL, season)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "armband/1.0")
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	r := csv.NewReader(resp.Body)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	if _, ok := col["region"]; !ok {
		return nil, fmt.Errorf("%s has no region column", season)
	}
	out := map[int]int{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		code, _ := strconv.Atoi(rec[col["code"]])
		reg, _ := strconv.Atoi(rec[col["region"]])
		if code > 0 && reg > 0 {
			out[code] = reg
		}
	}
	return out, nil
}
