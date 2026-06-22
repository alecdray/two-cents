import { test, expect, Page } from '@playwright/test';
import { resetActivity, seedUnpairedTransfer } from '../helpers/db';

// Scenarios from e2e/feat/transfer-destination.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). The deterministic stand-in
// backfills, among others, a $500 outflow transfer out of checking ("Rainy Day
// Savings") and its matching −$500 inflow on the counts-as-savings savings
// account ("Transfer from Checking"), so the outflow auto-pairs into the savings
// account and resolves to a Savings contribution. Rows are selected by merchant
// (a stable testid-scoped text filter) rather than a positional index.

// linkBankFromOverview links the fake bank from the overview and waits for the
// in-place swap to settle, by which point the connect handler has backfilled,
// auto-categorized, and resolved the transfer destinations of the bank's
// transactions.
async function linkBankFromOverview(page: Page) {
  await page.goto('/accounts');
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
// canned-set reorder can't point an assertion at the wrong row.
function rowByMerchant(page: Page, merchant: string) {
  return page.getByTestId('transactions-row').filter({ hasText: merchant });
}

// openEditor opens a row's shared transaction-editing modal and waits for the
// editor body to render.
async function openEditor(page: Page, row: ReturnType<typeof rowByMerchant>) {
  await row.getByTestId('transactions-row-edit').click();
  await expect(page.getByTestId('transaction-editor')).toBeVisible();
}

// closeEditor dismisses the modal via its close control and waits for the dialog to
// leave the DOM.
async function closeEditor(page: Page) {
  await page.getByTestId('modal').getByRole('button', { name: 'Close' }).click();
  await expect(page.locator('dialog[open]')).toHaveCount(0);
}

test('An auto-paired savings transfer shows a savings contribution chip', async ({ page }) => {
  resetActivity();
  await linkBankFromOverview(page);
  await openTransactions(page);

  // The $500 outflow transfer paired to the counts-as-savings savings account, so
  // its chip reads a Savings contribution naming that account.
  const savingsTransfer = rowByMerchant(page, 'Rainy Day Savings');
  const chip = savingsTransfer.getByTestId('txn-transfer-destination');
  await expect(chip).toBeVisible();
  await expect(chip).toContainText('Savings');
  await expect(chip).toContainText('High-Yield Savings');

  // It is resolved, not flagged unknown.
  await expect(savingsTransfer.getByTestId('txn-destination-unknown')).toHaveCount(0);
});

test("Marking an unknown transfer's destination sticks across a sync", async ({ page }) => {
  resetActivity();
  await linkBankFromOverview(page);

  // Seed an outflow transfer with no matching inflow leg — the auto-pairing pass
  // leaves it destination-unknown.
  seedUnpairedTransfer({
    id: 'e2e-unpaired-transfer',
    merchant: 'Mystery Transfer',
    amount: 321.0,
    date: '2026-06-10 00:00:00',
  });

  await openTransactions(page);

  const mystery = rowByMerchant(page, 'Mystery Transfer');
  await expect(mystery.getByTestId('txn-destination-unknown')).toBeVisible();

  // From the shared modal, mark it a savings contribution into the savings account
  // and save. The save announces transaction-changed, so the list row self-refreshes.
  await openEditor(page, mystery);
  await page.getByTestId('txn-destination-picker-account').selectOption({ label: 'High-Yield Savings' });
  await page.getByTestId('txn-destination-picker-subtype').selectOption('savings_contribution');
  await page.getByTestId('txn-edit-submit').click();

  // The chip updates in place to the savings contribution; the unknown flag clears.
  await expect(rowByMerchant(page, 'Mystery Transfer').getByTestId('txn-transfer-destination')).toContainText(
    'Savings',
  );
  await expect(rowByMerchant(page, 'Mystery Transfer').getByTestId('txn-destination-unknown')).toHaveCount(0);
  await closeEditor(page);

  // Sync the same bank state; the manual mark must persist (the auto pass skips an
  // overridden transfer facet).
  const synced = page.waitForResponse(
    (r) => r.url().includes('/transactions/sync') && r.request().method() === 'POST',
  );
  await page.getByTestId('transactions-sync').click();
  await synced;

  await expect(page.getByTestId('transactions-list')).toBeVisible();
  await expect(rowByMerchant(page, 'Mystery Transfer').getByTestId('txn-transfer-destination')).toContainText(
    'Savings',
  );
  await expect(rowByMerchant(page, 'Mystery Transfer').getByTestId('txn-destination-unknown')).toHaveCount(0);
});

test('A non-transfer row offers no transfer-destination control', async ({ page }) => {
  resetActivity();
  await linkBankFromOverview(page);
  await openTransactions(page);

  // The groceries spending row is not a transfer, so it carries no transfer chip on
  // the row, and its editor keeps the transfer-destination controls hidden unless the
  // row is changed to a Transfer.
  const groceries = rowByMerchant(page, 'Whole Foods');
  await expect(groceries.getByTestId('txn-classification')).toHaveText('Spending');
  await expect(groceries.getByTestId('txn-transfer-destination')).toHaveCount(0);
  await expect(groceries.getByTestId('txn-destination-unknown')).toHaveCount(0);

  await openEditor(page, groceries);
  await expect(page.getByTestId('txn-edit-submit')).toBeVisible();
  await expect(page.getByTestId('txn-destination-picker')).toBeHidden();
});
