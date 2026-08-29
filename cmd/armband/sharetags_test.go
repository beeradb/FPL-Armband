package main

import (
	"net/http"
	"strings"
	"testing"
)

// TestTheFrontDoorCarriesItsShareTags pins the tags on the page "/" ACTUALLY
// SERVES, which is the only form of this test that would have caught the
// regression it exists for.
//
// The tags were written, correctly, into landing.html. Then "/" was switched to
// serve app.html — the tool became the front door — and the tags stayed behind.
// Neither change was wrong on its own and nothing failed: the file that had the
// tags still had them, and the route that changed had no reason to think about
// them. Measured live on 2026-08-29, eight days later: "/" returned 0 og:
// and twitter: tags while /about returned 6.
//
// ⚠️ So this test must resolve the page through the router rather than reading a
// filename. A test asserting "app.html has og:image" would pass the day someone
// points "/" at a third document, which is exactly the move that broke it.
//
// Why it matters more than it looks: a shared link is the mechanism by which a
// manager-to-manager product spreads, and without these every share of
// fplarmband.com unfurls as a bare URL.
func TestTheFrontDoorCarriesItsShareTags(t *testing.T) {
	s := fixtureServer(t)

	res := get(t, s, routeLanding)
	if res.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d", routeLanding, res.Code)
	}
	body := res.Body.String()

	// Each of these does a different job in a preview card, and a card missing
	// any one of them degrades rather than fails — which is why they are checked
	// individually instead of counted.
	for _, want := range []string{
		`property="og:title"`,
		`property="og:description"`,
		`property="og:image"`,
		`property="og:url"`,
		`name="twitter:card"`,
		`name="twitter:image"`,
		`name="description"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page served at %q carries no %s. Every share of the "+
				"site unfurls as a bare URL without it", routeLanding, want)
		}
	}

	// ⚠️ og:url must name the page carrying the tag. Pointing it at /about — the
	// document these tags were copied from — would canonicalise every share of
	// the front door to a page the sharer did not link.
	if !strings.Contains(body, `property="og:url" content="https://fplarmband.com/"`) {
		t.Error(`og:url on the front door is not "https://fplarmband.com/". A ` +
			`crawler canonicalises the share to whatever it names, so this must ` +
			`be the page that carries it and not the one the tags came from.`)
	}

	// A summary_large_image card with no image is a worse unfurl than none: it
	// reserves the space and leaves it blank.
	if strings.Contains(body, `content="summary_large_image"`) &&
		!strings.Contains(body, `og:image" content="https://fplarmband.com/assets/`) {
		t.Error("the card is declared summary_large_image but no same-origin " +
			"og:image backs it")
	}
}

// TestTheAboutPageKeepsItsOwnShareTags guards the other half.
//
// The fix for the front door was to ADD tags, not to move them. /about is the
// fixed pitch and is still linked and shared on its own; stripping it to avoid
// "duplication" would trade one bare unfurl for another.
func TestTheAboutPageKeepsItsOwnShareTags(t *testing.T) {
	s := fixtureServer(t)

	res := get(t, s, routeAbout)
	if res.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d", routeAbout, res.Code)
	}
	body := res.Body.String()
	for _, want := range []string{`property="og:image"`, `name="twitter:card"`} {
		if !strings.Contains(body, want) {
			t.Errorf("%s lost %s — the front door gaining tags is not a reason "+
				"to strip the page they came from", routeAbout, want)
		}
	}
}
