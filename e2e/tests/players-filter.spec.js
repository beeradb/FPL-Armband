// The players list's client-side filters — search, position, and the afford toggle — none of
// which round-trip the server, so this is the one spec in the suite that never waits on a
// save. Optional per the design this suite implements (§7.8); included because the selectors
// were cheap to verify once the players view was already open for instructions-persist.spec.js.
//
// ⚠️ DEVIATION found by reading app.js's renderPlayers(), not assumed. The players list is
// TWO markups built from the same filtered array in one render pass, not one: a desktop
// <table id="ptable"><tbody id="ptbody"> of <tr data-id> rows, and a `#plist` of `.prow`
// cards that CSS shows only below 720px (armband.css: `.plist{display:none}` in the base
// rules, `.plist{display:flex}` inside `@media(max-width:720px)`) — both always exist in the
// DOM regardless of viewport. The design this suite implements assumed one `.prow` list at
// every width; picking the wrong one is either invisible (a selector matching hidden rows,
// which Playwright's actionability checks correctly refuse to click/assert-visible on) or
// silently checks the wrong 40 elements. `rows()` below picks the one CSS actually shows.
const { test, expect } = require('../fixtures');

function rows(page) {
  return test.info().project.name === 'phone'
    ? page.locator('#plist .prow')
    : page.locator('#ptbody tr[data-id]');
}

test.beforeEach(async ({ page }) => {
  await page.goto('/');
  // .first(): app.html carries the view tabs TWICE — a top nav and a bottom nav duplicate
  // for narrow widths (both real elements at every width; CSS, not JS, decides which shows).
  await page.locator('[data-view="players"]:visible').click();
  await expect(rows(page).first()).toBeVisible();
});

test('the position filter shows only that position', async ({ page }) => {
  await page.locator('#posfilter [data-pos="GKP"]').click();
  await expect(page.locator('#posfilter [data-pos="GKP"]')).toHaveAttribute('aria-pressed', 'true');
  const list = rows(page);
  const count = await list.count();
  expect(count).toBeGreaterThan(0);
  // Every visible row's own position badge must read GKP — read off the row rather than
  // re-deriving position from a club/price heuristic, which would be a second
  // implementation of the exact rule under test.
  const texts = await list.evaluateAll((els) => els.map((el) => el.textContent));
  for (const text of texts) {
    expect(text).toContain('GKP');
  }
});

test('the search box filters by name', async ({ page }) => {
  const firstName = (await rows(page).first().textContent()).trim();
  // Take a distinctive fragment of whatever the top row actually is, rather than a
  // hardcoded player name — the pool is seasonal and a hardcoded name rots.
  const needle = firstName.slice(0, 4);
  await page.locator('#psearch').fill(needle);
  await expect(rows(page).first()).toBeVisible();
  const texts = await rows(page).evaluateAll((els) => els.map((el) => el.textContent.toLowerCase()));
  for (const text of texts) {
    expect(text).toContain(needle.toLowerCase());
  }
});

test('clear filters resets the search box and position filter', async ({ page }) => {
  await page.locator('#psearch').fill('zzzznomatch');
  await page.locator('#posfilter [data-pos="MID"]').click();
  // #clearFilters only renders into the empty-state markup (renderPlayers's emptyHtml),
  // which needs a genuinely empty result — 'zzzznomatch' guarantees that regardless of pool.
  await page.locator('#clearFilters').click();
  await expect(page.locator('#psearch')).toHaveValue('');
  await expect(page.locator('#posfilter [data-pos="ALL"]')).toHaveAttribute('aria-pressed', 'true');
  await expect(rows(page).first()).toBeVisible();
});

test('the afford toggle narrows the list to affordable players', async ({ page }) => {
  const before = await rows(page).count();
  await page.locator('#affordToggle').click();
  await expect(page.locator('#affordToggle')).toHaveAttribute('aria-pressed', 'true');
  const after = await rows(page).count();
  // Narrows or ties, never widens — affordability is a strictly tighter filter than none.
  expect(after).toBeLessThanOrEqual(before);
});
