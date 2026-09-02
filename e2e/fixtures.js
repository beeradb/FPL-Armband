// The shared test fixture: the auth cookie, the egress guard and the console guard. Every
// spec imports `test`/`expect` from here rather than from '@playwright/test' directly.
const base = require('@playwright/test');
const fs = require('fs');
const path = require('path');

const TOKEN_FILE = path.join(__dirname, '.token');

/** The write token serve-fixture.mjs lifted off the server's own stderr. */
function readToken() {
  if (!fs.existsSync(TOKEN_FILE)) {
    throw new Error(
      `${TOKEN_FILE} does not exist — serve-fixture.mjs never saw cmdServe's token line. ` +
        'Run the suite through `npm test`, which starts the server itself; a manually ' +
        'started `armband serve` was never wired to write this file.',
    );
  }
  return fs.readFileSync(TOKEN_FILE, 'utf8').trim();
}

const test = base.test.extend({
  // Installs the fpl_auth cookie (cmd/armband/webroutes.go's authCookieName) before any
  // navigation, so no spec has to remember to open the printed `?t=` URL — see that file's
  // own comment on why the token is handed out only in exchange for itself. Using the
  // cookie fixture is equivalent to visiting the `?t=` URL once and keeps every spec's first
  // navigation looking like an ordinary page load.
  context: async ({ context, baseURL }, use) => {
    const token = readToken();
    const url = new URL(baseURL);
    await context.addCookies([
      {
        name: 'fpl_auth',
        value: token,
        domain: url.hostname,
        path: '/',
        httpOnly: true,
        sameSite: 'Strict',
      },
    ]);

    // The egress guard. Anything whose origin is not baseURL is aborted and recorded — this
    // is what makes "never post the landing page's email form to the live site" structural
    // rather than a matter of every spec remembering not to click it. See
    // egress-guard.spec.js for the positive control that proves this actually fires.
    const blocked = [];
    await context.route('**/*', (route) => {
      const reqURL = route.request().url();
      if (reqURL.startsWith(baseURL)) {
        route.continue();
        return;
      }
      blocked.push(reqURL);
      route.abort();
    });
    context.__blockedRequests = blocked;

    await use(context);
  },

  page: async ({ page, context }, use) => {
    // The console guard: any page error or console.error fails the test. Collected rather
    // than asserted immediately, so a spec's own failure message is not shadowed by the
    // first console line and a test that expects an error (there are none today) has
    // somewhere to override this.
    const consoleErrors = [];
    page.on('pageerror', (err) => consoleErrors.push(`pageerror: ${err.message}`));
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(`console.error: ${msg.text()}`);
    });

    // One EXPECTED, structural console error: opening the player sheet always fetches
    // /api/player/{code} (playerdetail.go), which the server answers 502 (StatusBadGateway
    // in that file) whenever the upstream per-player history fetch fails — and it always
    // fails here, on purpose. capture.Endpoints (internal/capture/capture.go) deliberately
    // excludes element-summary/{id}: "500-odd requests for per-player history... this
    // capture already implies" it, so this suite never primes it, and serve-fixture.mjs's
    // dead-proxy env vars mean the live fetch behind it cannot succeed either. That is a
    // real, deliberate gap in what this fixture can reach (see e2e/README.md), not a page
    // defect, so it must not fail the console guard. The browser's own auto-logged message
    // for a failed resource carries no URL in msg.text() to filter on directly, so this
    // listens for the matching 502 response instead and drops exactly one generic message
    // per one it sees — a real, unrelated 502 elsewhere still fails the guard.
    let expectedPlayerDetail502s = 0;
    page.on('response', (res) => {
      if (res.status() === 502 && new URL(res.url()).pathname.startsWith('/api/player/')) {
        expectedPlayerDetail502s++;
      }
    });

    await use(page);

    const GENERIC_502 = 'Failed to load resource: the server responded with a status of 502 (Bad Gateway)';
    const filtered = [];
    for (const e of consoleErrors) {
      if (e === `console.error: ${GENERIC_502}` && expectedPlayerDetail502s > 0) {
        expectedPlayerDetail502s--;
        continue;
      }
      filtered.push(e);
    }
    consoleErrors.length = 0;
    consoleErrors.push(...filtered);
    page.__consoleErrors = consoleErrors;

    if (consoleErrors.length > 0) {
      throw new Error(
        `${consoleErrors.length} console error(s)/page error(s) during this test:\n` +
          consoleErrors.map((e) => `  - ${e}`).join('\n'),
      );
    }
    // egress-guard.spec.js's positive control deliberately triggers a blocked request, then
    // asserts against context.__blockedRequests and empties it (`blocked.length = 0`) —
    // arrays are shared by reference, so that consumption is visible here too. Every OTHER
    // spec never touches the array, so it fails here instead, naming every URL.
    const blocked = context.__blockedRequests || [];
    if (blocked.length > 0) {
      throw new Error(
        `${blocked.length} request(s) left baseURL and were blocked by the egress guard ` +
          `unexpectedly — a spec must never trigger an external request unless it is the ` +
          `positive control in egress-guard.spec.js, which consumes this list itself:\n` +
          blocked.map((u) => `  - ${u}`).join('\n'),
      );
    }
  },
});

module.exports = { test, expect: base.expect, readToken, TOKEN_FILE };
