import { test, expect, Page } from '@playwright/test';

// Scenarios from e2e/feat/request-progress.feature
//
// The progress bar is client-only ephemeral UI: it has no server-rendered state
// and no database representation, so the suite's usual "seed the DB" path cannot
// position it. The only way to observe its transient in-flight behaviour
// deterministically is to shape a real request's timing or transport — hold a
// genuine request open long enough to see the bar, or abort it, and confirm the
// bar clears either way. That is the one sanctioned use of `page.route(...)` in
// this suite: it shapes TIMING/TRANSPORT of otherwise-real requests only — it
// fabricates no data and no responses. The two moves used here both inject zero
// data: `slow()` then `route.continue()` (the REAL server still handles the
// request; latency only), and `route.abort()` (a real transport outcome). No
// response is ever synthesized — a non-success HTTP settle is exercised against a
// genuine server 404, not a fabricated status or body.
//
// The bar's in-flight counter decrements on each request's own XHR `loadend`,
// which the browser fires uniformly for a success, an HTTP error response, and an
// abort — so all three settle outcomes share a single hide path.

const slow = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

// htmx attaches itself to `window`; the vendored script ships no types.
type HtmxWindow = Window & {
  htmx: { ajax(method: string, url: string, target: string): Promise<unknown> };
};

function bar(page: Page) {
  return page.getByTestId('request-progress-bar');
}

// A boosted navigation to /transactions is the request we drive: it is always
// available (no bank connection required) and always renders the transactions
// page, so a successful run is observable.

test('No progress bar is shown while the app is idle', async ({ page }) => {
  await page.goto('/');
  // The navbar marks a settled, authenticated page; nothing is in flight.
  await expect(page.getByTestId('app-navbar')).toBeVisible();
  await expect(bar(page)).toBeHidden();
});

test('Starting a request reveals the progress bar at the top of the page', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  // Hold the navigation open so the in-flight state is observable.
  await page.route('**/transactions', async (route) => {
    await slow(900);
    await route.continue();
  });

  await page.getByTestId('nav-transactions').click();
  await expect(bar(page)).toBeVisible();
});

test('The progress bar appears for an ordinary fast request with no injected delay', async ({
  page,
}) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  // No route shaping at all — a genuine, fast boosted navigation. The bar must
  // still appear: it shows the instant the request starts and is held for a brief
  // minimum window, so even a sub-100ms response reads as a sweep. A reveal delay
  // longer than the response would suppress the bar entirely — the regression this
  // guards against, which a delay-injecting test cannot catch.
  await page.getByTestId('nav-transactions').click();
  await expect(bar(page)).toBeVisible();

  // It still settles back to hidden once the page lands and the window passes.
  await expect(page.getByTestId('transactions-page')).toBeVisible();
  await expect(bar(page)).toBeHidden();
});

test('The progress bar hides after a request succeeds', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  await page.route('**/transactions', async (route) => {
    await slow(700);
    await route.continue();
  });

  await page.getByTestId('nav-transactions').click();
  await expect(bar(page)).toBeVisible();

  // The boosted navigation lands its page (success), and the bar clears.
  await expect(page.getByTestId('transactions-page')).toBeVisible();
  await expect(bar(page)).toBeHidden();
});

test('The progress bar hides after a request gets an error response', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  // Drive a REAL request the real server answers non-2xx: the bogus path matches
  // no route, so the real mux returns its real 404. We only delay it (continue,
  // never fulfill) so the in-flight state is observable; the response and status
  // are the server's own. The bar's decrement is bound to the XHR `loadend`,
  // which fires for an HTTP error just as for a success, so the bar must clear.
  await page.route('**/__no-such-route', async (route) => {
    await slow(700);
    await route.continue();
  });

  const settled = page.waitForResponse((r) => r.url().endsWith('/__no-such-route'));

  // htmx is global; fire-and-forget (don't await the ajax promise) so the bar is
  // observable mid-flight.
  await page.evaluate(() => {
    (window as unknown as HtmxWindow).htmx.ajax('GET', '/__no-such-route', 'body');
  });
  await expect(bar(page)).toBeVisible();

  const response = await settled;
  expect(response.status(), 'the bogus path must genuinely 404 against the real server').toBe(404);
  await expect(bar(page)).toBeHidden();
});

test('The progress bar hides after a request is aborted', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  // Hold it open, then abort the request mid-flight: the bar must still clear.
  await page.route('**/transactions', async (route) => {
    await slow(700);
    await route.abort('aborted');
  });

  await page.getByTestId('nav-transactions').click();
  await expect(bar(page)).toBeVisible();
  await expect(bar(page)).toBeHidden();
});

test('The progress bar stays visible until the last of several overlapping requests settles', async ({
  page,
}) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  // Two concurrent boosted navigations with different durations: budget settles
  // first, transactions lingers. The bar must outlast the shorter one.
  await page.route('**/transactions', async (route) => {
    await slow(1400);
    await route.continue();
  });
  await page.route('**/budget', async (route) => {
    await slow(400);
    await route.continue();
  });

  const longSettled = page.waitForResponse((r) => r.url().endsWith('/transactions'));
  const shortSettled = page.waitForResponse((r) => r.url().endsWith('/budget'));

  await page.getByTestId('nav-transactions').click();
  await page.getByTestId('nav-budget').click();

  // Both in flight.
  await expect(bar(page)).toBeVisible();

  // The shorter request settles; the longer is still in flight, so the bar stays.
  await shortSettled;
  await expect(bar(page)).toBeVisible();

  // The last request settles; only now does the bar clear.
  await longSettled;
  await expect(bar(page)).toBeHidden();
});

test('Every page shows exactly one progress bar that survives boosted navigation', async ({
  page,
}) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();
  await expect(bar(page)).toHaveCount(1);

  // Boosted navigation morphs the body. Walk across pages and confirm the bar is
  // never duplicated.
  await page.getByTestId('nav-transactions').click();
  await expect(page.getByTestId('transactions-page')).toBeVisible();
  await expect(bar(page)).toHaveCount(1);

  await page.getByTestId('nav-budget').click();
  await expect(page.getByTestId('budget-page')).toBeVisible();
  await expect(bar(page)).toHaveCount(1);

  await page.getByTestId('nav-spending').click();
  await expect(page.getByTestId('tracker-page')).toBeVisible();
  await expect(bar(page)).toHaveCount(1);

  // The driver (counter + listeners) survived the morphs: a fresh in-flight
  // request still reveals the single bar, and it still clears afterwards — not
  // stuck, not duplicated.
  await page.route('**/transactions', async (route) => {
    await slow(700);
    await route.continue();
  });
  await page.getByTestId('nav-transactions').click();
  await expect(bar(page)).toBeVisible();
  await expect(bar(page)).toHaveCount(1);
  await expect(page.getByTestId('transactions-page')).toBeVisible();
  await expect(bar(page)).toBeHidden();
});

test('The progress bar overlays the top edge without colliding with the bottom navigation', async ({
  page,
}) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  await page.route('**/transactions', async (route) => {
    await slow(900);
    await route.continue();
  });

  await page.getByTestId('nav-transactions').click();
  await expect(bar(page)).toBeVisible();

  const barBox = await bar(page).boundingBox();
  const navBox = await page.getByTestId('app-navbar').boundingBox();
  expect(barBox, 'progress bar must have a measurable box while visible').toBeTruthy();
  expect(navBox, 'bottom navigation must have a measurable box').toBeTruthy();

  // Pinned to the very top edge.
  expect(barBox!.y).toBeLessThanOrEqual(1);
  // A thin strip, not a panel that displaces content.
  expect(barBox!.height).toBeLessThanOrEqual(8);
  // Clears the fixed bottom navigation entirely — no collision.
  expect(barBox!.y + barBox!.height).toBeLessThanOrEqual(navBox!.y);
});
