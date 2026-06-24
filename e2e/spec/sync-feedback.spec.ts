import { test, expect, Page } from '@playwright/test';
import { resetActivity } from '../helpers/db';

// Scenarios from e2e/feat/sync-feedback.feature
//
// The "Sync now" control owns its own action-result feedback (ADR-0015): an
// in-flight disabled/working state and a transient success confirmation. Both are
// observed against the real fake-bank backend.
//
// The in-flight state is only observable while a request is open, and the fake
// sync settles almost instantly. So this spec uses the suite's one sanctioned
// `page.route(...)`: it shapes the real sync request's TIMING only — hold it open
// with a delay, then `route.continue()` so the REAL server still handles it. It
// fabricates no data and no response (no `route.fulfill`), matching the policy the
// request-progress spec follows. The confirmation/auto-clear is observed against a
// fully real, un-shaped sync.

const slow = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

// linkBankFromOverview links the fake bank from the overview and waits for the
// in-place swap to settle (the cash group appearing), by which point the connect
// handler has also backfilled the bank's transactions. Mirrors transactions.spec.ts.
async function linkBankFromOverview(page: Page) {
  await page.goto('/accounts');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

// openTransactions navigates to the activity surface via the navbar (not a direct
// goto) and waits for the backfilled list to land.
async function openTransactions(page: Page) {
  await page.getByTestId('nav-transactions').click();
  await expect(page.getByTestId('transactions-page')).toBeVisible();
  await expect(page.getByTestId('transactions-row')).toHaveCount(6);
}

test('The Sync-now control is disabled and working while a sync is in flight', async ({ page }) => {
  resetActivity();
  await linkBankFromOverview(page);
  await openTransactions(page);

  const sync = page.getByTestId('transactions-sync');
  await expect(sync).toBeEnabled();

  // Hold the real sync request open so the in-flight state is observable; the real
  // server still handles it on continue (timing only — no fabricated response).
  await page.route('**/transactions/sync', async (route) => {
    await slow(900);
    await route.continue();
  });

  await sync.click();
  // While the request is in flight htmx disables the control — a real DOM signal,
  // and the reason a second activation can't fire.
  await expect(sync).toBeDisabled();

  // Once the swap lands the control is the fresh, interactive one again.
  await expect(page.getByTestId('transactions-sync')).toBeEnabled();
});

test('A successful sync shows a transient confirmation that then clears itself', async ({ page }) => {
  resetActivity();
  await linkBankFromOverview(page);
  await openTransactions(page);

  await page.getByTestId('transactions-sync').click();

  // The handler renders the confirmation into the region the sync swapped; it
  // appears once the swap lands.
  const confirmation = page.getByTestId('transactions-sync-confirmation');
  await expect(confirmation).toBeVisible();

  // Showing it neither reloaded nor displaced the list.
  await expect(page.getByTestId('transactions-list')).toBeVisible();
  await expect(page.getByTestId('transactions-row')).toHaveCount(6);

  // It auto-clears on its own client-side timer (Alpine x-show) — no manual
  // dismissal. The auto-retrying expect is a DOM-signal wait, not a fixed sleep.
  await expect(confirmation).toBeHidden();

  // The list stayed put across the confirmation's whole lifecycle.
  await expect(page.getByTestId('transactions-list')).toBeVisible();
  await expect(page.getByTestId('transactions-row')).toHaveCount(6);
});
