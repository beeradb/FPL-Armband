package present

import (
	"sort"
	"strings"
)

// WatchPageSize is how many watchlist rows one page holds.
//
// ⚠️ Neither this nor WatchCap binds the product any more. Both apply inside Apply,
// which is reached only from Render — and Render has no product caller since the
// application replaced the served page. /api/state sends every row and the client
// paginates. They are described below as they behave, not as they bind; removing
// them is real work and is owed.
const WatchPageSize = 50

// WatchCap is how many rows the UNFILTERED watchlist shows at most. It is a
// display cap, not a selection: it applies in Apply, over the whole pool the
// caller built, and any filter lifts it — a reader filtering for a name or a
// position is looking for a specific player, who may score nowhere near the
// hundred.
const WatchCap = 100

// WatchQuery is a reader's filter, sort and page over the watchlist.
//
// The served page carries these as query parameters — sortable headers are
// links, filters are a GET form, pagination is a pair of links — because the
// page is deliberately script-free. The zero value is not useful; build from
// DefaultWatchQuery and override.
type WatchQuery struct {
	// Sort is one of name, pos, team, mins, own, score, delta, price.
	// Anything else falls back to the default.
	Sort string
	// Desc is the sort direction. Numeric columns descend by default; the
	// name-like ones ascend. The caller resolves the default direction for a
	// column with WatchNaturalDir.
	Desc bool
	// Q is a case-insensitive substring over the player's name. Empty filters
	// nothing.
	Q string
	// Pos is one of GKP, DEF, MID, FWD. Empty means any position.
	Pos string
	// Team is a club short name. Empty means any club.
	Team string
	// Page is 1-based, clamped to the page count.
	Page int
	// Pageable reports whether Apply should slice to one page. The served page
	// sets it; the static export leaves it false and shows every row.
	Pageable bool
}

// DefaultWatchQuery is the page's opening state: sorted by price, descending.
func DefaultWatchQuery() WatchQuery {
	return WatchQuery{Sort: "price", Desc: true, Page: 1}
}

// watchSortColumns is the set of sortable columns, and watchNaturalDesc marks
// the ones that read naturally downward — a number column opens with the
// biggest first, a name column with A first.
var watchSortColumns = map[string]bool{
	"name": true, "pos": true, "team": true,
	"mins": true, "own": true, "score": true, "delta": true, "price": true,
}

var watchNumeric = map[string]bool{
	"mins": true, "own": true, "score": true, "delta": true, "price": true,
}

// ValidWatchSort reports whether a column is sortable.
func ValidWatchSort(col string) bool { return watchSortColumns[col] }

// WatchNaturalDir is the direction a column opens on: "desc" for numbers, "asc"
// for names. A caller that receives no explicit direction uses this.
func WatchNaturalDir(col string) string {
	if watchNumeric[col] {
		return "desc"
	}
	return "asc"
}

// Normalise pins a query to something Apply can run: an unknown sort becomes
// the default, and the page number is at least 1. Unknown position or team
// filters are left alone — filtering on a club nobody in the list plays for
// answers with zero rows, which is a true answer, not a parse failure.
func (q WatchQuery) Normalise() WatchQuery {
	if !ValidWatchSort(q.Sort) {
		q.Sort = "price"
		q.Desc = true
	}
	if q.Page < 1 {
		q.Page = 1
	}
	return q
}

// Apply filters, sorts and pages the watchlist's rows for one query. It
// returns the rows for the resolved page, the resolved page number, the page
// count, and how many rows matched the filter before slicing — the number the
// pager reports. With Pageable false every matching row comes back on page 1.
func (w *Watchlist) Apply(q WatchQuery) (rows []WatchRow, page, pages, total int) {
	q = q.Normalise()

	for _, r := range w.Rows {
		if q.Q != "" && !strings.Contains(strings.ToLower(r.Player.Name), strings.ToLower(q.Q)) {
			continue
		}
		if q.Pos != "" && r.Player.Position != q.Pos {
			continue
		}
		if q.Team != "" && r.Player.Team != q.Team {
			continue
		}
		rows = append(rows, r)
	}
	// The unfiltered view is capped at the best WatchCap rows — the caller's
	// ranking, which is score — so the opening list stays short enough to
	// read. A filter lifts the cap: it has already searched the whole pool.
	// The cap applies BEFORE the sort, so it always means the best hundred by
	// the caller's ranking, whatever column the reader then orders by.
	if q.Q == "" && q.Pos == "" && q.Team == "" && len(rows) > WatchCap {
		rows = rows[:WatchCap]
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return watchLess(rows[i], rows[j], q.Sort, q.Desc)
	})

	total = len(rows)
	pages = (total + WatchPageSize - 1) / WatchPageSize
	if pages < 1 {
		pages = 1
	}
	page = q.Page
	if page > pages {
		page = pages
	}
	if !q.Pageable {
		return rows, 1, pages, total
	}
	from := (page - 1) * WatchPageSize
	to := from + WatchPageSize
	if to > total {
		to = total
	}
	return rows[from:to], page, pages, total
}

// watchLess compares two rows on one column. The direction flips the PRIMARY
// comparison only; ties always break name-ascending then id-ascending, so the
// same two players sit in the same order whichever way a column is sorted,
// and a re-sort cannot shuffle equal rows from run to run.
func watchLess(a, b WatchRow, col string, desc bool) bool {
	primary := 0
	if watchNumeric[col] {
		na, nb := watchNum(a, col), watchNum(b, col)
		switch {
		case na < nb:
			primary = -1
		case na > nb:
			primary = 1
		}
	} else {
		sa, sb := watchStr(a, col), watchStr(b, col)
		primary = strings.Compare(sa, sb)
	}
	if primary != 0 {
		if desc {
			return primary > 0
		}
		return primary < 0
	}
	if a.Player.Name != b.Player.Name {
		return a.Player.Name < b.Player.Name
	}
	return a.Player.ID < b.Player.ID
}

func watchNum(r WatchRow, col string) float64 {
	switch col {
	case "mins":
		return r.Player.ExpectedMinutes
	case "own":
		return r.Player.Ownership
	case "score":
		return r.Player.Score
	case "delta":
		return r.Delta
	}
	return r.Player.Price
}

func watchStr(r WatchRow, col string) string {
	switch col {
	case "pos":
		return r.Player.Position
	case "team":
		return r.Player.Team
	}
	return strings.ToLower(r.Player.Name)
}
