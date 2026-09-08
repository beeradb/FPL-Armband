package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/viewmodel"
)

// TestThePageHeadlineIsTheModelsNumber is the guard that would have caught the largest
// defect in this work, and it is worth stating plainly why an ordinary test could not.
//
// The client used to score players itself. `xpFor` multiplied an already-fixture-adjusted
// per-90 figure by a hand-rolled difficulty ladder, dropped availability, congestion, role
// certainty and fixture load, and used expected minutes where the model uses a reliability
// figure. Every headline on the pitch was built on it: the projection, the captain's
// arithmetic, the formation comparison and the armband picker — while squad.xi_score and
// squad.expected arrived in the contract and were read by nothing.
//
// Nothing failed. The API tests passed, because the API was right. The visual goldens
// passed, because they were generated from the same wrong arithmetic. The page looked
// exactly as designed. The only way to see it is to compare what the BROWSER renders
// against what the MODEL says, which is what this does.
//
// It is the sharpest test in this package, because it has no tolerance to argue about:
// the two numbers are printed to one decimal place and they must be equal.
//
// ⚠️ It covers the totals and the cards. The armband picker still sorts client-side, but
// the sort key is week_xp from the server (horizon 1); TestXpForReadsThisWeekScores
// pins that, and TestGameweekArmbandRanksThisWeekScores pins the server half.
func TestThePageHeadlineIsTheModelsNumber(t *testing.T) {
	browser := browsertest.Find(t)

	s := fixtureServer(t)
	srv := httptest.NewServer(s)
	defer srv.Close()

	w := get(t, s, "/api/state")
	if w.Code != 200 {
		t.Fatalf("GET /api/state answered %d", w.Code)
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}

	dom := browsertest.DumpDOM(t, browser, srv.URL+"/app#pitch")

	// The score bug prints the projection at the top of the pitch, and the supporting
	// cell spells out the arithmetic as "XI <n> + armband".
	projected := oneFigure(t, dom, `id="sbTotal"[^>]*>([0-9]+\.[0-9])<`)
	xi := oneFigure(t, dom, `XI ([0-9]+\.[0-9]) \+ armband`)

	wantXI, wantTotal, capXP := thisWeekHeadline(st)
	if !closeTo(projected, wantTotal) {
		t.Errorf("the page projects %.1f and the model says %.1f (this-week XI + armband).\n"+
			"The page is not drawing the model's number — check whether something in "+
			"app.js has started scoring players again.", projected, wantTotal)
	}
	if !closeTo(xi, wantXI) {
		t.Errorf("the page's eleven totals %.1f and the model says %.1f (this-week xi)",
			xi, wantXI)
	}
	if capXP == 0 {
		t.Fatal("no captain found in the squad; the arithmetic below cannot be checked")
	}
	if diff := math.Abs((wantTotal - wantXI) - capXP); diff > 0.05 {
		t.Errorf("headline − xi is %.2f but the captain projects %.2f. The armband "+
			"is meant to add exactly one more copy of him.",
			wantTotal-wantXI, capXP)
	}
}

// TestEveryCardShowsTheModelsProjection walks the pitch cards rather than the totals.
//
// A total can be right while individual cards are wrong — two errors that cancel is not a
// hypothetical here, since the client's arithmetic was a rescaling of the same inputs. So
// every projection printed on a card is matched against that player's own figure.
func TestEveryCardShowsTheModelsProjection(t *testing.T) {
	browser := browsertest.Find(t)

	s := fixtureServer(t)
	srv := httptest.NewServer(s)
	defer srv.Close()

	w := get(t, s, "/api/state")
	if w.Code != 200 {
		t.Fatalf("GET /api/state answered %d", w.Code)
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	want := map[int]float64{}
	for _, p := range st.Squad.Players {
		want[p.ID] = weekXP(st, p.ID)
	}

	dom := browsertest.DumpDOM(t, browser, srv.URL+"/app#pitch")

	// Split first, then match inside each card.
	//
	// Deliberately not one regex spanning both. Go's regexp is RE2 -- no backtracking, so
	// no catastrophic blow-up -- but a bounded repetition like [\s\S]{0,1400} is expanded
	// into an automaton proportional to the bound, and over a 700KB document that is slow
	// enough to be mistaken for a hung browser. Splitting is also simply clearer.
	idOf := regexp.MustCompile(`^(\d+)"`)
	// "xPts" → "pts": the primary-page redesign replaced the jargon unit with plain
	// language everywhere on the card (see armband.css/app.js, "the standing constraint:
	// write for a manager, not a quant").
	xpOf := regexp.MustCompile(`<b>([0-9]+\.[0-9]{2})</b><span class="u">pts`)
	found := 0
	for _, chunk := range strings.Split(dom, `data-id="`)[1:] {
		im := idOf.FindStringSubmatch(chunk)
		xm := xpOf.FindStringSubmatch(chunk)
		if im == nil || xm == nil {
			continue
		}
		id, err := strconv.Atoi(im[1])
		if err != nil {
			continue
		}
		got, err := strconv.ParseFloat(xm[1], 64)
		if err != nil {
			continue
		}
		model, ok := want[id]
		if !ok {
			continue
		}
		found++
		// The captain's card shows the doubled figure, which is FPL's rule rather than a
		// model quantity, so either reading is correct for him.
		if math.Abs(got-model) > 0.011 && math.Abs(got-2*model) > 0.011 {
			t.Errorf("player %d renders %.2f; the model says %.2f", id, got, model)
		}
	}
	if found < 11 {
		t.Fatalf("only matched %d cards to a player, so this test is not covering the "+
			"pitch. The card markup has probably changed.", found)
	}
}

// weekXP is the live picker's sort key: this gameweek at horizon 1 when the
// rail sent it, otherwise the horizon-average Player.XP.
func weekXP(st viewmodel.State, id int) float64 {
	for _, gw := range st.Gameweeks {
		if !gw.Current {
			continue
		}
		for _, row := range gw.WeekXP {
			if row.ID == id {
				return row.XP
			}
		}
	}
	for _, p := range st.Squad.Players {
		if p.ID == id {
			return p.XP
		}
	}
	return 0
}

func thisWeekHeadline(st viewmodel.State) (xi, total, captain float64) {
	capID := st.Squad.Captain
	for _, gw := range st.Gameweeks {
		if gw.Current && gw.Captain != 0 {
			inXI := false
			for _, id := range st.Squad.XI {
				if id == gw.Captain {
					inXI = true
					break
				}
			}
			if inXI {
				capID = gw.Captain
			}
		}
	}
	for _, id := range st.Squad.XI {
		xi += weekXP(st, id)
	}
	captain = weekXP(st, capID)
	return xi, xi + captain, captain
}

func oneFigure(t *testing.T, dom, pattern string) float64 {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(dom)
	if m == nil {
		t.Fatalf("could not find %s in the rendered page", pattern)
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("could not read %q as a number: %v", m[1], err)
	}
	return f
}

// closeTo compares at the precision the page prints — one decimal place — with a little
// room for the rounding that happens on the way there.
func closeTo(page, model float64) bool {
	return fmt.Sprintf("%.1f", model) == fmt.Sprintf("%.1f", page) ||
		math.Abs(page-model) < 0.06
}
