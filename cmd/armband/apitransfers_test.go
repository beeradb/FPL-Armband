package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// getTransfers issues GET /api/transfers with the given cookie, unlike getWith/get which
// either decode the response as viewmodel.State or take no cookie at all — this route's own
// document has a different shape, and most of what this file tests is a non-200 status.
func getTransfers(t *testing.T, s *squadServer, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", routeTransfers, nil)
	req.Host = "127.0.0.1:8080"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func TestTransfersRejectsWrongMethod(t *testing.T) {
	s := fixtureServer(t)
	req := httptest.NewRequest("POST", routeTransfers, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/transfers answered %d, want 405", w.Code)
	}
	if got := w.Header().Get("Allow"); got != "GET" {
		t.Errorf("Allow header = %q, want GET", got)
	}
}

// TestTransfersRefusesWithNoImportedEntry pins the first 409: sess.Entry == 0, the same
// "import a team first" shape GET /api/results already answers with.
func TestTransfersRefusesWithNoImportedEntry(t *testing.T) {
	s := fixtureServer(t)
	w := getTransfers(t, s, nil)
	if w.Code != http.StatusConflict {
		t.Errorf("GET /api/transfers with no imported entry answered %d, want 409: %s",
			w.Code, w.Body.String())
	}
}

// TestTransfersResolvesAnEmptySquadRatherThanRefusing is the fplarmband.com production
// incident of 2026-08-23: an Entry on record but no stored fifteen (len(sess.Squad) != 15)
// used to answer 409 "Your fifteen is incomplete" outright — but an empty sess.Squad is
// the ORDINARY state after Optimise or Reset (see session's own doc comment on why the
// fifteen is only stored once a reader diverges from the model's live answer), not a
// broken one, so any reader who imported a team and then pressed either button hit this
// wall on the very next "Suggest transfers" click. Reachable through a hand-crafted
// session — Entry alone, no Squad — which validateSession accepts (a session may carry
// only corrections and no team; see its own comment) and is otherwise how Optimise/Reset's
// own save shape reads once decoded.
//
// resolvedSquadCodes now resolves this the same way buildState would — the live-optimised
// default, no FPL account needed for THAT part — so the request clears the old 409 and
// proceeds to the real FPL fetches for entry 1234567, a fake id with no client configured
// to answer it: 503, not 409. That the failure moved this far is the fix; a real client
// and a real entry would carry it all the way to a suggestion, which
// TestApiTransfersPassesTheReadersOwnSquadNotTheHouseTeams (source scan) and
// TestTransfersAnswers503WithNoClientConfigured cover from the other side.
func TestTransfersResolvesAnEmptySquadRatherThanRefusing(t *testing.T) {
	s := fixtureServer(t)
	w0, _ := put(t, s, session{Version: sessionVersion, Entry: 1234567}, nil)
	if w0.Code != http.StatusOK {
		t.Fatalf("priming the entry-only session answered %d: %s", w0.Code, w0.Body.String())
	}
	cookie := sessionCookie(t, w0)

	w := getTransfers(t, s, cookie)
	if w.Code == http.StatusConflict {
		t.Errorf("GET /api/transfers with Entry set but no stored squad answered 409 — "+
			"an empty squad must resolve to the live-optimised default, not refuse: %s",
			w.Body.String())
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /api/transfers with Entry set but no stored squad answered %d, want "+
			"503 (squad resolution succeeds; the fake entry id then fails to fetch): %s",
			w.Code, w.Body.String())
	}
}

// legalCookieFor mints a session cookie carrying Entry and a legal fifteen, so a test can
// reach past the first two 409s and into the FPL-fetch branches.
func legalCookieFor(t *testing.T, s *squadServer, entry int) *http.Cookie {
	t.Helper()
	gk, def, mid, fwd := legalFifteenElements(t, s.engine.Boot)
	var codes []int
	for _, group := range [][]fpl.Element{gk, def, mid, fwd} {
		for _, el := range group {
			codes = append(codes, el.Code)
		}
	}
	w, _ := put(t, s, session{Version: sessionVersion, Entry: entry, Squad: codes}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("priming a legal session answered %d: %s", w.Code, w.Body.String())
	}
	return sessionCookie(t, w)
}

// TestTransfersAnswers503WhenTheEntryIsUnreachable pins the 503 branch for a failed entry
// fetch — "no fallback", the same doctrine results.go's own routes follow.
func TestTransfersAnswers503WhenTheEntryIsUnreachable(t *testing.T) {
	s := fixtureServer(t)
	cookie := legalCookieFor(t, s, 1234567)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return nil, errors.New("dial tcp: connection refused")
	}
	w := getTransfers(t, s, cookie)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /api/transfers with the entry unreachable answered %d, want 503: %s",
			w.Code, w.Body.String())
	}
}

// TestTransfersRefusesBeforeAnyDeadline pins the third 409: entry.CurrentEvent == nil, FPL's
// own "no picks yet" state — before GW1's deadline this route has nothing to improve.
func TestTransfersRefusesBeforeAnyDeadline(t *testing.T) {
	s := fixtureServer(t)
	cookie := legalCookieFor(t, s, 1234567)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id, CurrentEvent: nil}, nil
	}
	w := getTransfers(t, s, cookie)
	if w.Code != http.StatusConflict {
		t.Errorf("GET /api/transfers with no CurrentEvent answered %d, want 409: %s",
			w.Code, w.Body.String())
	}
}

// TestTransfersAnswers503WhenPicksAreUnreachable is the picks-fetch half of the same 503
// doctrine, once an entry with a current event is on record.
func TestTransfersAnswers503WhenPicksAreUnreachable(t *testing.T) {
	s := fixtureServer(t)
	cookie := legalCookieFor(t, s, 1234567)
	cur := 1
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id, CurrentEvent: &cur}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
		return nil, errors.New("dial tcp: connection refused")
	}
	w := getTransfers(t, s, cookie)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /api/transfers with picks unreachable answered %d, want 503: %s",
			w.Code, w.Body.String())
	}
}

// TestTransfersAnswers503WithNoClientConfigured pins the guard buildTransferBoard itself
// needs: it calls client.Entry/Picks/History directly rather than through the
// entryCached/picksCached seams just exercised above, so a nil client would panic inside it
// rather than answer gracefully. This is also, in practice, the branch every entry/picks
// success in this test binary falls into — internal/fpl.Client has no way to point at a
// fake server (baseURL is a package constant), so this is as close to the 200 path as a Go
// test in this repository can reach without a live FPL account. See transferBoard's own
// comment in transfers.go, which already documents needing "an entry, a squad and the
// network" as the reason its own wiring is covered by a source scan rather than a live call.
func TestTransfersAnswers503WithNoClientConfigured(t *testing.T) {
	s := fixtureServer(t)
	cookie := legalCookieFor(t, s, 1234567)
	cur := 1
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id, CurrentEvent: &cur}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
		return &fpl.EntryPicks{}, nil
	}
	w := getTransfers(t, s, cookie)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /api/transfers with entry/picks reachable but no client answered %d, "+
			"want 503: %s", w.Code, w.Body.String())
	}
}

// TestApiTransfersPassesTheReadersOwnSquadNotTheHouseTeams is a source scan, the same
// tripwire idiom TestTheTransferBoardWiresTheBankingDecision (transfers_test.go) already
// uses for buildTransferBoard's own internal wiring: a review deleting the sess.Entry
// argument, reintroducing sess.Squad directly in place of the resolved squad, or handing
// sell a non-nil value, would build, vet and pass the rest of this suite clean, because
// nothing else in the repository calls apiTransfers with a real FPL account behind it. It
// matches on the exact call shape rather than proving behaviour — the acknowledged limit
// of a scan like this — but a rewritten call is what would actually happen if this hazard
// regressed.
//
// The middle argument reads `squad`, not `sess.Squad`, since
// TestTransfersResolvesAnEmptySquadRatherThanRefusing: squad is
// resolvedSquadCodes(ctx, cfg, sess)'s answer, sess.Squad's own value on the fast path
// where one is already stored, and the live-optimised default otherwise — but always the
// reader's OWN current pitch either way, never cfg.EntryID's or e.SellPrices' house-team
// data, which is the property this scan actually pins.
func TestApiTransfersPassesTheReadersOwnSquadNotTheHouseTeams(t *testing.T) {
	src, err := os.ReadFile("apitransfers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "sess.Entry, squad, nil") {
		t.Error("apiTransfers no longer calls buildTransferBoard with (sess.Entry, squad, nil) " +
			"— sess.Entry is the READER's own entry, squad is their resolved current pitch " +
			"(resolvedSquadCodes), and nil sell means sell-at-market. cfg.EntryID or " +
			"e.SellPrices here would price a visitor's search off the house team's own account.")
	}
}

// TestTransferAnswerOutcomeSwitch pins the one value both the terminal and this route
// switch on and nothing else — transferBoard.outcome() — and that transferAnswer never
// invents a fourth. Both callers with no plans and a board that was never consulted must
// answer "recommend"/"nothing" exactly as boardOutcome's own doc comment states.
func TestTransferAnswerOutcomeSwitch(t *testing.T) {
	plans := []analysis.Plan{{GainPerGW: 1.5, Transfers: 1}}
	cfg := config.Default()
	for _, c := range []struct {
		name   string
		board  transferBoard
		want   string
		reason string
	}{
		{"recommend", transferBoard{Plans: plans, Free: 1}, "recommend", ""},
		{"bank", transferBoard{
			Plans: plans, Free: 1, Consulted: true, Advice: analysis.BankAdvice{Bank: true},
		}, "bank", analysis.BankAdvice{Bank: true}.Explain()},
		// Reads the constant rather than a literal, like the "bank" case above reads
		// Explain(). The literal that was here pinned wording the CLI and the page had
		// already stopped using, and it described a threshold that is not applied.
		{"nothing", transferBoard{Free: 1}, "nothing",
			emptyBoardReason + " " + bankingIsFirstClass},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := transferAnswer(&c.board, cfg, nil)
			if doc.Outcome != c.want {
				t.Errorf("outcome = %q, want %q", doc.Outcome, c.want)
			}
			if doc.Reason != c.reason {
				t.Errorf("reason = %q, want %q", doc.Reason, c.reason)
			}
			if doc.SellPrices != "market" {
				t.Errorf("sell_prices = %q, want the stated assumption \"market\"", doc.SellPrices)
			}
		})
	}
}

// TestTransferPlanDocHitCostAndBreakeven pins the per-plan pricing GET /api/transfers
// carries: max(0, transfers-free) hits at fpl.HitCost each, and breakeven_gws =
// ceil(cost/gain_per_gw) computed server-side — the division the design this implements
// offers instead of a single net "worth +N points" figure, which would add a per-gameweek
// estimate to a one-off cost and be a unit error.
func TestTransferPlanDocHitCostAndBreakeven(t *testing.T) {
	s := fixtureServer(t)
	boot := s.engine.Boot
	out := boot.Elements[0]
	in := boot.Elements[1]
	outM, inM := s.engine.Metrics(&out), s.engine.Metrics(&in)

	plan := analysis.Plan{
		Moves:        []analysis.Swap{{Out: outM, In: inM}},
		Transfers:    2,
		GainPerGW:    0.62,
		DependsOn:    outM,
		SurvivesLoss: true,
	}

	// Two spent, one free: one hit at fpl.HitCost, breakeven = ceil(4/0.62) = 7.
	got := transferPlanDoc(&transferBoard{Free: 1}, plan, boot)
	if got.Hits != 1 {
		t.Errorf("hits = %d, want 1 (2 spent - 1 free)", got.Hits)
	}
	if got.Cost != fpl.HitCost {
		t.Errorf("cost = %d, want %d", got.Cost, fpl.HitCost)
	}
	if got.BreakevenGWs != 7 {
		t.Errorf("breakeven_gws = %d, want 7 (ceil(%d/0.62))", got.BreakevenGWs, fpl.HitCost)
	}
	if len(got.Moves) != 1 {
		t.Fatalf("got %d moves, want 1", len(got.Moves))
	}
	if got.Moves[0].OutCode != out.Code || got.Moves[0].InCode != in.Code {
		t.Errorf("move codes = out %d in %d, want the PERMANENT codes %d/%d (not element ids "+
			"%d/%d)", got.Moves[0].OutCode, got.Moves[0].InCode, out.Code, in.Code, out.ID, in.ID)
	}
	if got.DependsOn != outM.Name || !got.SurvivesLoss {
		t.Errorf("depends_on/survives_loss = %q/%v, want %q/true", got.DependsOn, got.SurvivesLoss, outM.Name)
	}

	// Unlimited transfers: zero hits and zero cost regardless of how much was spent.
	unlimited := transferPlanDoc(&transferBoard{Free: fpl.UnlimitedTransfers}, plan, boot)
	if unlimited.Hits != 0 || unlimited.Cost != 0 || unlimited.BreakevenGWs != 0 {
		t.Errorf("with unlimited transfers got hits=%d cost=%d breakeven=%d, want all 0",
			unlimited.Hits, unlimited.Cost, unlimited.BreakevenGWs)
	}

	// Enough free transfers to cover the spend: no hit, so no breakeven either.
	covered := transferPlanDoc(&transferBoard{Free: 2}, plan, boot)
	if covered.Hits != 0 || covered.Cost != 0 || covered.BreakevenGWs != 0 {
		t.Errorf("with free >= transfers got hits=%d cost=%d breakeven=%d, want all 0",
			covered.Hits, covered.Cost, covered.BreakevenGWs)
	}
}

// TestValidateSessionBoundsBase pins §8.1's addition: `base` is ranged in validateSession's
// maxList map exactly like squad/xi/bench/lock/exclude/dismissed, because it is a code list
// arriving from a cookie a hand-crafted PUT /api/session could set to anything.
func TestValidateSessionBoundsBase(t *testing.T) {
	s := fixtureServer(t)

	// An oversized Base is refused, the same as an oversized Squad.
	huge := make([]int, 65)
	for i := range huge {
		huge[i] = s.engine.Boot.Elements[0].Code
	}
	if err := s.validateSession(session{Base: huge}); err == nil {
		t.Error("a 65-entry Base was accepted; maxList (64) should refuse it")
	}

	// A Base code the bootstrap does not carry is refused, the same as any other list.
	if err := s.validateSession(session{Base: []int{-999999}}); err == nil {
		t.Error("a Base carrying an unresolvable code was accepted")
	}

	// A legal Base — real codes, within bound — is accepted.
	codes := fifteenCodes(t, s.engine.Boot)
	if err := s.validateSession(session{Base: codes}); err != nil {
		t.Errorf("a legal 15-code Base was refused: %v", err)
	}
}
