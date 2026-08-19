package present

import (
	"fmt"
	"html/template"
	"io"
	"strings"

	"armband/internal/analysis"
)

// Page is everything a squad page renders. One struct and one entry point.
//
// It replaced a ladder of four functions — HTML, HTMLWeeks, HTMLFull, HTMLBriefed —
// each of which existed only to add one more parameter and delegate to the next. That
// shape puts the caller in a positional argument list ten long, where transposing two
// strings compiles and renders a page with the subtitle in the title; and it means
// four doc comments describing one renderer.
//
// Zero values are meaningful throughout: no Weeks means no week strip, a nil Reasoning
// means no "why" view and no tab for it, a nil Watch the same. A page with neither is
// exactly the old single-view page, which is what the replay renders.
type Page struct {
	Title, Subtitle string
	Squad           analysis.Squad
	Plan            *analysis.Plan
	// NoPlan explains an ABSENT transfer plan. An empty Transfers section cannot
	// distinguish "you have no squad yet" from "the model wants no move this week",
	// and only the second is a recommendation — doing nothing is a first-class
	// outcome and usually the right one, which is worth stating rather than leaving
	// as a gap.
	NoPlan string
	Weeks  []analysis.WeekView
	Brief  *Brief
	// Analysis is the reasoning layer's verdict, in Markdown, and it is PASSED IN
	// rather than generated. This package renders; it does not decide. A page that
	// composed its own verdict would be a second opinion competing with the one in
	// the report, and the whole point of the split is that the deterministic half
	// never pretends to judgement.
	Analysis string
	// Horizon is the gameweek span the scores are computed over. It labels the
	// fixture column, which must not claim five when the config says three.
	Horizon int
	// Overrides binds a standing correction to the element it acts on. Built by the
	// caller, which is the only layer that can see both config and the squad.
	Overrides map[int]Override
	// FixtureLoadInScore is analysis.Engine.FixtureLoadInScore(), passed in because
	// only the engine knows it and this package may not ask. It decides whether the
	// why-card presents a double or a blank as already inside the score or as a
	// separate fact about the weeks ahead — the two read identically on the page and
	// mean opposite things.
	FixtureLoadInScore bool
	Reasoning          *Reasoning
	Watch              *Watchlist
}

// HTML writes a squad and its transfers as a self-contained page: no external CSS, no
// fonts, no scripts — it opens from a file:// URL with nothing fetched, which is the
// point of writing it to disk rather than hosting it.
//
// html/template, not text/template, and not fmt.Fprintf. Player names and club names
// come from FPL's API and are attacker-adjacent in the weak sense that nobody here
// controls them; a name containing a quote or an angle bracket would otherwise break
// the document or worse. This project already treats FPL-supplied strings as untrusted
// where they reach the agent, and a file the user opens in a browser deserves the same
// care.
func HTML(w io.Writer, sq analysis.Squad, plan *analysis.Plan, title, subtitle string) error {
	return Render(w, Page{Title: title, Subtitle: subtitle, Squad: sq, Plan: plan})
}

// Render writes the page.
func Render(w io.Writer, p Page) error {
	var an strings.Builder
	if p.Analysis != "" {
		renderMarkdown(&an, p.Analysis)
	}
	data := pageData{
		Title:     p.Title,
		Subtitle:  p.Subtitle,
		Squad:     p.Squad,
		NoPlan:    p.NoPlan,
		Brief:     p.Brief,
		Analysis:  template.HTML(an.String()),
		Horizon:   p.Horizon,
		Reasoning: p.Reasoning,
		Watch:     p.Watch,
		HasWhy:    p.Reasoning.Any(),
		HasWatch:  p.Watch.Any(),
	}
	data.Tabs = data.HasWhy || data.HasWatch

	// The bar behind each score is scaled against the best score in the FIFTEEN, not
	// against the league. The question the column answers is "which of my own players
	// carry this eleven", and scaling to a league maximum would flatten every bar on a
	// good squad into the same near-full block.
	best := 0.0
	for _, pl := range p.Squad.Players {
		if pl.Score > best {
			best = pl.Score
		}
	}

	card := func(m analysis.PlayerMetrics) htmlCard {
		c := newCard(m, p.Squad.Captain.ID, p.Squad.ViceCaptain.ID, p.Overrides,
			p.FixtureLoadInScore)
		c.ScorePct = pct(m.Score, best)
		return c
	}

	for _, r := range formationRows(p.Squad.StartingXI) {
		line := htmlRow{Position: r[0].Position}
		for _, m := range r {
			line.Players = append(line.Players, card(m))
		}
		data.Rows = append(data.Rows, line)
	}
	for i, m := range p.Squad.Bench {
		b := card(m)
		b.Slot = fmt.Sprintf("%d", i+1)
		if m.Position == "GKP" {
			b.Slot = "GK"
		}
		data.Bench = append(data.Bench, b)
	}

	for i, wv := range p.Weeks {
		tab := htmlWeek{
			Event:     wv.Event,
			Formation: wv.Formation,
			First:     i == 0,
			XIScore:   wv.XIScore,
			Expected:  wv.Expected,
			Captain:   wv.Captain.Name,
			Chip:      wv.Chip,
			Rebuilt:   wv.Rebuilt,
			Caveat:    wv.Caveat,
		}
		for _, r := range formationRows(wv.XI) {
			line := htmlRow{Position: r[0].Position}
			for _, m := range r {
				c := newCard(m, wv.Captain.ID, wv.ViceCaptain.ID, p.Overrides,
					p.FixtureLoadInScore)
				c.Opponent = opponentLabel(wv.Opponents[m.ID])
				line.Players = append(line.Players, c)
			}
			tab.Rows = append(tab.Rows, line)
		}
		// Blanks are read against THIS week's fifteen, not the one you own — a
		// wildcard week is scored on a rebuilt squad, so asking the old squad who
		// blanks would name players who are not in it.
		for _, m := range wv.Blanks(wv.Squad) {
			tab.Blanks = append(tab.Blanks, m.Name+" ("+m.Team+")")
		}
		data.Weeks = append(data.Weeks, tab)
	}

	if p.Plan != nil && len(p.Plan.Moves) > 0 {
		data.Plan = p.Plan
		for _, m := range p.Plan.Moves {
			data.Moves = append(data.Moves, htmlMove{
				Out: card(m.Out), In: card(m.In), Delta: m.In.Score - m.Out.Score,
			})
		}
	}

	if p.Reasoning != nil {
		for _, g := range p.Reasoning.Research {
			gv := researchGroupView{Name: g.Name, Why: g.Why, Ask: g.Ask}
			for _, m := range g.Targets {
				gv.Targets = append(gv.Targets, card(m))
			}
			data.Research = append(data.Research, gv)
		}
	}

	// Watchlist candidates go through newCard too, so a badge added once shows up in
	// every view. They carry no score bar — the bar answers "which of MY players
	// carry this eleven", which is a question about the fifteen and not about the
	// pool — so ScorePct is left at zero here rather than computed and unused.
	if p.Watch != nil {
		for _, g := range p.Watch.Groups {
			gv := watchGroupView{
				Position:       g.Position,
				BenchmarkName:  g.BenchmarkName,
				BenchmarkScore: g.BenchmarkScore,
				BenchmarkPrice: g.BenchmarkPrice,
			}
			for _, r := range g.Rows {
				gv.Rows = append(gv.Rows, watchRowView{
					Card: card(r.Player), Delta: r.Delta, DeltaClass: r.DeltaClass(),
				})
			}
			data.WatchGroups = append(data.WatchGroups, gv)
		}
	}
	return pageTmpl.Execute(w, withPalette(data))
}

// pct is the bar width, in whole percent, clamped. Presentation arithmetic — the same
// thing summaryBar.Height does for the replay chart — and deliberately the only
// division in this file.
func pct(v, max float64) int {
	if max <= 0 {
		return 0
	}
	n := int(v / max * 100)
	switch {
	case n < 0:
		return 0
	case n > 100:
		return 100
	}
	return n
}

// newCard is the one place a PlayerMetrics becomes a row. Every view uses it, so a
// badge added here appears in all of them and cannot be added to one and forgotten in
// another.
func newCard(m analysis.PlayerMetrics, capID, viceID int, ovr map[int]Override,
	loadInScore bool) htmlCard {
	c := htmlCard{
		ID: m.ID, Name: m.Name, Team: m.Team, Position: m.Position,
		Price: m.Price, Score: m.Score, ExpMins: m.ExpectedMinutes,
		Own: m.Ownership, Mins: m.Minutes,
		IsCaptain: m.ID == capID, IsVice: m.ID == viceID,
	}
	if cls, show := bandClass(m.RotationRisk); show {
		c.Band, c.BandClass = m.RotationRisk, cls
	}
	if m.Status != "" && m.Status != "available" {
		c.Status = strings.ToUpper(m.Status)
		if m.ChancePlay != nil {
			c.Chance = fmt.Sprintf("%d%%", *m.ChancePlay)
		}
	}
	// Penalty order only, on the row. The full set-piece note stays in the why-card.
	//
	// The note was a badge and it was wrong three ways: it truncated mid-token, so
	// "penalties #1, corners/…" told a reader nothing; it cost ~140px of the player
	// column on the most important table; and it gave set-piece duty more prominence
	// than the model gives it weight — B.Fernandes carried it while his own card reads
	// base 6.00, after fixtures 6.06, with no set-piece term at all. A badge louder
	// than the term it names is the page arguing with itself.
	//
	// Penalty order survives because it is the one set-piece fact a human picks a
	// player for, and it is five characters that never truncate.
	if m.PenaltyOrder != nil && *m.PenaltyOrder > 0 {
		c.Pen = fmt.Sprintf("PEN %d", *m.PenaltyOrder)
	}
	c.Fixtures, c.MoreFix = fdrCells(m.Fixtures)

	w := &whyCard{
		Score: m.Score, Value: m.ValueScore,
		Base: m.BaseXP90, SetPiece: m.SetPieceXP90, FixtureAdj: m.FixtureAdjXP90,
		ExpMins: m.ExpectedMinutes, Reliability: m.MinutesRating,
		Band: m.RotationRisk, Matches: m.MatchesAvailable, Absence: m.TournamentAbsence,
		Corrections: corrections(m, loadInScore), Status: c.Status, Chance: c.Chance,
		News:   m.News,
		AvgFDR: m.AvgDifficulty, SetPieceNote: m.SetPieceNote,
	}
	w.Fixtures, _ = fdrCells(m.Fixtures)
	if m.PriorWeight > 0 && m.PriorWeight < 1 {
		w.PriorPct = int(m.PriorWeight*100 + 0.5)
	}
	if o, ok := ovr[m.ID]; ok {
		c.Override = &o
		w.Override = &o
	}
	c.Why = w
	return c
}

// withPalette fills the shared token block. Every Execute goes through it, so a new
// entry point cannot ship a page with no colours.
func withPalette(d pageData) pageData {
	d.Palette = template.CSS(paletteCSS)
	return d
}

type pageData struct {
	Title, Subtitle string
	Squad           analysis.Squad
	Rows            []htmlRow
	Bench           []htmlCard
	Plan            *analysis.Plan
	Moves           []htmlMove
	Weeks           []htmlWeek
	NoPlan          string
	Horizon         int
	// Replay switches the page from a forecast to a finished season: the score
	// column becomes points already scored, and the transfer strip reports what
	// each side of a move actually returned rather than what was predicted.
	Replay    bool
	Palette   template.CSS
	Brief     *Brief
	Analysis  template.HTML
	Summary   *replaySummary
	Reasoning *Reasoning
	Watch     *Watchlist
	// WatchGroups is Watch.Groups with every player turned into a card, so the
	// watchlist and the roster render a name the same way.
	WatchGroups []watchGroupView
	Research    []researchGroupView
	// Tabs is false when there is only one view — the replay, and any squad page
	// built without the reasoning and watchlist data. A tab bar with one tab is
	// furniture pretending to be a control.
	Tabs, HasWhy, HasWatch bool
}

// replaySummary is the shape of the season, read before any single week.
//
// A reader opening a replay wants three things in this order: how did it go, what
// did it cost, and then week by week what happened. Answering the third first is
// what makes a page like this feel like a log rather than a report.
type replaySummary struct {
	Points, Transfers, Hits int
	// HitMoves is how many of the transfers cost a hit. The points figure alone
	// under-describes the policy: 8 points could be two hits or one double move.
	HitMoves          int
	Best, Worst       int // gameweek numbers
	BestPts, WorstPts int
	Bars              []summaryBar
	// Moves is every transfer of the season in gameweek order, which is the one
	// view the per-week tabs cannot give: a reader asking "what did this policy
	// actually do" would otherwise have to open all 38.
	Moves []htmlMove
	// Unlisted is how many of Transfers have no row in Moves — a wildcard's
	// swaps, which the replay counts but does not record individually.
	Unlisted int
	// Chips is what was played, e.g. "wildcard GW6". Empty means none WERE
	// played, which is a fact about the run and not a rendering gap — so the
	// template says so rather than showing nothing. A silent absence here reads
	// as "chips happened and went unmarked".
	Chips []string
}

type summaryBar struct {
	GW, Points int
	// Height is a percentage, so the template does no arithmetic.
	Height int
	Class  string
}

// Brief is the standing context a squad page is read against.
//
// A squad with no deadline beside it is a squad you cannot act on, and one with no
// alerts beside it looks safe whether or not it is. The full briefing lives in its own
// document and this is deliberately not that: it is the half-dozen facts that change
// what the squad below MEANS, so the page stops being a list and becomes something
// you can decide from.
//
// Everything here is passed in rather than derived. This package renders; it does not
// get to compute a deadline or judge a player, because a second implementation of
// either is how the page starts disagreeing with the brief it is summarising.
type Brief struct {
	Gameweek string
	Deadline string
	// Countdown is human, e.g. "7 days" — empty once the deadline has passed.
	Countdown string
	Played    int
	Horizon   int
	Transfers string
	// Chips is the plan, already formatted, e.g. "wildcard GW6".
	Chips []string
	// Alerts are the things that should change a decision: a flagged player in the
	// squad, an unverified budget, a stale competition check. Rendered in the
	// warning colour because an alert that looks like body text is not an alert.
	Alerts []string
	// Overrides is how many hand-set corrections bind this squad. Stated as a count
	// because a reader who sees one should go and read them, and stated at all
	// because a squad built under six overrides is not the model's unaided answer.
	Overrides int
}

type htmlWeek struct {
	Event     int
	Formation string
	Rows      []htmlRow
	Blanks    []string
	First     bool
	XIScore   float64
	Expected  float64
	Captain   string
	Chip      string
	Rebuilt   bool
	Caveat    string
	// Actual marks a week whose numbers are results rather than predictions.
	Actual bool
	// Moves are the transfers made going into this gameweek, and Running is the
	// points total to the end of it. Both are replay-only.
	Moves   []htmlMove
	Running int
	// Hit is what the week's transfers cost, as a positive number of points.
	//
	// It is carried separately from Expected rather than folded into it because
	// the two answer different questions: Expected is what the manager banked,
	// Hit is what the decision charged to bank it. A week that scored 60 after a
	// -8 is a different week from one that scored 60 for free, and a page that
	// shows only the 60 cannot tell them apart.
	Hit int
}

// opponentLabel renders a gameweek's fixtures for one club: "BOU (H)", both legs of
// a double, or an empty string for a blank. Empty is meaningful — it is what a
// player with no match looks like, and the template shows it as such rather than
// leaving the card looking ordinary.
func opponentLabel(fs []analysis.FixtureBrief) string {
	if len(fs) == 0 {
		return ""
	}
	s := ""
	for i, f := range fs {
		if i > 0 {
			s += " + "
		}
		where := "A"
		if f.Home {
			where = "H"
		}
		s += fmt.Sprintf("%s (%s)", f.Opponent, where)
	}
	return s
}

type htmlRow struct {
	Position string
	Players  []htmlCard
}

type htmlMove struct {
	Out, In htmlCard
	Delta   float64
	// GW is the gameweek the move was made into. Zero on a forecast, where the
	// suggestion is for the week the page is about and repeating it is noise.
	GW int
	// Hit marks a move that was taken above the free allowance and cost 4 points.
	// The season figure alone cannot say which moves paid it, and "36 transfers,
	// 8 points of hits" leaves a reader unable to find the two that charged it.
	Hit bool
}

type htmlCard struct {
	ID                         int
	Name, Team, Position, Slot string
	// Risk is free-text carried by the replay only, where it marks a player who has
	// just arrived. It is NOT the rotation band — see Band.
	Risk string
	// Band is the rotation label, and it is EMPTY for a nailed starter.
	//
	// The distinction is load-bearing and this package has now made the same mistake
	// twice in two renderers. analysis.rotationLabel returns a band for every player
	// alive, so "nailed" is a value and not an absence; a template that prints the
	// field whenever it is non-empty marks the entire squad. The terminal pitch did
	// it in red and it looked deliberate. This page then did it again — `.risk` in
	// var(--bad) on eleven of fifteen rows, all of them saying "nailed", which is
	// good news printed as an injury.
	//
	// So the filtering happens in bandClass, once, in Go, where a test can reach it.
	Band, BandClass string
	// Status is FPL's availability flag, uppercased, and empty when the player is
	// available. Chance is his percentage when FPL gives one.
	Status, Chance string
	// Pen is "PEN 1"/"PEN 2" and empty for everyone else. It replaced a badge that
	// carried the whole SetPieceNote and truncated it — see newCard.
	Pen          string
	Opponent     string
	Price, Score float64
	ExpMins, Own float64
	Fixtures     []fdrCell
	// MoreFix is how many fixtures did not fit the strip. Reported rather than
	// dropped: a player with eight matches in the window is the interesting case.
	MoreFix int
	// Mins is raw season league minutes, as distinct from ExpMins per gameweek. The
	// research tables are read on it — the "no Premier League data" category is
	// defined by that column reading 0 — so it is carried on the card rather than
	// re-read from PlayerMetrics at the one table that wanted it.
	Mins              int
	ScorePct          int
	IsCaptain, IsVice bool
	Override          *Override
	Why               *whyCard
}

// The page is light-first on cool paper, and the ordering is a table rather than a
// formation. The pitch layout it replaced showed a shape well and did everything
// else this page is read for badly: comparing a player with his own backup, reading
// a column of scores, and scanning a season for the week something changed. All
// three theme states are defined so it holds however the viewer has set theirs.
var pageTmpl = template.Must(template.New("page").Funcs(template.FuncMap{
	"money": func(f float64) string { return fmt.Sprintf("£%.1fm", f) },
	// gross undoes a week's hit so the page can show the arithmetic a hit week
	// actually scored: "57 − 4 hit = 53". Expected is already net; this exists
	// because a page that shows only the net lets a −8 week read as a quiet one.
	"gross": func(net float64, hit int) float64 { return net + float64(hit) },
	"pts":   func(f float64) string { return fmt.Sprintf("%.2f", f) },
	"pts1":  func(f float64) string { return fmt.Sprintf("%.1f", f) },
	"delta": func(f float64) string { return fmt.Sprintf("%+.2f", f) },
	"mins":  func(f float64) string { return fmt.Sprintf("%.0f", f) },
	"mult":  func(f float64) string { return fmt.Sprintf("×%.2f", f) },
	"pc":    func(f float64) string { return fmt.Sprintf("%.1f%%", f) },
	// chipShort abbreviates a chip for the gameweek tabs, where the full name
	// would push the strip several rows deep on a 38-week season. Anything it
	// does not recognise falls through to its own first two letters rather than
	// to an empty string, so a chip added later is still marked rather than
	// silently unlabelled.
	"chipShort": func(s string) string {
		switch s {
		case "wildcard":
			return "WC"
		case "free hit":
			return "FH"
		case "bench boost":
			return "BB"
		case "triple captain":
			return "TC"
		}
		if len(s) >= 2 {
			return strings.ToUpper(s[:2])
		}
		return strings.ToUpper(s)
	},
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
{{.Palette}}
  * { box-sizing:border-box; }
  body { margin:0; padding:2.2rem 1.25rem 5rem; background:var(--bg); color:var(--ink);
         font-family:var(--sans); line-height:1.55; -webkit-font-smoothing:antialiased; }
  .wrap { max-width:64rem; margin:0 auto; display:flex; flex-direction:column; gap:1.9rem; }

  .k { font-family:var(--mono); font-size:.63rem; letter-spacing:.14em;
       text-transform:uppercase; color:var(--ink3); font-weight:600; }
  .num, td.n, .v { font-variant-numeric:tabular-nums; }

  header { border-bottom:2px solid var(--ink); padding-bottom:1.1rem; }
  header h1 { font-size:clamp(1.6rem,3.6vw,2.4rem); line-height:1.06; margin:.35rem 0 .4rem;
              letter-spacing:-.03em; font-weight:800; text-wrap:balance; }
  header p { margin:0; color:var(--ink2); max-width:62ch; font-size:.95rem; }

  .facts { display:flex; flex-wrap:wrap; gap:1.8rem; }
  .facts .v { font-size:1.6rem; font-weight:800; letter-spacing:-.02em; line-height:1.1; }
  .facts .v small { font-size:.8rem; font-weight:400; color:var(--ink3); margin-left:.25rem; }
  .facts .v.acc { color:var(--accent); }
  .facts .v.bad { color:var(--bad); }

  .chips { display:flex; align-items:center; gap:.4rem; flex-wrap:wrap; }
  .chips .none { color:var(--ink3); font-style:italic; font-size:.85rem; }
  .chip { font-family:var(--mono); font-size:.68rem; font-weight:700; padding:.14rem .5rem;
          border-radius:3px; background:var(--chipbg); color:var(--chipink); }
  .alerts { margin:0; padding:.7rem 1rem .7rem 2.1rem; background:var(--warnbg);
            border-left:3px solid var(--warn); color:var(--warnink);
            border-radius:0 4px 4px 0; font-size:.86rem; }
  .alerts li { margin:.18rem 0; }

  .shape { display:flex; align-items:flex-end; gap:3px; height:46px; }
  .shape i { flex:1; background:var(--accent); border-radius:2px 2px 0 0; min-height:2px;
             display:block; opacity:.42; }
  .shape i.best { opacity:1; }
  .shape i.worst { background:var(--bad); opacity:1; }

  section > h2 { font-family:var(--mono); font-size:.7rem; text-transform:uppercase;
                 letter-spacing:.14em; color:var(--ink3); margin:0 0 .7rem; font-weight:600; }

  .panel { background:var(--panel); border:1px solid var(--line); border-radius:5px; }
  .tscroll { overflow-x:auto; }
  table { width:100%; border-collapse:collapse; font-size:.88rem; }
  th { text-align:left; padding:.55rem .85rem; border-bottom:1px solid var(--line);
       font-family:var(--mono); font-size:.63rem; letter-spacing:.13em;
       text-transform:uppercase; color:var(--ink3); font-weight:600; white-space:nowrap; }
  td { padding:.5rem .85rem; border-bottom:1px solid var(--line); vertical-align:middle; }
  tr:last-child td { border-bottom:0; }
  td.n { text-align:right; white-space:nowrap; font-family:var(--mono); }
  .sub td { background:var(--panel2); border-bottom:1px solid var(--line2); padding:.35rem .85rem; }
  tr.benchrow td { background:var(--panel2); color:var(--ink2); }
  .pos { font-family:var(--mono); font-size:.63rem; font-weight:700; letter-spacing:.07em;
         color:var(--ink3); }
  .who { font-weight:600; }
  .club { color:var(--ink3); font-size:.78rem; margin-left:.4rem; }
  .tag { font-family:var(--mono); font-size:.6rem; font-weight:700; padding:.1rem .32rem;
         border-radius:2px; margin-left:.4rem; }
  .tag.c { background:var(--accent); color:var(--onacc); }
  .tag.v { background:var(--line2); color:var(--ink2); }
  .tag.in { background:var(--goodbg); color:var(--good); }
  /* A hit is a cost, so it takes the semantic bad colour rather than the accent.
     The accent already means "the number to read first" on this page. */
  .tag.hit { background:var(--badbg); color:var(--bad); }
  /* "free" is stated rather than left blank so the absence of a hit badge is
     never ambiguous between "no hit" and "not recorded". */
  .tag.free { background:var(--panel2); color:var(--ink3); }
  .hitmark { font-family:var(--mono); font-size:.68rem; font-weight:700;
             padding:.1rem .4rem; border-radius:2px;
             background:var(--badbg); color:var(--bad); }
  .opp { font-family:var(--mono); font-size:.72rem; color:var(--ink3); }
  .opp.blank { color:var(--bad); font-weight:700; }

  /* ---- top-level views. Same radio-and-sibling mechanism as the week strip,
         one level up, so the page has one way of doing this rather than two. ---- */
  .views > input { position:absolute; opacity:0; pointer-events:none; }
  .view { display:none; }
  .viewtabs { position:sticky; top:0; z-index:20; display:flex; gap:.15rem; flex-wrap:wrap;
              background:var(--bg); border-bottom:1px solid var(--line);
              margin:0 0 1.6rem; padding:.15rem 0 0; }
  .viewtabs label { font-family:var(--mono); font-size:.72rem; letter-spacing:.11em;
                    text-transform:uppercase; font-weight:600; color:var(--ink3);
                    padding:.62rem 1rem .55rem; cursor:pointer; user-select:none;
                    border-bottom:2px solid transparent; margin-bottom:-1px;
                    display:flex; align-items:center; gap:.42rem; }
  .viewtabs label:hover { color:var(--ink); }
  .viewtabs label .n { font-size:.62rem; font-weight:700; padding:.05rem .34rem;
                       border-radius:2px; background:var(--line); color:var(--ink2); }
  .view > section + section { margin-top:1.9rem; }
  /* A label styled as a link, so the page can point at its own hidden content with
     no JavaScript. Never the ONLY route anywhere — Space/Enter on a focused label
     is not universal — and the tab bar always reaches the same place. */
  .xlink { color:var(--accent); cursor:pointer; text-decoration:underline;
           text-decoration-thickness:1px; text-underline-offset:2px; font-weight:600; }
  .xlink:hover { text-decoration-thickness:2px; }
  .xlink:focus-visible { outline:2px solid var(--accent); outline-offset:2px; }

  /* ---- the eleven ---- */
  .squadbar { display:flex; flex-wrap:wrap; gap:1.6rem; align-items:baseline;
              padding:.85rem 1rem; border-bottom:1px solid var(--line);
              background:var(--panel2); border-radius:5px 5px 0 0; }
  .squadbar > div { display:flex; flex-direction:column; gap:.1rem; }
  .squadbar .v { font-size:1.15rem; font-weight:800; letter-spacing:-.02em; line-height:1.1;
                 font-variant-numeric:tabular-nums; }
  .squadbar .v small { font-size:.72rem; font-weight:400; color:var(--ink3);
                       margin-left:.28rem; letter-spacing:0; }
  .squadbar .v.acc { color:var(--accent); }
  .squadbar .v.bad { color:var(--bad); }

  /* The roster is deliberately NOT inside .tscroll: overflow-x establishes a
     clipping context, and it would cut the why-card off on every row. Six columns
     fit 64rem with room to spare, and two of them drop below 34rem. */
  td.c-min, td.c-score, td.c-price { text-align:right; white-space:nowrap;
                                     font-family:var(--mono);
                                     font-variant-numeric:tabular-nums; }
  th.c-min, th.c-score, th.c-price { text-align:right; }
  td.c-score { position:relative; width:6.5rem; }
  /* max-width is load-bearing, not belt-and-braces: a percentage width on an
     absolutely-positioned element resolves against the containing block's PADDING
     box, so the top scorer's 100% bar started .85rem outside its own cell and bled
     into the column to its left. */
  /* A rule, not a filled block. A block with no visible track has no remainder for the
     eye to read a length against, so it reads as a highlight swatch — fifteen grey
     boxes. At 3px the near-equal lengths across the eleven read as information, and
     the bottom of the bench reads as what it is: Kusi-Asare at 0.03 has no bar at all
     next to F.Kadıoğlu's 3.21, and that is the only thing on the page saying the bench
     is empty. Scaled against the best score in the fifteen, never against the column's
     own minimum — a truncated baseline manufactures a shape. */
  td.c-score .bar { position:absolute; right:.85rem; bottom:.34rem; height:3px;
                    max-width:calc(100% - 1.7rem);
                    background:var(--barfill); border-radius:2px; pointer-events:none;
                    display:block; }
  td.c-score .fig { position:relative; font-weight:700; }
  tr.iscap td:first-child { box-shadow:inset 3px 0 0 var(--accent); }

  .fdr { display:inline-flex; gap:2px; }
  .fdr i { display:block; width:1.2rem; height:1.2rem; line-height:1.2rem; text-align:center;
           font-family:var(--mono); font-size:.66rem; font-weight:700; border-radius:2px;
           color:var(--ink2); font-style:normal; }
  .fdr i.d1 { background:var(--fdr1); }
  .fdr i.d2 { background:var(--fdr2); }
  .fdr i.d3 { background:var(--fdr3); }
  .fdr i.d4 { background:var(--fdr4); }
  .fdr i.d5 { background:var(--fdr5); }
  .fdr .more { background:none; color:var(--ink3); width:auto; padding:0 .15rem; }
  .fdr .none { font-family:var(--mono); font-size:.7rem; color:var(--bad); font-weight:700; }

  /* Rotation bands. "nailed" never reaches here — see htmlCard.Band. */
  .band { font-family:var(--mono); font-size:.6rem; font-weight:700; letter-spacing:.03em;
          padding:.1rem .34rem; border-radius:2px; margin-left:.4rem; white-space:nowrap; }
  .band.b2 { background:var(--panel2); color:var(--ink2); }
  .band.b3, .band.b4 { background:var(--warnbg); color:var(--warnink); }
  .band.b5 { background:var(--badbg); color:var(--bad); }
  .st { font-family:var(--mono); font-size:.6rem; font-weight:700; padding:.1rem .34rem;
        border-radius:2px; margin-left:.4rem; background:var(--badbg); color:var(--bad);
        white-space:nowrap; }
  .sp { font-family:var(--mono); font-size:.6rem; font-weight:700; padding:.1rem .34rem;
        border-radius:2px; margin-left:.4rem; background:var(--panel2); color:var(--ink3);
        white-space:nowrap; }

  /* ---- overrides. A sixth hue, because an override is a statement about
         authorship and carries no valence: green or red would say a hand-set
         correction was good or bad, and the accent would fight the armband. ---- */
  .ovb { font-family:var(--mono); font-size:.6rem; font-weight:700; letter-spacing:.04em;
         padding:.1rem .36rem; border-radius:2px; margin-left:.4rem; white-space:nowrap;
         background:var(--ovbg); color:var(--ovink); border:1px solid var(--ovline); }
  .ovb.club { background:transparent; border-style:dashed; }
  .ovb.check { border-color:var(--warn); }
  .ovb.lapsed { background:transparent; border-style:dashed; border-color:var(--line2);
                color:var(--ink3); text-decoration:line-through; }
  .ovr { border:1px solid var(--line); border-left:3px solid var(--ovline);
         border-radius:0 5px 5px 0; background:var(--panel);
         padding:.75rem .95rem; margin:0 0 .7rem; }
  .ovr.check { border-left-color:var(--warn); }
  .ovr.lapsed { border-left-color:var(--line2); background:var(--panel2); opacity:.72; }
  .ovr-h { display:flex; flex-wrap:wrap; align-items:baseline; gap:.5rem; margin-bottom:.4rem; }
  .ovr-h b { font-size:.98rem; }
  .ovr .meta { font-family:var(--mono); font-size:.66rem; color:var(--ink3);
               font-variant-numeric:tabular-nums; }
  .ovr .flag { font-family:var(--mono); font-size:.6rem; font-weight:700; padding:.1rem .36rem;
               border-radius:2px; background:var(--warnbg); color:var(--warnink);
               border:1px solid var(--warn); white-space:nowrap; }
  /* An override that is binding a player in the fifteen is doing something right
     now; one on a player the optimiser declined anyway is a note. The rule is solid
     rather than 3px so the difference survives a column of nine warn borders. */
  .ovr.insquad { border-left-width:5px; border-left-color:var(--ovink); }
  .ovr .inxi { font-family:var(--mono); font-size:.6rem; font-weight:700; padding:.1rem .36rem;
               border-radius:2px; background:var(--ovbg); color:var(--ovink);
               border:1px solid var(--ovline); white-space:nowrap; }
  .ovr p { margin:0; max-width:70ch; font-size:.88rem; color:var(--ink2); }
  .ovr .touches { margin:.45rem 0 0; font-size:.82rem; color:var(--ink3); }

  /* ---- the why-card ---- */
  /* A div rather than a span: the card contains block content (rows, rules,
     paragraphs), and flow content inside a span is invalid HTML. It parses correctly
     everywhere, but the cheap remedy is to be valid. inline-block keeps the name,
     club and badges on one line. */
  .pop-wrap { position:relative; display:inline-block; }
  .whytrig { cursor:help; text-decoration:underline dotted var(--line2);
             text-decoration-thickness:1px; text-underline-offset:3px; }
  .whytrig:hover, .whytrig:focus-visible { text-decoration-color:var(--accent); }
  .whytrig:focus-visible { outline:2px solid var(--accent); outline-offset:2px;
                           border-radius:2px; }
  /* visibility, not display:none — the card stays in the accessibility tree so
     aria-describedby resolves whether or not it is open. */
  .pop { position:absolute; top:calc(100% + 7px); left:0; z-index:60;
         width:min(30rem, calc(100vw - 2.5rem));
         visibility:hidden; opacity:0;
         background:var(--panel); border:1px solid var(--line2); border-radius:6px;
         box-shadow:var(--pop-shadow); padding:.85rem 1rem;
         font-weight:400; text-align:left; white-space:normal;
         max-height:27rem; overflow-y:auto; transition:opacity .07s linear; }
  /* bridges the 7px gap so the pointer can travel into the card without it closing */
  .pop::before { content:""; position:absolute; top:-8px; left:0; right:0; height:8px; }
  .pop-wrap:hover > .pop, .pop-wrap:focus-within > .pop { visibility:visible; opacity:1; }
  .pop .hl { font-size:1.15rem; font-weight:800; font-variant-numeric:tabular-nums;
             letter-spacing:-.02em; }
  .pop .row { display:flex; gap:.5rem; align-items:baseline; margin:.3rem 0;
              font-size:.8rem; color:var(--ink2); }
  .pop .row .k { flex:0 0 5.6rem; }
  .pop .mult { font-family:var(--mono); font-weight:700; color:var(--ink);
               font-variant-numeric:tabular-nums; flex:0 0 3.4rem; }
  .pop hr { border:0; border-top:1px solid var(--line); margin:.6rem 0; }
  .pop .note { font-size:.78rem; color:var(--ink3); }
  .pop .ovhead { color:var(--ovink); font-weight:700; font-size:.82rem; }
  /* The reason is clamped INSIDE the card, and only inside it.
     Reasons run to 126 words. Printed whole they filled the card's 24rem and pushed
     the corrections line — the thing the card exists to show — below its internal
     scroll fold, on precisely the players whose numbers most need explaining. Since
     the standing rule is that the card is an accelerator over prose that also appears
     unhidden in the "why" view, the card shows the opening and points at the rest.
     Four lines is a summary; the rest is one click away. */
  .pop .ovreason { margin:.25rem 0 .5rem; display:-webkit-box; -webkit-line-clamp:4;
                   line-clamp:4; -webkit-box-orient:vertical; overflow:hidden; }

  /* ---- research categories ---- */
  .cat { margin:0 0 1.8rem; }
  .cat h3 { font-size:1rem; font-weight:700; letter-spacing:-.01em; margin:0 0 .35rem;
            text-wrap:balance; }
  .cat .why { margin:0 0 .5rem; max-width:66ch; font-size:.88rem; color:var(--ink2); }
  .cat .ask { margin:0 0 .7rem; max-width:66ch; font-size:.88rem; padding:.5rem .8rem;
              background:var(--panel2); border-left:3px solid var(--line2);
              border-radius:0 4px 4px 0; }
  .cat .ask .k { display:block; margin-bottom:.15rem; }
  .blind { border:1px solid var(--line); border-radius:5px; padding:.9rem 1.1rem;
           background:var(--panel); }
  .blind ul { margin:0; padding-left:1.1rem; max-width:68ch; font-size:.88rem; }
  .blind li { margin:.28rem 0; color:var(--ink2); }
  .rules { display:grid; grid-template-columns:repeat(auto-fit,minmax(11rem,1fr));
           gap:.9rem 1.6rem; margin:0 0 1rem; }
  .rules > div .v { font-size:1.05rem; font-weight:800; font-variant-numeric:tabular-nums; }

  /* ---- watchlist ---- */
  tr.grp td { background:var(--panel2); border-bottom:1px solid var(--line2);
              padding:.45rem .85rem; }
  tr.grp .k { margin-right:.9rem; }
  tr.grp .bench { font-size:.78rem; color:var(--ink3); font-variant-numeric:tabular-nums; }
  td.c-delta { font-family:var(--mono); font-variant-numeric:tabular-nums; text-align:right;
               white-space:nowrap; }
  td.c-delta.up { color:var(--good); font-weight:700; }
  td.c-delta.down { color:var(--ink3); }
  td.c-own { text-align:right; font-family:var(--mono); white-space:nowrap;
             font-variant-numeric:tabular-nums; color:var(--ink3); }
  th.c-delta, th.c-own { text-align:right; }
  .empty { color:var(--ink3); font-style:italic; }

  .tabs { display:flex; gap:.35rem; flex-wrap:wrap; margin-bottom:.9rem; }
  .tabs label { font-family:var(--mono); font-size:.72rem; padding:.32rem .7rem;
                border:1px solid var(--line); border-radius:3px; cursor:pointer;
                background:var(--panel); color:var(--ink2); user-select:none;
                font-variant-numeric:tabular-nums; }
  .tabs label:hover { border-color:var(--line2); color:var(--ink); }
  /* A chip week is the one a reader scans 38 tabs looking for, so it carries the
     chip hue on its border as well as a badge — colour alone would be lost to
     anyone who cannot see it, and the badge alone is lost in a long strip. */
  .tabs label.haschip { border-color:var(--chipbg); }
  .tabs label .tc { display:inline-block; margin-left:.4rem; padding:.02rem .28rem;
                    border-radius:2px; font-size:.62rem; font-weight:700;
                    background:var(--chipbg); color:var(--chipink); }
  /* A hit week carries its cost on the tab, so the strip itself marks which
     gameweeks paid for their transfers. */
  .tabs label .tc.hitcost { background:var(--badbg); color:var(--bad); }
  .weeks input { position:absolute; opacity:0; pointer-events:none; }
  .weeks input:focus-visible + label, .tabs label:focus-within { outline:2px solid var(--accent); }
  .weekpanel { display:none; }
  .weekhead { display:flex; gap:1.4rem; flex-wrap:wrap; align-items:baseline;
              padding:.75rem .85rem; border-bottom:1px solid var(--line);
              font-family:var(--mono); font-size:.75rem; color:var(--ink3);
              font-variant-numeric:tabular-nums; }
  .weekhead b { color:var(--ink); font-size:1rem; font-family:var(--sans); font-weight:700; }

  .moves { padding:.75rem .85rem; border-bottom:1px solid var(--line); background:var(--panel2);
           display:flex; flex-direction:column; gap:.4rem; font-size:.86rem; }
  .moves.none { color:var(--ink3); font-style:italic; }
  .mv { display:flex; align-items:center; gap:.55rem; flex-wrap:wrap; }
  .mv .out { text-decoration:line-through; color:var(--ink3); }
  .mv .in { font-weight:700; }
  .mv .arr { color:var(--ink3); }
  .mv .meta { font-family:var(--mono); font-size:.7rem; color:var(--ink3); }
  .mv .gw { font-family:var(--mono); font-size:.68rem; font-weight:700; color:var(--ink3);
            min-width:2.9rem; }
  .foot { color:var(--ink3); font-size:.8rem; margin-top:.7rem; max-width:70ch; }
  .mv .verdict { font-family:var(--mono); font-size:.7rem; padding:.1rem .38rem;
                 border-radius:3px; font-variant-numeric:tabular-nums; }
  .mv .verdict.up { background:var(--goodbg); color:var(--good); }
  .mv .verdict.down { background:var(--badbg); color:var(--bad); }

  .brief h1 { font-size:1.5rem; letter-spacing:-.02em; margin:0 0 .5rem; }
  .brief h2 { font-size:1.12rem; letter-spacing:-.01em; margin:1.9rem 0 .6rem;
              padding-bottom:.3rem; border-bottom:2px solid var(--line2); text-wrap:balance; }
  .brief h3 { font-family:var(--mono); font-size:.72rem; text-transform:uppercase;
              letter-spacing:.12em; color:var(--ink3); margin:1.4rem 0 .5rem; }
  .brief h4 { font-size:.98rem; margin:1rem 0 .35rem; }
  .brief p { margin:0 0 .7rem; max-width:70ch; font-size:.9rem; }
  .brief ul { margin:0 0 .9rem; padding-left:1.1rem; max-width:70ch; font-size:.9rem; }
  .brief li { margin:.2rem 0; }
  .brief hr { border:0; border-top:1px solid var(--line); margin:1.6rem 0; }
  .brief code { font-family:var(--mono); font-size:.85em; background:var(--panel2);
                border:1px solid var(--line); border-radius:3px; padding:.03rem .28rem; }
  .brief blockquote { margin:.9rem 0; padding:.7rem .9rem; background:var(--warnbg);
                      border-left:3px solid var(--warn); color:var(--warnink);
                      border-radius:0 4px 4px 0; }
  .brief blockquote p { margin:0; }
  .brief table { font-size:.82rem; }
  .brief .tscroll { border:1px solid var(--line); border-radius:5px; margin:0 0 1rem; }
  details > summary { cursor:pointer; font-family:var(--mono); font-size:.7rem;
                      letter-spacing:.13em; text-transform:uppercase; color:var(--ink3);
                      font-weight:600; padding:.2rem 0; }
  details[open] > summary { margin-bottom:.9rem; }

  /* Scoped to the roster, which is the ONE table not allowed to scroll — it sits
     outside .tscroll so the why-card is not clipped, so on a narrow screen it has to
     shed columns instead. Every other table can simply scroll, and applying this
     globally stripped Mins from the research tables, where it is load-bearing: the
     "no Premier League data" category is defined by that column reading 0. */
  @media (max-width:34rem) {
    .roster th.c-fix, .roster td.c-fix, .roster th.c-min, .roster td.c-min { display:none; }
    /* Dropping two columns is not enough on a 320px screen: the set-piece badge is
       the score column is a fixed 6.5rem, which pushed PRICE off the right edge —
       the body scrolling sideways, which is the one thing this layout may never do.
       (The set-piece badge was the other half of this and is now gone entirely; PEN 1
       is five characters and costs nothing.) */
    .roster td, .roster th { padding-left:.5rem; padding-right:.5rem; }
    .roster td.c-score, .roster th.c-score { width:3.9rem; }
    .roster td.c-score .bar { right:.5rem; max-width:calc(100% - 1rem); }
  }
  @media (prefers-reduced-motion:reduce) { * { transition:none !important; } }
  /* Print flattens the view tabs — all three views, but not 38 week panels, which
     would be forty pages. Why-cards are hover objects and their content is
     duplicated unhidden in the other views. */
  @media print {
    .viewtabs, .tabs { display:none; }
    .view { display:block !important; }
    .view + .view { border-top:2px solid var(--ink); margin-top:2rem; padding-top:1rem; }
    .pop { display:none; }
  }
</style></head><body><div class="wrap">

<header>
  <h1>{{.Title}}</h1>
  {{if .Subtitle}}<p>{{.Subtitle}}</p>{{end}}
</header>

{{with .Brief}}<section>
  <div class="facts">
    {{if .Gameweek}}<div><div class="k">Gameweek</div><div class="v">{{.Gameweek}}</div></div>{{end}}
    {{if .Deadline}}<div><div class="k">Deadline</div><div class="v" style="font-size:1.05rem">{{.Deadline}}{{if .Countdown}}<small>{{.Countdown}}</small>{{end}}</div></div>{{end}}
    <div><div class="k">Played</div><div class="v">{{.Played}}<small>of 38</small></div></div>
    <div><div class="k">Horizon</div><div class="v">{{.Horizon}}<small>gw</small></div></div>
    {{if .Transfers}}<div><div class="k">Free transfers</div><div class="v acc">{{.Transfers}}</div></div>{{end}}
    {{if .Overrides}}<div><div class="k">Overrides binding</div><div class="v">{{.Overrides}}{{if $.HasWhy}}<small><label class="xlink" for="v-why" tabindex="0">read them</label></small>{{end}}</div></div>{{end}}
  </div>
  {{if .Chips}}<div class="chips" style="margin-top:.9rem"><span class="k">Chip plan</span>
    {{range .Chips}}<span class="chip">{{.}}</span>{{end}}</div>{{end}}
  {{if .Alerts}}<ul class="alerts" style="margin-top:.9rem">{{range .Alerts}}<li>{{.}}</li>{{end}}</ul>{{end}}
</section>{{end}}

{{with .Summary}}<section>
  <div class="facts">
    <div><div class="k">Points</div><div class="v acc">{{.Points}}</div></div>
    <div><div class="k">Transfers</div><div class="v">{{.Transfers}}{{if .Unlisted}}<small>
      {{.Unlisted}} in a chip rebuild</small>{{end}}</div></div>
    <div><div class="k">Hits</div><div class="v{{if .Hits}} bad{{end}}">{{if .Hits}}&minus;{{end}}{{.Hits}}<small>
      {{if .HitMoves}}on {{.HitMoves}} of {{.Transfers}} moves{{else}}all moves free{{end}}</small></div></div>
    <div><div class="k">Best</div><div class="v">{{.BestPts}}<small>GW{{.Best}}</small></div></div>
    <div><div class="k">Worst</div><div class="v">{{.WorstPts}}<small>GW{{.Worst}}</small></div></div>
    <div style="flex:1;min-width:14rem">
      <div class="k">Points by gameweek</div>
      <div class="shape">{{range .Bars}}<i class="{{.Class}}" style="height:{{.Height}}%"
        title="GW{{.GW}}: {{.Points}} pts"></i>{{end}}</div>
    </div>
  </div>
  <div class="chips" style="margin-top:.9rem">
    <span class="k">Chips</span>
    {{if .Chips}}{{range .Chips}}<span class="chip">{{.}}</span>{{end}}
    {{else}}<span class="none">none played &mdash; this replay was run without a chip plan,
      so no wildcard, bench boost, free hit or triple captain was available to it</span>{{end}}
  </div>
  <p class="foot">Every number on this page already happened. The model decided each week from
    data available before that deadline, and never saw a result it had not yet been shown.</p>
</section>{{end}}

<div class="views">
<input type="radio" name="view" id="v-team" checked>
{{if .HasWhy}}<input type="radio" name="view" id="v-why">{{end}}
{{if .HasWatch}}<input type="radio" name="view" id="v-watch">{{end}}
<style>
  #v-team:checked ~ #view-team { display:block; }
  #v-team:checked ~ .viewtabs label[for="v-team"] { color:var(--ink); font-weight:700;
    border-bottom-color:var(--accent); }
  #v-team:focus-visible ~ .viewtabs label[for="v-team"] { outline:2px solid var(--accent);
    outline-offset:-2px; }
  {{if .HasWhy}}
  #v-why:checked ~ #view-why { display:block; }
  #v-why:checked ~ .viewtabs label[for="v-why"] { color:var(--ink); font-weight:700;
    border-bottom-color:var(--accent); }
  #v-why:focus-visible ~ .viewtabs label[for="v-why"] { outline:2px solid var(--accent);
    outline-offset:-2px; }
  {{end}}
  {{if .HasWatch}}
  #v-watch:checked ~ #view-watch { display:block; }
  #v-watch:checked ~ .viewtabs label[for="v-watch"] { color:var(--ink); font-weight:700;
    border-bottom-color:var(--accent); }
  #v-watch:focus-visible ~ .viewtabs label[for="v-watch"] { outline:2px solid var(--accent);
    outline-offset:-2px; }
  {{end}}
</style>

{{if .Tabs}}<nav class="viewtabs">
  <label for="v-team">The eleven</label>
  {{if .HasWhy}}<label for="v-why">Why this eleven{{with .Reasoning}}{{if .Overrides}}<span class="n">{{len .Overrides}}</span>{{end}}{{end}}</label>{{end}}
  {{if .HasWatch}}<label for="v-watch">Watchlist{{with .Watch}}{{if .Count}}<span class="n">{{.Count}}</span>{{end}}{{end}}</label>{{end}}
</nav>{{end}}

<div class="view" id="view-team">

{{if .Rows}}<section>
  <h2>{{if .Replay}}Squad{{else}}The starting eleven{{end}}</h2>
  <div class="panel">
  {{if not .Replay}}<div class="squadbar">
    <div><div class="k">Formation</div><div class="v">{{.Squad.Formation}}</div></div>
    <div><div class="k">XI</div><div class="v acc">{{pts1 .Squad.XIScore}}<small>pts/gw</small></div></div>
    <div><div class="k">With captain</div><div class="v">{{pts1 .Squad.ExpectedPoints}}</div></div>
    <div><div class="k">Spent</div><div class="v">{{money .Squad.TotalCost}}</div></div>
    <div><div class="k">Left</div><div class="v{{if lt .Squad.Remaining 0.0}} bad{{end}}">{{money .Squad.Remaining}}</div></div>
  </div>{{end}}
  <table class="roster">
    <thead><tr>
      <th>Pos</th><th>Player</th>
      {{if .Replay}}<th>Opponent</th>{{else}}<th class="c-fix">Next {{.Horizon}}</th><th class="c-min">Mins</th>{{end}}
      <th class="c-score">{{if .Replay}}Pts{{else}}Score{{end}}</th>
      {{if not .Replay}}<th class="c-price">&pound;</th>{{end}}
    </tr></thead>
    <tbody>
    {{range .Rows}}{{range .Players}}<tr{{if .IsCaptain}} class="iscap"{{end}}>
      <td class="pos">{{.Position}}</td>
      <td>{{template "rostername" .}}</td>
      {{if $.Replay}}<td>{{if .Opponent}}<span class="opp">{{.Opponent}}</span>{{else}}<span class="opp blank">blank</span>{{end}}</td>
      {{else}}<td class="c-fix">{{template "fdr" .}}</td><td class="c-min">{{mins .ExpMins}}</td>{{end}}
      <td class="c-score"><span class="bar" style="width:{{.ScorePct}}%"></span><span class="fig">{{pts .Score}}</span></td>
      {{if not $.Replay}}<td class="c-price">{{money .Price}}</td>{{end}}
    </tr>{{end}}{{end}}
    {{if .Bench}}<tr class="sub"><td colspan="6"><span class="k">Bench &mdash; substitution order</span></td></tr>
    {{range .Bench}}<tr class="benchrow">
      <td class="pos">{{.Position}}</td>
      <td>{{template "rostername" .}}{{if .Slot}}<span class="tag v">{{.Slot}}</span>{{end}}</td>
      {{if $.Replay}}<td>{{if .Opponent}}<span class="opp">{{.Opponent}}</span>{{else}}<span class="opp blank">blank</span>{{end}}</td>
      {{else}}<td class="c-fix">{{template "fdr" .}}</td><td class="c-min">{{mins .ExpMins}}</td>{{end}}
      <td class="c-score"><span class="bar" style="width:{{.ScorePct}}%"></span><span class="fig">{{pts .Score}}</span></td>
      {{if not $.Replay}}<td class="c-price">{{money .Price}}</td>{{end}}
    </tr>{{end}}{{end}}
    </tbody>
  </table></div>
  {{if not .Replay}}<p class="foot">Rotation risk is shown only when it is <em>not</em> nailed, so a
    row with no band is a regular starter. Hover or tab to any name for the numbers behind it.
    This eleven is picked over the whole {{.Horizon}}-gameweek horizon; the eleven actually
    fielded changes week to week &mdash; see <a href="#weeks">week by week</a>.</p>{{end}}
</section>{{end}}

{{if .Weeks}}
<section class="weeks" id="weeks">
  <h2>{{if .Replay}}Gameweek by gameweek{{else}}Week by week{{end}}</h2>
  {{range $i, $w := .Weeks}}<input type="radio" name="wk" id="wk{{$w.Event}}"{{if $w.First}} checked{{end}}>{{end}}
  <style>
  {{range .Weeks}}#wk{{.Event}}:checked ~ #panel{{.Event}} { display:block; }
  #wk{{.Event}}:checked ~ .tabs label[for="wk{{.Event}}"] {
    background:var(--ink); border-color:var(--ink); color:var(--bg); font-weight:700; }
  {{end}}
  </style>
  <div class="tabs">
    {{range .Weeks}}<label for="wk{{.Event}}"{{if .Chip}} class="haschip" title="{{.Chip}}"{{end}}>GW{{.Event}}{{if .Chip}}<span class="tc">{{chipShort .Chip}}</span>{{end}}{{if .Hit}}<span class="tc hitcost" title="{{.Hit}}-point hit this week">&minus;{{.Hit}}</span>{{end}}</label>{{end}}
  </div>
  {{range .Weeks}}<div class="weekpanel" id="panel{{.Event}}"><div class="panel">
    <div class="weekhead">
      <span><b>GW{{.Event}}</b></span>
      <span>{{.Formation}}</span>
      <span>captain <b style="font-size:.9rem">{{.Captain}}</b></span>
      {{if .Actual}}<span>
        {{if .Hit}}<b style="font-size:.9rem">{{printf "%.0f" (gross .Expected .Hit)}}</b> &minus; <span class="hitmark">{{.Hit}} hit{{if gt .Hit 4}}s{{end}}</span> = <b style="font-size:.9rem">{{printf "%.0f" .Expected}}</b> pts
        {{else}}<b style="font-size:.9rem">{{printf "%.0f" .Expected}}</b> pts scored{{end}}</span>
        <span>{{.Running}} to date</span>{{else}}<span><b style="font-size:.9rem">{{pts1 .Expected}}</b> pts projected</span>{{end}}
      {{if .Chip}}<span class="chip">{{.Chip}}</span>{{end}}
    </div>
    {{if .Actual}}{{if .Moves}}<div class="moves">
      {{range .Moves}}<div class="mv">
        <span class="out">{{.Out.Name}}</span><span class="arr">&rarr;</span><span class="in">{{.In.Name}}</span>
        {{if .Hit}}<span class="tag hit">&minus;4 hit</span>{{else}}<span class="tag free">free</span>{{end}}
        <span class="meta">model {{delta .Delta}}/gw</span>
        <span class="verdict {{if ge .In.Score .Out.Score}}up{{else}}down{{end}}">actual {{printf "%.0f" .In.Score}} v {{printf "%.0f" .Out.Score}}</span>
      </div>{{end}}
    </div>{{else}}<div class="moves none">No transfer &mdash; the gate found nothing worth a free move.</div>{{end}}{{end}}
    {{if .Rebuilt}}<div class="moves none">Rebuilt fifteen &mdash; a {{.Chip}} re-picks the whole squad.</div>{{end}}
    {{if .Caveat}}<div class="moves none">{{.Caveat}}</div>{{end}}
    <div class="tscroll"><table>
      <thead><tr><th>Pos</th><th>Player</th><th>Opponent</th><th class="c-score">{{if .Actual}}Pts{{else}}Score{{end}}</th></tr></thead>
      <tbody>
      {{range .Rows}}{{range .Players}}<tr{{if eq .Position "BENCH"}} class="benchrow"{{end}}>
        <td class="pos">{{.Position}}</td>
        <td>{{template "flatname" .}}{{if .Risk}}<span class="tag in">{{.Risk}}</span>{{end}}</td>
        <td>{{if .Opponent}}<span class="opp">{{.Opponent}}</span>{{else}}<span class="opp blank">blank</span>{{end}}</td>
        <td class="c-score">{{pts .Score}}</td>
      </tr>{{end}}{{end}}
      </tbody>
    </table></div>
    {{if .Blanks}}<div class="moves none">No fixture: {{range $j, $b := .Blanks}}{{if $j}}, {{end}}{{$b}}{{end}}</div>{{end}}
  </div></div>{{end}}
</section>
{{end}}

{{/* The replay's own transfer history. It is a separate section from the
     per-week strips because the tabs can only ever show one week: a reader
     asking what the policy did across the season would otherwise have to open
     every gameweek and remember. The first version of this page fell through to
     the NoPlan placeholder here and showed none of them. */}}
{{if and .Replay .Summary}}{{if .Summary.Moves}}<section>
  <h2>Every transfer, in order</h2>
  <div class="panel"><div class="moves">
    {{range .Summary.Moves}}<div class="mv">
      <span class="gw">GW{{.GW}}</span>
      <span class="out">{{.Out.Name}}</span><span class="arr">&rarr;</span><span class="in">{{.In.Name}}</span>
      {{if .Hit}}<span class="tag hit">&minus;4 hit</span>{{else}}<span class="tag free">free</span>{{end}}
      <span class="meta">model {{delta .Delta}}/gw</span>
      <span class="verdict {{if ge .In.Score .Out.Score}}up{{else}}down{{end}}">actual {{printf "%.0f" .In.Score}} v {{printf "%.0f" .Out.Score}}</span>
    </div>{{end}}
  </div></div>
  {{if .Summary.Unlisted}}<p class="foot">{{.Summary.Unlisted}} further transfers are not listed here.
    A wildcard replaces the fifteen in one act, so the replay counts its swaps toward the season total
    but records no out-for-in pair for them &mdash; there is no single player each one replaced.</p>{{end}}
  <p class="foot">&ldquo;Actual&rdquo; is each player's raw points over the five gameweeks the move was
    judged on, whether or not he was picked; &ldquo;model&rdquo; is the per-gameweek gain the policy
    expected from the eleven. The two are not on the same footing, so read the sign rather than the
    difference.</p>
</section>{{end}}{{end}}

{{if and (not .Moves) .NoPlan}}<section>
  <h2>Transfers</h2>
  <div class="panel"><div class="moves none">{{.NoPlan}}</div></div>
  {{template "gate" .}}
</section>{{end}}

{{if .Moves}}<section>
  <h2>Suggested transfers</h2>
  <div class="panel"><div class="moves">
    {{range .Moves}}<div class="mv">
      <span class="out">{{.Out.Name}}</span><span class="arr">&rarr;</span><span class="in">{{.In.Name}}</span>
      <span class="meta">{{delta .Delta}} pts/gw</span>
    </div>{{end}}
  </div></div>
  {{template "gate" .}}
</section>{{end}}

{{with .Analysis}}<section class="brief">
  <h2>The judgement on top</h2>
  {{.}}
</section>{{end}}

</div>{{/* end view-team */}}

{{if .HasWhy}}{{with .Reasoning}}<div class="view" id="view-why">

<section>
  <h2>Standing overrides</h2>
  {{if .Overrides}}
    <p class="foot" style="margin:0 0 .9rem">Hand-set corrections, binding on every squad build
      and transfer search on this page. An expiry is a guess made when the injury happened and
      it is wrong in both directions &mdash; a setback lets one lapse while the player is still
      out, and a player back early is never reconsidered.
      {{if .Due}}<br><b>{{.Due}} of {{len .Overrides}} are due a look</b> &mdash; either unverified
      for a week or within two gameweeks of lapsing, which is when the expiry date itself becomes
      the decision. Oldest: {{.Oldest}}. The badge carries each one's age, and the ones binding a
      player in your fifteen sort first.{{else}}All have been verified in the last week, and none
      is close enough to lapsing for the date to matter yet.{{end}}</p>
    {{range .Overrides}}<article class="ovr {{.CardClass}}">
      <div class="ovr-h">
        <span class="ovb {{.BadgeClass}}">{{.Label}}</span>
        <b>{{.Player}}</b>{{if .Team}}<span class="club">{{.Team}}</span>{{end}}
        <span class="meta">{{if .SetOn}}set {{.SetOn}}{{end}}{{if .Until}} &middot; {{.Until}}{{end}}{{if .Checked}} &middot; checked {{.Checked}}{{end}}</span>
        {{if .InSquad}}<span class="inxi">in the fifteen</span>{{end}}
        {{if .NeedsCheck}}<span class="flag">{{.Flag}}</span>{{end}}
      </div>
      <p>{{.Reason}}</p>
      {{if .Inherited}}<p class="touches"><span class="k">In this squad</span>
        {{if .AffectsInSquad}}{{range $i, $n := .AffectsInSquad}}{{if $i}}, {{end}}{{$n}}{{end}}
        {{else}}none &mdash; it moves the watchlist only.{{end}}</p>{{end}}
    </article>{{end}}
  {{else}}<p class="foot">No standing overrides. This squad is the model's unaided answer.</p>{{end}}
  {{if .Lapsed}}<details style="margin-top:1rem"><summary>Lapsed ({{len .Lapsed}})</summary>
    {{range .Lapsed}}<article class="ovr {{.CardClass}}">
      <div class="ovr-h">
        <span class="ovb {{.BadgeClass}}">{{.Label}}</span>
        <b>{{.Player}}</b>{{if .Team}}<span class="club">{{.Team}}</span>{{end}}
        {{if .SetOn}}<span class="meta">set {{.SetOn}}</span>{{end}}
        {{if .Until}}<span class="meta">{{.Until}}</span>{{end}}
      </div>
      <p>{{.Reason}}</p>
    </article>{{end}}
    <p class="foot">Kept rather than deleted: dropping them would hide that a player's treatment
      changed, which is the failure the check dates exist to catch.</p>
  </details>{{end}}
</section>

{{if $.Research}}<section>
  <h2>Where the model says it is blind</h2>
  <p class="foot" style="margin:0 0 1.2rem">Computed by the deterministic layer, not by a
    judgement on top. It cannot see roles &mdash; it infers minutes from last season's totals,
    which say nothing about a manager's plan for this one &mdash; so this names where it is
    structurally wrong rather than verifying players already under consideration.</p>
  {{range $.Research}}<article class="cat">
    <h3>{{.Name}}</h3>
    <p class="why">{{.Why}}</p>
    <p class="ask"><span class="k">Ask</span>{{.Ask}}</p>
    {{if .Targets}}<div class="panel tscroll"><table>
      <thead><tr><th>Player</th><th>Pos</th><th class="c-price">&pound;</th>
        <th class="c-own">Own</th><th class="c-score">Score</th><th class="c-min">Mins</th></tr></thead>
      <tbody>{{range .Targets}}<tr>
        <td>{{template "flatname" .}}</td>
        <td class="pos">{{.Position}}</td>
        <td class="c-price">{{money .Price}}</td>
        <td class="c-own">{{pc .Own}}</td>
        <td class="c-score">{{pts .Score}}</td>
        <td class="c-min">{{.Mins}}</td>
      </tr>{{end}}</tbody>
    </table></div>{{end}}
  </article>{{end}}
</section>{{end}}

{{if .Blind}}<section>
  <h2>What this model cannot see at all</h2>
  <div class="blind"><ul>{{range .Blind}}<li>{{.}}</li>{{end}}</ul>
  {{if $.Reasoning.Breaks}}<p class="foot">{{$.Reasoning.Breaks}}</p>{{end}}</div>
</section>{{end}}

{{if .Policy.Rules}}<section>
  <h2>The rules it is deciding under</h2>
  <div class="rules">
    <div><div class="k">Free transfer needs</div><div class="v">{{pts .Policy.MinGainTransfer}}<small class="k"> pts/gw</small></div></div>
    <div><div class="k">A &minus;4 needs</div><div class="v">{{pts .Policy.MinGainHit}}<small class="k"> pts</small></div></div>
    <div><div class="k">Bank up to</div><div class="v">{{.Policy.BankUpTo}}</div></div>
    <div><div class="k">Max hits per week</div><div class="v">{{.Policy.MaxHitsPerWeek}}</div></div>
  </div>
  <div class="blind"><ul>{{range .Policy.Rules}}<li>{{.}}</li>{{end}}</ul></div>
</section>{{end}}

</div>{{end}}{{end}}{{/* end view-why */}}

{{if .HasWatch}}<div class="view" id="view-watch">
{{if .WatchGroups}}<section>
  <h2>Best available, not in the fifteen</h2>
  <p class="foot" style="margin:0 0 .9rem">Ranked by the model's score. &Delta; is the gap to the
    weakest starter you already own in that position, named in each group header &mdash; which is
    the comparison a transfer actually has to win, where a league rank is not.
    {{with .Watch}}{{if .Gate}}<br><b>Green clears the free-transfer gate of {{pts .Gate}} pts/gw.</b>
    {{if .Clearing}}{{.Clearing}} of {{.Count}} do.{{else}}<b>Nothing on this list does</b> &mdash;
    these are ordered, not recommended.{{end}}{{end}}{{end}}</p>
  <div class="panel tscroll"><table class="wtable">
    <thead><tr><th>Player</th><th class="c-fix">Next</th><th class="c-min">Mins</th>
      <th class="c-own">Own</th><th class="c-score">Score</th><th class="c-delta">&Delta;</th>
      <th class="c-price">&pound;</th></tr></thead>
    <tbody>
    {{range .WatchGroups}}<tr class="grp"><td colspan="7">
      <span class="k">{{.Position}}</span>
      {{if .BenchmarkName}}<span class="bench">vs your weakest starter &mdash; {{.BenchmarkName}} {{pts .BenchmarkScore}}, {{money .BenchmarkPrice}}</span>
      {{else}}<span class="bench">no starter in this position to compare against</span>{{end}}
    </td></tr>
    {{if .Rows}}{{range .Rows}}<tr>
      <td>{{template "flatname" .Card}}</td>
      <td class="c-fix">{{template "fdr" .Card}}</td>
      <td class="c-min">{{mins .Card.ExpMins}}</td>
      <td class="c-own">{{pc .Card.Own}}</td>
      <td class="c-score">{{pts .Card.Score}}</td>
      <td class="c-delta {{.DeltaClass}}">{{delta .Delta}}</td>
      <td class="c-price">{{money .Card.Price}}</td>
    </tr>{{end}}{{else}}<tr><td colspan="7" class="empty">&mdash; no candidate outside the fifteen</td></tr>{{end}}
    {{end}}
    </tbody>
  </table></div>
</section>{{end}}

{{with .Watch}}{{if .Excluded}}<section>
  <h2>Excluded by a standing override ({{len .Excluded}})</h2>
  <p class="foot" style="margin:0 0 .9rem">Not candidates &mdash; decisions. These players are kept
    out of every squad built on this page, so the reason is printed in full rather than hidden:
    &ldquo;why is this obviously good player not here&rdquo; is the question this view exists to
    answer.</p>
  {{range .Excluded}}<article class="ovr {{.CardClass}}">
    <div class="ovr-h">
      <span class="ovb {{.BadgeClass}}">{{.Label}}</span>
      <b>{{.Player}}</b>{{if .Team}}<span class="club">{{.Team}}</span>{{end}}
      {{if .Pos}}<span class="meta">{{.Pos}}</span>{{end}}
      {{if .Price}}<span class="meta">{{money .Price}}</span>{{end}}
      {{/* What he would score is the cost of the exclusion, and it is the number a
           reader is here to weigh against the reason — so it is printed even when it
           is 0.00, which is the commonest case and the most informative one. An
           injury exclusion zeroes availabilityFactor and so zeroes the score, and a
           truthiness guard dropped the line on exactly those players, leaving "the
           exclusion costs nothing" indistinguishable from "the page did not say".
           Same reasoning as AvailabilityFactor not being omitempty. */}}
      <span class="meta">would score {{pts .Score}}</span>
      <span class="meta">{{if .SetOn}}set {{.SetOn}}{{end}}{{if .Until}} &middot; {{.Until}}{{end}}{{if .Checked}} &middot; checked {{.Checked}}{{end}}</span>
      {{if .NeedsCheck}}<span class="flag">{{.Flag}}</span>{{end}}
    </div>
    <p>{{.Reason}}</p>
  </article>{{end}}
</section>{{end}}{{end}}
</div>{{end}}{{/* end view-watch */}}

</div>{{/* end views */}}
</div></body></html>

{{define "gate"}}{{if .Reasoning}}{{if .Reasoning.Policy.MinGainTransfer}}<p class="foot">The gate:
  a free transfer needs {{pts .Reasoning.Policy.MinGainTransfer}} pts/gw, a &minus;4 needs
  {{pts .Reasoning.Policy.MinGainHit}} pts across the horizon.
  {{if .HasWhy}}<label class="xlink" for="v-why" tabindex="0">The rules in full</label>.{{end}}</p>{{end}}{{end}}{{end}}

{{/* The fixture strip. One cell per FIXTURE rather than per gameweek: pack it by
     gameweek and a club playing twice in one week lines up against a club playing
     once, silently. The difficulty digit is always printed, so the tint is redundant
     encoding and is never the sole carrier of the rating. */}}
{{define "fdr"}}{{if .Fixtures}}<span class="fdr">{{range .Fixtures}}<i class="d{{.Diff}}" title="GW{{.Event}} {{.Opponent}} ({{.Where}}) difficulty {{.Diff}}">{{.Diff}}</i>{{end}}{{if .MoreFix}}<i class="more">+{{.MoreFix}}</i>{{end}}</span>{{else}}<span class="fdr"><span class="none">no fixture</span></span>{{end}}{{end}}

{{/* The badges, in one fixed order so a scanning eye learns one position per
     meaning: armband, then who overrode this, then how nailed he is, then set
     pieces, then availability. */}}
{{define "badges"}}{{if .IsCaptain}}<span class="tag c">C</span>{{end}}{{if .IsVice}}<span class="tag v">V</span>{{end}}{{if .Override}}<span class="ovb {{.Override.BadgeClass}}" title="{{.Override.Label}} &mdash; hand-set correction">{{.Override.Label}}</span>{{end}}{{if .Band}}<span class="band {{.BandClass}}">{{.Band}}</span>{{end}}{{if .Pen}}<span class="sp">{{.Pen}}</span>{{end}}{{if .Status}}<span class="st">{{.Status}}{{if .Chance}} {{.Chance}}{{end}}</span>{{end}}{{end}}

{{/* The roster name cell, with the why-card behind it. Only used inside the roster
     table, which is deliberately not wrapped in .tscroll — overflow-x would clip the
     card on every row. The other tables scroll and get the flat name instead. */}}
{{define "rostername"}}<div class="pop-wrap"><span class="who whytrig" tabindex="0" aria-describedby="why-{{.ID}}">{{.Name}}</span><div class="pop" id="why-{{.ID}}" role="note">{{template "why" .Why}}</div></div><span class="club">{{.Team}}</span>{{template "badges" .}}{{end}}

{{define "flatname"}}<span class="who">{{.Name}}</span><span class="club">{{.Team}}</span>{{template "badges" .}}{{end}}

{{/* The card. Note there is no total and no equals sign anywhere in it: Score is not
     the product of the multipliers below — the engine splits rate from threshold
     terms, scales them by two different appearance probabilities and clamps a
     negative rate — so a waterfall here would be a second implementation of the
     scoring arithmetic, and wrong for the first player who hits the clamp. */}}
{{define "why"}}
{{with .Override}}<div class="ovhead">{{.Label}} &mdash; {{if .Inherited}}club correction, not personal{{else}}hand-set override{{end}}</div>
<p class="note ovreason">{{.Reason}}</p>
<p class="note" style="margin:0 0 .5rem">{{if .SetOn}}Set {{.SetOn}}{{end}}{{if .Until}} &middot; {{.Until}}{{end}}{{if .Checked}} &middot; checked {{.Checked}}{{end}} &middot; <label class="xlink" for="v-why" tabindex="0">full reasoning</label></p>
<hr>{{end}}
<div class="hl">{{pts .Score}} <span class="k">pts/gw</span></div>
<div class="row"><span class="k">Per &pound;m</span><span>{{pts .Value}}</span></div>
<hr>
<div class="row"><span class="k">Per 90</span><span>base {{pts .Base}}{{if .SetPiece}} &middot; set pieces +{{pts .SetPiece}}{{end}} &middot; after fixtures {{pts .FixtureAdj}}</span></div>
{{if .SetPieceNote}}<div class="row"><span class="k">Set pieces</span><span>{{.SetPieceNote}}</span></div>{{end}}
<div class="row"><span class="k">Minutes</span><span>{{mins .ExpMins}} min/gw &middot; {{.Band}} &middot; reliability {{pts .Reliability}}</span></div>
{{if lt .Matches 38}}<div class="row"><span class="k"></span><span class="note">{{.Matches}} of 38 matches available{{if .Absence}} &mdash; {{.Absence}}{{end}}</span></div>{{end}}
{{if .PriorPct}}<div class="row"><span class="k"></span><span class="note">{{.PriorPct}}% of his rates come from this season, the rest from last</span></div>{{end}}
<hr>
{{if .NoCorrections}}<div class="row"><span class="k">Corrections</span><span>{{.NoCorrectionsLine}}</span></div>
{{else}}{{range .Corrections}}<div class="row"><span class="mult">{{mult .Factor}}</span><span>{{.Label}}{{if .Note}} <span class="note">&mdash; {{.Note}}</span>{{end}}</span></div>{{end}}{{end}}
{{if .News}}<hr><div class="row"><span class="k">News</span><span>{{.News}}</span></div>{{end}}
<hr>
<div class="row"><span class="k">Fixtures</span><span>{{pts1 .AvgFDR}} avg &middot; {{range $i, $f := .Fixtures}}{{if $i}} {{end}}{{$f.Opponent}}({{$f.Where}},{{$f.Diff}}){{end}}</span></div>
{{end}}
`))
