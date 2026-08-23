package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// apiTransfers answers GET /api/transfers: the session's own imported entry, run through
// the same deterministic search `armband transfers` already prints — analysis.BuildPlans
// over analysis.NewSquadState, through the shared transferBoard buildTransferBoard
// assembles (see transfers.go's own comment for why nothing here may reimplement that
// search). This is the page's Suggest transfers button.
//
// # Why this is unauthenticated
//
// Like GET /api/state and GET /api/results, this reads only the caller's own session
// cookie and mutates nothing, so there is no write for a CSRF token to protect.
//
// # Why the FPL fetches run before the lock
//
// The same discipline importTeam's numbered steps and apiResults both follow: the method
// and session gates run first and cost nothing; entry, picks and history are then fetched
// OUTSIDE s.mu, so one reader's network latency cannot stall every other reader's render
// behind it; the render lock is taken only around the plan search itself, which is the one
// piece that touches the shared engine.
//
// entry/picks/history are fetched here purely to answer with the right status code before
// buildTransferBoard runs — 409 for "no picks yet", 503 for "FPL is not answering". They
// are fetched again inside buildTransferBoard, for the bank, squad value and current
// event; that second fetch is a disk read within the process's cache TTL, not a second
// round trip to FPL — see buildTransferBoard's own comment on why `squad` is a parameter
// rather than something it derives itself, which is what forces this route to already know
// the entry is reachable before it can hand over a squad to price.
func (s *squadServer) apiTransfers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "the transfers route takes a GET", http.StatusMethodNotAllowed)
		return
	}

	sess := s.readValidSession(r)
	if sess.Entry == 0 {
		http.Error(w, "Import a team first — there is no squad to suggest transfers for.",
			http.StatusConflict)
		return
	}
	if len(sess.Squad) != 15 {
		http.Error(w, "Your fifteen is incomplete, so there is no squad to suggest "+
			"transfers for.", http.StatusConflict)
		return
	}

	entry, err := s.entryCached(r.Context(), sess.Entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: transfers: fetching entry %d: %v\n", sess.Entry, err)
		http.Error(w, "FPL is not answering just now. Try again in a minute.",
			http.StatusServiceUnavailable)
		return
	}
	if entry.CurrentEvent == nil {
		http.Error(w, "FPL exposes picks only after a deadline has passed, so there is "+
			"nothing to improve yet.", http.StatusConflict)
		return
	}
	if _, err := s.picksCached(r.Context(), sess.Entry, *entry.CurrentEvent); err != nil {
		fmt.Fprintf(os.Stderr, "serve: transfers: fetching picks for entry %d gw%d: %v\n",
			sess.Entry, *entry.CurrentEvent, err)
		http.Error(w, "FPL is not answering just now. Try again in a minute.",
			http.StatusServiceUnavailable)
		return
	}
	// buildTransferBoard below calls client.Entry/Picks/History directly rather than
	// through the entryCached/picksCached seams just used above — the same seam
	// cmdTransfers's own callers have never needed, because the CLI always has a real
	// client. A nil client here would panic inside it, so it is refused explicitly and
	// early, with the same answer a genuinely unreachable FPL gets: there is no
	// meaningful difference between "no client configured" and "FPL cannot be reached"
	// from the reader's side of this request.
	if s.client == nil {
		http.Error(w, "FPL is not answering just now. Try again in a minute.",
			http.StatusServiceUnavailable)
		return
	}
	if _, err := s.client.History(r.Context(), sess.Entry); err != nil {
		fmt.Fprintf(os.Stderr, "serve: transfers: fetching history for entry %d: %v\n",
			sess.Entry, err)
		http.Error(w, "FPL is not answering just now. Try again in a minute.",
			http.StatusServiceUnavailable)
		return
	}

	// The render lock, taken only now — see this function's own comment. Every FPL round
	// trip above ran outside it; buildTransferBoard's own entry/picks/history calls below
	// are cache hits at this point, so the search is the only real work happening under
	// s.mu.
	defer s.lockRender("transfers")()

	// sess.Entry, not cfg.EntryID: this is the READER's own squad. sess.Squad, not the
	// picks just fetched above: the reader has been editing, and the picks are what FPL
	// holds, not what is on screen. sell is nil — sell-at-market — because
	// engine.SellPrices is cfg.EntryID's OWN purchase history and handing it to a
	// visitor's search would apply the house team's prices to a stranger's players. See
	// buildTransferBoard's own comment for all three.
	cfg := s.effectiveCfgFrom(sess)
	board, why := buildTransferBoard(r.Context(), cfg, s.client, s.engine,
		sess.Entry, sess.Squad, nil)
	if board == nil {
		fmt.Fprintf(os.Stderr, "serve: transfers: %s\n", why)
		http.Error(w, "the suggestion could not be built just now — try again",
			http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(transferAnswer(board, cfg, s.engine.Boot))
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: transfers: %v\n", err)
		http.Error(w, "the suggestion could not be built just now — try again",
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Session-scoped and a live recommendation -- nothing here is ever `final`, unlike
	// GET /api/results' settled-gameweek branch. See writeState's own comment for the
	// same rule stated for /api/state.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// transferDoc is GET /api/transfers' whole response body. A standalone shape rather than
// anything in internal/viewmodel: State.Transfers is the reader's OWN baseline diff, sent
// on every /api/state; this is a fetched, on-demand recommendation with no baseline
// involved at all, and the two must not be confused for one contract.
type transferDoc struct {
	// Outcome is transferBoard.outcome(), one of "recommend"/"bank"/"nothing". The client
	// switches on this and nothing else — see transfers.go's own comment on why that
	// single value exists.
	Outcome string `json:"outcome"`
	Free    int    `json:"free"`
	// FreeUnknown mirrors viewmodel.Transfers.FreeUnknown -- the history read failed, so
	// the allowance below is a placeholder rather than a fact.
	FreeUnknown bool `json:"free_unknown,omitempty"`
	Bank        int  `json:"bank"`
	MoveLimit   int  `json:"move_limit"`
	// SellPrices is always "market" -- a stated assumption, never a hidden one. See
	// buildTransferBoard's own comment for why this route never prices at a visitor's
	// real purchase history.
	SellPrices string `json:"sell_prices"`
	// Reason is Advice.Explain() for a banked week, the fixed sentence for "nothing
	// clears the bar", and empty for a recommendation -- the moves say what to do.
	Reason string         `json:"reason"`
	Plans  []transferPlan `json:"plans"`
}

// transferPlan is one candidate package: plans[0] is the recommendation, the rest are
// "also considered" -- the same distinction the terminal renderer draws (transfers.go).
type transferPlan struct {
	Moves     []viewmodel.Move `json:"moves"`
	Transfers int              `json:"transfers"`
	GainPerGW float64          `json:"gain_per_gw"`
	Spend     int              `json:"spend"`
	Hits      int              `json:"hits"`
	Cost      int              `json:"cost"`
	// BreakevenGWs is ceil(cost/gain_per_gw), zero when cost is zero. Server-side, so a
	// reader who checks it back against the two figures beside it gets the same answer.
	BreakevenGWs int    `json:"breakeven_gws"`
	DependsOn    string `json:"depends_on"`
	SurvivesLoss bool   `json:"survives_loss"`
}

// transferAnswer translates a transferBoard into the wire document. It never prints a
// single net "worth +N points" figure — GainPerGW is a per-gameweek estimate over a
// horizon and Cost is a one-off, so adding them is a unit error; the break-even division is
// offered instead because a reader can check it.
func transferAnswer(board *transferBoard, cfg config.Config, boot *fpl.Bootstrap) transferDoc {
	doc := transferDoc{
		Free:       board.Free,
		Bank:       board.Bank,
		MoveLimit:  liveMoveLimit(cfg, board.Free),
		SellPrices: "market",
	}
	switch board.outcome() {
	case outcomeBank:
		doc.Outcome = "bank"
		doc.Reason = board.Advice.Explain()
	case outcomeNothing:
		doc.Outcome = "nothing"
		doc.Reason = "No move clears the threshold this week. Doing nothing is a real " +
			"answer and usually the right one."
	default:
		doc.Outcome = "recommend"
	}
	for _, p := range board.Plans {
		doc.Plans = append(doc.Plans, transferPlanDoc(board, p, boot))
	}
	return doc
}

// transferPlanDoc translates one analysis.Plan, pricing its own hit exactly as
// viewmodel.Transfers prices the reader's baseline diff: max(0, transfers-free) hits at
// fpl.HitCost each, zero when free is unlimited.
func transferPlanDoc(board *transferBoard, p analysis.Plan, boot *fpl.Bootstrap) transferPlan {
	hits := 0
	if board.Free != fpl.UnlimitedTransfers {
		if h := p.Transfers - board.Free; h > 0 {
			hits = h
		}
	}
	cost := hits * fpl.HitCost
	breakeven := 0
	if cost > 0 && p.GainPerGW > 0 {
		breakeven = int(math.Ceil(float64(cost) / p.GainPerGW))
	}
	moves := make([]viewmodel.Move, 0, len(p.Moves))
	for _, m := range p.Moves {
		moves = append(moves, moveFromSwap(boot, m))
	}
	return transferPlan{
		Moves: moves, Transfers: p.Transfers, GainPerGW: p.GainPerGW, Spend: p.Spend,
		Hits: hits, Cost: cost, BreakevenGWs: breakeven,
		DependsOn: p.DependsOn.Name, SurvivesLoss: p.SurvivesLoss,
	}
}

// moveFromSwap translates one analysis.Swap into a viewmodel.Move for the wire. Swap
// carries PlayerMetrics, whose ID is the season-scoped element id -- OutCode/InCode must
// be the permanent code, the keyspace every other write and diff in this codebase uses, so
// each side is re-resolved through the bootstrap.
func moveFromSwap(boot *fpl.Bootstrap, sw analysis.Swap) viewmodel.Move {
	outCode, inCode := sw.Out.ID, sw.In.ID
	if el := boot.ElementByID(sw.Out.ID); el != nil {
		outCode = el.Code
	}
	if el := boot.ElementByID(sw.In.ID); el != nil {
		inCode = el.Code
	}
	return viewmodel.Move{
		Pos:     sw.Out.Position,
		OutCode: outCode, OutName: sw.Out.Name, OutClub: sw.Out.Team, OutPrice: sw.Out.Price,
		InCode: inCode, InName: sw.In.Name, InClub: sw.In.Team, InPrice: sw.In.Price,
	}
}
