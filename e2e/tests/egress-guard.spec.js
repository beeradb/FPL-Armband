// The egress guard in fixtures.js aborts and records anything that leaves baseURL, and every
// OTHER spec's afterEach fails if that list is non-empty. This file is the positive control
// for the guard itself: a check that "no external requests occurred" is indistinguishable from
// "the guard silently stopped working" unless something proves the guard still fires. See
// AGENTS.md's "a check that degrades to a vacuous pass is worse than an absent one".
const { test, expect } = require('../fixtures');

test('the guard actually blocks a request that leaves baseURL', async ({ context }) => {
  // ⚠️ DEVIATION from the design's own sketch, found by running it: a page.evaluate(() =>
  // fetch(url)) from an /app page never reaches this guard at all, because the page's own
  // Content-Security-Policy (connect-src 'self' — cmd/armband/webroutes.go's
  // connectSrcFor/withGA4) rejects the fetch in the RENDERER before it is ever dispatched to
  // the network layer Playwright's context.route hooks into. That proved something real (the
  // page's own CSP is itself a second, independent guard against exactly this), but it meant
  // the positive control for THIS suite's guard never fired, and the two blocked CSP console
  // messages then failed the console guard on top of that.
  //
  // A second page in the SAME context sidesteps this cleanly: a top-level navigation is not
  // subject to the document's connect-src, context.route still applies to every page the
  // context opens, and this page is never wrapped by the `page` fixture (only `context` is
  // requested above), so there is no console-guard/afterEach entanglement to work around.
  const target = 'https://example.invalid/does-not-matter';
  const probe = await context.newPage();
  await probe.goto(target).catch(() => {});
  await expect
    .poll(() => context.__blockedRequests.includes(target))
    .toBeTruthy();
  await probe.close();

  // Consumed here so fixtures.js's afterEach — which fails any OTHER spec that leaves this
  // list non-empty — sees it empty for this spec too.
  context.__blockedRequests.length = 0;
});

test('the landing page email form targets the live site, and this suite never submits it', async ({
  page,
}) => {
  // /about is the marketing document (routeAbout in cmd/armband/webroutes.go) and carries
  // gate.js's landing configuration: an ABSOLUTE https://fplarmband.com/gate target, on
  // purpose, so a signup against a local copy of this binary lands in the live list rather
  // than nowhere — see gate.js's own doc comment. /app's in-product nudge forms carry the
  // RELATIVE /gate instead and are same-origin, so they are not what this test is about.
  await page.goto('/about');
  const form = page.locator('form.gatecard[data-gate]').first();
  const target = await form.getAttribute('data-gate');
  expect(target).toBe('https://fplarmband.com/gate');

  // Structural, not a matter of this spec's own discipline: fixtures.js's egress guard
  // would abort a real submission to that origin and fail this test's afterEach. This
  // spec deliberately never fills in the email input or clicks submit — a form target
  // pointing off-origin is not itself a violation, only sending to it would be.
});
