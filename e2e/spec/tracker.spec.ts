import { test, expect, Page } from '@playwright/test';
import { resetActivity, resetBudget, resetCategorization } from '../helpers/db';

// Scenarios from e2e/feat/tracker.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake). Linking the bank posts
// directly and the deterministic stand-in backfills a fixed transaction set on
// the connect, all dated in the current month:
//   - Whole Foods         outflow  posted   -> Spending / General Merchandise  $84.32
//   - Acme Payroll        inflow   posted   -> Income                       $2,400.00
//   - Blue Bottle Coffee  outflow  PENDING  -> Spending / Food & Drink          $5.75
//   - Rainy Day Savings   outflow  posted   -> Transfer (savings contribution) $500.00
//   - Transfer from Checking inflow posted  -> Transfer (plain mirror)        $500.00
//   - Side Hustle Co      inflow   posted   -> needs-review                   $150.00

// linkBankFromAccounts links the fake bank from the accounts overview and waits
// for the in-place swap to settle (the cash group appearing), by which point the
// connect handler has also backfilled the bank's transactions.
async function linkBankFromAccounts(page: Page) {
  await page.goto('/accounts');
  await page.getByTestId('accounts-overview-connect').getByRole('button').click();
  await expect(page.getByTestId('accounts-overview-cash')).toBeVisible();
}

// limitInput selects the limit number input on a budget row by the Category name
// it contains, rather than a positional index.
function limitInput(page: Page, category: string) {
  return page
    .getByTestId('budget-limit-row')
    .filter({ hasText: category })
    .getByRole('spinbutton');
}

// addCategory adds a Category's limit row via the add-category select and waits
// for the row to appear. On the empty-plan baseline every Category is unbudgeted,
// so its row is hidden until it is added.
async function addCategory(page: Page, category: string) {
  await page.getByTestId('budget-add-category').selectOption({ label: category });
  await expect(limitInput(page, category)).toBeVisible();
}

// setBudget fills the editor and saves, waiting for the htmx swap to settle.
async function setBudget(page: Page) {
  await page.goto('/budget');
  await expect(page.getByTestId('budget-page')).toBeVisible();
  await page.getByTestId('budget-income').fill('3000');
  await page.getByTestId('budget-savings').fill('1000');
  // General Merchandise limit ($50) is below the $84.32 grocery spend -> over
  // budget; Food & Drink limit ($200) comfortably covers the $5.75 coffee.
  await addCategory(page, 'General Merchandise');
  await addCategory(page, 'Food & Drink');
  await limitInput(page, 'General Merchandise').fill('50');
  await limitInput(page, 'Food & Drink').fill('200');
  const saved = page.waitForResponse(
    (r) => r.url().includes('/budget') && r.request().method() === 'POST',
  );
  await page.getByTestId('budget-save').click();
  await saved;
  await expect(page.getByTestId('budget-balance-banner')).toBeVisible();
}

test('A budget set against the month\'s activity shows remaining, pace, progress, and an over-budget category', async ({
  page,
}) => {
  // Reset rules too: a lingering Rule from another spec would re-classify the
  // side-gig inflow on the re-sync and skew income progress.
  resetActivity();
  resetBudget();
  resetCategorization();
  await linkBankFromAccounts(page);
  await setBudget(page);

  await page.goto('/');
  await expect(page.getByTestId('tracker-page')).toBeVisible();

  // The current month's header is shown (like a past-month wrap's).
  await expect(page.getByTestId('tracker-month')).toBeVisible();

  // Each budgeted Category shows a row.
  await expect(page.getByTestId('tracker-category-row')).toHaveCount(2);

  // General Merchandise spending ($84.32) exceeds its $50 limit -> over budget.
  const overBudget = page.getByTestId('tracker-over-budget');
  await expect(overBudget).toHaveCount(1);
  await expect(overBudget).toBeVisible();
  const overRow = page.getByTestId('tracker-category-row').filter({ hasText: 'General Merchandise' });
  await expect(overRow).toContainText('General Merchandise');

  // The everything-else line and total remaining are present; the total heads the
  // section and shows the daily pace to hold the plan.
  await expect(page.getByTestId('tracker-everything-else')).toBeVisible();
  await expect(page.getByTestId('tracker-total')).toBeVisible();
  await expect(page.getByTestId('tracker-total')).toContainText('/day');

  // Every row carries a budget-used bar: the two Categories, everything-else, and
  // the total.
  await expect(page.getByTestId('tracker-budget-bar')).toHaveCount(4);

  // Income progress reflects the $2,400 paycheck against the $3,000 target.
  await expect(page.getByTestId('tracker-income-progress')).toContainText('$2,400.00');
  await expect(page.getByTestId('tracker-income-progress')).toContainText('$3,000.00');

  // Savings progress reflects the auto-paired $500 contribution against $1,000.
  await expect(page.getByTestId('tracker-savings-progress')).toContainText('$500.00');
  await expect(page.getByTestId('tracker-savings-progress')).toContainText('$1,000.00');

  // The month rail is present and the current month is the active chip, linking
  // to the root Tracker. The fixed fake set is all current-month, so the earliest
  // transaction is this month and the rail is just the one current chip.
  await expect(page.getByTestId('month-rail')).toBeVisible();
  const currentChip = page.getByTestId('month-rail-chip').last();
  await expect(currentChip).toHaveAttribute('aria-current', 'page');
  await expect(currentChip).toHaveAttribute('href', '/');

  // The actuals-only needs-budget prompt is absent while a budget is set.
  await expect(page.getByTestId('tracker-needs-budget')).toHaveCount(0);
});

test('With no budget set the Tracker prompts to create one', async ({ page }) => {
  resetActivity();
  resetBudget();

  await page.goto('/');
  await expect(page.getByTestId('tracker-page')).toBeVisible();

  // With no budget, the page shows the actuals-only prompt and no budgeted rows.
  await expect(page.getByTestId('tracker-needs-budget')).toBeVisible();
  await expect(page.getByTestId('tracker-category-row')).toHaveCount(0);

  // The month rail is present; with no transactions it collapses to the current
  // month, which is the active chip linking to the root Tracker.
  await expect(page.getByTestId('month-rail')).toBeVisible();
  const currentChip = page.getByTestId('month-rail-chip').last();
  await expect(currentChip).toHaveAttribute('aria-current', 'page');
  await expect(currentChip).toHaveAttribute('href', '/');
});
