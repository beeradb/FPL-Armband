// A session-cookie round trip the Go goldens cannot exercise: lock a player, reload, and the
// lock must still be there — because it lives in the fpl_session cookie (cmd/armband/
// session.go), not in the DOM. This suite runs the server WITHOUT -persist (see
// e2e/testdata/config.json and serve-fixture.mjs), so nothing here ever touches config.json or
// a team file — every override lives and dies with the browser session.
//
// ⚠️ Three deviations from the design this suite implements, all found by reading app.js and
// running the interaction rather than assumed:
//
// 1. Desktop only. rowActsHtml's own comment: lock/block buttons render ONLY inside the
//    desktop <tr> — "mobile puts these into the row's detail sheet instead, the same
//    'controls move into the sheet below 720px' rule the pitch card already follows".
//    #plist's `.prow` mobile cards carry no lock/block button at all.
//
// 2. This does NOT verify the override through the players-LIST row the design sketched,
//    because a row that gets locked or blocked does not stay put to be re-checked: Market
//    (internal/viewmodel/state.go) documents Rows as "everyone worth considering, measured
//    against the man they would actually displace" and Excluded as "NOT candidates and NOT
//    in Rows" — so BOTH actions remove the row from #ptbody, one because he joins the squad
//    (nothing left for him to displace), the other because he is now excluded outright.
//    Measured directly: locking the market's top row removed its `data-id` from #ptbody
//    entirely on the very next render. The robust, still-real signal is the "Your
//    instructions" panel (#instrpanel/#instrbody, renderInstructions in app.js) — it lists
//    every override this SESSION owns regardless of where the player has since moved, is
//    driven by a fresh server round trip every time (not optimistic client state), and is
//    keyed on the permanent player CODE the server actually stores overrides against
//    (config.Roster is "keyed by permanent player code — element ids are reassigned every
//    summer", AGENTS.md) rather than the season-scoped element id every DOM `data-id` here
//    otherwise uses.
//
// 3. SCOPE CUT, reported honestly rather than forced green: the design also asked for
//    "blocking the same player clears the lock", reached by finding the just-locked player's
//    card on the pitch/bench and using the sheet's own block button. That card is
//    intermittently unlocatable in this harness — measured directly on two different
//    players, `.card[data-id=N]` matched a HIDDEN duplicate (carrying an `isvc`
//    vice-captain-badge class, so evidently a badge/preview element sharing the `.card`
//    class rather than the interactive pitch card) on one run, and matched NOTHING at all
//    on another, despite the server log confirming the lock succeeded both times. That
//    smells like a real timing or duplicate-markup question worth someone's attention, but
//    chasing it further was not a good trade against this suite's own scope, so the
//    mutual-exclusivity half is left out here. What IS covered instead: the instructions
//    panel's own "Undo" control, reached without needing to relocate the player anywhere
//    else in the DOM, which is a real and different interaction from a plain reload.
//
// 4. #instrpanel lives on the PITCH tab, not the Players tab — app.html's own comment on
//    the element ("#leftout (below, on the Players tab) carries the same two classes on a
//    different surface") names #leftout as the Players-tab equivalent and implies
//    #instrpanel is not it. Measured directly: while the Players tab is active, the panel
//    is present with the right text (toHaveCount/toContainText do not check layout) but its
//    getBoundingClientRect() is all zeros — it is there, but its containing view is
//    display:none. A `.click()` correctly refuses a zero-size target, which is what
//    surfaced this rather than a silent false pass. So every read of the panel below
//    switches to the Pitch tab first.
const { test, expect } = require('../fixtures');

test('locking a player survives a reload, and Undo clears it', async ({ page }) => {
  test.skip(test.info().project.name !== 'desktop', 'desktop-only: see file comment');
  await page.goto('/');
  await page.locator('[data-view="players"]:visible').click();

  const row = page.locator('#ptbody tr[data-id]').first();
  await expect(row).toBeVisible();
  const elementId = await row.getAttribute('data-id');

  // "Lock in" from the players LIST is a two-step arm-then-confirm flow, not a direct
  // toggle (wireMarketRows): the first click only sets S.armLock and re-renders an inline
  // "Rebuild around <player>? … Yes, rebuild / Cancel" row (armNoteRowHtml) — locking an
  // unowned player is a squad-rebuild decision, so it asks first.
  await row.locator('[data-act="lock"]').click();
  await page.locator(`[data-armgo="${elementId}"]`).click();

  // #instrpanel lives on the Pitch tab (see file comment §4) — switch there to read it.
  await page.locator('[data-view="pitch"]:visible').click();
  const instrRow = page.locator('#instrbody .instrrow');
  await expect(instrRow).toBeVisible();
  await expect(instrRow).toHaveCount(1);
  await expect(instrRow).toContainText('Locked in');
  const code = await instrRow.locator('[data-instr="undo"]').getAttribute('data-code');
  expect(Number(code)).toBeGreaterThan(0);

  // Reload: a fresh document, same browser session (the fpl_session cookie the fixture's
  // context already carries survives navigation on its own — nothing to re-set). A reload
  // lands back on the Pitch tab by default, so #instrpanel is already the active view.
  await page.reload();
  const instrRowAfterReload = page.locator('#instrbody .instrrow');
  await expect(instrRowAfterReload).toBeVisible();
  await expect(instrRowAfterReload).toHaveCount(1);
  await expect(instrRowAfterReload).toContainText('Locked in');
  await expect(instrRowAfterReload.locator('[data-instr="undo"]')).toHaveAttribute('data-code', code);

  // Undo: a real write (a second PUT /api/session — see app.js's [data-instr] handler,
  // which calls toggleCorrectionByCode again to clear it) rather than a client-only removal.
  await instrRowAfterReload.locator('[data-instr="undo"]').click();
  await expect(page.locator('#instrpanel')).toBeHidden();

  // And the clear survives a reload too, the same session-cookie claim in the other
  // direction — an override that fails to clear server-side would come back on reload even
  // though the panel looked empty a moment before.
  await page.reload();
  await expect(page.locator('#instrpanel')).toBeHidden();
});
