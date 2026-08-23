package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

func testToolbox(t *testing.T) *Toolbox {
	t.Helper()
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	ctx := context.Background()
	boot, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	fx, err := c.Fixtures(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	cfg := config.Default()
	e := analysis.NewEngineFull(boot, fx, cfg.Weights, cfg.Congestion, cfg.RoleRisk)
	return &Toolbox{Client: c, Engine: e, Cfg: cfg}
}

// TestSuggestTransfersPricesAFreeTransfer runs the tool end to end and checks
// the agent is told what a free move costs.
//
// Without the charge the replay churned — twelve round-trips across three
// seasons — so a candidate list that only reports a positive gain invites the
// same behaviour from the agent.
func TestSuggestTransfersPricesAFreeTransfer(t *testing.T) {
	tb := testToolbox(t)
	sq, err := tb.Engine.Optimize(analysis.OptimizeRequest{
		MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02,
	})
	if err != nil {
		t.Skipf("optimiser unavailable: %v", err)
	}
	// Several squads, because the tool returns only the best fifteen candidates and
	// the charge has to be exercised from both sides.
	//
	// The above-threshold side is easy: a violently degraded squad — its three best
	// players replaced by the cheapest bodies available — has fifteen obvious moves.
	//
	// **The sub-threshold side needs a squad that is only slightly wrong, and this
	// used to be left to luck.** The test previously took it from the optimiser's own
	// squad with a little bank, on the observation that this "surfaces a few marginal
	// moves". It surfaced exactly one, and when a scoring change removed that one the
	// test failed with the guard at the bottom of this function — correctly, since it
	// had stopped testing the threshold, but for a reason that was nothing to do with
	// the charge. Raising the bank does not help: at £8m the optimal squad yields two
	// candidates and both clear the charge.
	//
	// `gently` replaces one squad player with the unowned same-position player whose
	// score sits closest BELOW his, so the move back is a small gain BY CONSTRUCTION.
	// That is a mechanism rather than an observation, which is what makes it robust:
	// two ranks are used so the sub-threshold side does not depend on one player.
	intact := make([]int, 0, 15)
	for _, p := range sq.Players {
		intact = append(intact, p.ID)
	}
	degraded, bank := degrade(t, tb, sq.Players, 3)
	gentleMid, gentleMidBank := gently(t, tb, sq.Players, 5)
	gentleLow, gentleLowBank := gently(t, tb, sq.Players, 10)

	sources := []struct {
		name string
		ids  []int
		bank float64
	}{
		{"optimal squad", intact, 2.0},
		{"degraded squad", degraded, bank},
	}
	if gentleMid != nil {
		sources = append(sources, struct {
			name string
			ids  []int
			bank float64
		}{"gently degraded, 6th best", gentleMid, gentleMidBank})
	}
	if gentleLow != nil {
		sources = append(sources, struct {
			name string
			ids  []int
			bank float64
		}{"gently degraded, 11th best", gentleLow, gentleLowBank})
	}

	var cands []candidateRow
	for _, c := range sources {
		out, err := tb.suggestTransfersFor(context.Background(),
			suggestTransfersInput{SquadIDs: c.ids, Bank: &c.bank})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got, ok := out["free_transfer_value"].(float64); !ok || got <= 0 || got >= 4 {
			t.Fatalf("%s: free_transfer_value is %v; must be present and below a hit's 4",
				c.name, out["free_transfer_value"])
		}
		rows := candidateRows(t, out)
		t.Logf("%s: %d candidates", c.name, len(rows))
		cands = append(cands, rows...)
	}
	if len(cands) == 0 {
		t.Skip("no candidates from either squad")
	}

	charged := 0
	for _, m := range cands {
		free, hit := m.GainOverFree, m.GainOverHit
		// A hit costs strictly more than a free transfer, so its net must be
		// lower. If these ever match, the charge has stopped being applied.
		if hit >= free {
			t.Errorf("hit nets %.2f against a free transfer's %.2f; the charge is not applied", hit, free)
		}
		if m.WorthAFree != (free >= 0) {
			t.Errorf("worth_spending_a_free_transfer=%v but net over free is %.2f",
				m.WorthAFree, free)
		}
		if !m.WorthAFree {
			charged++
		}
	}
	t.Logf("%d candidates in total, %d of them not worth a free transfer", len(cands), charged)
	if charged == 0 || charged == len(cands) {
		t.Errorf("every candidate fell the same side of the charge (%d of %d); the test is "+
			"not exercising the threshold", charged, len(cands))
	}
}

// degrade replaces a squad's best players with the cheapest legal alternatives,
// banking the difference, so a transfer search has real work to do.
// gently swaps one squad player for the unowned same-position player whose score
// sits closest BELOW his — the smallest degradation the pool allows at his position.
//
// It exists to manufacture the sub-threshold side of the free-transfer charge on
// purpose rather than hoping the optimiser's own squad happens to carry a marginal
// move. Because the replacement is the nearest worse player, the move back is a
// small gain by construction, and a small gain is what "not worth a free transfer"
// means. `rank` is a position in the squad ordered by score, best first.
//
// Returns nil when no worse-and-no-dearer same-position player exists, which is
// normal for the weakest slots — the caller skips that source rather than failing.
func gently(t *testing.T, tb *Toolbox, squad []analysis.PlayerMetrics, rank int) ([]int, float64) {
	t.Helper()
	byScore := append([]analysis.PlayerMetrics(nil), squad...)
	sort.Slice(byScore, func(a, b int) bool { return byScore[a].Score > byScore[b].Score })
	if rank >= len(byScore) {
		return nil, 0
	}
	owned := map[int]bool{}
	clubs := map[string]int{}
	for _, p := range squad {
		owned[p.ID] = true
		clubs[p.Team]++
	}
	cur := byScore[rank]
	var best *analysis.PlayerMetrics
	pool := tb.Engine.AllMetrics()
	for i, c := range pool {
		if owned[c.ID] || c.Position != cur.Position || c.Minutes < 600 {
			continue
		}
		// Worse, and not dearer, so the squad stays legal on money.
		if c.Score >= cur.Score || c.Price > cur.Price {
			continue
		}
		if c.Team != cur.Team && clubs[c.Team] >= analysis.MaxPerClub {
			continue
		}
		if best == nil || c.Score > best.Score {
			best = &pool[i]
		}
	}
	if best == nil {
		return nil, 0
	}
	ids := make([]int, 0, len(squad))
	for _, p := range squad {
		if p.ID == cur.ID {
			ids = append(ids, best.ID)
			continue
		}
		ids = append(ids, p.ID)
	}
	return ids, cur.Price - best.Price
}

func degrade(t *testing.T, tb *Toolbox, squad []analysis.PlayerMetrics, n int) ([]int, float64) {
	t.Helper()
	byScore := append([]analysis.PlayerMetrics(nil), squad...)
	sort.Slice(byScore, func(a, b int) bool { return byScore[a].Score > byScore[b].Score })

	owned := map[int]bool{}
	clubs := map[string]int{}
	for _, p := range squad {
		owned[p.ID] = true
		clubs[p.Team]++
	}
	pool := tb.Engine.AllMetrics()
	replaced := map[int]analysis.PlayerMetrics{}
	var freed float64
	for _, cur := range byScore[:n] {
		var best *analysis.PlayerMetrics
		for i, c := range pool {
			if owned[c.ID] || c.Position != cur.Position || c.Minutes < 600 {
				continue
			}
			if c.Team != cur.Team && clubs[c.Team] >= analysis.MaxPerClub {
				continue
			}
			if best == nil || c.Price < best.Price {
				best = &pool[i]
			}
		}
		if best == nil {
			continue
		}
		owned[best.ID] = true
		clubs[cur.Team]--
		clubs[best.Team]++
		freed += cur.Price - best.Price
		replaced[cur.ID] = *best
	}
	ids := make([]int, 0, len(squad))
	for _, p := range squad {
		if r, ok := replaced[p.ID]; ok {
			ids = append(ids, r.ID)
			continue
		}
		ids = append(ids, p.ID)
	}
	return ids, freed
}

// candidateRow mirrors the tool's JSON so the test reads the same field names
// the agent does — a rename that broke the contract would fail here.
type candidateRow struct {
	GainPerGW    float64 `json:"xi_gain_per_gw"`
	GainOverFree float64 `json:"net_gain_if_transfer_is_free"`
	GainOverHit  float64 `json:"net_gain_if_it_costs_a_4pt_hit"`
	WorthAFree   bool    `json:"worth_spending_a_free_transfer"`
}

func candidateRows(t *testing.T, out map[string]any) []candidateRow {
	t.Helper()
	b, err := json.Marshal(out["candidates"])
	if err != nil {
		t.Fatal(err)
	}
	var rows []candidateRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("decoding candidates: %v", err)
	}
	return rows
}

// TestExcludedPlayersAreNeverOffered is the gap this feature exists to close.
//
// Locking and excluding already worked in optimize_squad, so a review could keep
// an injured player out of the squad it built — and then suggest_transfers,
// which knew nothing about it, would offer to buy him back the following week.
// An exclusion the transfer search ignores is worse than no exclusion, because
// it looks like the decision was made.
func TestExcludedPlayersAreNeverOffered(t *testing.T) {
	tb := testToolbox(t)
	sq, err := tb.Engine.Optimize(analysis.OptimizeRequest{
		MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02,
	})
	if err != nil {
		t.Skipf("optimiser unavailable: %v", err)
	}
	ids, bank := degrade(t, tb, sq.Players, 3)

	base, err := tb.suggestTransfersFor(context.Background(),
		suggestTransfersInput{SquadIDs: ids, Bank: &bank})
	if err != nil {
		t.Fatal(err)
	}
	rows := candidateRows(t, base)
	if len(rows) == 0 {
		t.Skip("no candidates to exclude")
	}
	// Exclude the single best target and confirm he disappears.
	target := topTarget(t, base)
	el := tb.Engine.Boot.ElementByID(target)
	if el == nil {
		t.Fatalf("candidate %d is not a real element", target)
	}
	if err := tb.Cfg.Roster.Set("exclude", config.RosterOverride{
		Code: el.Code, Name: el.WebName, Reason: "test", SetOn: "2026-08-05",
	}, nil); err != nil {
		t.Fatal(err)
	}

	after, err := tb.suggestTransfersFor(context.Background(),
		suggestTransfersInput{SquadIDs: ids, Bank: &bank})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range allTargets(t, after) {
		if id == target {
			t.Errorf("%s is excluded and still offered as a transfer target", el.WebName)
		}
	}
	// The result must say which overrides shaped it, or the agent cannot tell a
	// short candidate list from a filtered one.
	if notes, ok := after["standing_overrides"].([]string); !ok || len(notes) == 0 {
		t.Errorf("standing_overrides is %v; the result does not report the override "+
			"that shaped it", after["standing_overrides"])
	}
}

// TestLockedPlayersAreNeverSold — the other half. A player the analysis layer
// has decided the squad is built around must not be offered as the outgoing
// side of a swap.
func TestLockedPlayersAreNeverSold(t *testing.T) {
	tb := testToolbox(t)
	sq, err := tb.Engine.Optimize(analysis.OptimizeRequest{
		MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02,
	})
	if err != nil {
		t.Skipf("optimiser unavailable: %v", err)
	}
	ids, bank := degrade(t, tb, sq.Players, 3)

	base, err := tb.suggestTransfersFor(context.Background(),
		suggestTransfersInput{SquadIDs: ids, Bank: &bank})
	if err != nil {
		t.Fatal(err)
	}
	sold := outgoing(t, base)
	if len(sold) == 0 {
		t.Skip("no swaps to lock against")
	}
	el := tb.Engine.Boot.ElementByID(sold[0])
	if el == nil {
		t.Fatalf("outgoing %d is not a real element", sold[0])
	}
	if err := tb.Cfg.Roster.Set("lock", config.RosterOverride{
		Code: el.Code, Name: el.WebName, Reason: "test", SetOn: "2026-08-05",
	}, nil); err != nil {
		t.Fatal(err)
	}

	after, err := tb.suggestTransfersFor(context.Background(),
		suggestTransfersInput{SquadIDs: ids, Bank: &bank})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range outgoing(t, after) {
		if id == sold[0] {
			t.Errorf("%s is locked and still offered for sale", el.WebName)
		}
	}
}

type idRow struct {
	In  struct{ ID int } `json:"in"`
	Out struct{ ID int } `json:"out"`
}

func decodeRows(t *testing.T, out map[string]any, key string) []idRow {
	t.Helper()
	b, err := json.Marshal(out[key])
	if err != nil {
		t.Fatal(err)
	}
	var rows []idRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("decoding %s: %v", key, err)
	}
	return rows
}

func topTarget(t *testing.T, out map[string]any) int {
	t.Helper()
	rows := decodeRows(t, out, "candidates")
	if len(rows) == 0 {
		t.Fatal("no candidates")
	}
	return rows[0].In.ID
}

func allTargets(t *testing.T, out map[string]any) []int {
	t.Helper()
	var ids []int
	for _, r := range decodeRows(t, out, "candidates") {
		ids = append(ids, r.In.ID)
	}
	return ids
}

func outgoing(t *testing.T, out map[string]any) []int {
	t.Helper()
	var ids []int
	for _, r := range decodeRows(t, out, "candidates") {
		ids = append(ids, r.Out.ID)
	}
	return ids
}

// TestConcurrentOverridesAllPersist guards a bug that lost real findings.
//
// The tool runner fans tool calls out through an errgroup, so an agent that
// records several overrides in one turn runs them at the same moment. Each was a
// read-modify-write of the whole config file, so they raced and the last writer
// won.
//
// The first live run after set_player_status shipped made five calls and
// persisted two: batches of (Isak, Saliba, Dubravka) and (Raya, Gabriel) left
// only Dubravka and Gabriel. Nothing errored — three findings simply vanished,
// which is the worst possible failure for a mechanism whose entire purpose is
// that findings survive.
func TestConcurrentOverridesAllPersist(t *testing.T) {
	tb := testToolbox(t)
	dir := t.TempDir()
	tb.ConfigPath = filepath.Join(dir, "config.json")

	// Ten real players, written at once.
	var codes []int
	var names []string
	for i := range tb.Engine.Boot.Elements {
		el := tb.Engine.Boot.Elements[i]
		if el.Code == 0 {
			continue
		}
		codes = append(codes, el.Code)
		names = append(names, el.WebName)
		if len(codes) == 10 {
			break
		}
	}
	if len(codes) < 10 {
		t.Skip("not enough players")
	}

	var wg sync.WaitGroup
	for i, code := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tb.updateConfig(func(cfg *config.Config) error {
				return cfg.Roster.Set("exclude", config.RosterOverride{
					Code: code, Name: names[i], Reason: "concurrency test",
					SetOn: "2026-08-05", LastChecked: "2026-08-05",
				}, nil)
			})
		}()
	}
	wg.Wait()

	// Every one must be in memory and on disk.
	if got := len(tb.Cfg.Roster.Exclude); got != 10 {
		t.Errorf("%d of 10 overrides survived in memory; the writes are racing", got)
	}
	saved, err := config.Load(tb.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(saved.Roster.Exclude); got != 10 {
		t.Errorf("%d of 10 overrides reached disk; the last writer is winning", got)
	}
	onDisk := map[int]bool{}
	for _, o := range saved.Roster.Exclude {
		onDisk[o.Code] = true
	}
	for i, code := range codes {
		if !onDisk[code] {
			t.Errorf("%s was recorded and is not in the saved config", names[i])
		}
	}
}

// TestSuppliedSquadStillGetsTheBank — the balance must not depend on who
// supplied the fifteen.
//
// The bank used to be read only on the path where the tool fetched the squad
// itself, so an agent that passed current_squad_ids and left bank_m out ran the
// search with £0.0m. Nothing errors in that state: RankSwaps simply finds no
// affordable upgrade, which reads exactly like a squad with nothing worth
// buying. The transfer is lost silently, which is the failure mode this whole
// mechanism exists to avoid.
func TestSuppliedSquadStillGetsTheBank(t *testing.T) {
	tb := testToolbox(t)
	sq, err := tb.Engine.Optimize(analysis.OptimizeRequest{
		MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02,
	})
	if err != nil {
		t.Skipf("optimiser unavailable: %v", err)
	}
	ids := make([]int, 0, 15)
	for _, p := range sq.Players {
		ids = append(ids, p.ID)
	}

	// Nothing known and nothing supplied: £0.0m is the honest answer, and it
	// has to be announced rather than passed off as the real balance.
	out, err := tb.suggestTransfersFor(context.Background(), suggestTransfersInput{SquadIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if got := out["bank_m"]; got != 0.0 {
		t.Errorf("bank_m is %v with no balance available, want 0", got)
	}
	if _, ok := out["bank_warning"]; !ok {
		t.Error("searched with an assumed £0.0m and said nothing about it")
	}

	// Reconstructed from the squad's price history, as main.go supplies it.
	twenty := 20
	tb.Engine.Bank = &twenty
	out, err = tb.suggestTransfersFor(context.Background(), suggestTransfersInput{SquadIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if got := out["bank_m"]; got != 2.0 {
		t.Errorf("bank_m is %v with £2.0m reconstructed, want 2 — the balance is not "+
			"reaching a search whose squad was supplied by the caller", got)
	}
	if _, ok := out["bank_warning"]; ok {
		t.Error("warned about an assumed bank when a real one was available")
	}

	// An explicit zero is a scenario, not a missing value, and must survive.
	zero := 0.0
	out, err = tb.suggestTransfersFor(context.Background(),
		suggestTransfersInput{SquadIDs: ids, Bank: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if got := out["bank_m"]; got != 0.0 {
		t.Errorf("bank_m is %v when the caller asked for £0.0m; an explicit zero must "+
			"outrank the real balance, or a what-if cannot be asked", got)
	}
}
