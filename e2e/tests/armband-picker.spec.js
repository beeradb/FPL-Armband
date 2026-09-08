// The live captain picker (#capPick → openArmbandPicker). Reachable under this fixture:
// smoke-boot already settles a legal XI on /, and the picker ranks that XI — no import,
// no live FPL, no transfer planner. The Go pin (TestXpForReadsThisWeekScores) reads the
// source; this drives the sheet itself and checks its order against the page's own
// week_xp (same key xpFor reads) rather than inventing a second ranking or re-fetching
// /api/state, which can disagree with the hydrated squad under this capture.
const { test, expect } = require('../fixtures');

test('the armband picker ranks this week and carries the TC / race footnotes', async ({
  page,
}) => {
  await page.goto('/');
  await expect(page.locator('#pitch .card')).toHaveCount(11);

  await page.locator('#capPick').click();
  await expect(page.locator('#scrim')).toHaveClass(/\bopen\b/);

  const note = page.locator('#sheet .storenote');
  await expect(note).toContainText("this week's projected points");
  await expect(note).toContainText('triple captain triples');

  // Evaluate against the same bindings openArmbandPicker uses (classic script globals).
  // Displayed pts are toFixed(2), so compare the ranking key itself — not a re-parsed figure.
  const check = await page.evaluate(() => {
    const w = typeof gwState === 'function' ? gwState() : null;
    const weekXp = w && w.week_xp;
    const gotIds = [...document.querySelectorAll('#sheet .caprow')].map((el) => +el.dataset.pick);
    const wantIds = [...S.xi]
      .map((id) => byId(id))
      .sort((a, b) => xpFor(b) - xpFor(a))
      .map((p) => p.id);
    const gap =
      wantIds.length > 1 ? xpFor(byId(wantIds[0])) - xpFor(byId(wantIds[1])) : 0;
    return {
      rowCount: gotIds.length,
      hasWeekXp: !!(weekXp && typeof weekXp === 'object'),
      orderMatchesXpFor: JSON.stringify(gotIds) === JSON.stringify(wantIds),
      // xpFor must be reading week_xp for every XI row, not falling back to Player.xp.
      xpForIsWeekXp: wantIds.every((id) => {
        const week = weekXp[id];
        return week != null && Number.isFinite(week) && xpFor(byId(id)) === week;
      }),
      gap,
      noteText: document.querySelector('#sheet .storenote')?.textContent || '',
    };
  });

  expect(check.rowCount).toBe(11);
  expect(check.hasWeekXp, 'gwState().week_xp must be populated for the live week').toBe(true);
  expect(check.orderMatchesXpFor, 'picker order must be S.xi sorted by xpFor').toBe(true);
  expect(check.xpForIsWeekXp, 'xpFor must equal week_xp[id] for every XI player').toBe(true);

  if (check.gap < 0.5) {
    expect(check.noteText).toContain("inside the model's noise");
  } else {
    expect(check.noteText).not.toContain("inside the model's noise");
  }
});
