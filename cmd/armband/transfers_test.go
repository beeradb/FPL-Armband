package main

import (
	"testing"

	"armband/internal/analysis"
)

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
	// {"snapshot", "capture", "ask"} by hand — it had already drifted from the map
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
		"backfill": true, "ask": true,
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
