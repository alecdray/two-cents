import { test, expect, Page } from '@playwright/test';
import { resetActivity, seedOverview, seedTransfer, type SeedAccount } from '../helpers/db';

// Scenarios from e2e/feat/account-custom-name.feature

// Every scenario drives the rename UI against directly-seeded accounts — the
// rename handler reads and writes one account row, no token decryption. Scenario 3
// additionally seeds a transfer directly onto the seeded checking account so the
// editor's destination picker exists, proving the custom name reaches the
// transactions surface through the facets without linking a fake bank.

// renameAccount drives the inline rename affordance on the given account row:
// reveal the input, replace the name, and save. The save posts and swaps the
// overview region back in, so callers assert on the refreshed DOM afterwards.
async function renameAccount(row: ReturnType<Page['getByTestId']>, name: string) {
  await row.getByTestId('accounts-overview-account-rename').click();
  const input = row.getByTestId('accounts-overview-account-name-input');
  await expect(input).toBeVisible();
  await input.fill(name);
  await row.getByTestId('accounts-overview-account-name-save').click();
}

test('Renaming an account shows the custom name on the overview', async ({ page }) => {
  const seed: SeedAccount[] = [
    { name: 'Everyday Checking', bankType: 'checking', kind: 'cash', balanceKnown: true, amount: 1200, connection: 'active' },
  ];
  seedOverview(seed);

  await page.goto('/accounts');
  const row = page.getByTestId('accounts-overview-account-row');
  await expect(row.getByTestId('accounts-overview-account-name')).toContainText('Everyday Checking');

  await renameAccount(row, 'Joint Checking');

  // The region swaps back in with the custom name shown in place of the bank name.
  const renamed = page.getByTestId('accounts-overview-account-row');
  await expect(renamed.getByTestId('accounts-overview-account-name')).toContainText('Joint Checking');
  await expect(renamed.getByTestId('accounts-overview-account-name')).not.toContainText('Everyday Checking');
});

test('Clearing a custom name reverts to the bank name', async ({ page }) => {
  const seed: SeedAccount[] = [
    { name: 'Everyday Checking', bankType: 'checking', kind: 'cash', balanceKnown: true, amount: 1200, connection: 'active' },
  ];
  seedOverview(seed);

  await page.goto('/accounts');
  let row = page.getByTestId('accounts-overview-account-row');

  // First give it a custom name, then clear it.
  await renameAccount(row, 'Joint Checking');
  row = page.getByTestId('accounts-overview-account-row');
  await expect(row.getByTestId('accounts-overview-account-name')).toContainText('Joint Checking');

  await renameAccount(row, '');

  // An empty submit clears the override; the bank name returns.
  row = page.getByTestId('accounts-overview-account-row');
  await expect(row.getByTestId('accounts-overview-account-name')).toContainText('Everyday Checking');
  await expect(row.getByTestId('accounts-overview-account-name')).not.toContainText('Joint Checking');
});

test('A renamed account shows its custom name in the transaction editor', async ({ page }) => {
  // Seed a checking account (acct-0) and a counts-as-savings savings account
  // (acct-1), then hang one outflow transfer directly off the checking account —
  // no fake bank, no token decryption. The picker lists every active account, so
  // the renamed savings account must appear by its custom name.
  resetActivity();
  seedOverview([
    { name: 'Everyday Checking', bankType: 'checking', kind: 'cash', balanceKnown: true, amount: 1200, connection: 'active' },
    { name: 'High-Yield Savings', bankType: 'savings', kind: 'cash', balanceKnown: true, amount: 8000, connection: 'active', countsAsSavings: true },
  ]);
  seedTransfer({
    id: 'e2e-customname-transfer',
    accountId: 'acct-0',
    merchant: 'Move to Savings',
    amount: 250.0,
    date: '2026-07-01 00:00:00',
  });

  // Rename the savings account from the overview.
  await page.goto('/accounts');
  const savingsRow = page
    .getByTestId('accounts-overview-account-row')
    .filter({ hasText: 'High-Yield Savings' });
  await renameAccount(savingsRow, 'Emergency Fund');
  await expect(
    page.getByTestId('accounts-overview-account-row').filter({ hasText: 'Emergency Fund' }),
  ).toBeVisible();

  // Open the transfer's editor and its destination account picker.
  await page.getByTestId('nav-transactions').click();
  await expect(page.getByTestId('transactions-list')).toBeVisible();
  const transfer = page.getByTestId('transactions-row').filter({ hasText: 'Move to Savings' });
  await transfer.click();
  await expect(page.getByTestId('transaction-editor')).toBeVisible();

  // The picker is fed live from ConnectedAccountFacets, so it offers the account by
  // its custom name — selecting by that label succeeds only if the option exists.
  await page.getByTestId('txn-destination-picker-account').selectOption({ label: 'Emergency Fund' });
  await expect(page.getByTestId('txn-destination-picker-account')).not.toHaveValue('');
});
