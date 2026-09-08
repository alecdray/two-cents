import { test, expect } from '@playwright/test';
import { seedOverview, type SeedAccount } from '../helpers/db';

// Scenarios from e2e/feat/stale-balance.feature

// The app marks a balance stale after 24 hours without a refresh (the bank sync
// runs every 6 hours, so this tolerates four consecutive failed passes). These
// ages sit far enough either side of that threshold that a slow run can never
// drift a fixture across it.
const FRESH_HOURS_AGO = 2;
const STALE_HOURS_AGO = 72;

function account(overrides: Partial<SeedAccount> = {}): SeedAccount {
  return {
    name: 'Everyday Checking',
    bankType: 'checking',
    kind: 'cash',
    balanceKnown: true,
    amount: 1200,
    connection: 'active',
    ...overrides,
  };
}

test('An account that stopped refreshing is marked stale', async ({ page }) => {
  seedOverview([account({ lastSyncedHoursAgo: STALE_HOURS_AGO })]);

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  // The balance still renders — the mark qualifies the figure, it does not
  // replace or hide it.
  await expect(page.getByTestId('accounts-overview-stale-balance')).toBeVisible();
  await expect(page.getByTestId('accounts-overview-account-balance').first()).toHaveText('$1,200.00');
});

test('A recently refreshed account is not marked stale', async ({ page }) => {
  seedOverview([account({ lastSyncedHoursAgo: FRESH_HOURS_AGO })]);

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  await expect(page.getByTestId('accounts-overview-account-row').first()).toBeVisible();
  await expect(page.getByTestId('accounts-overview-stale-balance')).toHaveCount(0);
});

test('A needs-reconnect bank shows only the reconnect badge', async ({ page }) => {
  // Same aged balance, but hanging off the needs-reconnect connection: the
  // reconnect badge already explains why the figure stopped moving, so the row
  // must not carry a second mark saying the same thing.
  seedOverview([account({ connection: 'reconnect', lastSyncedHoursAgo: STALE_HOURS_AGO })]);

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  await expect(page.getByTestId('accounts-overview-needs-reconnect').first()).toBeVisible();
  await expect(page.getByTestId('accounts-overview-stale-balance')).toHaveCount(0);
});
