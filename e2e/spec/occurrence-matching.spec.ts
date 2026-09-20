import { test, expect } from '@playwright/test';
import {
  resetSweep,
  seedCheckingTransaction,
  seedOverview,
  seedSchedule,
  settleOccurrence,
  type SeedAccount,
} from '../helpers/db';

// Scenarios from e2e/feat/occurrence-matching.feature

const FRESH_HOURS_AGO = 2;
const RENT = 2000;

function checking(amount: number): SeedAccount {
  return {
    name: 'Everyday Checking',
    bankType: 'checking',
    kind: 'cash',
    balanceKnown: true,
    amount,
    connection: 'active',
    countsAsSavings: false,
    lastSyncedHoursAgo: FRESH_HOURS_AGO,
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

function dayOfMonthIn(days: number): number {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return d.getDate();
}

// The occurrence one cadence back from the one falling `days` from now — the one
// that already fell due and is inside the sweep's lookback.
function occurrenceBefore(days: number): string {
  const next = new Date();
  next.setDate(next.getDate() + days);
  const prev = new Date(next);
  prev.setMonth(prev.getMonth() - 1);
  if (prev.getDate() !== next.getDate()) {
    prev.setDate(0);
  }
  const month = `${prev.getMonth() + 1}`.padStart(2, '0');
  const day = `${prev.getDate()}`.padStart(2, '0');
  return `${prev.getFullYear()}-${month}-${day}`;
}

function usd(amount: number): string {
  return `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

// A monthly bill declared to fall in three days also projects the occurrence a
// month earlier, which is inside the lookback. Unmatched, both are held back.
const BILL_DAY = 3;

test('Matching the occurrence that already fell due releases the money', async ({ page }) => {
  resetSweep();
  seedOverview([checking(10000), savings(1000)]);
  seedSchedule([
    { name: 'Rent', direction: 'out', amount: RENT, cadence: 'monthly', dayOfMonth: dayOfMonthIn(BILL_DAY) },
  ]);
  seedCheckingTransaction({
    id: 'txn-rent-paid',
    date: occurrenceBefore(BILL_DAY),
    amount: RENT,
    merchant: 'Greystone Property',
  });

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();
  // Both occurrences: the one still ahead, and the one that fell due with
  // nothing yet saying it was paid.
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(RENT * 2));

  await expect(page.getByTestId('schedule-occurrence-outstanding')).toBeVisible();
  await page.getByTestId('schedule-occurrence-candidate').selectOption('txn-rent-paid');
  await page.getByTestId('schedule-occurrence-match').click();
  await expect(page.getByTestId('schedule-occurrence-settled')).toBeVisible();

  // The occurrence is settled, so it leaves the timeline and only the bill still
  // ahead is held back.
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(RENT));
});

test('Rejecting a match hands the occurrence back to the timeline', async ({ page }) => {
  resetSweep();
  seedOverview([checking(10000), savings(1000)]);
  seedSchedule([
    { name: 'Rent', direction: 'out', amount: RENT, cadence: 'monthly', dayOfMonth: dayOfMonthIn(BILL_DAY) },
  ]);
  settleOccurrence({
    itemIndex: 0,
    occurrence: occurrenceBefore(BILL_DAY),
    amount: RENT,
    merchant: 'Greystone Property',
  });

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(RENT));

  await page.getByTestId('schedule-occurrence-clear').click();
  await expect(page.getByTestId('schedule-occurrence-cleared')).toBeVisible();

  // Rejecting a match is a decision, not the absence of one: the occurrence is
  // owed again, and the money goes back behind it.
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(RENT * 2));
});
