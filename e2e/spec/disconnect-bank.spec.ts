import { test, expect, type Page } from '@playwright/test';
import { resetAccounts } from '../helpers/db';

// Scenarios from e2e/feat/disconnect-bank.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). The fake exposes a single
// connection holding all three accounts (checking, savings, credit card), so
// disconnecting that one bank empties the overview entirely.

// connectFakeBank links the fake bank from the empty overview and waits until
// its accounts are on the page. Linking through the UI (rather than seeding)
// stores a real, decryptable access token — disconnect decrypts it to sever the
// login at the provider.
async function connectFakeBank(page: Page) {
  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-empty')).toBeVisible();
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

test('Cancelling the confirmation keeps the bank', async ({ page }) => {
  resetAccounts();
  await connectFakeBank(page);

  // Opening the disconnect control raises a confirmation dialog.
  await page.getByTestId('accounts-overview-account-disconnect').first().click();
  await expect(page.locator('dialog[open]')).toBeVisible();

  // Cancelling dismisses the dialog without firing the disconnect.
  await page.keyboard.press('Escape');
  await expect(page.locator('dialog[open]')).toHaveCount(0);

  // Nothing was removed: the bank's accounts remain on the overview.
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
  await expect(page.getByTestId('accounts-overview-credit')).toBeVisible();
  await expect(page.getByTestId('accounts-overview-empty')).toHaveCount(0);
});

test('Confirming the disconnect removes the bank', async ({ page }) => {
  resetAccounts();
  await connectFakeBank(page);

  // Disconnect fires only after confirming inside the dialog.
  await page.getByTestId('accounts-overview-account-disconnect').first().click();
  const dialog = page.locator('dialog[open]');
  await expect(dialog).toBeVisible();
  await dialog.getByTestId('accounts-overview-disconnect-confirm').click();

  // The fake's single connection held every account, so the overview returns to
  // the empty state with its connect control — and no groups remain.
  await expect(page.getByTestId('accounts-overview-empty')).toBeVisible();
  await expect(page.getByTestId('accounts-overview-cash')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-credit')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-connect')).toBeVisible();
});
