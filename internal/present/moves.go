package present

import (
	"fmt"
	"io"
	"strings"

	"armband/internal/analysis"
)

// Moves renders a transfer plan: what goes out, what comes in, and the number
// that justified it.
//
// The layout puts OUT and IN on one line with an arrow between, because a
// transfer is a single decision about a pair and splitting it across two tables
// is how a reader loses which sale funded which buy. The per-move `Δ` column is
// deliberately absent for funding legs — `decide` reports a pair's gain once, on
// its lead move, and zeroes every funding leg, so printing a per-leg gain here
// would invent a slope that the diagnostic explicitly warns does not exist.
func Moves(w io.Writer, p analysis.Plan, t Theme) {
	if len(p.Moves) == 0 {
		fmt.Fprintf(w, "\n  %s\n", t.bold("No move this week."))
		fmt.Fprintf(w, "  %s\n\n",
			t.dim("Banking the transfer is a first-class outcome and usually the right one."))
		return
	}

	nameW := 0
	for _, m := range p.Moves {
		for _, n := range []string{m.Out.Name, m.In.Name} {
			if w := width(truncate(n, 22)); w > nameW {
				nameW = w
			}
		}
	}

	fmt.Fprintln(w)
	label := fmt.Sprintf("%d transfer", p.Transfers)
	if p.Transfers != 1 {
		label += "s"
	}
	fmt.Fprintf(w, "  %s\n", t.bold(strings.ToUpper(label)))
	fmt.Fprintln(w)

	for _, m := range p.Moves {
		out := t.red(pad(truncate(m.Out.Name, 22), nameW))
		in := t.green(pad(truncate(m.In.Name, 22), nameW))
		fmt.Fprintf(w, "  %s %s  %s  %s %s  %s\n",
			out,
			t.dim(fmt.Sprintf("%-4s £%.1fm", m.Out.Team, m.Out.Price)),
			t.dim("→"),
			in,
			t.dim(fmt.Sprintf("%-4s £%.1fm", m.In.Team, m.In.Price)),
			deltaCell(m, t),
		)
	}

	fmt.Fprintln(w)
	money := "level"
	switch {
	case p.Spend > 0:
		money = fmt.Sprintf("costs £%.1fm", float64(p.Spend)/10)
	case p.Spend < 0:
		money = fmt.Sprintf("frees £%.1fm", float64(-p.Spend)/10)
	}
	fmt.Fprintf(w, "  %s  %s pts/gw   %s\n",
		t.dim("gain "), t.bold(fmt.Sprintf("%+.2f", p.GainPerGW)), t.dim(money))

	// The robustness question, when the plan carries it. This is the "is there
	// anything left to learn before acting?" field, and it is the one number
	// here that can turn a good-looking move into a wait.
	if p.DependsOn.ID != 0 {
		fmt.Fprintf(w, "  %s  %s — without him the plan is worth %s pts/gw\n",
			t.dim("hinges"), t.bold(p.DependsOn.Name),
			fmt.Sprintf("%+.2f", p.GainIfOut))
	}
	fmt.Fprintln(w)
}

// deltaCell shows the per-move score change, and says nothing where the move is
// a funding leg carrying a reported gain of zero.
func deltaCell(m analysis.Swap, t Theme) string {
	d := m.In.Score - m.Out.Score
	s := fmt.Sprintf("%+.2f", d)
	if d >= 0 {
		return t.green(s)
	}
	return t.dim(s)
}
