// #optimise is the interaction the goldens structurally cannot exercise: a real click, a real
// PUT /api/session round trip, and the busy state in between. See app.js's sendSave/save
// (the CHAIN promise) and armband.css's `body.saving` rule (pointer-events:none on every
// .btn/.card while a save is in flight — there is no separate `disabled` attribute).
const { test, expect } = require('../fixtures');

test('#optimise fires exactly one PUT /api/session, shows a busy body, and settles', async ({
  page,
}) => {
  // Clicked as the FIRST action in a fresh context (§11.5 of the design this suite
  // implements): the session is otherwise empty, so this is the only save in flight and the
  // request count below is unambiguous.
  await page.goto('/');

  const puts = [];
  page.on('request', (req) => {
    if (req.method() === 'PUT' && new URL(req.url()).pathname === '/api/session') puts.push(req);
  });

  await expect(page.locator('body')).not.toHaveClass(/\bsaving\b/);
  await page.locator('#optimise').click();
  await expect(page.locator('body')).toHaveClass(/\bsaving\b/);
  await expect(page.locator('body')).not.toHaveClass(/\bsaving\b/, { timeout: 15_000 });

  expect(puts.length, 'exactly one PUT /api/session for one click').toBe(1);

  // The re-render: #vsmodel is empty/dim before any arrangement has diverged from the
  // model's own pick, and always carries text once a save has round-tripped and hydrate()
  // has run — see app.js's renderReadout. Preferred over comparing #pitch's data-id set
  // because a FRESH session's first Optimise can legitimately return the same fifteen it
  // already showed (nothing to diverge from yet), which would make an identity check flaky
  // rather than wrong.
  await expect(page.locator('#vsmodel')).not.toBeEmpty();
});

test('the busy body class survives a slow response (negative control)', async ({ page }) => {
  // Without this, "the busy marker clears after" could pass on a marker that was never
  // added in the first place — the same "a check that degrades to a vacuous pass" trap
  // AGENTS.md names for this whole suite (§0.3 of the design). Delaying the response proves
  // the marker really is tied to the request being in flight.
  await page.goto('/');
  await page.route('**/api/session', async (route) => {
    await new Promise((r) => setTimeout(r, 2000));
    await route.continue();
  });

  await page.locator('#optimise').click();
  await expect(page.locator('body')).toHaveClass(/\bsaving\b/);
  await page.waitForTimeout(1000);
  await expect(page.locator('body')).toHaveClass(/\bsaving\b/);
  await expect(page.locator('body')).not.toHaveClass(/\bsaving\b/, { timeout: 15_000 });
});

// The optimistic ".working" class landed on `optimistic-button-feedback-on-the-served-page`
// (PR #184, merged the same session this suite itself landed) — markWorking (app.js) adds
// `.working` and `aria-busy` synchronously inside #optimise's click handler, before the fetch
// is even sent, and clearWorking('session') sweeps it once the save settles.
test(
  '#optimise gets .working synchronously on click and loses it once the save settles',
  async ({ page }) => {
    await page.goto('/');
    const sync = await page.evaluate(() => {
      const b = document.getElementById('optimise');
      b.click();
      return b.classList.contains('working');
    });
    expect(sync).toBe(true);
    await expect(page.locator('#optimise')).not.toHaveClass(/\bworking\b/);
  },
);
