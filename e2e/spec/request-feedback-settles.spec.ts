import { test, expect, Page } from '@playwright/test';
import { resetActivity, resetCategorization } from '../helpers/db';

// Scenarios from e2e/feat/request-feedback-settles.feature
//
// The top progress bar is app-wide chrome driven by the aggregate HTMX request
// lifecycle. The whole-product invariant exercised here is the settled-state
// (idle) one: after any real interaction finishes, the bar returns to hidden and
// is never left visible while the page sits idle. These are idle assertions on
// genuinely real flows, so they need no request shaping — drive the actual flow,
// wait on the DOM signal it lands, then assert the bar is hidden. `toBeHidden`
// auto-retries, so for a flow that fans out into several requests it naturally
// holds until the last one settles.
//
// The app runs in fake-bank mode (BANK_PROVIDER=fake): linking the bank backfills
// a fixed transaction set on the connect, so the sync and edit flows have real
// rows to act on.

function bar(page: Page) {
  return page.getByTestId('request-progress-bar');
}

// linkBankFromOverview links the fake bank from the overview and waits for the
// in-place swap to settle (the cash group appearing), by which point the connect
// handler has also backfilled the bank's transactions. Mirrors transactions.spec.ts.
async function linkBankFromOverview(page: Page) {
  await page.goto('/accounts');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

// openTransactions navigates to the activity surface via the navbar and waits for
// the backfilled list to land.
async function openTransactions(page: Page) {
  await page.getByTestId('nav-transactions').click();
  await expect(page.getByTestId('transactions-list')).toBeVisible();
}

// rowByMerchant refines the row testid locator by its contained merchant text, so
// an assertion targets a stable row regardless of the canned set's order.
function rowByMerchant(page: Page, merchant: string) {
  return page.getByTestId('transactions-row').filter({ hasText: merchant });
}

test('After a manual sync, the progress bar is hidden', async ({ page }) => {
  resetActivity();
  await linkBankFromOverview(page);
  await openTransactions(page);

  // Run a real Sync now against the fake bank.
  await page.getByTestId('transactions-sync').click();

  // The swap lands the sync confirmation into the region it refreshed — the DOM
  // signal that the request settled.
  await expect(page.getByTestId('transactions-sync-confirmation')).toBeVisible();

  // The flow is over; the bar must have returned to hidden.
  await expect(bar(page)).toBeHidden();
});

test('After a boosted navigation, the progress bar is hidden', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  // Drive a real boosted navigation via a nav control.
  await page.getByTestId('nav-budget').click();

  // The destination page renders — the DOM signal the navigation settled.
  await expect(page.getByTestId('budget-page')).toBeVisible();

  // With the page idle on its new surface, the bar is hidden.
  await expect(bar(page)).toBeHidden();
});

test('After editing a transaction through the modal, the progress bar is hidden', async ({
  page,
}) => {
  resetActivity();
  resetCategorization();
  await linkBankFromOverview(page);
  await openTransactions(page);

  // Open the shared editor on a spending row that starts classified Spending.
  const wholeFoods = rowByMerchant(page, 'Whole Foods');
  await expect(wholeFoods.getByTestId('txn-classification')).toHaveText('Spending');
  await wholeFoods.click();
  await expect(page.getByTestId('transaction-editor')).toBeVisible();

  // Make a real change and save. Saving announces transaction-changed, so the list
  // row self-refreshes — the save fans out into several settling requests.
  await page.getByTestId('txn-categorize-classification').selectOption('transfer');
  await page.getByTestId('txn-edit-submit').click();

  // The self-refresh lands: the row now reads Transfer — the post-save DOM signal.
  await expect(rowByMerchant(page, 'Whole Foods').getByTestId('txn-classification')).toHaveText(
    'Transfer',
  );

  // Only once every fanned-out request has settled is the bar hidden; the
  // auto-retrying assertion holds until the last one lands.
  await expect(bar(page)).toBeHidden();
});
