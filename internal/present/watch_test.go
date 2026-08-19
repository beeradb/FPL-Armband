package present

import (
	"strings"
	"testing"

	"armband/internal/analysis"
)

// watchRow builds a candidate row from the fields a sort or filter reads.
func watchRow(id int, name, team, pos string, mins, own, score, price float64) WatchRow {
	return WatchRow{
		Player: analysis.PlayerMetrics{
			ID: id, Name: name, Team: team, Position: pos,
			ExpectedMinutes: mins, Ownership: own, Score: score, Price: price,
		},
		Delta: score - 4,
	}
}

// TestWatchApplyDefaultsToPriceDescending pins the one ordering the ask named:
// the page opens with the most expensive candidate first, whatever order the
// caller built the rows in.
func TestWatchApplyDefaultsToPriceDescending(t *testing.T) {
	w := &Watchlist{Rows: []WatchRow{
		watchRow(1, "Cheap", "AAA", "GKP", 90, 5, 4.0, 4.0),
		watchRow(2, "Dear", "BBB", "MID", 90, 5, 6.0, 12.5),
		watchRow(3, "Mid", "CCC", "DEF", 90, 5, 5.0, 6.0),
	}}
	rows, page, pages, total := w.Apply(DefaultWatchQuery())
	if page != 1 || pages != 1 || total != 3 {
		t.Fatalf("page %d of %d (%d rows); want 1 of 1 (3)", page, pages, total)
	}
	if rows[0].Player.Name != "Dear" || rows[2].Player.Name != "Cheap" {
		t.Errorf("default order is %s, %s, %s; want Dear, Mid, Cheap",
			rows[0].Player.Name, rows[1].Player.Name, rows[2].Player.Name)
	}
}

// TestWatchApplyFiltersOnNamePositionAndTeam. The three filters the ask names,
// each combinable, each leaving the sort alone.
func TestWatchApplyFiltersOnNamePositionAndTeam(t *testing.T) {
	w := &Watchlist{Rows: []WatchRow{
		watchRow(1, "Saka", "ARS", "MID", 90, 5, 6.0, 10.0),
		watchRow(2, "Saliba", "ARS", "DEF", 90, 5, 5.0, 6.5),
		watchRow(3, "Salah", "LIV", "MID", 90, 5, 7.0, 13.0),
	}}
	q := DefaultWatchQuery()
	q.Q = "sal"
	rows, _, _, total := w.Apply(q)
	if total != 2 || rows[0].Player.Name != "Salah" || rows[1].Player.Name != "Saliba" {
		t.Fatalf("name filter returned %+v, want Salah and Saliba by price", names(rows))
	}

	q = DefaultWatchQuery()
	q.Pos = "MID"
	if _, _, _, total = w.Apply(q); total != 2 {
		t.Errorf("position filter returned %d rows, want 2", total)
	}
	q.Team = "LIV"
	if _, _, _, total = w.Apply(q); total != 1 {
		t.Errorf("position+team filter returned %d rows, want 1", total)
	}
}

// TestWatchApplyPaginatesFiftyAtATime. The served page shows 50 rows a page,
// clamps an out-of-range page to the last one, and reports the row span.
func TestWatchApplyPaginatesFiftyAtATime(t *testing.T) {
	w := &Watchlist{Rows: make([]WatchRow, 120)}
	for i := range w.Rows {
		w.Rows[i] = watchRow(i+1, "P"+string(rune('A'+i%26)), "AAA", "DEF", 90, 5,
			5.0, float64(120-i))
	}
	q := DefaultWatchQuery()
	q.Pageable = true

	rows, page, pages, total := w.Apply(q)
	if page != 1 || pages != 3 || total != 120 || len(rows) != 50 {
		t.Fatalf("page 1: got page %d of %d, %d rows shown, %d total",
			page, pages, len(rows), total)
	}
	q.Page = 3
	rows, page, _, _ = w.Apply(q)
	if page != 3 || len(rows) != 20 {
		t.Fatalf("page 3: got page %d with %d rows, want 3 with 20", page, len(rows))
	}
	q.Page = 99
	rows, page, _, _ = w.Apply(q)
	if page != 3 || len(rows) != 20 {
		t.Fatalf("page 99 clamped to page %d with %d rows, want 3 with 20", page, len(rows))
	}

	// The static export pages nothing: every row comes back on page 1.
	static := DefaultWatchQuery()
	rows, page, pages, _ = w.Apply(static)
	if page != 1 || pages != 3 || len(rows) != 120 {
		t.Fatalf("unpaged apply: page %d of %d with %d rows; want 1 of 3 with all 120",
			page, pages, len(rows))
	}
}

// TestWatchSortIsDeterministicThroughTies. Rows equal on the sort column must
// break the same way regardless of direction — name ascending, then id — or
// every re-sort reshuffles the list.
func TestWatchSortIsDeterministicThroughTies(t *testing.T) {
	w := &Watchlist{Rows: []WatchRow{
		watchRow(2, "Zeta", "AAA", "MID", 90, 5, 5.0, 6.0),
		watchRow(1, "Alpha", "AAA", "MID", 90, 5, 5.0, 6.0),
		watchRow(3, "Alpha", "BBB", "MID", 90, 5, 5.0, 6.0),
	}}
	for _, desc := range []bool{true, false} {
		rows, _, _, _ := w.Apply(WatchQuery{Sort: "score", Desc: desc})
		got := names(rows)
		want := []string{"Alpha", "Alpha", "Zeta"}
		if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("score %v: ties ordered %v, want %v", desc, got, want)
		}
	}
}

// TestWatchSortColumnsCoverEveryHeading. The template names eight sortable
// columns; a heading whose column is not sortable would render a dead link or
// a silent fallback. The set is enumerated once, in watchSortColumns, and this
// asserts the headings' set against it.
func TestWatchSortColumnsCoverEveryHeading(t *testing.T) {
	for _, col := range []string{"name", "pos", "team", "mins", "own", "score", "delta", "price"} {
		if !ValidWatchSort(col) {
			t.Errorf("heading column %q is not sortable", col)
		}
	}
	if ValidWatchSort("fixtures") {
		t.Error("the fixtures strip is not a sortable column")
	}
	if WatchNaturalDir("price") != "desc" || WatchNaturalDir("name") != "asc" {
		t.Error("natural directions are wrong: numbers descend, names ascend")
	}
}

// TestWatchSortHeadsRenderLinksOnlyWhenServed. The served page's headings are
// links carrying the query; the static export's are bare labels, because a
// sort link on a file:// page is a link to a state that page cannot render.
func TestWatchSortHeadsRenderLinksOnlyWhenServed(t *testing.T) {
	static := watchQueryView{}
	if got := string(static.SortHead("price", "&pound;")); !strings.Contains(got, "&pound;") || strings.Contains(got, "<a") {
		t.Errorf("static heading is %q; want the bare label with no anchor", got)
	}
	served := watchQueryView{Interactive: true, Sort: "price", Dir: "desc", Q: "sal", Team: "ARS"}
	head := string(served.SortHead("price", "&pound;"))
	if !strings.Contains(head, `href="?`) || !strings.Contains(head, "sort=price") {
		t.Errorf("served heading %q is not a sort link", head)
	}
	if !strings.Contains(head, "q=sal") || !strings.Contains(head, "team=ARS") {
		t.Errorf("sort link %q drops the active filters", head)
	}
	flipped := string(served.SortHead("price", "&pound;"))
	if !strings.Contains(flipped, "dir=asc") {
		t.Errorf("clicking the active column does not flip it: %q", flipped)
	}
	if pager := string(served.Pager()); pager != "" {
		t.Errorf("a one-page list renders a pager: %q", pager)
	}
	pager := string((watchQueryView{Interactive: true, Sort: "price", Dir: "desc",
		Page: 2, Pages: 3, From: 51, To: 100, Total: 120}).Pager())
	if !strings.Contains(pager, "51–100 of 120") || !strings.Contains(pager, "p=3") {
		t.Errorf("pager does not report the span and the next page: %q", pager)
	}
	if strings.Contains(pager, "p=1") {
		t.Errorf("the first page is a bare sort link, not an explicit p=1: %q", pager)
	}
}

func names(rows []WatchRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Player.Name
	}
	return out
}
