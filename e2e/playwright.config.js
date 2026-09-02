// @ts-check
const { defineConfig, devices } = require('@playwright/test');

const PORT = process.env.ARMBAND_E2E_PORT || '8123';
const BASE_URL = `http://127.0.0.1:${PORT}`;

// Where the browser comes from, in two environments that cannot share an answer.
//
// Locally this VM has only the SNAP chromium, and Playwright's own download does not exist
// here — so ARMBAND_E2E_CHROMIUM=/snap/bin/chromium points it at the snap. In CI the runner
// has normal network access and `npx playwright install chromium` puts the revision the
// pinned @playwright/test names under ~/.cache/ms-playwright, which is the one we WANT to be
// judged by: it is the same build on every run and every runner.
//
// A hardcoded /snap/bin/chromium would break CI; a hardcoded bundled path would break here.
// Unset means "Playwright's own", which is the correct default because it is the pinned one.
const EXECUTABLE = process.env.ARMBAND_E2E_CHROMIUM || undefined;

// --no-sandbox is a real (small) loosening and is applied ONLY to the snap.
//
// The snap's own confinement plus an unprivileged-userns restriction makes chromium's
// sandbox unavailable on this box, which is why the manual probe needed these flags. The
// runner's bundled chromium sandboxes fine, and a suite that disables the sandbox
// unconditionally would carry that weakening into CI for no reason. Everything either
// browser ever navigates to is 127.0.0.1 (see fixtures.js's egress guard), so the exposure
// under the snap arm is bounded by that.
const LAUNCH_ARGS = EXECUTABLE ? ['--no-sandbox', '--disable-dev-shm-usage'] : [];

module.exports = defineConfig({
  testDir: './tests',
  globalSetup: require.resolve('./globalSetup.js'),

  // Artefacts land where the NODE process writes them, under the repo, gitignored.
  //
  // ⚠️ Not os.tmpdir(). internal/browsertest's package comment records why: a snap gets a
  // private /tmp, so a path the browser wrote is not a path the host can read. Playwright is
  // mostly immune — a screenshot comes back over the protocol as a buffer and Node writes the
  // file — but traces and videos are large, the rule costs nothing, and encoding it here means
  // nobody has to re-derive which artefacts cross that boundary and which do not.
  outputDir: './test-results',

  // The server holds ONE engine behind ONE mutex, and every /api/state rebuilds the page
  // rather than caching it. Parallel workers would not run in parallel; they would queue on
  // that mutex and then time out. Serial is slower and truthful. It also matches how a person
  // uses this tool, which is one reader.
  workers: 1,
  fullyParallel: false,

  // No retries, in either environment. A retry converts a flake into a pass and this project
  // has a standing objection to a check that can succeed without having run. A test that is
  // flaky here is a bug in the test or in the page, and both are worth seeing.
  retries: 0,
  forbidOnly: !!process.env.CI,

  // Three timeouts, each with a different job, all stated rather than defaulted — the same
  // reasoning browsertest's `deadline` const carries: the job is to turn a WEDGE into a fast
  // named failure, not to catch a slow render, so every one of these is deliberately loose.
  // A page build runs the optimiser (~1.6s at the shipped horizon, more under CI's load).
  timeout: 90_000, // one test
  globalTimeout: 15 * 60_000, // the whole suite; caps a wedged run inside the job's 20 minutes
  expect: { timeout: 15_000 },

  reporter: process.env.CI
    ? [['github'], ['html', { open: 'never' }], ['list']]
    : [['list'], ['html', { open: 'never' }]],

  use: {
    baseURL: BASE_URL,

    // ⚠️ LOAD-BEARING, and the direct analogue of browsertest's TZ=UTC.
    // app.js formats every deadline with toLocaleDateString('en-GB', …). The LOCALE is
    // pinned in the page's own source; the TIMEZONE is the browser's, so a 17:30Z deadline
    // renders as a different DAY west of UTC. Playwright emulates both per context through
    // CDP, which is stronger than an env var: it cannot be lost to a launcher that forgets to
    // pass the environment through.
    timezoneId: 'UTC',
    locale: 'en-GB',

    // Failure artefacts only. This suite asserts nothing about pixels — see
    // internal/webui/visual_test.go's "NOT RUN IN CI" block and the vault ruling behind it
    // ([[machine-dependent-visual-goldens-keep-ci-red]]). These exist so a red run can be
    // READ, not so a picture can be compared.
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',

    actionTimeout: 15_000,
    navigationTimeout: 30_000,
  },

  projects: [
    {
      name: 'desktop',
      use: {
        ...devices['Desktop Chrome'],
        // 1440 is the width every desktop golden in internal/webui/visual_test.go uses
        // (desktopW). Matching it means a finding here and a finding there describe the same
        // layout. 900 is a real laptop height, NOT the goldens' deliberately-tall 1400-2400:
        // those exist to fit a whole page into one image, which is exactly what stops them
        // exercising anything sized against the viewport.
        viewport: { width: 1440, height: 900 },
        deviceScaleFactor: 1,
        launchOptions: { executablePath: EXECUTABLE, args: LAUNCH_ARGS },
      },
    },
    {
      name: 'phone',
      use: {
        ...devices['Desktop Chrome'],
        // 390x844 — mobileW from visual_test.go, at the iPhone 14/15/16 height. That file's
        // own comment says why the height matters and must not be "fixed" by growing it: it
        // is the shortest common device, so it is where a vh clamp binds FIRST, and every
        // `vh`, `position:fixed` and `position:sticky` rule in armband.css is unexercised
        // above it.
        viewport: { width: 390, height: 844 },
        deviceScaleFactor: 1,
        // isMobile/hasTouch rather than a copied user-agent string — Playwright sets the
        // media features directly (coarse pointer / no hover), which is the property the CSS
        // actually reads, rather than a hand-written UA string.
        isMobile: true,
        hasTouch: true,
        launchOptions: { executablePath: EXECUTABLE, args: LAUNCH_ARGS },
      },
    },
  ],

  // The server is started BY Playwright, in both environments, from one command.
  //
  // Deliberately not a CI step that backgrounds the binary and polls with curl. That would be
  // two implementations of "how the e2e server starts" — the local one and the CI one — which
  // is this project's signature failure applied to a harness. Here CI runs exactly what a
  // developer runs, and the readiness probe is the same probe.
  //
  // reuseExistingServer is FALSE even locally, which is not the Playwright default and is not
  // an oversight: `armband serve` mints a RANDOM write token per process and hands it out only
  // in exchange for itself. A reused server is one whose token this harness never saw, so
  // every write test would silently drive the unauthenticated SHELL — a page that renders and
  // asserts nothing. See fixtures.js and tests/smoke-boot.spec.js's token control.
  webServer: {
    command: 'node scripts/serve-fixture.mjs',
    url: `${BASE_URL}/`, // the landing page: static, so readiness costs no optimiser pass
    reuseExistingServer: false,
    timeout: 120_000, // a cold `go build` is not in here, but a cold engine build is
    cwd: __dirname,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
