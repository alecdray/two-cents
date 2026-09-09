import { test, expect } from '@playwright/test';
import { resetSweep, seedOverview, type SeedAccount } from '../helpers/db';

// Scenarios from e2e/feat/sweep-snapshots.feature

// Well inside the app's 24h staleness threshold, and well past it. Both sit far
// enough from the boundary that a slow run cannot drift a fixture across it.
const FRESH_HOURS_AGO = 2;
const STALE_HOURS_AGO = 72;

// The sweep derives its accounts: exactly one active cash account that is not
// counted as savings is checking, and exactly one that is is savings.
function checking(amount: number, lastSyncedHoursAgo = FRESH_HOURS_AGO): SeedAccount {
  return {
    name: 'Everyday Checking',
    bankType: 'checking',
    kind: 'cash',
    balanceKnown: true,
    amount,
    connection: 'active',
    countsAsSavings: false,
    lastSyncedHoursAgo,
  };
}

function savings(amount: number): SeedAccount {
  return {
    name: 'Rainy Day',
    bankType: 'savings',
    kind: 'cash',
    balanceKnown: true,
    amount,
    connection: 'active',
    countsAsSavings: true,
    lastSyncedHoursAgo: FRESH_HOURS_AGO,
  };
}

test('Running the sweep on demand produces a snapshot', async ({ page }) => {
  resetSweep();
  seedOverview([checking(3000), savings(1000)]);

  await page.goto('/sweep');
  // Nothing has ever been run, so the page is in its first-run empty state —
  // which still offers the action, or a new user could not produce a first
  // snapshot before the 7th.
  await expect(page.getByTestId('sweep-empty')).toBeVisible();

  await page.getByTestId('sweep-run').click();

  // The run lands the user on the snapshot it just produced — swapped in place,
  // and at that snapshot's own address, so the fresh result is bookmarkable.
  await expect(page.getByTestId('sweep-numeric')).toBeVisible();
  await expect(page.getByTestId('sweep-checking')).toHaveText('$3,000.00');
  await expect(page.getByTestId('sweep-empty')).toHaveCount(0);
  await expect(page).toHaveURL(/\/sweep\/.+/);
});

test('A second run adds to the history instead of replacing the first', async ({ page }) => {
  resetSweep();
  seedOverview([checking(3000), savings(1000)]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-checking')).toHaveText('$3,000.00');

  // The balance moves, and the user asks again — the case the monthly tick is
  // too slow for. Re-seeding leaves the stored snapshot untouched.
  seedOverview([checking(9000), savings(1000)]);
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-checking')).toHaveText('$9,000.00');

  // The first run is still there, one step back.
  await page.getByTestId('sweep-older').click();
  await expect(page.getByTestId('sweep-checking')).toHaveText('$3,000.00');
  await expect(page.getByTestId('sweep-newer')).toBeVisible();
});

test('A stale checking balance blocks the recommendation', async ({ page }) => {
  resetSweep();
  seedOverview([checking(3000, STALE_HOURS_AGO), savings(1000)]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  // The account is perfectly well identified — what is wrong is the freshness of
  // the figure, so the page says that rather than producing a number.
  await expect(page.getByTestId('sweep-needs-attention')).toBeVisible();
  await expect(page.getByTestId('sweep-reason')).toContainText('refresh');
  await expect(page.getByTestId('sweep-numeric')).toHaveCount(0);
});
