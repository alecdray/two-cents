import { test, expect } from '@playwright/test';
import { seedOverview, type SeedAccount } from '../helpers/db';

// Scenarios from e2e/feat/account-kind-override.feature

// The override handlers read and write account rows only (no token decryption),
// so each scenario seeds the overview directly rather than linking the fake bank.

test('Overriding an account\'s kind re-buckets it and updates net cash', async ({ page }) => {
  // One cash account ($1,200) and one credit card ($450): net cash = 750.
  const seed: SeedAccount[] = [
    { name: 'Everyday Checking', bankType: 'checking', kind: 'cash', balanceKnown: true, amount: 1200, connection: 'active' },
    { name: 'Travel Rewards Card', bankType: 'credit card', kind: 'credit', balanceKnown: true, amount: 450, connection: 'active' },
  ];
  seedOverview(seed);

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();
  await expect(page.getByTestId('accounts-overview-net-cash')).toHaveText('$750.00');
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveText('$1,200.00');

  // The cash group holds exactly the checking account; move it to other.
  const cashGroup = page.getByTestId('accounts-overview-cash');
  await cashGroup.getByTestId('accounts-overview-account-kind').selectOption('other');

  // The row leaves cash for the other group, and net cash drops by its balance
  // (other accounts are excluded): now 0 − 450 = −450, with total cash at $0.
  const otherSection = page.getByTestId('accounts-overview-other');
  await expect(otherSection).toBeVisible();
  await expect(otherSection.getByTestId('accounts-overview-account-row')).toHaveCount(1);
  await expect(page.getByTestId('accounts-overview-cash')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-net-cash')).toHaveText('-$450.00');
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveText('$0.00');
});

test('Turning on counts-as-savings reflects on the row immediately', async ({ page }) => {
  const seed: SeedAccount[] = [
    { name: 'Everyday Checking', bankType: 'checking', kind: 'cash', balanceKnown: true, amount: 1200, connection: 'active', countsAsSavings: false },
  ];
  seedOverview(seed);

  await page.goto('/accounts');
  const toggle = page.getByTestId('accounts-overview-account-counts-as-savings');
  await expect(toggle).toBeVisible();
  await expect(toggle).not.toBeChecked();

  // Flipping the toggle posts and swaps the region back in with the flag set.
  await toggle.click();
  await expect(page.getByTestId('accounts-overview-account-counts-as-savings')).toBeChecked();
});

test('Overriding a savings account to credit drops its savings toggle', async ({ page }) => {
  const seed: SeedAccount[] = [
    { name: 'High-Yield Savings', bankType: 'savings', kind: 'cash', balanceKnown: true, amount: 3400, connection: 'active', countsAsSavings: true },
  ];
  seedOverview(seed);

  await page.goto('/accounts');
  // A cash account counted as savings shows the toggle, switched on.
  await expect(page.getByTestId('accounts-overview-account-counts-as-savings')).toBeChecked();

  // Override its kind to credit.
  await page.getByTestId('accounts-overview-cash').getByTestId('accounts-overview-account-kind').selectOption('credit');

  // It re-buckets into credit, where the savings toggle is never offered.
  const creditGroup = page.getByTestId('accounts-overview-credit');
  await expect(creditGroup).toBeVisible();
  await expect(creditGroup.getByTestId('accounts-overview-account-row')).toHaveCount(1);
  await expect(page.getByTestId('accounts-overview-cash')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-account-counts-as-savings')).toHaveCount(0);
});
