// The chip control (#chipctl: a .chippill trigger plus a .chipmenu popover) — three ways to
// open and dismiss it, each a real interaction the Go goldens cannot drive.
const { test, expect } = require('../fixtures');

test('the chip menu opens on the pill', async ({ page }) => {
  await page.goto('/');
  const pill = page.locator('#chipctl .chippill');
  const menu = page.locator('#chipctl .chipmenu');

  await expect(menu).toBeHidden();
  await expect(pill).toHaveAttribute('aria-expanded', 'false');
  await pill.click();
  await expect(menu).toBeVisible();
  await expect(pill).toHaveAttribute('aria-expanded', 'true');
});

test('re-clicking the pill dismisses the chip menu (desktop only)', async ({ page }) => {
  // ⚠️ DEVIATION found by reading armband.css, not assumed: #chipscrim is `display:none`
  // outside `@media(max-width:720px)`, i.e. it exists only on phone. There it sits above
  // the pill in stacking order (z-index:39, position:fixed;inset:0) and physically
  // intercepts a click aimed back at the pill — measured directly (Playwright's own
  // actionability trace: "<div id=chipscrim ...> intercepts pointer events"). So
  // re-clicking the pill to close the menu only works on desktop, where there is no scrim;
  // on phone the scrim (below) and Escape (chipmenu.spec.js) are the two working routes.
  test.skip(test.info().project.name !== 'desktop', 'phone: the scrim intercepts this click, see comment');
  await page.goto('/');
  const pill = page.locator('#chipctl .chippill');
  const menu = page.locator('#chipctl .chipmenu');
  await pill.click();
  await expect(menu).toBeVisible();
  await pill.click();
  await expect(menu).toBeHidden();
});

test('Escape dismisses the chip menu', async ({ page }) => {
  await page.goto('/');
  await page.locator('#chipctl .chippill').click();
  await expect(page.locator('#chipctl .chipmenu')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('#chipctl .chipmenu')).toBeHidden();
});

test('clicking the scrim dismisses the chip menu (phone only)', async ({ page }) => {
  // #chipscrim only ever appears (display:block) below 720px — see the comment above.
  test.skip(test.info().project.name !== 'phone', 'desktop: #chipscrim is display:none there');
  await page.goto('/');
  await page.locator('#chipctl .chippill').click();
  await expect(page.locator('#chipctl .chipmenu')).toBeVisible();
  await expect(page.locator('#chipscrim')).toHaveClass(/\bon\b/);
  // A plain click (Playwright's default: the element's own visible center) — an explicit
  // corner position landed on the sticky topbar sitting above the scrim at that corner.
  await page.locator('#chipscrim').click();
  await expect(page.locator('#chipctl .chipmenu')).toBeHidden();
});

test('placing a chip issues a save and marks the row pressed', async ({ page }) => {
  await page.goto('/');
  await page.locator('#chipctl .chippill').click();
  const menu = page.locator('#chipctl .chipmenu');
  await expect(menu).toBeVisible();

  // An unset, unplaced, enabled row — bench boost is never disabled by a squad-legality
  // rule the way triple captain or a second wildcard leg can be, so it is the stable choice
  // across whatever squad this capture's optimiser lands on.
  const row = menu.locator('.cmrow[data-chip="bboost"]');
  await expect(row).toHaveAttribute('aria-pressed', 'false');

  const puts = [];
  page.on('request', (req) => {
    if (req.method() === 'PUT' && new URL(req.url()).pathname === '/api/session') puts.push(req);
  });
  await row.click();
  await expect
    .poll(() => puts.length, { message: 'placing a chip must PUT /api/session' })
    .toBeGreaterThan(0);
  // ⚠️ No re-click of the pill here (a bug this suite had and fixed): the save's own
  // renderAll() rebuilds #chipctl's markup but S.chipOpen is untouched by placing a chip,
  // so the menu is STILL OPEN after the round trip — re-clicking the pill at this point
  // toggles it CLOSED instead of "re-opening" it, which is what made the removal click
  // below time out against a menu that had just been hidden by this test's own extra click.
  await expect(page.locator('#chipctl .chipmenu .cmrow[data-chip="bboost"]')).toHaveAttribute(
    'aria-pressed',
    'true',
  );

  // Remove it again, leaving the fixture's session as this suite found it for whatever
  // test runs after this one in the same worker.
  await page.locator('#chipctl .chipmenu .cmrow[data-chip="bboost"]').click();
  await expect(page.locator('#chipctl .chippill')).not.toContainText('Bench Boost');
});

test('the open chip menu does not overflow the phone viewport', async ({ page }) => {
  test.skip(test.info().project.name !== 'phone', 'phone-only geometry check');
  await page.goto('/');
  await page.locator('#chipctl .chippill').click();
  const box = await page.locator('#chipctl .chipmenu').boundingBox();
  // Measured against #chipscrim's OWN box, not page.viewportSize() or a DOM client-width
  // property: on this host, snap Chromium's reported CSS pixel box for a fixed/full-bleed
  // element (boundingBox(), which goes through CDP) runs a few pixels wider than either of
  // those (see phone-sheet.spec.js's comment on the same quirk, measured there as 393 for a
  // requested 390 — and document.documentElement.clientWidth turned out NOT to share the
  // offset, so it under-reported the true rendered edge and false-failed here). #chipscrim
  // is also `position:fixed;inset:0` on phone (armband.css), so it carries the identical
  // CDP-measured offset and cancels it out the same way #scrim does in phone-sheet.spec.js.
  const scrimBox = await page.locator('#chipscrim').boundingBox();
  expect(box.x).toBeGreaterThanOrEqual(scrimBox.x - 1);
  expect(box.x + box.width).toBeLessThanOrEqual(scrimBox.x + scrimBox.width + 1);
});
