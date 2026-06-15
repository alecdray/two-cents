import { test, expect, Page } from '@playwright/test';
import { resetActivity, resetCategorization } from '../helpers/db';

// Scenarios from e2e/feat/transaction-categorization.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). The deterministic stand-in
// backfills a fixed set spanning the categorization ladder, newest-first:
//   Side Hustle Co         inflow,  no usable bank category -> needs-review
//   Transfer from Checking inflow,  TRANSFER_IN             -> Transfer (savings mirror)
//   Rainy Day Savings      outflow, TRANSFER_OUT            -> Transfer
//   Blue Bottle Coffee     outflow (pending), FOOD_AND_DRINK -> Spending
//   Acme Payroll           inflow,  INCOME                  -> Income
//   Whole Foods            outflow, GENERAL_MERCHANDISE     -> Spending + General Merchandise
// Rows are selected by merchant (a stable testid-scoped text filter) rather than a
// positional index, so a later canned-set change does not silently shift the rows
// these assertions target.

// linkBankFromOverview links the fake bank from the overview and waits for the
// in-place swap to settle, by which point the connect handler has also backfilled
// (and auto-categorized) the bank's transactions.
async function linkBankFromOverview(page: Page) {
  await page.goto('/');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

// openTransactions navigates to the activity surface via the navbar and waits for
// the list to render.
async function openTransactions(page: Page) {
  await page.getByTestId('nav-transactions').click();
  await expect(page.getByTestId('transactions-list')).toBeVisible();
}

// rowByMerchant selects the single transactions row whose merchant matches, so a
// canned-set reorder can't point an assertion at the wrong row. It refines the
// row testid locator by its contained text rather than a positional index.
function rowByMerchant(page: Page, merchant: string) {
  return page.getByTestId('transactions-row').filter({ hasText: merchant });
}

test('Synced transactions are auto-categorized with chips including a transfer and a needs-review row', async ({
  page,
}) => {
  resetActivity();
  resetCategorization();
  await linkBankFromOverview(page);
  await openTransactions(page);

  await expect(page.getByTestId('transactions-row')).toHaveCount(6);

  // Every row carries a classification chip.
  await expect(page.getByTestId('txn-classification')).toHaveCount(6);

  // The transfer-signal row reads Transfer; the spending row carries its Category.
  await expect(rowByMerchant(page, 'Rainy Day Savings').getByTestId('txn-classification')).toHaveText('Transfer');
  await expect(rowByMerchant(page, 'Whole Foods').getByTestId('txn-category-chip')).toHaveText(
    'General Merchandise',
  );

  // The unclassifiable inflow is flagged needs-review.
  await expect(rowByMerchant(page, 'Side Hustle Co').getByTestId('txn-classification')).toHaveText(
    'Needs review',
  );
  await expect(rowByMerchant(page, 'Side Hustle Co').getByTestId('txn-needs-review')).toBeVisible();
});

test('A manual re-categorization survives a later sync', async ({ page }) => {
  resetActivity();
  resetCategorization();
  await linkBankFromOverview(page);
  await openTransactions(page);

  const wholeFoods = rowByMerchant(page, 'Whole Foods');
  await expect(wholeFoods.getByTestId('txn-classification')).toHaveText('Spending');

  // Re-categorize the spending row as Transfer (which clears its Category).
  // The picker saves on change — selecting a non-Spending outcome posts at once,
  // no submit button.
  const categorized = page.waitForResponse(
    (r) => r.url().includes('/categorize') && r.request().method() === 'POST',
  );
  await wholeFoods.getByTestId('txn-categorize-classification').selectOption('transfer');
  await categorized;

  await expect(rowByMerchant(page, 'Whole Foods').getByTestId('txn-classification')).toHaveText('Transfer');
  await expect(rowByMerchant(page, 'Whole Foods').getByTestId('txn-category-chip')).toHaveCount(0);

  // Sync the same unchanged bank state; the manual choice must persist.
  const synced = page.waitForResponse(
    (r) => r.url().includes('/transactions/sync') && r.request().method() === 'POST',
  );
  await page.getByTestId('transactions-sync').click();
  await synced;

  await expect(page.getByTestId('transactions-list')).toBeVisible();
  await expect(rowByMerchant(page, 'Whole Foods').getByTestId('txn-classification')).toHaveText('Transfer');
});

test('A custom category can be created and archived', async ({ page }) => {
  resetCategorization();
  await page.goto('/categories');
  await expect(page.getByTestId('categories-page')).toBeVisible();

  const active = page.getByTestId('categories-active');
  // The twelve seeded built-ins are the active baseline.
  await expect(active.getByTestId('category-row')).toHaveCount(12);

  // Create a custom category; its name sorts it last in the active list.
  const created = page.waitForResponse(
    (r) => r.url().includes('/categories') && r.request().method() === 'POST',
  );
  await page.getByTestId('category-create-name').fill('Zzz Side Projects');
  await page.getByTestId('category-create-submit').click();
  await created;

  await expect(active.getByTestId('category-row')).toHaveCount(13);

  // Archive the custom category (the last active row) — it moves to archived.
  const archived = page.waitForResponse(
    (r) => r.url().includes('/archive') && r.request().method() === 'POST',
  );
  await active.getByTestId('category-row').last().getByTestId('category-archive').click();
  await archived;

  await expect(page.getByTestId('categories-active').getByTestId('category-row')).toHaveCount(12);
  await expect(page.getByTestId('categories-archived')).toBeVisible();
  await expect(page.getByTestId('categories-archived').getByTestId('category-row')).toHaveCount(1);
});

test('Creating a rule re-categorizes a matching transaction and reports the count', async ({
  page,
}) => {
  resetActivity();
  resetCategorization();
  await linkBankFromOverview(page);
  await openTransactions(page);

  // The Side Hustle inflow starts needs-review.
  await expect(rowByMerchant(page, 'Side Hustle Co').getByTestId('txn-classification')).toHaveText(
    'Needs review',
  );

  // Create a rule on the rules page matching its merchant, classifying it Income.
  await page.getByTestId('nav-rules').click();
  await expect(page.getByTestId('rules-page')).toBeVisible();

  const ruled = page.waitForResponse(
    (r) => r.url().includes('/rules') && r.request().method() === 'POST',
  );
  await page.getByTestId('rule-create-substring').fill('Side Hustle');
  await page.getByTestId('rule-create-classification').selectOption('income');
  await page.getByTestId('rule-create-submit').click();
  await ruled;

  // The rule reports exactly one transaction re-categorized.
  await expect(page.getByTestId('rules-feedback')).toContainText('1 transaction re-categorized.');

  // The matching transaction is now Income on the transactions page.
  await openTransactions(page);
  await expect(rowByMerchant(page, 'Side Hustle Co').getByTestId('txn-classification')).toHaveText('Income');
  await expect(rowByMerchant(page, 'Side Hustle Co').getByTestId('txn-needs-review')).toHaveCount(0);
});
