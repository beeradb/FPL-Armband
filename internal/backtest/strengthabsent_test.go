package backtest

import (
	"testing"

	"armband/internal/fpl"
)

// A season may DECLARE that its source carries no club strength ratings, and a
// bare table of zeros may not.
//
// # Why the distinction is the whole feature
//
// `hasTeamStrength` gates `Load`'s cache-validity conjunction, and the else
// branch is `fetch`, not an error. So a season whose teams table fails this
// check is silently REFETCHED and overwritten — which is right for a file an
// older parser wrote, and wrong for a season whose source genuinely has none.
// The two shapes are byte-identical on disk. Only a declaration separates them.
//
// The failure this pins is the one that follows from fixing it the easy way: if
// `hasTeamStrength` simply stopped caring about zeros, every stale cache written
// before the parser read strength would start reading as valid, and the guard
// would be gone while still looking present.
//
// ⚠️ The 2016-17/2017-18 archives are NOT wired by this, and must not be wired
// by setting the flag alone — see Season.StrengthAbsent. FPL's ratings for those
// seasons survive only as mid-season captures that have already absorbed
// results. The flag is for a season with NO ratings, not for one whose ratings
// would be hindsight.
func TestAStrengthlessSeasonMustDeclareItRatherThanJustBeZero(t *testing.T) {
	zero := []fpl.Team{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}

	t.Run("undeclared zeros still fail, which is the guard working", func(t *testing.T) {
		s := &Season{Name: "2016-17", Teams: zero}
		if s.hasTeamStrength() {
			t.Fatal("a non-empty teams table with every strength at zero passed " +
				"hasTeamStrength. That shape is what a cache written by a parser " +
				"which did not read strength looks like, so Load must treat it as " +
				"stale and refetch. Accepting it deletes the guard while leaving " +
				"it in the file.")
		}
	})

	t.Run("declared absence passes", func(t *testing.T) {
		s := &Season{Name: "2016-17", Teams: zero, StrengthAbsent: true}
		if !s.hasTeamStrength() {
			t.Fatal("a season declaring StrengthAbsent was rejected. Load would " +
				"refetch and overwrite it, and upstream has nothing to give — so " +
				"the season would silently revert to prior-only, which is the " +
				"behaviour this field exists to make impossible.")
		}
	})

	t.Run("a real rating still passes without any declaration", func(t *testing.T) {
		s := &Season{Name: "2021-22", Teams: []fpl.Team{
			{ID: 1, Name: "A"}, {ID: 2, Name: "B", StrengthAttackHome: 1200},
		}}
		if !s.hasTeamStrength() {
			t.Fatal("a teams table carrying a genuine rating was rejected")
		}
	})

	t.Run("an empty table still passes; prior-only declares itself in Absent", func(t *testing.T) {
		s := &Season{Name: "2016-17"}
		if !s.hasTeamStrength() {
			t.Fatal("an empty teams table was rejected; a prior-only season " +
				"declares itself in Absent, which absentIsConsistent checks")
		}
	})
}

// The declaration must not be able to paper over a table that DOES carry
// ratings. A season wearing the label falsely is mislabelled in exactly the
// direction that pollutes a pooled comparison: the label is the only thing
// telling a later reader that this season's fixture difficulty is flat until the
// blend moves it.
func TestDeclaringStrengthAbsentWhileCarryingRatingsIsRejected(t *testing.T) {
	honest := &Season{Name: "2016-17", Teams: []fpl.Team{{ID: 1, Name: "A"}}, StrengthAbsent: true}
	if !honest.strengthDeclarationIsConsistent() {
		t.Fatal("a season with no ratings that declares StrengthAbsent was called inconsistent")
	}

	lying := &Season{Name: "2021-22", StrengthAbsent: true, Teams: []fpl.Team{
		{ID: 1, Name: "A", StrengthDefenceHome: 1150},
	}}
	if lying.strengthDeclarationIsConsistent() {
		t.Fatal("a season declared StrengthAbsent while carrying real ratings and " +
			"nothing objected. Every figure from it would be pooled with seasons " +
			"whose difficulty came from FPL's own assessment, and the label is the " +
			"only thing that could have kept them apart.")
	}

	undeclared := &Season{Name: "2021-22", Teams: []fpl.Team{
		{ID: 1, Name: "A", StrengthDefenceHome: 1150},
	}}
	if !undeclared.strengthDeclarationIsConsistent() {
		t.Fatal("an ordinary season carrying ratings and not declaring anything " +
			"was called inconsistent")
	}
}
