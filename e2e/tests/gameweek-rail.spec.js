// The gameweek rail (#gwrail) — one tab per planning gameweek, walked forward and back.
//
// ⚠️ Under the GW1 capture, on today's real calendar, GW1 and GW2 have already passed their
// deadlines and this session has imported nothing — buildGameweeks (internal/viewmodel/build.go)
// drops a closed gameweek entirely unless the reader has imported real picks, so the rail
// shows only the OPEN planning horizon (2 tabs as of this writing), never a past/closed tab.
// That is exactly the "past-gameweek tabs are unreachable under this fixture" gap the design
// this suite implements calls out (§6) — read the count from /api/state rather than hardcoding
// it, so this spec keeps working as the real calendar moves the open horizon forward.
//
// ⚠️ Also found by reading app.js rather than assumed: selectPlanningGameweek() (the rail's
// click handler for anything not closed) is PURELY client-side — it re-slices the gameweek
// data /api/state already returned on load and calls renderAll(); it issues no network
// request at all. So "no response in the walk is ≥400 status" is trivially true here rather
// than a meaningful network check; the response listener below is kept as a cheap safety net
// in case that ever changes, not as this test's load-bearing assertion.
const { test, expect } = require('../fixtures');

test('every rail tab is clickable and moves the planner without a failed request', async ({
  page,
  request,
}) => {
  const state = await (await request.get('/api/state')).json();
  const gameweeks = state.gameweeks || [];
  expect(gameweeks.length, 'at least one open planning gameweek').toBeGreaterThan(0);

  await page.goto('/');
  await expect(page.locator('#gwrail .gw')).toHaveCount(gameweeks.length);

  const badResponses = [];
  page.on('response', (res) => {
    if (res.status() >= 400) badResponses.push(`${res.status()} ${res.url()}`);
  });

  for (const gw of gameweeks) {
    const tab = page.locator(`#gwrail .gw[data-gw="${gw.gw}"]`);
    await tab.click();
    await expect(page.locator('#ddlLabel')).toHaveText(`GW${gw.gw} deadline`);
    // Presence, not the formatted text itself — the standing constraint this suite's
    // README states: the capture's own deadline is already in the past relative to
    // whenever this runs, so no spec may assert a countdown string or a formatted date.
    await expect(page.locator('#ddl')).not.toBeEmpty();
    await expect(tab).toHaveAttribute('aria-selected', 'true');
  }
  expect(badResponses, badResponses.join('\n')).toEqual([]);
});

test('clicking back returns the same player set as before', async ({ page }) => {
  const state = await (await page.request.get('/api/state')).json();
  const gameweeks = state.gameweeks || [];
  test.skip(gameweeks.length < 2, 'needs at least two open planning gameweeks to walk between');

  await page.goto('/');
  const idsAt = async (gw) => {
    await page.locator(`#gwrail .gw[data-gw="${gw}"]`).click();
    return page.locator('#pitch .card').evaluateAll((els) => els.map((e) => e.dataset.id).sort());
  };

  const first = await idsAt(gameweeks[0].gw);
  await idsAt(gameweeks[1].gw);
  const backToFirst = await idsAt(gameweeks[0].gw);
  expect(backToFirst).toEqual(first);
});
