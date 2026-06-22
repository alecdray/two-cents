import { test, expect, Page } from '@playwright/test';
import { resetActivity, resetBudget, resetCategorization } from '../helpers/db';

// Scenarios from e2e/feat/spend-drill.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). Linking the bank backfills
// the fixed transaction set (see tracker.spec.ts), all dated in the current month:
// Whole Foods $84.32 -> General Merchandise; Blue Bottle Coffee $5.75 (pending) ->
// Food & Drink; a $2,400 paycheck; a $500 savings contribution; a $150 side-gig
// (needs-review). So the wrap's spend-by-Category is General Merchandise $84.32 and
// Food & Drink $5.75 — the two figures the drill reconciles to.

async function linkBankFromAccounts(page: Page) {
  await page.goto('/accounts');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

// openCurrentWrap navigates the wraps list into the current (most-recent) month's
// wrap, the source of the Category figures the drill is reached from.
async function openCurrentWrap(page: Page) {
  await page.goto('/wraps');
  await expect(page.getByTestId('wraps-page')).toBeVisible();
  await page.getByTestId('wrap-row').first().click();
  await expect(page.getByTestId('wrap-page')).toBeVisible();
}

test('Drilling a wrap category lists the transactions making up its total', async ({ page }) => {
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);
  await openCurrentWrap(page);

  // Drill the General Merchandise figure ($84.32, the single Whole Foods charge).
  await page.getByTestId('wrap-category-row').filter({ hasText: 'General Merchandise' }).click();
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

test('Re-categorizing a drilled transaction out of the bucket updates the list and net total', async ({
  page,
}) => {
  resetActivity();
  resetCategorization();
  await linkBankFromAccounts(page);
  await openCurrentWrap(page);

  // Reach the General Merchandise drill via a full load (not a boosted click):
  // htmx form interaction after an hx-boost swap is unreliable under headless
  // automation, and how we arrive is incidental to what this scenario tests.
  const drillHref = await page
    .getByTestId('wrap-category-row')
    .filter({ hasText: 'General Merchandise' })
    .getAttribute('href');
  expect(drillHref, 'General Merchandise row should link to its drill').toBeTruthy();
  await page.goto(drillHref!);
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
