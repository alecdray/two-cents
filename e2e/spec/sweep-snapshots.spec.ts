import { test, expect } from '@playwright/test';
import { resetSchedule, resetSweep, seedOverview, seedSchedule, type SeedAccount } from '../helpers/db';

// Scenarios from e2e/feat/sweep-snapshots.feature

// Well inside the app's 24h staleness threshold, and well past it. Both sit far
// enough from the boundary that a slow run cannot drift a fixture across it.
const FRESH_HOURS_AGO = 2;
const STALE_HOURS_AGO = 72;

// The fixed cushion held in checking beyond what the timeline requires.
const SAFETY_MARGIN = 500;

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

function card(amount: number, lastSyncedHoursAgo = FRESH_HOURS_AGO): SeedAccount {
  return {
    name: 'Travel Rewards Card',
    bankType: 'credit card',
    kind: 'credit',
    balanceKnown: true,
    amount,
    connection: 'active',
    lastSyncedHoursAgo,
  };
}

// A monthly item declared for the day-of-month N days from today falls exactly
// once inside the one-month window, on that day — so a scenario can place an
// event a known distance ahead of the run without pinning the clock.
function dayOfMonthIn(days: number): number {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return d.getDate();
}

function usd(amount: number): string {
  return `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

test('Running the sweep on demand produces a snapshot', async ({ page }) => {
  resetSweep();
  resetSchedule();
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
  await expect(page.getByTestId('sweep-checking')).toHaveText(usd(3000));
  await expect(page.getByTestId('sweep-empty')).toHaveCount(0);
  await expect(page).toHaveURL(/\/sweep\/.+/);
});

test('A second run adds to the history instead of replacing the first', async ({ page }) => {
  resetSweep();
  resetSchedule();
  seedOverview([checking(3000), savings(1000)]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-checking')).toHaveText(usd(3000));

  // The balance moves, and the user asks again — the case the monthly tick is
  // too slow for. Re-seeding leaves the stored snapshot untouched.
  seedOverview([checking(9000), savings(1000)]);
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-checking')).toHaveText(usd(9000));

  // The first run is still there, one step back.
  await page.getByTestId('sweep-older').click();
  await expect(page.getByTestId('sweep-checking')).toHaveText(usd(3000));
  await expect(page.getByTestId('sweep-newer')).toBeVisible();
});

test('A stale checking balance blocks the recommendation', async ({ page }) => {
  resetSweep();
  resetSchedule();
  seedOverview([checking(3000, STALE_HOURS_AGO), savings(1000)]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  // The account is perfectly well identified — what is wrong is the freshness of
  // the figure, so the page says that rather than producing a number.
  await expect(page.getByTestId('sweep-needs-attention')).toBeVisible();
  await expect(page.getByTestId('sweep-reason')).toContainText('refresh');
  await expect(page.getByTestId('sweep-numeric')).toHaveCount(0);
});

test('A card balance that stopped refreshing blocks the recommendation', async ({ page }) => {
  resetSweep();
  resetSchedule();
  seedOverview([checking(10000), savings(1000), card(6000, STALE_HOURS_AGO)]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  await expect(page.getByTestId('sweep-needs-attention')).toBeVisible();
  await expect(page.getByTestId('sweep-reason')).toContainText('card');
  await expect(page.getByTestId('sweep-numeric')).toHaveCount(0);
});

test('A card with no statement detail still produces a result', async ({ page }) => {
  resetSweep();
  resetSchedule();
  seedOverview([checking(10000), savings(1000), card(6000)]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  // No bank reports statement detail yet, so a known balance with no due date
  // falls due immediately. That is the most conservative reading, not a failure
  // — the number still forms.
  await expect(page.getByTestId('sweep-numeric')).toBeVisible();
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(6000));
  await expect(page.getByTestId('sweep-timeline-label').first()).toContainText('Travel Rewards Card');
});

test('A bill falling due before the next paycheck raises what must stay put', async ({ page }) => {
  resetSweep();
  seedOverview([checking(10000), savings(1000)]);
  seedSchedule([
    { name: 'Rent', direction: 'out', amount: 2000, cadence: 'monthly', dayOfMonth: dayOfMonthIn(3) },
    { name: 'Paycheck', direction: 'in', amount: 5000, cadence: 'monthly', dayOfMonth: dayOfMonthIn(10) },
  ]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  // The paycheck is bigger than the bill, but it lands a week later — and a
  // paycheck cannot pay a bill that already came due. The peak is the bill.
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(2000));
  await expect(page.getByTestId('sweep-timeline-peak')).toBeVisible();
  await expect(page.getByTestId('sweep-action-line')).toContainText(
    usd(10000 - 2000 - SAFETY_MARGIN).replace('.00', ''),
  );
});

test('The same bill falling due after the paycheck lowers it again', async ({ page }) => {
  resetSweep();
  seedOverview([checking(10000), savings(1000)]);
  seedSchedule([
    { name: 'Paycheck', direction: 'in', amount: 5000, cadence: 'monthly', dayOfMonth: dayOfMonthIn(10) },
    { name: 'Rent', direction: 'out', amount: 2000, cadence: 'monthly', dayOfMonth: dayOfMonthIn(17) },
  ]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  // Same bill, same paycheck, same balance — only the order changed. The income
  // arrives first, so nothing in checking is spoken for and the whole balance
  // above the margin is free to earn interest.
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(0));
  await expect(page.getByTestId('sweep-action-line')).toContainText(
    usd(10000 - SAFETY_MARGIN).replace('.00', ''),
  );
});

test('A card statement falling after the paycheck lowers what must stay put', async ({ page }) => {
  resetSweep();
  resetSchedule();
  seedOverview([
    checking(10000),
    savings(1000),
    { ...card(840), statement: { billed: 500, issuedDaysFromNow: -5, dueDaysFromNow: 17 } },
  ]);
  seedSchedule([
    { name: 'Paycheck', direction: 'in', amount: 5000, cadence: 'monthly', dayOfMonth: dayOfMonthIn(10) },
  ]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  // The statement is dated, and it falls after the paycheck lands — so the
  // income covers it and none of today's checking balance is spoken for. Before
  // the statement was known, this same card put its whole balance at the run
  // instant and forced 840 to stay put.
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(0));
  await expect(page.getByTestId('sweep-action-line')).toContainText(
    usd(10000 - SAFETY_MARGIN).replace('.00', ''),
  );
});

test('Unbilled card spending is not reserved for', async ({ page }) => {
  resetSweep();
  resetSchedule();
  seedOverview([
    checking(10000),
    savings(1000),
    // Balance 840, but only 500 has been billed: 340 was spent this cycle and
    // has no due date inside the window.
    { ...card(840), statement: { billed: 500, issuedDaysFromNow: -5, dueDaysFromNow: 3 } },
  ]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();

  // Only the billed 500 is reserved. This is the one place the model holds back
  // less than it did before statements were read (ADR-0026).
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(500));
});

test('Declaring a scheduled item from the sweep page', async ({ page }) => {
  resetSweep();
  resetSchedule();
  seedOverview([checking(10000), savings(1000)]);

  await page.goto('/sweep');
  await expect(page.getByTestId('schedule-empty')).toBeVisible();

  await page.getByTestId('schedule-add-name').fill('Rent');
  await page.getByTestId('schedule-add-amount').fill('2000');
  await page.getByTestId('schedule-add-day').fill(String(dayOfMonthIn(3)));
  await page.getByTestId('schedule-add-submit').click();

  await expect(page.getByTestId('schedule-row')).toHaveCount(1);
  await expect(page.getByTestId('schedule-empty')).toHaveCount(0);

  // A declaration is only worth making if it reaches the timeline, so the run is
  // part of what this scenario is checking.
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(2000));
});

test('Taking a scheduled item off the timeline without deleting it', async ({ page }) => {
  resetSweep();
  seedOverview([checking(10000), savings(1000)]);
  seedSchedule([
    { name: 'Rent', direction: 'out', amount: 2000, cadence: 'monthly', dayOfMonth: dayOfMonthIn(3) },
  ]);

  await page.goto('/sweep');
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(2000));

  await page.getByTestId('schedule-row-active').uncheck();
  await page.getByTestId('schedule-row-save').click();

  // Kept, so the declaration is not lost — but off the timeline, so the money is
  // released.
  await expect(page.getByTestId('schedule-row')).toHaveCount(1);
  await page.getByTestId('sweep-run').click();
  await expect(page.getByTestId('sweep-required')).toHaveText(usd(0));
});
