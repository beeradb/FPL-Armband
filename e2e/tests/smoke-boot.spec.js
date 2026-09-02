// The harness checkpoint (AGENTS.md sequencing, e2e/README.md): every route boots, the auth
// token actually reached the pages that carry one, the pitch settles at a legal eleven, and
// /api/state genuinely comes from the frozen capture rather than a live fetch. If this file is
// red, nothing built on top of it can be trusted, so it runs first and alone before any other
// spec is written.
const { readFileSync } = require('fs');
const { gunzipSync } = require('zlib');
const path = require('path');
const { test, expect } = require('../fixtures');
const { LIVE_CAPTURE } = require('../scripts/live-capture.js');

const ROUTES = ['/', '/app', '/armband-team', '/wildcard'];

// Only "/" and "/app" (both routeLanding — see cmd/armband/webroutes.go's routeFor) serve
// app.html, the one embedded document that carries the armband-token placeholder at all.
// /armband-team and /wildcard are the spectator pages, deliberately ungated — their own doc
// comments in webroutes.go say so — and team.html/wildcard.html carry no such tag to fill.
// VERIFIED against the actual assets: `grep -c armband-token` on all four page templates
// finds it only in app.html. The design this suite implements assumed all four carried one;
// they do not, and this is that correction.
const TOKEN_ROUTES = new Set(['/', '/app']);

for (const route of ROUTES) {
  test(`${route} boots`, async ({ page }) => {
    const response = await page.goto(route);
    expect(response.status(), `${route} responded`).toBe(200);

    if (!TOKEN_ROUTES.has(route)) return;
    // The positive control for the auth-cookie fixture (§5.3 of the design this suite
    // implements): an empty token means the page rendered the unauthenticated shell, and
    // every write action in every other spec would then be silently refused. See
    // cmd/armband/webroutes.go's withToken/tokenMeta.
    const token = await page.locator('meta[name="armband-token"]').getAttribute('content');
    expect(token, `${route}'s armband-token meta tag must be non-empty, or this run is ` +
      'driving the unauthenticated shell and every write test downstream is meaningless')
      .toBeTruthy();
  });
}

test('the pitch settles at a legal eleven and the bench at four', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#pitch .card')).toHaveCount(11);
  await expect(page.locator('#benchrow .card')).toHaveCount(4);

  const formation = await page.locator('#formation').textContent();
  expect(formation.trim()).toMatch(/^\d-\d-\d$/);
});

test('GET /api/state genuinely comes from the frozen capture (determinism control)', async ({
  page,
  request,
}) => {
  // ⚠️ DEVIATION from the design this suite implements. The original plan compared
  // /api/state's "current" planning gameweek and deadline against the manifest's captured
  // event/event_deadline. That does not hold: buildGameweeks (internal/viewmodel/build.go)
  // computes "current" from the LIVE server clock at request time, not from the captured
  // moment — so which gameweek reads `current: true` drifts forward with the real calendar
  // even though the underlying bootstrap is frozen. Measured directly: against this LIVE_CAPTURE
  // (GW1, deadline 2026-08-21), a run today returns gameweeks 3-4 as the open planning
  // horizon, not gameweek 1 — GW1's deadline has already passed and the entry is unimported,
  // so buildGameweeks drops it entirely. Comparing gameweek numbers is therefore a comparison
  // against the CALENDAR, not the capture, and would need updating every week regardless of
  // whether priming ever regressed.
  //
  // What genuinely proves this response came from the committed capture and not a live fetch
  // is something the capture fixes and the calendar cannot move: which clubs exist. Decoded
  // directly from the same bootstrap-static.json.gz this suite primed the cache from.
  await page.goto('/');
  const res = await request.get('/api/state');
  expect(res.status()).toBe(200);
  const state = await res.json();

  const captureDir = path.resolve(__dirname, '..', '..', 'data', 'captures', LIVE_CAPTURE);
  const gz = readFileSync(path.join(captureDir, 'bootstrap-static.json.gz'));
  const boot = JSON.parse(gunzipSync(gz).toString('utf8'));
  const expectedClubs = boot.teams.map((t) => t.short_name).sort();

  expect(Array.isArray(state.clubs), 'state.clubs must be an array of club codes').toBe(true);
  expect([...state.clubs].sort()).toEqual(expectedClubs);
});

test('the import card is hidden at GW1', async ({ page }) => {
  // The GW1 capture's import window is closed by construction — importWindow
  // (cmd/armband/importwindow.go) requires next.ID >= 2, and this capture's IsNext is
  // event 1. See e2e/README.md for the full list of what is unreachable under this fixture.
  await page.goto('/');
  await expect(page.locator('#importCard')).toBeHidden();
});

test('phone viewport is the real 390px, not the --dump-dom 500px floor', async ({ page }) => {
  // The positive control for browsertest's own documented trap: --dump-dom silently widens
  // a narrow window to 500px while --screenshot (and Playwright's device-metrics-driven
  // viewport) honours the exact request. Without this, a "phone" run at 500px would still
  // look mobile — every max-width:720px rule stays active — and nothing would say the
  // viewport was wrong. See internal/browsertest/browsertest.go's minDumpDOMWidth comment.
  test.skip(test.info().project.name !== 'phone', 'phone-only control');
  await page.goto('/');
  expect(await page.evaluate(() => window.innerWidth)).toBe(390);
  expect(await page.evaluate(() => matchMedia('(max-width: 720px)').matches)).toBe(true);
  expect(await page.evaluate(() => matchMedia('(hover: hover)').matches)).toBe(false);
});
