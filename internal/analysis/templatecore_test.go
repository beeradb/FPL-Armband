package analysis

import (
	"sort"
	"testing"

	"armband/internal/fpl"
)

// Template-core tests drive the shipped Optimize lock path. Ownership is set
// on a constructed bootstrap so the core IDs are known without pinning live
// names or this week's scores. No applyTiebreak / TiebreakOwnership — this
// overlay is a LockIDs constraint, not the closed separable-band tiebreak.

func TestTemplateCoreKShipsOff(t *testing.T) {
	if got := DefaultWeights().TemplateCoreK; got != 0 {
		t.Errorf("TemplateCoreK ships at %d; 0 is off and must stay the default until a HOLD comparison resolves", got)
	}
}

func TestTemplateCoreIDsEmptyWhenKNonPositive(t *testing.T) {
	players := []PlayerMetrics{
		{ID: 1, Position: "MID", Team: "A", Ownership: 50},
		{ID: 2, Position: "FWD", Team: "B", Ownership: 40},
	}
	for _, k := range []int{0, -1, -4} {
		if got := TemplateCoreIDs(players, k); got != nil {
			t.Errorf("k=%d: got %v, want nil", k, got)
		}
	}
}

// Zero ownership must not invent a ranking. The ownership-tiebreak sweep
// already paid for a silent all-zero null that read as a measured tie.
func TestTemplateCoreIDsSkipsZeroOwnership(t *testing.T) {
	players := []PlayerMetrics{
		{ID: 3, Position: "MID", Team: "A", Ownership: 0},
		{ID: 1, Position: "DEF", Team: "B", Ownership: 0},
		{ID: 2, Position: "FWD", Team: "C", Ownership: 0},
	}
	if got := TemplateCoreIDs(players, 4); got != nil {
		t.Errorf("all-zero ownership yielded %v; want empty core (void trap)", got)
	}
}

func TestTemplateCoreIDsOrdersByOwnershipThenLowerID(t *testing.T) {
	players := []PlayerMetrics{
		{ID: 10, Position: "MID", Team: "A", Ownership: 40},
		{ID: 2, Position: "DEF", Team: "B", Ownership: 50},
		{ID: 7, Position: "FWD", Team: "C", Ownership: 50}, // same own as 2 → lower id first
		{ID: 3, Position: "GKP", Team: "D", Ownership: 30},
		{ID: 9, Position: "MID", Team: "E", Ownership: 20},
	}
	got := TemplateCoreIDs(players, 4)
	want := []int{2, 7, 10, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestTemplateCoreIDsRespectsPositionAndClubCaps(t *testing.T) {
	// Four highest-owned are all GKs — quota is 2, so the helper must skip
	// after two and take the next feasible players.
	players := []PlayerMetrics{
		{ID: 1, Position: "GKP", Team: "A", Ownership: 90},
		{ID: 2, Position: "GKP", Team: "B", Ownership: 80},
		{ID: 3, Position: "GKP", Team: "C", Ownership: 70},
		{ID: 4, Position: "GKP", Team: "D", Ownership: 60},
		{ID: 5, Position: "DEF", Team: "E", Ownership: 50},
		{ID: 6, Position: "MID", Team: "F", Ownership: 40},
	}
	got := TemplateCoreIDs(players, 4)
	want := []int{1, 2, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	// Four from one club — MaxPerClub is 3.
	sameClub := []PlayerMetrics{
		{ID: 11, Position: "MID", Team: "X", Ownership: 90},
		{ID: 12, Position: "DEF", Team: "X", Ownership: 80},
		{ID: 13, Position: "FWD", Team: "X", Ownership: 70},
		{ID: 14, Position: "GKP", Team: "X", Ownership: 60},
		{ID: 15, Position: "MID", Team: "Y", Ownership: 50},
	}
	got = TemplateCoreIDs(sameClub, 4)
	want = []int{11, 12, 13, 15}
	if len(got) != len(want) {
		t.Fatalf("club cap: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("club cap: got %v, want %v", got, want)
		}
	}
}

func TestTemplateCoreIDsIsDeterministic(t *testing.T) {
	players := []PlayerMetrics{
		{ID: 5, Position: "MID", Team: "A", Ownership: 33},
		{ID: 1, Position: "DEF", Team: "B", Ownership: 44},
		{ID: 9, Position: "FWD", Team: "C", Ownership: 44},
		{ID: 3, Position: "GKP", Team: "D", Ownership: 22},
		{ID: 8, Position: "MID", Team: "E", Ownership: 11},
	}
	a := TemplateCoreIDs(players, 3)
	b := TemplateCoreIDs(players, 3)
	if len(a) != len(b) {
		t.Fatalf("deterministic lengths diverge: %v vs %v", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("deterministic ids diverge: %v vs %v", a, b)
		}
	}
}

// squadIDsSorted returns the fifteen element ids sorted ascending.
func squadIDsSorted(sq *Squad) []int {
	ids := make([]int, 0, len(sq.Players))
	for _, p := range sq.Players {
		ids = append(ids, p.ID)
	}
	sort.Ints(ids)
	return ids
}

// coreOptimizeEngine is a synthetic Optimize-capable pool with controllable
// ownership. Built the same way as benchOptimizeEngine so it needs no network.
func coreOptimizeEngine(t *testing.T, n int) *Engine {
	t.Helper()
	e := benchOptimizeEngine(n)
	if len(e.Boot.Elements) < n {
		t.Fatalf("fixture has %d elements, want %d", len(e.Boot.Elements), n)
	}
	return e
}

// setOwnership writes SelectedByPercent on the bootstrap elements for the
// given ids. Metrics reads Ownership from that field.
func setOwnership(e *Engine, own map[int]float64) {
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if v, ok := own[el.ID]; ok {
			el.SelectedByPercent = fpl.Num(v)
		} else {
			el.SelectedByPercent = 0
		}
	}
}

// TestTemplateCoreOverlayOffMatchesUnconstrained pins that k=0 leaves
// Optimize bit-identical to the unconstrained path on the same engine.
func TestTemplateCoreOverlayOffMatchesUnconstrained(t *testing.T) {
	e := coreOptimizeEngine(t, 130)

	off := e.Weights
	off.TemplateCoreK = 0
	e.Weights = off
	sqOff, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("k=0 Optimize: %v", err)
	}

	plain := e.Weights
	plain.TemplateCoreK = 0 // same as DefaultWeights; field omitted is also 0
	e.Weights = plain
	sqPlain, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("plain Optimize: %v", err)
	}

	a, b := squadIDsSorted(sqOff), squadIDsSorted(sqPlain)
	if len(a) != SquadSize || len(b) != SquadSize {
		t.Fatalf("squad sizes %d and %d, want %d", len(a), len(b), SquadSize)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("k=0 diverged from unconstrained: %v vs %v", a, b)
		}
	}
}

// TestTemplateCoreOverlayLocksTheCore drives real Optimize with k=4 and a
// known ownership table. The four highest-owned feasible players must appear
// in the fifteen; budget, club and position rules still hold.
func TestTemplateCoreOverlayLocksTheCore(t *testing.T) {
	e := coreOptimizeEngine(t, 130)

	// Pick four players of distinct positions and clubs so the helper keeps
	// all four, then stamp ownership high enough that nothing else competes.
	byPos := map[string][]PlayerMetrics{}
	for _, m := range e.AllMetrics() {
		byPos[m.Position] = append(byPos[m.Position], m)
	}
	pick := func(pos string) PlayerMetrics {
		for _, p := range byPos[pos] {
			if p.Status == "available" || p.Status == "a" || p.Status == "" {
				// StatusLabel maps "a" → "available"; accept either.
				return p
			}
		}
		t.Fatalf("no %s in fixture", pos)
		return PlayerMetrics{}
	}
	// Prefer distinct clubs, then the lowest Score so unconstrained
	// Optimize is unlikely to already hold all four — a lock that never
	// binds is the void trap, not a test of the overlay.
	usedClub := map[string]bool{}
	pickDistinct := func(pos string) PlayerMetrics {
		var best PlayerMetrics
		found := false
		for _, p := range byPos[pos] {
			if usedClub[p.Team] {
				continue
			}
			if !found || p.Score < best.Score || (p.Score == best.Score && p.ID < best.ID) {
				best = p
				found = true
			}
		}
		if found {
			usedClub[best.Team] = true
			return best
		}
		return pick(pos)
	}
	corePlayers := []PlayerMetrics{
		pickDistinct("GKP"),
		pickDistinct("DEF"),
		pickDistinct("MID"),
		pickDistinct("FWD"),
	}
	own := map[int]float64{}
	wantCore := make([]int, 0, 4)
	for i, p := range corePlayers {
		if p.ID == 0 {
			t.Fatal("failed to pick a core player")
		}
		own[p.ID] = float64(90 - i*10)
		wantCore = append(wantCore, p.ID)
	}
	setOwnership(e, own)

	// Sanity: the helper agrees before Optimize runs.
	gotCore := TemplateCoreIDs(e.AllMetrics(), 4)
	if len(gotCore) != 4 {
		t.Fatalf("helper returned %v, want 4 ids from %v", gotCore, wantCore)
	}
	inHelper := map[int]bool{}
	for _, id := range gotCore {
		inHelper[id] = true
	}
	for _, id := range wantCore {
		if !inHelper[id] {
			t.Fatalf("helper missed core id %d; got %v want %v", id, gotCore, wantCore)
		}
	}

	off := e.Weights
	off.TemplateCoreK = 0
	e.Weights = off
	sq0, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("unconstrained Optimize: %v", err)
	}
	have0 := map[int]bool{}
	for _, p := range sq0.Players {
		have0[p.ID] = true
	}
	already := 0
	for _, id := range wantCore {
		if have0[id] {
			already++
		}
	}

	w := e.Weights
	w.TemplateCoreK = 4
	e.Weights = w
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("Optimize with template core: %v", err)
	}
	if len(sq.Players) != SquadSize {
		t.Fatalf("squad has %d players, want %d", len(sq.Players), SquadSize)
	}
	have := map[int]bool{}
	pos := map[string]int{}
	for _, p := range sq.Players {
		have[p.ID] = true
		pos[p.Position]++
	}
	for _, id := range wantCore {
		if !have[id] {
			t.Errorf("core id %d missing from Optimize result %v", id, squadIDsSorted(sq))
		}
	}
	if already == 4 {
		t.Errorf("unconstrained already held the whole core %v; pick lower-Score players so the lock has to bind", wantCore)
	}
	for want, n := range squadQuota {
		if pos[want] != n {
			t.Errorf("position %s = %d, want %d", want, pos[want], n)
		}
	}
	for club, n := range sq.ClubCounts {
		if n > MaxPerClub {
			t.Errorf("club %s has %d players, limit is %d", club, n, MaxPerClub)
		}
	}
	if sq.TotalCost > float64(DefaultBudget)/10+1e-9 {
		t.Errorf("cost £%.1fm exceeds budget £%.1fm", sq.TotalCost, float64(DefaultBudget)/10)
	}
}

// TestTemplateCoreOverlayZeroOwnershipIsUnconstrained: k=4 with every player
// at 0% owned must not change the fifteen — the lock never binds.
func TestTemplateCoreOverlayZeroOwnershipIsUnconstrained(t *testing.T) {
	e := coreOptimizeEngine(t, 130)
	setOwnership(e, nil) // all zero

	base := e.Weights
	base.TemplateCoreK = 0
	e.Weights = base
	sq0, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("k=0: %v", err)
	}

	on := e.Weights
	on.TemplateCoreK = 4
	e.Weights = on
	if core := TemplateCoreIDs(e.AllMetrics(), 4); core != nil {
		t.Fatalf("zero ownership produced core %v", core)
	}
	sq4, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("k=4 zero-own: %v", err)
	}
	a, b := squadIDsSorted(sq0), squadIDsSorted(sq4)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("zero-ownership k=4 changed the squad: %v vs %v", a, b)
		}
	}
}

// TestTemplateCoreOverlayRespectsCapsUnderOptimize: the four highest-owned
// are all GKs; the lock takes only two keepers plus the next feasible
// players, and Optimize still succeeds with that feasible prefix locked.
func TestTemplateCoreOverlayRespectsCapsUnderOptimize(t *testing.T) {
	e := coreOptimizeEngine(t, 130)

	var keepers, others []PlayerMetrics
	for _, m := range e.AllMetrics() {
		switch m.Position {
		case "GKP":
			keepers = append(keepers, m)
		default:
			others = append(others, m)
		}
	}
	if len(keepers) < 4 || len(others) < 2 {
		t.Fatalf("fixture too small: %d keepers, %d others", len(keepers), len(others))
	}
	own := map[int]float64{}
	// Four keepers outrank everyone.
	for i, k := range keepers[:4] {
		own[k.ID] = float64(90 - i)
	}
	own[others[0].ID] = 50
	own[others[1].ID] = 40
	setOwnership(e, own)

	core := TemplateCoreIDs(e.AllMetrics(), 4)
	if len(core) != 4 {
		t.Fatalf("feasible core length %d, want 4: %v", len(core), core)
	}
	// Exactly two keepers in the core.
	nGK := 0
	for _, id := range core {
		el := e.Boot.ElementByID(id)
		if el != nil && el.ElementType == 1 {
			nGK++
		}
	}
	if nGK != 2 {
		t.Fatalf("core %v has %d keepers, want 2 (squadQuota)", core, nGK)
	}

	w := e.Weights
	w.TemplateCoreK = 4
	e.Weights = w
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("Optimize with GK-heavy core: %v", err)
	}
	have := map[int]bool{}
	for _, p := range sq.Players {
		have[p.ID] = true
	}
	for _, id := range core {
		if !have[id] {
			t.Errorf("feasible core id %d missing from squad", id)
		}
	}
}

// Existing locks consume quota so the overlay cannot add a third GK.
func TestTemplateCoreRespectsAlreadyLockedQuota(t *testing.T) {
	e := coreOptimizeEngine(t, 130)
	var keepers []PlayerMetrics
	for _, m := range e.AllMetrics() {
		if m.Position == "GKP" {
			keepers = append(keepers, m)
		}
	}
	if len(keepers) < 3 {
		t.Fatalf("need 3 keepers, have %d", len(keepers))
	}
	own := map[int]float64{}
	for i, k := range keepers[:3] {
		own[k.ID] = float64(90 - i)
	}
	setOwnership(e, own)

	w := e.Weights
	w.TemplateCoreK = 4
	e.Weights = w
	sq, err := e.Optimize(OptimizeRequest{
		Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0,
		LockIDs: []int{keepers[2].ID}, // a lower-owned GK, already locked
	})
	if err != nil {
		t.Fatalf("Optimize with existing GK lock: %v", err)
	}
	nGK := 0
	for _, p := range sq.Players {
		if p.Position == "GKP" {
			nGK++
		}
	}
	if nGK != squadQuota["GKP"] {
		t.Errorf("GKP count %d, want %d", nGK, squadQuota["GKP"])
	}
}

func TestTemplateCoreSkipsInjured(t *testing.T) {
	players := []PlayerMetrics{
		{ID: 1, Position: "FWD", Team: "A", Ownership: 90, Status: "injured"},
		{ID: 2, Position: "MID", Team: "B", Ownership: 80, Status: "available"},
		{ID: 3, Position: "DEF", Team: "C", Ownership: 70, Status: "available"},
	}
	got := TemplateCoreIDs(players, 2)
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("got %v, want [2 3] (injured id 1 skipped)", got)
	}
}

func TestTemplateCoreDoesNotApplyToBoundedRevision(t *testing.T) {
	e := coreOptimizeEngine(t, 130)
	sq0, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	held := make([]int, 0, SquadSize)
	for _, p := range sq0.Players {
		held = append(held, p.ID)
	}
	own := map[int]float64{}
	// Stamp huge ownership on someone almost certainly outside the held 15.
	for _, m := range e.AllMetrics() {
		if !containsInt(held, m.ID) && m.Position == "MID" {
			own[m.ID] = 99
			break
		}
	}
	setOwnership(e, own)
	w := e.Weights
	w.TemplateCoreK = 4
	e.Weights = w
	sq, err := e.Optimize(OptimizeRequest{
		Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0,
		CurrentSquad: held, MaxChanges: 1,
	})
	if err != nil {
		t.Fatalf("bounded revision with TemplateCoreK set: %v (overlay must not apply)", err)
	}
	if len(sq.Players) != SquadSize {
		t.Fatalf("revision squad size %d", len(sq.Players))
	}
}

func TestTemplateCoreDoesNotMutateCallerLockIDs(t *testing.T) {
	e := coreOptimizeEngine(t, 130)
	own := map[int]float64{}
	n := 0
	for _, m := range e.AllMetrics() {
		if m.Position == "MID" && n < 4 {
			own[m.ID] = float64(80 - n)
			n++
		}
	}
	setOwnership(e, own)
	w := e.Weights
	w.TemplateCoreK = 4
	e.Weights = w
	caller := make([]int, 0, 8) // spare capacity
	orig := append([]int(nil), caller...)
	_, err := e.Optimize(OptimizeRequest{
		Budget: DefaultBudget, MinMinutes: 0, MinExpectedMinutes: 0,
		LockIDs: caller,
	})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if len(caller) != len(orig) {
		t.Fatalf("caller LockIDs mutated: len %d -> %d", len(orig), len(caller))
	}
}

func containsInt(xs []int, id int) bool {
	for _, x := range xs {
		if x == id {
			return true
		}
	}
	return false
}
