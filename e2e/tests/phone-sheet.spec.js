// The rendered-box case the vault note make-the-planner-readable-on-a-phone calls "still
// owed": internal/webui/sheet_test.go's TestThePhoneSheetHasALabelledWayBack only regexes the
// CSS text (no max-height:Nvh clamp survives), which is a NEGATIVE guard — absence of a clamp
// is not presence of full height, and deleting `flex:1` from armband.css would leave that test
// green while returning the strip of exposed pitch the whole route exists to remove. This file
// is the positive geometric check: does the sheet's box actually fill the viewport.
const { test, expect } = require('../fixtures');

// ⚠️ DEVIATION from the design this suite implements, found by reading app.js rather than
// assumed. The design asked for "sheet's bounding box height equals viewport height (±1px)
// and y=0" on #sheet alone. That is not what the markup does even on the route that shipped:
// #sheet sits BELOW a full-width #sheetback bar (app.html's <div class="sheet-scrim" id=
// "scrim"> wraps both as siblings), so #sheet's own top is the bar's height, not 0. What
// "fills the viewport, no exposed pitch strip" actually means for this markup is that the
// bar and the sheet TOGETHER tile the viewport with no gap: the bar starts at y=0 and the
// sheet's bottom edge reaches the viewport's bottom edge. That is what is asserted below.
//
// ⚠️ A second, more consequential gap found the same way: there is NO Escape handler for this
// sheet at all (only the separate chip menu has one — app.js's single `keydown` listener
// checks `S.chipOpen`, nothing else), and closeSheet() never moves or restores focus. The
// vault note's own "what was verified" section lists both as defects against the PRE-fix
// corner-close design; the shipped full-screen route closed the reachability problem but,
// as far as this suite could find in the current app.js, did not add either. This suite
// cannot assert what is not there, so those two assertions are test.fixme() below rather
// than silently dropped — flagged for whoever reviews this PR to file as its own finding,
// since a modal with role="dialog" aria-modal="true" and no Escape/focus handling is a real
// accessibility gap independent of this suite's own scope.
test('the sheet fills the viewport on phone: no exposed pitch strip', async ({ page }) => {
  test.skip(test.info().project.name !== 'phone', 'phone-only: the route only applies at ≤720px');
  await page.goto('/');
  const card = page.locator('#pitch .card').first();
  await card.click();
  await expect(page.locator('#scrim')).toHaveClass(/\bopen\b/);

  // Measured against #scrim's OWN box, not page.viewportSize(): on this host the snap
  // Chromium's reported CSS pixel box for a fixed, inset:0 element is consistently a few
  // pixels larger than the viewport Playwright was asked for (measured here: 393×851 for a
  // requested 390×844) — a host/renderer quirk, not a page defect, and #scrim carries the
  // exact same offset because it too is `position:fixed;inset:0`. Comparing the bar and the
  // sheet against #scrim's box rather than against the requested viewport size is what makes
  // the "no exposed strip" claim survive that quirk instead of being a false positive on it.
  const scrimBox = await page.locator('#scrim').boundingBox();
  const barBox = await page.locator('#sheetback').boundingBox();
  const sheetBox = await page.locator('#sheet').boundingBox();
  expect(barBox, '#sheetback must be visible on phone').toBeTruthy();
  expect(sheetBox, '#sheet must be visible on phone').toBeTruthy();

  expect(Math.abs(barBox.y - scrimBox.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(sheetBox.x - scrimBox.x)).toBeLessThanOrEqual(1);
  expect(Math.abs(sheetBox.width - scrimBox.width)).toBeLessThanOrEqual(1);
  expect(Math.abs(sheetBox.y + sheetBox.height - (scrimBox.y + scrimBox.height))).toBeLessThanOrEqual(1);
  // No gap between the bar and the sheet — the strip of pitch the route was built to remove.
  expect(Math.abs(sheetBox.y - (barBox.y + barBox.height))).toBeLessThanOrEqual(1);
  // A loose sanity bound that this is really the whole screen and not some small modal that
  // merely fills ITS OWN small container — #scrim itself should be within a few CSS pixels
  // of the requested viewport, well inside the host quirk measured above (max seen: 7px).
  const vp = page.viewportSize();
  expect(Math.abs(scrimBox.width - vp.width)).toBeLessThan(10);
  expect(Math.abs(scrimBox.height - vp.height)).toBeLessThan(10);
});

test('the same sheet does NOT fill the viewport on desktop (negative control)', async ({
  page,
}) => {
  // Without this, the phone assertion above could pass on a constant — e.g. a #sheet whose
  // CSS always renders at 100% of SOMETHING regardless of viewport.
  test.skip(test.info().project.name !== 'desktop', 'desktop-only control');
  await page.goto('/');
  await page.locator('#pitch .card').first().click();
  await expect(page.locator('#scrim')).toHaveClass(/\bopen\b/);

  const vp = page.viewportSize();
  const sheetBox = await page.locator('#sheet').boundingBox();
  expect(sheetBox.height).toBeLessThan(vp.height);
  // .sheetback is display:none above 720px (armband.css) — the corner ✕ is the desktop
  // dismissal instead.
  await expect(page.locator('#sheetback')).toBeHidden();
});

test('the back control is reachable, readable and dismisses the sheet', async ({ page }) => {
  test.skip(test.info().project.name !== 'phone', 'phone-only: the route only applies at ≤720px');
  await page.goto('/');
  await page.locator('#pitch .card').first().click();
  await expect(page.locator('#scrim')).toHaveClass(/\bopen\b/);

  const label = page.locator('#sheetbacklabel');
  const text = (await label.textContent()).trim();
  // Words, not a glyph — the vault note's own contrast for what the route replaced (a
  // corner ✕ with no aria-label reachable at the far corner from a thumb).
  expect(text.length).toBeGreaterThan(1);

  const box = await page.locator('#sheetback').boundingBox();
  // The vault note's own measured defect on the button the route replaced: 34×34px, "under
  // the 44px touch-target minimum". Asserted on the CONTROL's box, not a CSS declaration —
  // the same rendered-box standard this file applies to the fill claim.
  expect(box.height).toBeGreaterThanOrEqual(44);

  await page.locator('#sheetback').click();
  await expect(page.locator('#scrim')).not.toHaveClass(/\bopen\b/);
});

test.fixme(
  'Escape dismisses the sheet',
  async ({ page }) => {
    await page.goto('/');
    await page.locator('#pitch .card').first().click();
    await expect(page.locator('#scrim')).toHaveClass(/\bopen\b/);
    await page.keyboard.press('Escape');
    await expect(page.locator('#scrim')).not.toHaveClass(/\bopen\b/);
  },
);

test.fixme(
  'focus returns to the card that opened the sheet',
  async ({ page }) => {
    await page.goto('/');
    const card = page.locator('#pitch .card').first();
    await card.click();
    await page.locator('#sheetback').click();
    await expect(card).toBeFocused();
  },
);
