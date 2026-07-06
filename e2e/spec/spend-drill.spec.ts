import { test, expect, Page } from '@playwright/test';
import { resetActivity, resetBudget, resetCategorization, currentMonthYM } from '../helpers/db';

// Scenarios from e2e/feat/spend-drill.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). Linking the bank backfills
// the fixed transaction set (see tracker.spec.ts), all dated in the current month:
// Whole Foods $84.32 -> General Merchandise; Blue Bottle Coffee $5.75 (pending) ->
// Food & Drink; a $2,400 paycheck; a $500 savings contribution; a $150 side-gig
// (needs-review). So the wrap's spend-by-Category is General Merchandise $84.32 and
// Food & Drink $5.75 — the two figures the drill reconciles to.
//
// The current month has no wrap page (its wrap address redirects to the Tracker),
// but its drill routes /wraps/{currentYM}/spend/{bucket} are NOT redirected — a
// current-month figure still drills. So the drills below are reached by their
// canonical URL for the current month rather than through a (now-absent) wrap page.

async function linkBankFromAccounts(page: Page) {
  await page.goto('/accounts');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

// drillURL builds the current month's drill route for a bucket (a Category id, or
// income/savings/everything-else) — the address the corresponding figure links to.
function drillURL(bucket: string): string {
  return `/wraps/${currentMonthYM()}/spend/${bucket}`;
}

test('Drilling a wrap category lists the transactions making up its total', async ({ page }) => {
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);

  // Drill the General Merchandise figure ($84.32, the single Whole Foods charge).
  await page.goto(drillURL('general_merchandise'));
  await expect(page.getByTestId('spend-drill-page')).toBeVisible();

  await expect(page.getByTestId('spend-drill-label')).toHaveText('General Merchandise');
  // The net total equals the wrap figure it was reached from.
  await expect(page.getByTestId('spend-drill-total')).toHaveText('$84.32');
  // The list is exactly the transactions making up that total.
  await expect(page.getByTestId('spend-drill-row')).toHaveCount(1);
  await expect(page.getByTestId('spend-drill-row-amount')).toHaveText('$84.32');
  // The back-link returns to the month's wrap.
  await expect(page.getByTestId('spend-drill-back')).toBeVisible();
});

test("Drilling the wrap's Income figure lists the month's income", async ({ page }) => {
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);

  await page.goto(drillURL('income'));
  await expect(page.getByTestId('spend-drill-page')).toBeVisible();
  await expect(page.getByTestId('spend-drill-label')).toHaveText('Income');
  // Gross income is the single $2,400 paycheck (the side-gig inflow is needs-review,
  // not income), and the income row is oriented positive so it sums to the total.
  await expect(page.getByTestId('spend-drill-total')).toHaveText('$2,400.00');
  await expect(page.getByTestId('spend-drill-row')).toHaveCount(1);
});

test("Drilling the wrap's Savings figure lists the savings contributions", async ({ page }) => {
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);

  await page.goto(drillURL('savings'));
  await expect(page.getByTestId('spend-drill-page')).toBeVisible();
  await expect(page.getByTestId('spend-drill-label')).toHaveText('Savings contributed');
  // The $500 source leg only (the mirror inflow is never counted).
  await expect(page.getByTestId('spend-drill-total')).toHaveText('$500.00');
  await expect(page.getByTestId('spend-drill-row')).toHaveCount(1);
});

test('Re-categorizing a drilled transaction out of the bucket updates the list and net total', async ({
  page,
}) => {
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);

  // Reach the General Merchandise drill via a full load (not a boosted click):
  // htmx form interaction after an hx-boost swap is unreliable under headless
  // automation, and how we arrive is incidental to what this scenario tests.
  await page.goto(drillURL('general_merchandise'));
  await expect(page.getByTestId('spend-drill-page')).toBeVisible();
  await expect(page.getByTestId('spend-drill-row')).toHaveCount(1);

  // Re-categorize the one row as Income from the shared editing modal — it leaves the
  // Spending bucket entirely. Saving announces transaction-changed, so the drill
  // region self-refreshes: the row drops and the net total zeroes.
  await page.getByTestId('spend-drill-row').click();
  await expect(page.getByTestId('transaction-editor')).toBeVisible();
  await page.getByTestId('txn-categorize-classification').selectOption('income');
  await page.getByTestId('txn-edit-submit').click();

  await expect(page.getByTestId('spend-drill-empty')).toBeVisible();
  await expect(page.getByTestId('spend-drill-row')).toHaveCount(0);
  await expect(page.getByTestId('spend-drill-total')).toHaveText('$0.00');
});

test('Drilling the Tracker’s everything else lists the residual spend', async ({ page }) => {
  resetActivity();
  resetCategorization();
  resetBudget();
  await linkBankFromAccounts(page);

  // Budget only General Merchandise, leaving the $5.75 Food & Drink coffee
  // unbudgeted — so it falls into the everything-else residual.
  await page.goto('/budget');
  await expect(page.getByTestId('budget-page')).toBeVisible();
  await page.getByTestId('budget-income').fill('3000');
  await page.getByTestId('budget-savings').fill('1000');
  await page.getByTestId('budget-add-category').selectOption({ label: 'General Merchandise' });
  await page
    .getByTestId('budget-limit-row')
    .filter({ hasText: 'General Merchandise' })
    .getByRole('spinbutton')
    .fill('50');
  const saved = page.waitForResponse(
    (r) => r.url().includes('/budget') && r.request().method() === 'POST',
  );
  await page.getByTestId('budget-save').click();
  await saved;

  // The Tracker's everything-else line drills into the unbudgeted residual spend.
  await page.goto('/');
  await expect(page.getByTestId('tracker-page')).toBeVisible();
  await page.getByTestId('tracker-everything-else').click();

  await expect(page.getByTestId('spend-drill-page')).toBeVisible();
  await expect(page.getByTestId('spend-drill-label')).toHaveText('Everything else');
  // The residual is exactly the unbudgeted Food & Drink coffee ($5.75).
  await expect(page.getByTestId('spend-drill-total')).toHaveText('$5.75');
  await expect(page.getByTestId('spend-drill-row')).toHaveCount(1);
  await expect(page.getByTestId('spend-drill-row-merchant')).toHaveText('Blue Bottle Coffee');
});
