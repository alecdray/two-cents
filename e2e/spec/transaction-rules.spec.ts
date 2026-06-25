import { test, expect, Page } from '@playwright/test';
import { seedRules, seedTransactions } from '../helpers/db';

// Scenarios from e2e/feat/transaction-rules.feature
//
// The transaction editor's Rules section ([ADR-0016]) lists the Rules governing a
// row (governing one marked Applied) or, when none match, offers a Create-rule
// control prefilled from the transaction. Both open the shared rule editor modal by
// URL, passing the transaction's own edit endpoint as the return handle, so a Rule
// save or dismiss re-mounts the transaction editor refreshed. State is seeded
// directly into SQLite — there is no public write API. A seeded Rule is matched
// live on editor open by the row's merchant, so it surfaces regardless of the row's
// stored classification.

// rowByMerchant selects the single transactions row whose merchant matches, so the
// editor opens for a known transaction.
function rowByMerchant(page: Page, merchant: string) {
  return page.getByTestId('transactions-row').filter({ hasText: merchant });
}

// openTransactionEditor goes to the activity surface and opens the editor for the
// named merchant's row, waiting for the editor body to render.
async function openTransactionEditor(page: Page, merchant: string) {
  await page.goto('/transactions');
  await expect(page.getByTestId('transactions-list')).toBeVisible();
  await rowByMerchant(page, merchant).click();
  await expect(page.getByTestId('transaction-editor')).toBeVisible();
}

test('Editing a transaction lists the governing rule and opening it returns on save', async ({
  page,
}) => {
  seedTransactions([{ id: 'txn-wf', merchant: 'WHOLEFOODS', amount: 42 }]);
  seedRules([
    { id: 'rule-wf', merchantSubstring: 'WHOLEFOODS', classification: 'spending', categoryId: 'food_and_drink' },
  ]);

  await openTransactionEditor(page, 'WHOLEFOODS');

  // The Rules section lists the governing rule, marked Applied.
  await expect(page.getByTestId('transaction-editor-rules')).toBeVisible();
  const governing = page
    .getByTestId('transaction-editor-rule')
    .filter({ has: page.getByTestId('transaction-editor-rule-applied') });
  await expect(governing).toHaveCount(1);

  // Opening it replaces the transaction modal with the rule editor, prefilled.
  await governing.click();
  await expect(page.getByTestId('rule-editor')).toBeVisible();
  await expect(page.getByTestId('transaction-editor')).toHaveCount(0);
  await expect(page.getByTestId('rule-editor-substring')).toHaveValue('WHOLEFOODS');

  // Editing the substring so it still matches, then saving, re-mounts the
  // transaction editor refreshed — still listing the (now governing) rule.
  await page.getByTestId('rule-editor-substring').fill('WHOLE');
  await page.getByTestId('rule-editor-submit').click();

  await expect(page.getByTestId('transaction-editor')).toBeVisible();
  await expect(page.getByTestId('rule-editor')).toHaveCount(0);
  await expect(page.getByTestId('transaction-editor-rule-applied')).toBeVisible();
});

test('A transaction with no matching rule offers Create rule, which returns on save', async ({
  page,
}) => {
  seedTransactions([{ id: 'txn-bb', merchant: 'BLUEBOTTLE', amount: 6 }]);
  seedRules([]); // no rules: nothing matches the row

  await openTransactionEditor(page, 'BLUEBOTTLE');

  // No rule matches: only the Create-rule control is offered.
  await expect(page.getByTestId('transaction-editor-rule-create')).toBeVisible();
  await expect(page.getByTestId('transaction-editor-rule')).toHaveCount(0);

  // It opens the rule editor prefilled with the transaction's merchant.
  await page.getByTestId('transaction-editor-rule-create').click();
  await expect(page.getByTestId('rule-editor')).toBeVisible();
  await expect(page.getByTestId('transaction-editor')).toHaveCount(0);
  await expect(page.getByTestId('rule-editor-substring')).toHaveValue('BLUEBOTTLE');

  // Completing a spending rule and saving creates it and returns to the transaction
  // editor, which now lists the new rule as governing.
  await page.getByTestId('rule-editor-classification').selectOption('spending');
  await page.getByTestId('rule-editor-category').selectOption('food_and_drink');
  await page.getByTestId('rule-editor-submit').click();

  await expect(page.getByTestId('transaction-editor')).toBeVisible();
  await expect(page.getByTestId('rule-editor')).toHaveCount(0);
  await expect(page.getByTestId('transaction-editor-rule-applied')).toBeVisible();
  await expect(page.getByTestId('transaction-editor-rule-create')).toHaveCount(0);
});

test('Dismissing the rule editor returns to the transaction editor', async ({ page }) => {
  seedTransactions([{ id: 'txn-wf', merchant: 'WHOLEFOODS', amount: 42 }]);
  seedRules([
    { id: 'rule-wf', merchantSubstring: 'WHOLEFOODS', classification: 'spending', categoryId: 'food_and_drink' },
  ]);

  await openTransactionEditor(page, 'WHOLEFOODS');

  // Open the governing rule, then dismiss the rule editor via its Close control.
  await page.getByTestId('transaction-editor-rule').first().click();
  await expect(page.getByTestId('rule-editor')).toBeVisible();

  await page.getByTestId('modal').getByRole('button', { name: 'Close' }).click();

  // Dismissing re-mounts the transaction editor rather than closing to the bare page.
  await expect(page.getByTestId('transaction-editor')).toBeVisible();
  await expect(page.getByTestId('rule-editor')).toHaveCount(0);
});

test('Deleting the governing rule from a transaction returns to the transaction', async ({
  page,
}) => {
  seedTransactions([{ id: 'txn-wf', merchant: 'WHOLEFOODS', amount: 42 }]);
  seedRules([
    { id: 'rule-wf', merchantSubstring: 'WHOLEFOODS', classification: 'spending', categoryId: 'food_and_drink' },
  ]);

  await openTransactionEditor(page, 'WHOLEFOODS');

  // Open the governing rule, then delete it from inside the rule modal.
  await page.getByTestId('transaction-editor-rule').first().click();
  await expect(page.getByTestId('rule-editor')).toBeVisible();
  await page.getByTestId('rule-editor-delete-submit').click();

  // Deleting re-mounts the transaction editor; with the rule gone, nothing governs the
  // row, so it now offers to create one.
  await expect(page.getByTestId('transaction-editor')).toBeVisible();
  await expect(page.getByTestId('rule-editor')).toHaveCount(0);
  await expect(page.getByTestId('transaction-editor-rule-create')).toBeVisible();
  await expect(page.getByTestId('transaction-editor-rule')).toHaveCount(0);
});
