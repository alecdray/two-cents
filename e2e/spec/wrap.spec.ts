import { test, expect } from '@playwright/test';
import { seedPriorMonthWrap, currentMonthYM } from '../helpers/db';

// Scenarios from e2e/feat/wrap.feature
//
// The current month no longer has a wrap page — GET /wraps/{currentYM} redirects
// to the root Tracker (the current month's one canonical face). So a real wrap
// lives on an EARLIER month: seedPriorMonthWrap plants a deterministic,
// fully-classified set dated in the prior calendar month (reckoned in the app
// timezone), the only month with transactions — so its wrap is settling (a pending
// coffee charge) and partial (it is the backfill edge), and the month rail spans
// prior→current (two chips). The seed returns the exact rendered figure strings.

test('A prior month\'s wrap shows its figures and sits on the month rail', async ({ page }) => {
  const wrap = seedPriorMonthWrap();

  // Reach the wrap from the Tracker's month rail: the earliest (prior) chip is the
  // first, the current month the last. Clicking it navigates to that month's wrap.
  await page.goto('/');
  await expect(page.getByTestId('tracker-page')).toBeVisible();
  await expect(page.getByTestId('month-rail-chip')).toHaveCount(2);
  const priorChip = page.getByTestId('month-rail-chip').first();
  await expect(priorChip).toHaveAttribute('href', `/wraps/${wrap.ym}`);
  await priorChip.click();
  await expect(page.getByTestId('wrap-page')).toBeVisible();

  // Net income = $2,000 income − ($120 + $30) spending = $1,850.00.
  await expect(page.getByTestId('wrap-net-income')).toContainText(wrap.netIncome);

  // Gross income is the $2,000 paycheck alone (the drillable income figure).
  await expect(page.getByTestId('wrap-income')).toContainText(wrap.grossIncome);

  // Savings contributed is the $300 source leg only (the mirror is never counted).
  await expect(page.getByTestId('wrap-savings')).toContainText(wrap.savingsContributed);

  // Surplus = net income ($1,850) − savings contributed ($300) = $1,550.00.
  await expect(page.getByTestId('wrap-surplus')).toContainText(wrap.surplus);

  // The inline full-month list shows every transaction in the month (all six rows).
  await expect(page.getByTestId('wrap-month-row')).toHaveCount(wrap.monthRowCount);

  // Spend breaks down by Category: General Merchandise and Food & Drink.
  await expect(page.getByTestId('wrap-category-row')).toHaveCount(2);
  await expect(
    page.getByTestId('wrap-category-row').filter({ hasText: 'General Merchandise' }),
  ).toContainText(wrap.generalMerchandise);
  await expect(
    page.getByTestId('wrap-category-row').filter({ hasText: 'Food & Drink' }),
  ).toContainText(wrap.foodAndDrink);

  // The pending coffee charge makes the wrap settling.
  await expect(page.getByTestId('wrap-state')).toContainText('Settling');

  // The prior month is the earliest one we hold, so it sits at the backfill edge.
  await expect(page.getByTestId('wrap-partial')).toBeVisible();

  // On the wrap, the rail's active chip is this month; the current-month chip
  // (the last) still links back to the root Tracker.
  const activeChip = page.getByTestId('month-rail-chip').first();
  await expect(activeChip).toHaveAttribute('aria-current', 'page');
  await expect(activeChip).toHaveAttribute('href', `/wraps/${wrap.ym}`);
  await expect(page.getByTestId('month-rail-chip').last()).toHaveAttribute('href', '/');
});

test('Visiting the current month\'s wrap redirects to the Tracker', async ({ page }) => {
  seedPriorMonthWrap();

  // The current month's wrap address has no page of its own: it 302-redirects to
  // the root Tracker, which page.goto follows.
  await page.goto(`/wraps/${currentMonthYM()}`);
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByTestId('tracker-page')).toBeVisible();
});

test("Editing a transaction from the wrap list refreshes the wrap's figures", async ({ page }) => {
  const wrap = seedPriorMonthWrap();

  // Reach the wrap via a full load (not a boosted click) so the modal interaction is
  // reliable under headless automation.
  await page.goto(`/wraps/${wrap.ym}`);
  await expect(page.getByTestId('wrap-page')).toBeVisible();

  // Gross income starts at the $2,000 paycheck.
  await expect(page.getByTestId('wrap-income')).toContainText(wrap.grossIncome);

  // Re-categorize the needs-review side-gig inflow ($150) to Income from the wrap's
  // list. Saving announces transaction-changed, so the wrap figure region
  // self-refreshes — gross income rises to $2,150.
  await page.getByTestId('wrap-month-row').filter({ hasText: 'Side Hustle Co' }).click();
  await expect(page.getByTestId('transaction-editor')).toBeVisible();
  await page.getByTestId('txn-categorize-classification').selectOption('income');
  await page.getByTestId('txn-edit-submit').click();

  await expect(page.getByTestId('wrap-income')).toContainText(wrap.grossIncomeAfterSidegig);
});
