package analysis

import (
	"sort"

	"armband/internal/stats"
)

// Research targeting: which players the model is most likely to be wrong about,
// computed deterministically so the agent knows where to spend its searches.
//
// The model cannot see roles. It infers minutes from last season's totals, which
// say nothing about a manager's plan for this one. Every miss this system has
// made has been of that shape:
//
//   - a promoted club's nailed starter scored 0.00, because he has no Premier
//     League minutes and the model has no other way to rate him;
//   - a defender's absence silently invalidated his club's expected goals
//     conceded, and with it every clean sheet behind him;
//   - a forward moved from the wing to centre-forward, which changes both his
//     minutes and his shot volume, and is visible only in press conferences.
//
// The existing team-news step is reactive: it verifies players already under
// consideration. That cannot catch a player the model scored so low he was never
// considered. This produces the list the other way round — start from where the
// model is structurally blind, then go and look.
//
// Deterministic and free, per the rule that analysis never calls the LLM. It
// decides *who* to research; the agent does the researching.
type ResearchCategory struct {
	// Name is a short label for the blind spot.
	Name string `json:"name"`
	// Why states what the model cannot see, once for the whole group rather
	// than repeated on every row.
	Why string `json:"why"`
	// Ask is the question to answer for each player.
	Ask     string          `json:"ask"`
	Targets []PlayerMetrics `json:"targets"`
}

// Research thresholds. Deliberately loose: a target that turns out to be a
// non-story costs one search, while a miss costs a season.
const (
	// researchOwnershipFloor is the ownership at which the crowd's opinion is
	// worth checking against the model's.
	researchOwnershipFloor = 3.0
	// researchDisagreementOwnership is where a high-owned, low-scored player
	// suggests the market knows about a role the model cannot see.
	researchDisagreementOwnership = 10.0
	// researchPerCategory caps each group, so one broad category cannot crowd
	// out the others. Roughly one web search per target.
	researchPerCategory = 5
	// researchKeyDefenderMinutes is the minutes that make an absent defender's
	// club worth re-checking: enough that the club's expected goals conceded
	// was largely earned with him in the side.
	researchKeyDefenderMinutes = 2000
)

// ResearchTargets groups the players worth checking before a deadline. squad may
// be nil; when given, its members are always checked, because the
// recommendation depends on them.
func (e *Engine) ResearchTargets(squad []PlayerMetrics) []ResearchCategory {
	all := e.AllMetrics()

	byPos := map[string][]float64{}
	for _, m := range all {
		if m.Minutes > 0 {
			byPos[m.Position] = append(byPos[m.Position], m.Score)
		}
	}
	median := map[string]float64{}
	for pos, xs := range byPos {
		median[pos] = stats.Median(xs)
	}

	claimed := map[int]bool{}
	byOwnership := func(ps []PlayerMetrics) []PlayerMetrics {
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Ownership > ps[j].Ownership })
		var out []PlayerMetrics
		for _, p := range ps {
			if claimed[p.ID] || len(out) >= researchPerCategory {
				continue
			}
			claimed[p.ID] = true
			out = append(out, p)
		}
		return out
	}

	var cats []ResearchCategory
	push := func(name, why, ask string, ps []PlayerMetrics) {
		if t := byOwnership(ps); len(t) > 0 {
			cats = append(cats, ResearchCategory{Name: name, Why: why, Ask: ask, Targets: t})
		}
	}

	// A club's expected goals conceded is stale the moment a defender who played
	// most of it stops being available. Every clean sheet behind him is affected,
	// and nothing in the model can express it.
	var keyOut []PlayerMetrics
	for _, m := range all {
		if (m.Position == "DEF" || m.Position == "GKP") && m.Minutes >= researchKeyDefenderMinutes &&
			(m.Status == "injured" || m.Status == "unavailable" || m.Status == "suspended") {
			keyOut = append(keyOut, m)
		}
	}
	push("Defences whose numbers no longer describe the side",
		"these players are unavailable but played most of last season, so their club's expected "+
			"goals conceded — and every clean-sheet and goals-conceded term behind it — was earned "+
			"with them in the team. The model has no way to adjust for that.",
		"how long is he out, who replaces him, and should every defensive asset at that club be "+
			"marked down?", keyOut)

	// The model cannot rate a player with no Premier League minutes at all. A
	// promoted club's first-choice defender scores 0.00 exactly like their
	// fourth-choice keeper. Ownership is the only signal that separates them.
	var blind []PlayerMetrics
	for _, m := range all {
		if m.Minutes == 0 && m.Ownership >= researchOwnershipFloor {
			blind = append(blind, m)
		}
	}
	push("No Premier League data — scored 0.00 regardless of role",
		"promoted clubs and overseas arrivals have no minutes for the model to read, so a nailed "+
			"starter and a fourth-choice keeper score identically. A cheap nailed starter here is "+
			"the classic enabler, and the model will never find one.",
		"is he in the predicted XI? If so he is badly underrated and worth a squad slot.", blind)

	// Heavy ownership on a player the model rates below his position's median
	// usually means a role change, a promotion, or a return from injury.
	var disagree []PlayerMetrics
	for _, m := range all {
		if m.Ownership >= researchDisagreementOwnership && m.Minutes > 0 && m.Score < median[m.Position] {
			disagree = append(disagree, m)
		}
	}
	push("The market disagrees with the model",
		"heavily owned but scored below the median for the position. The crowd is usually pricing "+
			"something the raw minutes cannot show.",
		"what does the market know — a new role, a return from injury, a set-piece promotion, a "+
			"first-choice place won in pre-season?", disagree)

	// The recommendation rests on these, so verify before relying.
	var shaky []PlayerMetrics
	for _, m := range squad {
		if m.RotationRisk != "nailed" {
			shaky = append(shaky, m)
		}
	}
	push("In the squad but not nailed",
		"the recommendation depends on these players starting, and last season's minutes do not "+
			"establish that they will.",
		"does he start? If not, the squad needs changing before the deadline.", shaky)

	// Set-piece duty is reported but no longer scored, so a change of taker is
	// completely invisible to the model.
	var setPiece []PlayerMetrics
	for _, m := range all {
		if m.SetPieceNote != "" && m.Ownership >= researchDisagreementOwnership {
			setPiece = append(setPiece, m)
		}
	}
	push("Set-piece duty, reported but not scored",
		"the model reports penalty and set-piece order but no longer prices it, because FPL's "+
			"expected goals already contain those returns. A player who has just *taken over* the "+
			"duty is therefore underrated, and a player who has lost it is overrated.",
		"does he still have the duty, and has anyone taken it from him?", setPiece)

	// New signings' numbers were earned somewhere else.
	var signings []PlayerMetrics
	for _, m := range all {
		if m.NewSigning && m.Ownership >= researchDisagreementOwnership {
			signings = append(signings, m)
		}
	}
	push("New signings with unproven roles",
		"last season's numbers came from a different club, so both the minutes and the underlying "+
			"rates describe a team he no longer plays for.",
		"where does he fit in the new side, and is he starting in pre-season?", signings)

	return cats
}
