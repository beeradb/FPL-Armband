package analysis

import "sort"

// templateCoreUnavailable is the same status set reachesExpectedMinutesCut
// uses to drop injured/suspended players from the *pool*. The core lock must
// not force those players into the fifteen — high ownership often persists
// through an absence, and a mid-season HOLD cell would otherwise buy a
// passenger.
func templateCoreUnavailable(status string) bool {
	switch status {
	case "injured", "suspended", "unavailable", "not in squad":
		return true
	}
	return false
}

// TemplateCoreIDs returns up to k element ids ordered by Ownership descending,
// subject to the 15-man position quotas and MaxPerClub so the result is a
// feasible LockIDs prefix. Ties break toward the lower element id — the same
// determinism convention as bestArmband.
//
// Ownership <= 0 is skipped. An all-zero ownership table must yield an empty
// core, not an arbitrary id order: that is the liveness/void trap the
// ownership-tiebreak sweep already paid for (PreSeasonWith unwired ⇒ 0% owned
// ⇒ byte-identical null).
//
// Unavailable players are skipped. Existing locks/excludes are the caller's
// job: Optimize passes them through selectTemplateCore so the merged LockIDs
// still respect squadQuota and MaxPerClub.
//
// This is a search-constraint helper, not a Score term. Ownership must not
// enter Score; this is not the closed separable-band ownership tiebreak
// (AGENTS.md: "Do not break ties on ownership inside the separable band").
func TemplateCoreIDs(players []PlayerMetrics, k int) []int {
	return selectTemplateCore(players, k, nil, nil, nil)
}

// selectTemplateCore is the one implementation. skip is already-locked or
// excluded ids. posCount and clubCount are quota already consumed by those
// locks; they are copied, not mutated.
func selectTemplateCore(players []PlayerMetrics, k int, skip map[int]bool, posCount, clubCount map[string]int) []int {
	if k <= 0 {
		return nil
	}
	pos := map[string]int{}
	club := map[string]int{}
	for p, n := range posCount {
		pos[p] = n
	}
	for c, n := range clubCount {
		club[c] = n
	}
	ranked := append([]PlayerMetrics(nil), players...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Ownership != ranked[j].Ownership {
			return ranked[i].Ownership > ranked[j].Ownership
		}
		return ranked[i].ID < ranked[j].ID
	})
	var out []int
	for _, p := range ranked {
		if p.Ownership <= 0 {
			break
		}
		if skip[p.ID] {
			continue
		}
		if templateCoreUnavailable(p.Status) {
			continue
		}
		if pos[p.Position] >= squadQuota[p.Position] {
			continue
		}
		if club[p.Team] >= MaxPerClub {
			continue
		}
		out = append(out, p.ID)
		pos[p.Position]++
		club[p.Team]++
		if len(out) >= k {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
