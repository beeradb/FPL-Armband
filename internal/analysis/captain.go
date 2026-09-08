package analysis

// CaptainAndVice picks who wears the armband from a fielded eleven.
//
// Highest Score is captain; the distinct second-highest is vice. Ties keep the
// earlier slice element (strict greater-than), which is the walk HOLD already
// scores: a later equal score never steals the armband. An empty eleven, or one
// player, returns a zero vice rather than making the captain his own backup —
// FPL would otherwise forfeit the double when that one player blanks.
//
// Every surface that names a captain — Optimize, WeekViews, the transfer plan,
// squad rebuild, the replay — calls this. The Score vector is the caller's: the
// horizon average for construction and HOLD, this gameweek at horizon 1 for the
// live picker. Same rule, different question.
func CaptainAndVice(xi []PlayerMetrics) (captain, vice PlayerMetrics) {
	var capScore, viceScore float64
	for _, p := range xi {
		switch {
		case p.Score > capScore:
			viceScore, vice = capScore, captain
			capScore, captain = p.Score, p
		case p.Score > viceScore:
			viceScore, vice = p.Score, p
		}
	}
	return captain, vice
}
