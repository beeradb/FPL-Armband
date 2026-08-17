package main

import (
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
	if cfg.Review.BankTransfersLookahead {
		t.Fatal("the setting ships off; this test's off-arm would be vacuous")
	}
	// Off: the rule is not consulted, and the command must therefore print
	// nothing about a decision nobody made.
	if _, consulted := adviseBanking(cfg, nil, analysis.SquadState{}, nil, 0, 1, 10); consulted {
		t.Error("the rule was consulted with the setting off")
	}
	// On, but before the first deadline: unlimited transfers is not a bankable
	// state, since there is no allowance to accumulate.
	cfg.Review.BankTransfersLookahead = true
	if _, consulted := adviseBanking(cfg, nil, analysis.SquadState{}, nil,
		0, fpl.UnlimitedTransfers, 1); consulted {
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
	if cr := chipCreditNow(cfg, nil, 10); cr.Bench != 0 || cr.Captain != 0 || cr.WeekLoad != nil {
		t.Errorf("preparation off must credit nothing even with a chip planned: %+v", cr)
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
		"snapshot": true, "reviewkey": true, "capture": true,
		"backfill": true,
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
