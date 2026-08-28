package main

import (
	"os"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// TestTheBankingSwitchReachesTheRule is the reachability guard for the whole
// capability.
//
// Banking existed for a long time as a replay-only field: wired, correct, and
// unreachable from any command a user runs. A config knob that does not arrive
// at its consumer is this project's signature failure — a byte-identical result
// that looks exactly like a knob that does nothing — so what has to be pinned is
// not the decision but the fact that the switch is consulted at all.
//
// Asserted on adviseBanking's second return value, which is precisely "the rule
// was asked". No network and no squad: both refusals are checked before the
// engine is touched, which is also why a nil engine is safe here.
func TestTheBankingSwitchReachesTheRule(t *testing.T) {
	cfg := config.Default()
	// The setting ships ON now; the off-arm is written against an explicit off,
	// which is the state an old config file loads into.
	cfg.Review.BankTransfersLookahead = false
	// Off: the rule is not consulted, and the command must therefore print
	// nothing about a decision nobody made.
	if _, consulted := adviseBanking(cfg, nil, analysis.SquadState{}, nil, 1, 10, 5); consulted {
		t.Error("the rule was consulted with the setting off")
	}
	// On, but before the first deadline: unlimited transfers is not a bankable
	// state, since there is no allowance to accumulate.
	cfg.Review.BankTransfersLookahead = true
	if _, consulted := adviseBanking(cfg, nil, analysis.SquadState{}, nil,
		fpl.UnlimitedTransfers, 10, 5); consulted {
		t.Error("the rule was consulted on an unlimited allowance")
	}
}

// TestChipPreparationIsOffUntilAsked pins the switch on the other half.
//
// The credit is the only channel by which a chip can be prepared for, and
// switched off it must credit nothing however the chips are planned. A version
// that fired on the chip plan alone would reprice every transfer for a chip the
// manager never asked to prepare for.
func TestChipPreparationIsOffUntilAsked(t *testing.T) {
	cfg := config.Default()
	if cfg.Review.PrepareForChips {
		t.Fatal("the setting ships off; this test's off-arm would be vacuous")
	}
	cfg.Chips.First.BenchBoost = 12
	// Compared field by field: ChipCredit carries a map, so it is not comparable,
	// and the fields are what the search actually reads.
	if cr := chipCreditNow(cfg, nil, 10, 5); cr.Bench != 0 || cr.Captain != 0 || cr.WeekLoad != nil {
		t.Errorf("preparation off must credit nothing even with a chip planned: %+v", cr)
	}
}

// stubPlans is a planFn returning fixed packages, so the live banking rule can be
// exercised without a bootstrap, a squad or a network.
//
// The limit is the whole point: it returns a two-move package only when two moves
// are affordable, which is the capability one more free transfer buys and the
// only thing the rule can be deciding about.
func stubPlans(solo, pair float64) planFn {
	return func(_ analysis.SquadState, limit int) []analysis.Plan {
		out := []analysis.Plan{{GainPerGW: solo, Transfers: 1}}
		if limit >= 2 {
			out = append(out, analysis.Plan{GainPerGW: pair, Transfers: 2})
		}
		return out
	}
}

// TestTheLiveBankingRuleDecidesBothWays is the positive arm the switch test
// cannot reach.
//
// The reachability test above asserts only that the rule is NOT consulted when it
// should not be — both refusals return before the engine is touched. So the suite
// pinned that the switch turns the rule off and nothing pinned that it turns it
// on, which is the identical hole a review had just proved for the sweep's own
// wiring one commit earlier. The planFn seam exists to close it.
//
// A nil engine is safe here because `liveHorizon` is the only engine read on this
// path and the test supplies the horizon through a config with no chip plan —
// which is exactly why the horizon clamp is checked separately, below.
func TestTheLiveBankingRuleDecidesBothWays(t *testing.T) {
	cfg := config.Default()
	cfg.Review.BankTransfersLookahead = true
	cfg.Review.MaxHitsPerWeek = 0 // so one more free transfer is a real capability
	cfg.Review.FreeTransferValue = 0
	cfg.Review.MinGainForTransfer = 0

	// A two-move package worth far more than any single swap: waiting buys the
	// capability, and wins even after losing a gameweek of it.
	//   now   = 1.0 x 5 = 5      later = 4.0 x 4 = 16
	got, consulted := adviseBanking(cfg, nil, analysis.SquadState{}, stubPlans(1.0, 4.0), 1, 10, 5)
	if !consulted {
		t.Fatal("the rule was not consulted with the setting on and a finite allowance")
	}
	if !got.Bank || !got.Weighed() {
		t.Errorf("waiting buys a package worth four times the best single swap and "+
			"the rule declined to bank: %+v", got)
	}
	// And the mirror: when the extra move buys nothing, acting now wins because
	// waiting costs a gameweek.
	//   now   = 1.0 x 5 = 5      later = 1.0 x 4 = 4
	got, _ = adviseBanking(cfg, nil, analysis.SquadState{}, stubPlans(1.0, 0.1), 1, 10, 5)
	if got.Bank {
		t.Errorf("the extra transfer buys nothing and the rule banked anyway: %+v", got)
	}
	if !got.Weighed() {
		t.Errorf("both arms were worth something, so the rule weighed a real choice: %+v", got)
	}
}

// TestTheLiveHorizonRunsOutWithTheSeason pins the clamp that was missing.
//
// `EffectiveHorizon` shortens for a planned wildcard and does NOT clamp at the
// end of the season, because it answers "how long must this squad serve" rather
// than "how many gameweeks are left". The replay clamps separately. Without the
// second clamp the banking rule's horizon guard is unreachable live, and a
// manager at GW38 is advised to hold a transfer into a gameweek that does not
// exist — with `BankGuardHorizon` dead code on the only path that renders it.
func TestTheLiveHorizonRunsOutWithTheSeason(t *testing.T) {
	// The guard fires on a horizon of one or less, whatever the arms are worth.
	a := analysis.AdviseBank(1, 5, 1, 0, 99)
	if a.Bank || a.Guard != analysis.BankGuardHorizon {
		t.Fatalf("the horizon guard did not fire at one gameweek left: %+v", a)
	}
	// And it is reachable, which is the half that was broken: at GW38 there is
	// one gameweek left however long the configured horizon is.
	for gw, want := range map[int]float64{38: 1, 37: 2, 30: 5} {
		if got := liveHorizonFor(gw, 5); got != want {
			t.Errorf("GW%d leaves %v gameweeks, want %v", gw, got, want)
		}
	}
}

// TestTheCommandAndThePageAgreeOnABankedWeek pins the decision both renderers
// obey.
//
// They each read the same board and each decided for themselves what it meant:
// the page returned no plan when the rule said wait, while the command printed
// the advice and then rendered the moves and a team sheet anyway. Same config,
// same squad, opposite recommendations — the exact failure the shared board was
// introduced to prevent, one layer up from where it was prevented.
//
// Pinned on `outcome` rather than on printed text, because that is now the one
// value both switch on: a renderer that stopped consulting it would be a new
// divergence and is what this refuses.
func TestTheCommandAndThePageAgreeOnABankedWeek(t *testing.T) {
	plans := []analysis.Plan{{GainPerGW: 1.5, Transfers: 1}}
	for _, c := range []struct {
		name string
		b    transferBoard
		want boardOutcome
	}{{
		name: "banking says wait, and a plan exists",
		b:    transferBoard{Plans: plans, Consulted: true, Advice: analysis.BankAdvice{Bank: true}},
		want: outcomeBank,
	}, {
		name: "banking consulted and says act",
		b:    transferBoard{Plans: plans, Consulted: true},
		want: outcomeRecommend,
	}, {
		name: "banking switched off",
		b:    transferBoard{Plans: plans},
		want: outcomeRecommend,
	}, {
		name: "no plan on offer",
		b:    transferBoard{},
		want: outcomeNothing,
	}, {
		// The case that matters most: a banked week must not become a
		// recommendation just because the search found something.
		name: "banking says wait and the search is full of ideas",
		b: transferBoard{
			Plans:     append(plans, analysis.Plan{GainPerGW: 9.0, Transfers: 2}),
			Consulted: true, Advice: analysis.BankAdvice{Bank: true},
		},
		want: outcomeBank,
	}} {
		if got := c.b.outcome(); got != c.want {
			t.Errorf("%s: outcome %d, want %d", c.name, got, c.want)
		}
	}
}

// TestTheTransferBoardWiresTheBankingDecision is a source scan, and it is a
// tripwire rather than a proof.
//
// A review deleted both wiring lines from `buildTransferBoard` — the banking call
// and the chip credit — and build, vet and the whole suite stayed green, because
// nothing in the repository calls that function: it needs an entry, a squad and
// the network. The decision logic beneath it is unit-tested, so what is left
// uncovered is precisely the join — the same shape as the sweep's own wiring one
// commit earlier, and the same shape as TestEveryCellRowIsStamped's subject.
//
// It matches an idiom keyed on one spelling, so a rename defeats it. That is the
// acknowledged limit of every scan in this project; the reason to have it anyway
// is that deleting a line is what actually happens.
func TestTheTransferBoardWiresTheBankingDecision(t *testing.T) {
	src, err := os.ReadFile("transfers.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func buildTransferBoard(")
	if start < 0 {
		t.Fatal("buildTransferBoard is gone; this scan needs rewriting rather than deleting")
	}
	end := strings.Index(body[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not find the end of buildTransferBoard")
	}
	fn := body[start : start+end]
	for _, want := range []struct{ call, why string }{
		{"adviseBanking(", "the banking decision would never be computed, and every " +
			"board would report Consulted false — indistinguishable from the switch " +
			"being off"},
		{"chipCreditNow(", "a planned chip would be priced into the banking comparison " +
			"and not into the plans printed beside it"},
	} {
		if !strings.Contains(fn, want.call) {
			t.Errorf("buildTransferBoard no longer calls %s — %s", want.call, want.why)
		}
	}
}

// TestPlanSquadPicksTheViceLikeTheModelDoes pins the one piece of judgement in
// the transfers command: a Plan carries a captain but no vice, and the pitch
// needs one.
//
// The rule has to be the model's own — the second-highest scorer in the picked
// eleven — because the replay's vice-captain fix ships on exactly that: FPL
// really does pass the armband when the captain records no minutes, and the
// scoring path and the objective must not disagree about who wears it. A
// display layer inventing a different vice would put a different player on the
// team sheet from the one the model priced.
func TestPlanSquadPicksTheViceLikeTheModelDoes(t *testing.T) {
	mk := func(id int, name string, score float64) analysis.PlayerMetrics {
		return analysis.PlayerMetrics{ID: id, Name: name, Score: score, Price: 5.0,
			Position: "MID", Team: "ARS"}
	}
	xi := []analysis.PlayerMetrics{
		mk(1, "best", 9.0),
		mk(2, "second", 7.0),
		mk(3, "third", 5.0),
	}
	p := analysis.Plan{XI: xi, Squad: xi, Captain: xi[0], Formation: "3-4-3"}

	sq := planSquad(p)
	if sq.ViceCaptain.ID != 2 {
		t.Errorf("vice is %q (id %d), want the second-highest scorer in the eleven",
			sq.ViceCaptain.Name, sq.ViceCaptain.ID)
	}
	if sq.Captain.ID != 1 {
		t.Errorf("captain changed: %d", sq.Captain.ID)
	}
	if sq.XIScore != 21.0 {
		t.Errorf("XIScore = %v, want 21", sq.XIScore)
	}
	// The armband doubles one player, and that difference is the whole reason
	// both totals are printed.
	if sq.ExpectedPoints != 30.0 {
		t.Errorf("ExpectedPoints = %v, want 30 (XI plus the captain again)",
			sq.ExpectedPoints)
	}
}

// TestPlanSquadNeverMakesTheCaptainHisOwnVice guards the off-by-one that the
// obvious implementation has: scanning for the highest scorer without excluding
// the captain returns the captain.
func TestPlanSquadNeverMakesTheCaptainHisOwnVice(t *testing.T) {
	mk := func(id int, score float64) analysis.PlayerMetrics {
		return analysis.PlayerMetrics{ID: id, Score: score, Position: "MID"}
	}
	xi := []analysis.PlayerMetrics{mk(1, 9.0), mk(2, 8.0)}
	sq := planSquad(analysis.Plan{XI: xi, Squad: xi, Captain: xi[0]})
	if sq.ViceCaptain.ID == sq.Captain.ID {
		t.Error("the captain is his own vice-captain, so a blank would double nobody")
	}
}

// TestRejectFlagsAfterCommand covers the silent no-op that `armband squad
// -html out.html` used to be: Go stops parsing flags at the command name, so
// the file was never written and nothing said so.
func TestRejectFlagsAfterCommand(t *testing.T) {
	if err := rejectFlagsAfterCommand("squad", []string{"-html", "out.html"}); err == nil {
		t.Error("a flag after the command was accepted; it would be ignored silently")
	}
	if err := rejectFlagsAfterCommand("squad", nil); err != nil {
		t.Errorf("a clean invocation was rejected: %v", err)
	}
	// backtest takes positional arguments, which must still work.
	if err := rejectFlagsAfterCommand("backtest", []string{"2023-24", "8"}); err != nil {
		t.Errorf("positional arguments were rejected: %v", err)
	}
	// The commands that parse their own flags must be left alone, or this guard
	// breaks `capture -list` and `snapshot -constants`.
	//
	// ⚠️ Iterate the MAP, never a hand-written copy of it. This loop used to list
	// {"snapshot", "capture", "ask"} by hand (`ask` has since been retired) — it
	// had already drifted from the map
	// by one entry (`backfill`), and when `reviewkey` was added to the map's
	// intended membership but not to the map, nothing failed: the command shipped
	// unusable, because every documented invocation of it was rejected here and no
	// test exercised the pairing. One quantity, two implementations, in the guard
	// against a silent no-op.
	for cmd := range commandsThatParseTheirOwnFlags {
		if err := rejectFlagsAfterCommand(cmd, []string{"-list"}); err != nil {
			t.Errorf("%s parses its own flags and must be exempt: %v", cmd, err)
		}
	}
}

// TestEverySelfParsingCommandIsDispatchedBeforeTheGuard pins the other half of the
// pairing: being exempt is useless if the command never runs.
//
// `reviewkey` was exempt-in-intent and absent from the map, so `armband reviewkey
// -out x` errored with "flags must come before the command" while `armband -out x
// reviewkey` errored with "flag provided but not defined". There was no third
// invocation, and `go build`, `go vet` and `go test ./...` were all clean.
func TestEverySelfParsingCommandIsDispatchedBeforeTheGuard(t *testing.T) {
	// Every exempt command must be a command main actually knows. A typo here is a
	// permanent exemption for a name nothing dispatches, which is how the guard
	// would silently stop covering a real command that was renamed.
	known := map[string]bool{
		"snapshot": true, "capture": true,
		"backfill": true, "serve": true, "drift": true,
	}
	for cmd := range commandsThatParseTheirOwnFlags {
		if !known[cmd] {
			t.Errorf("%q is exempt from the flag guard but is not a known command; "+
				"either it was renamed or the exemption is a typo", cmd)
		}
	}
	for cmd := range known {
		if !commandsThatParseTheirOwnFlags[cmd] {
			t.Errorf("%q parses its own flags but is not exempt, so every flag passed "+
				"to it is rejected before it runs", cmd)
		}
	}
}

// TestTheTransfersBankLineNamesTheChipPlan pins the second consumer of
// EffectiveHorizon's reason string.
//
// The chip plan has ALWAYS been in the banking arithmetic — liveHorizon calls
// EffectiveHorizon — but the reason was discarded with `h, _ :=` and nothing
// printed it, so the effect was invisible. Measured on the house squad at GW2 on
// 2026-08-24: moving the planned wildcard from GW6 to GW4 took the arms from
// "now 3.74 · waiting 2.31" to "now 0.87 · waiting 0.00" while the recommended
// transfers, their gain and their hinge were byte-identical. Two runs differing
// only in a number, with nothing saying why, is the shape this scan exists to
// stop coming back.
//
// A source scan rather than a behaviour test on purpose: the STRING is already
// pinned by analysis.TestWildcardShortensHorizon, and what is new here is only
// that a second command reads it. Re-deriving the wording here would be a second
// implementation of one quantity, which is this project's signature failure.
func TestTheTransfersBankLineNamesTheChipPlan(t *testing.T) {
	src, err := os.ReadFile("transfers.go")
	if err != nil {
		t.Fatalf("reading transfers.go: %v", err)
	}
	s := string(src)

	i := strings.Index(s, "bankLine(board.Advice)")
	if i < 0 {
		t.Fatal("bankLine call not found — this scan is anchored on it and needs updating")
	}
	after := s[i:]
	if j := strings.Index(after, "\n\t}"); j > 0 {
		after = after[:j]
	}
	if !strings.Contains(after, "EffectiveHorizon") {
		t.Error("the banking line no longer names the chip plan.\n" +
			"liveHorizon puts the planned wildcard into `now` and `waiting`; if nothing " +
			"prints EffectiveHorizon's reason beside them, a reader sees the arithmetic " +
			"move with no explanation. See this test's own comment for the measurement.")
	}
}
