package present

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"armband/internal/analysis"
)

// cellWidth is one player's column on the pitch. Wide enough for "Guimarães"
// plus the armband marker, narrow enough that five defenders fit an 80-column
// terminal: 5 x 16 = 80 exactly, which is the constraint that set it.
const cellWidth = 16

// Squad renders a fifteen as a pitch: the eleven in formation rows, the bench in
// substitution order beneath it.
//
// The formation rows are the point. A squad is a shape before it is a list, and
// a flat list of eleven names hides the thing a manager actually checks — that
// there are three at the back and the money is where it should be.
func Squad(w io.Writer, sq analysis.Squad, t Theme, title string) {
	rows := formationRows(sq.StartingXI)

	// Pitch width is set by the widest row, so a 5-at-the-back squad and a
	// 3-at-the-back squad both centre on the same axis.
	widest := 0
	for _, r := range rows {
		if n := len(r); n > widest {
			widest = n
		}
	}
	pitch := widest * cellWidth
	if pitch < 60 {
		pitch = 60
	}

	fmt.Fprintln(w)
	header(w, t, title, sq.Formation, pitch)

	for _, r := range rows {
		line1, line2, line3 := "", "", ""
		for _, p := range r {
			n, s, d := playerCell(p, sq, t)
			line1 += centre(n, cellWidth)
			line2 += centre(s, cellWidth)
			line3 += centre(d, cellWidth)
		}
		indent := strings.Repeat(" ", (pitch-len(r)*cellWidth)/2)
		fmt.Fprintln(w, indent+strings.TrimRight(line1, " "))
		fmt.Fprintln(w, indent+strings.TrimRight(line2, " "))
		fmt.Fprintln(w, indent+strings.TrimRight(line3, " "))
		fmt.Fprintln(w)
	}

	rule(w, t, pitch)
	fmt.Fprintf(w, "%s\n", t.dim(centre("B E N C H", pitch)))
	fmt.Fprintln(w)

	// Bench order is substitution order and is load-bearing: the first
	// outfielder is the one who actually comes on, and the derived slot weights
	// price him several times what the third is worth. Numbering says so.
	for i, p := range sq.Bench {
		marker := t.dim(fmt.Sprintf("%d.", i+1))
		if p.Position == "GKP" {
			marker = t.dim("GK")
		}
		fmt.Fprintf(w, "  %s %s  %s  %s  %s\n",
			marker,
			t.posColour(p.Position, pad(truncate(p.Name, 20), 20)),
			t.dim(pad(p.Team, 4)),
			padLeft(fmt.Sprintf("£%.1fm", p.Price), 7),
			t.dim(fmt.Sprintf("%.2f pts/gw", p.Score)),
		)
	}

	fmt.Fprintln(w)
	rule(w, t, pitch)
	footer(w, sq, t)
}

func header(w io.Writer, t Theme, title, formation string, pitch int) {
	if title == "" {
		title = "Squad"
	}
	label := fmt.Sprintf("%s  ·  %s", title, formation)
	fmt.Fprintf(w, "%s\n", t.bold(centre(label, pitch)))
	rule(w, t, pitch)
	fmt.Fprintln(w)
}

func rule(w io.Writer, t Theme, n int) {
	fmt.Fprintf(w, "%s\n", t.dim(strings.Repeat("─", n)))
}

func footer(w io.Writer, sq analysis.Squad, t Theme) {
	fmt.Fprintf(w, "  %s  %s        %s  %s\n",
		t.dim("captain"), t.bold(sq.Captain.Name),
		t.dim("vice"), sq.ViceCaptain.Name)
	fmt.Fprintf(w, "  %s  £%.1fm spent, £%.1fm left\n",
		t.dim("money  "), sq.TotalCost, sq.Remaining)
	// Both totals, because they answer different questions and the difference
	// between them IS the armband. XIScore is the plain eleven; ExpectedPoints
	// counts the captain twice, which is what the game pays.
	fmt.Fprintf(w, "  %s  %s pts/gw from the eleven, %s with the captain doubled\n",
		t.dim("points "),
		t.bold(fmt.Sprintf("%.1f", sq.XIScore)),
		t.bold(fmt.Sprintf("%.1f", sq.ExpectedPoints)))
}

// playerCell is the three lines one player occupies: name with any armband,
// club and price, and the score.
func playerCell(p analysis.PlayerMetrics, sq analysis.Squad, t Theme) (string, string, string) {
	name := truncate(p.Name, cellWidth-3)
	switch p.ID {
	case sq.Captain.ID:
		name = t.bold(name) + t.yellow(" (C)")
	case sq.ViceCaptain.ID:
		name = name + t.dim(" (V)")
	}
	name = t.posColour(p.Position, name)

	money := fmt.Sprintf("%s £%.1fm", p.Team, p.Price)
	score := fmt.Sprintf("%.2f", p.Score)

	// A rotation risk is the one thing worth interrupting the layout for: it is
	// the difference between a squad that scores what it says and one that does
	// not, and it is invisible in a price-and-score row.
	if isRisky(p.RotationRisk) {
		score += t.red(" !")
	}
	return name, t.dim(money), t.dim(score)
}

// isRisky reports whether a rotation band is the bad kind.
//
// `RotationRisk` is a BAND LABEL and is never empty — rotationLabel returns one
// of nailed / likely starter / rotation risk / squad player / fringe for every
// player alive. Testing it with `!= ""` therefore flags the entire squad, which
// is what the first version of this file did: eleven red marks, no information,
// and it looked deliberate. Name the bad bands instead.
func isRisky(band string) bool {
	switch band {
	case "rotation risk", "squad player", "fringe":
		return true
	}
	return false
}

// formationRows groups the eleven into GK / DEF / MID / FWD, each ordered by
// score so the strongest player in a line sits centre-left rather than wherever
// the optimiser happened to append him.
func formationRows(xi []analysis.PlayerMetrics) [][]analysis.PlayerMetrics {
	byPos := map[string][]analysis.PlayerMetrics{}
	for _, p := range xi {
		byPos[p.Position] = append(byPos[p.Position], p)
	}
	var rows [][]analysis.PlayerMetrics
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		r := byPos[pos]
		if len(r) == 0 {
			continue
		}
		sort.SliceStable(r, func(i, j int) bool { return r[i].Score > r[j].Score })
		rows = append(rows, r)
	}
	return rows
}
