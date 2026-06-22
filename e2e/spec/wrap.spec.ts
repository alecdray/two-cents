import { test, expect, Page } from '@playwright/test';
import { resetActivity, resetCategorization } from '../helpers/db';

// Scenarios from e2e/feat/wrap.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). Linking the bank backfills
// the fixed transaction set (see tracker.spec.ts for the full set), all dated in
// the current month, including a pending charge (-> settling) and the auto-paired
// $500 savings contribution. The bank is linked this month, so the month is the
// connect month (-> partial).

async function linkBankFromAccounts(page: Page) {
  await page.goto('/accounts');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

test('The wraps list shows the current month and links to its wrap', async ({ page }) => {
  // Reset rules too: a lingering Rule from another spec would re-classify the
  // side-gig inflow on the re-sync and skew the wrap's net income.
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);

  await page.goto('/wraps');
  await expect(page.getByTestId('wraps-page')).toBeVisible();

  // The current month is listed (the wraps list always includes it).
  const rows = page.getByTestId('wrap-row');
  await expect(rows.first()).toBeVisible();

  // Open the most-recent (current) month's wrap.
  await rows.first().click();
  await expect(page.getByTestId('wrap-page')).toBeVisible();

  // Net income = $2,400 income - ($84.32 + $5.75) spending = $2,309.93.
  await expect(page.getByTestId('wrap-net-income')).toContainText('$2,309.93');

  // Savings contributed is the $500 source leg only (the mirror is never counted).
  await expect(page.getByTestId('wrap-savings')).toContainText('$500.00');

  // Gross income is the $2,400 paycheck alone (the drillable income figure; net
  // income is the derived summary above and is not a drill).
  await expect(page.getByTestId('wrap-income')).toContainText('$2,400.00');

  // The inline full-month list shows every transaction in the month (all six rows).
  await expect(page.getByTestId('wrap-month-row')).toHaveCount(6);

  // Spend breaks down by Category: General Merchandise and Food & Drink.
  await expect(page.getByTestId('wrap-category-row')).toHaveCount(2);
  await expect(
    page.getByTestId('wrap-category-row').filter({ hasText: 'General Merchandise' }),
  ).toContainText('$84.32');
  await expect(
    page.getByTestId('wrap-category-row').filter({ hasText: 'Food & Drink' }),
  ).toContainText('$5.75');

  // The pending coffee charge makes the wrap settling.
  await expect(page.getByTestId('wrap-state')).toContainText('Settling');

  // The bank was linked this month, so the month is at the backfill edge.
  await expect(page.getByTestId('wrap-partial')).toBeVisible();
});

test("Editing a transaction from the wrap list refreshes the wrap's figures", async ({ page }) => {
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);

  // Reach the wrap via a full load (not a boosted click) so the modal interaction is
  // reliable under headless automation.
  await page.goto('/wraps');
  const wrapHref = await page.getByTestId('wrap-row').first().getAttribute('href');
  expect(wrapHref, 'the current month should link to its wrap').toBeTruthy();
  await page.goto(wrapHref!);
  await expect(page.getByTestId('wrap-page')).toBeVisible();

  // Gross income starts at the $2,400 paycheck.
  await expect(page.getByTestId('wrap-income')).toContainText('$2,400.00');

  // Re-categorize the needs-review side-gig inflow ($150) to Income from the wrap's
  // list. Saving announces transaction-changed, so the wrap figure region
  // self-refreshes — gross income rises to $2,550.
  await page.getByTestId('wrap-month-row').filter({ hasText: 'Side Hustle Co' }).click();
  await expect(page.getByTestId('transaction-editor')).toBeVisible();
  await page.getByTestId('txn-categorize-classification').selectOption('income');
  await page.getByTestId('txn-edit-submit').click();

  await expect(page.getByTestId('wrap-income')).toContainText('$2,550.00');
});
