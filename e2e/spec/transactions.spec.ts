import { test, expect, Page } from '@playwright/test';
import { resetActivity, seedConnectionWithoutActivity } from '../helpers/db';

// Scenarios from e2e/feat/transactions.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). Linking the bank posts
// directly and the deterministic stand-in backfills a fixed transaction set on
// the connect, spanning its fixed accounts and the categorization ladder:
//   - Whole Foods            outflow on Everyday Checking,    posted  -> -$84.32
//   - Acme Payroll           inflow  on Everyday Checking,    posted  -> +$2,400.00
//   - Blue Bottle Coffee     outflow on Travel Rewards Card,  pending -> -$5.75
//   - Rainy Day Savings      outflow on Everyday Checking,    posted  -> -$500.00 (transfer)
//   - Transfer from Checking inflow  on High-Yield Savings,   posted  -> +$500.00 (transfer mirror)
//   - Side Hustle Co         inflow  on Everyday Checking,    posted  -> +$150.00 (needs review)
// Amounts are stored with the seam's accounting sign (outflow positive, inflow
// negative); the page inverts them so spending reads negative and income positive.

// linkBankFromOverview links the fake bank from the overview and waits for the
// in-place swap to settle (the cash group appearing), by which point the connect
// handler has also backfilled the bank's transactions.
async function linkBankFromOverview(page: Page) {
  await page.goto('/');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

test("A connected bank's transactions appear with account names, signed amounts, and a pending marker", async ({
  page,
}) => {
  resetActivity();
  await linkBankFromOverview(page);

  // Navigate to the activity surface via the navbar, not a direct goto.
  await page.getByTestId('nav-transactions').click();
  await expect(page.getByTestId('transactions-page')).toBeVisible();

  const list = page.getByTestId('transactions-list');
  await expect(list).toBeVisible();
  await expect(page.getByTestId('transactions-row')).toHaveCount(6);

  // Account names render on the rows.
  await expect(list).toContainText('Everyday Checking');
  await expect(list).toContainText('Travel Rewards Card');

  // Display sign: spending negative, income positive.
  await expect(list).toContainText('-$84.32'); // outflow
  await expect(list).toContainText('+$2,400.00'); // inflow
  await expect(list).toContainText('-$5.75'); // pending outflow
  await expect(list).toContainText('-$500.00'); // transfer outflow
  await expect(list).toContainText('+$150.00'); // needs-review inflow

  // Exactly the unposted charge is marked pending.
  const pending = page.getByTestId('transactions-row-pending');
  await expect(pending).toHaveCount(1);
  await expect(pending).toBeVisible();
});

test('Re-syncing a connected bank does not duplicate its transactions', async ({ page }) => {
  resetActivity();
  await linkBankFromOverview(page);

  await page.getByTestId('nav-transactions').click();
  await expect(page.getByTestId('transactions-row')).toHaveCount(6);

  // Sync the same unchanged bank state again; wait for the swap to complete.
  const synced = page.waitForResponse(
    (r) => r.url().includes('/transactions/sync') && r.request().method() === 'POST',
  );
  await page.getByTestId('transactions-sync').click();
  await synced;

  // The list is idempotent: the same rows remain, no duplicates appear.
  await expect(page.getByTestId('transactions-list')).toBeVisible();
  await expect(page.getByTestId('transactions-row')).toHaveCount(6);
  await expect(page.getByTestId('transactions-row-pending')).toHaveCount(1);
});

test('The page prompts to connect a bank when none is connected', async ({ page }) => {
  resetActivity();

  await page.goto('/transactions');
  await expect(page.getByTestId('transactions-page')).toBeVisible();

  // The no-connections empty state stands in for the list and the sync control.
  await expect(page.getByTestId('transactions-empty-no-connections')).toBeVisible();
  await expect(page.getByTestId('transactions-list')).toHaveCount(0);
  await expect(page.getByTestId('transactions-sync')).toHaveCount(0);
});

test('A connected bank with nothing synced offers to sync', async ({ page }) => {
  seedConnectionWithoutActivity();

  await page.goto('/transactions');
  await expect(page.getByTestId('transactions-page')).toBeVisible();

  // With a bank connected but nothing pulled, the nothing-synced prompt shows
  // alongside the sync control, and no list is rendered.
  await expect(page.getByTestId('transactions-empty-no-transactions')).toBeVisible();
  await expect(page.getByTestId('transactions-sync')).toBeVisible();
  await expect(page.getByTestId('transactions-list')).toHaveCount(0);
  await expect(page.getByTestId('transactions-empty-no-connections')).toHaveCount(0);
});
