import { test, expect } from '@playwright/test';
import { resetAccounts, markConnectionsNeedsReconnect } from '../helpers/db';

// Scenarios from e2e/feat/reconnect-bank.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake), so the reconnect control
// posts directly and the fake's healthy login clears the flag immediately.

test('Reconnecting a needs-reconnect bank clears the badge', async ({ page }) => {
  // Link the fake bank through the UI so the stored access token is real and
  // decryptable, then flip the connection to needs_reconnect in place — the
  // reconnect flow decrypts that token to confirm the login.
  resetAccounts();
  await page.goto('/');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();

  markConnectionsNeedsReconnect();
  await page.goto('/');

  // The expired login is flagged: the badge and the reconnect control are shown.
  await expect(page.getByTestId('accounts-overview-needs-reconnect').first()).toBeVisible();
  const reconnect = page.getByTestId('accounts-overview-account-reconnect').first();
  await expect(reconnect).toBeVisible();

  // Reconnecting (fake mode posts directly) refreshes the login and clears every
  // badge, leaving the bank's accounts in place.
  await reconnect.getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-needs-reconnect')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
});
