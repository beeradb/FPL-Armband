package analysis

import (
	"fmt"
	"strings"

	"armband/internal/fpl"
)

// RoleRisk captures uncertainty about whether a player will hold the role his
// statistics imply. It is distinct from congestion (which is about load) and
// from expected minutes (which is about last season's evidence): this is about
// evidence that may no longer apply at all.
//
// Two sources, in descending order of severity:
//   - The player changed clubs. Every minute, goal and expected-goal figure
//     attached to him was earned somewhere else, under a different manager, in
//     a different system, competing with different team-mates.
//   - The club changed manager. The squad's numbers are real, but who starts
//     and in what shape is now an open question.
type RoleRisk struct {
	// NewSigningPenalty multiplies the score of a player who joined his club
	// this summer. Set to 1 to disable.
	//
	// Calibrated against last season's data rather than intuition. Comparing
	// players in their first season at a club against established team-mates:
	//
	//   population              mean pts   mean mins   pts/90
	//   first-season signings      69.3       1431      4.36
	//   already at the club        76.3       1623      4.23
	//
	// Signings are not worse players — they are marginally BETTER per 90. The
	// entire gap is minutes: roughly 12% fewer among players who feature at
	// all, and they are about a fifth less likely to become 60+ minute
	// regulars. Among those who do establish themselves (1800+ minutes) the
	// two groups are indistinguishable: 119.8 pts at 4.21 per 90 versus
	// 120.2 at 4.17.
	//
	// So this is a minutes-availability discount, not a quality discount, and
	// 0.88 reflects the observed gap. Raise it toward 0.75 if you want to be
	// deliberately more conservative than the evidence — that is a legitimate
	// risk preference, but know that you are overriding the data, not applying it.
	NewSigningPenalty float64 `json:"new_signing_penalty"`
	// NewSigningGameweeks is how long the penalty applies. After this many
	// gameweeks the FPL API's minutes reflect the new club, so the evidence is
	// real and the discount is no longer warranted.
	NewSigningGameweeks int `json:"new_signing_gameweeks"`
	// ConfirmedStarters exempts named players from the new-signing penalty —
	// use it for signings you are confident walk straight into the XI.
	// Names or FPL ids.
	ConfirmedStarters []string `json:"confirmed_starters"`

	// NewCoachClubs are club short names that changed manager. Applies to
	// every player at the club.
	NewCoachClubs []string `json:"new_coach_clubs"`
	// NewCoachPenalty ships at 1.0 — disabled — because a manager change does
	// not cost an established player points. See the calibration note below.
	// It is kept as a lever, not because the measured effect is a discount.
	NewCoachPenalty float64 `json:"new_coach_penalty"`
	// NewCoachGameweeks is how long that uncertainty is priced in.
	NewCoachGameweeks int `json:"new_coach_gameweeks"`
}

// DefaultRoleRisk. NewCoachPenalty was 0.93 by intuition and is now 1.0 by
// measurement — the effect is real but it is a variance effect, and a mean
// multiplier is the wrong instrument for it. The evidence is in
// TestDiagNewCoachPenalty: 82 established players (1500+ minutes the previous
// season, same club both seasons) across the three manager-change summers the
// archive reaches.
//
//	                     minutes   pts/90   points
//	new manager            0.797    1.072    0.895
//	manager continued      0.879    0.994    0.893
//
// Minutes fall about 8% — which is exactly what 0.93 encoded, and would have
// been the right number had the term multiplied expected minutes. It does not.
// It multiplies Score, which is expected points, and there the difference is
// +0.003: the players who keep their place raise their per-90 output by just
// enough to cancel the minutes the group loses. Discounting points by 7% was
// charging for a cost that is not paid.
//
// What a new manager actually does is redistribute. 35% of established players
// fell below half their previous minutes, against 21% under a continuing
// manager, and the club-level outcome ranged from 0.60 (West Ham under
// Lopetegui) to 1.17 (Liverpool under Slot). The direction is unpredictable and
// the magnitude is large, so shaving every player at the club by a fixed
// fraction penalises the ones about to benefit just as hard as the ones about
// to be dropped. If this is ever priced again it belongs in rotation risk, not
// in the score.
//
// There is also a selection confound worth naming: managers are appointed after
// bad seasons, so mean reversion and the appointment push the same way. That
// makes the measured points effect, if anything, flattering to the penalty.
func DefaultRoleRisk() RoleRisk {
	return RoleRisk{
		NewSigningPenalty:   0.88,
		NewSigningGameweeks: 5,
		ConfirmedStarters:   []string{},

		NewCoachClubs:     DefaultNewCoachClubs(),
		NewCoachPenalty:   1.0,
		NewCoachGameweeks: 6,
	}
}

// DefaultNewCoachClubs lists the 2026/27 managerial changes. The FPL API does
// not publish managers at all, so this is hand-maintained and MUST be re-derived
// every summer — a stale list silently penalises the wrong ten clubs.
//
// 2026/27 set a Premier League record for turnover. Nine clubs appointed a
// manager during the summer of 2026:
//
//	BOU Marco Rose      CHE Xabi Alonso     CRY Pierre Sage
//	FUL Álvaro Arbeloa  IPS Gary O'Neil     LIV Andoni Iraola
//	MCI Enzo Maresca    NEW Matthias Jaissle (1 Aug, replacing Eddie Howe)
//	NFO Oliver Glasner
//
// TOT is included as a tenth. Roberto De Zerbi arrived on 31 March 2026 with
// only eight matches of 2025/26 left, so essentially all of the Spurs data the
// model reads was earned under someone else — which is exactly the condition
// this penalty prices.
//
// MUN is deliberately excluded. Michael Carrick took over on 13 January 2026,
// so roughly half of last season's United minutes are already his; the evidence
// the model reads does reflect the current manager.
func DefaultNewCoachClubs() []string {
	return []string{"BOU", "CHE", "CRY", "FUL", "IPS", "LIV", "MCI", "NEW", "NFO", "TOT"}
}

// roleFactor returns the combined role-uncertainty multiplier and the reasons
// behind it. 1.0 means the player's statistical record can be taken at face value.
func (e *Engine) roleFactor(el *fpl.Element, isNewSigning bool) (float64, []string) {
	rr := e.Role
	next := e.Boot.NextEvent()
	gw := 1
	if next != nil {
		gw = next.ID
	}

	factor := 1.0
	var notes []string

	if isNewSigning && rr.NewSigningPenalty > 0 && rr.NewSigningPenalty < 1 &&
		gw <= rr.NewSigningGameweeks {
		if e.confirmedStarter(el) {
			notes = append(notes, "new signing, but flagged as a confirmed starter")
		} else {
			factor *= rr.NewSigningPenalty
			notes = append(notes,
				"new signing — minutes at this club unproven (historically ~12% fewer, "+
					"though per-90 output holds up)")
		}
	}

	if rr.NewCoachPenalty > 0 && rr.NewCoachPenalty < 1 && gw <= rr.NewCoachGameweeks {
		if team := e.Boot.TeamByID(el.Team); team != nil && containsFold(rr.NewCoachClubs, team.ShortName) {
			factor *= rr.NewCoachPenalty
			notes = append(notes, fmt.Sprintf("%s changed manager — selection and shape unsettled", team.ShortName))
		}
	}

	return factor, notes
}

// confirmedStarter reports whether the player is on the manual exemption list.
func (e *Engine) confirmedStarter(el *fpl.Element) bool {
	e.confirmedOnce.Do(func() {
		ids := map[int]bool{}
		for _, name := range e.Role.ConfirmedStarters {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			var id int
			if _, err := fmt.Sscanf(name, "%d", &id); err == nil && id > 0 {
				ids[id] = true
				continue
			}
			for _, match := range e.Boot.FindPlayers(name) {
				ids[match.ID] = true
				break
			}
		}
		e.confirmedIDs = ids
	})
	return e.confirmedIDs[el.ID]
}
