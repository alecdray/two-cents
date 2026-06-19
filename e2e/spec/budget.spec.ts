import { test, expect, Page } from '@playwright/test';
import { resetBudget } from '../helpers/db';

// Scenarios from e2e/feat/budget.feature

// The built-in Categories are seeded by migration, so "Food & Drink" can be
// budgeted without ever connecting a bank. Each scenario resets the single
// rolling budget config first so it starts from the empty-plan baseline, where
// no Category has a limit and so no limit row is shown — a Category is added
// through the add-category control before its limit can be set.

// addFoodCategory adds the Food & Drink limit row via the add-category select and
// waits for the row to appear. On the empty-plan baseline every Category is
// unbudgeted, so the row is hidden until it is added.
async function addFoodCategory(page: Page) {
  await page.getByTestId('budget-add-category').selectOption({ label: 'Food & Drink' });
  await expect(
    page.getByTestId('budget-limit-row').filter({ hasText: 'Food & Drink' }),
  ).toBeVisible();
}

// foodLimitInput selects the limit number input on the Food & Drink row, scoped
// by the row's contained Category name rather than a positional index.
function foodLimitInput(page: Page) {
  return page
    .getByTestId('budget-limit-row')
    .filter({ hasText: 'Food & Drink' })
    .getByRole('spinbutton');
}

// saveBudget submits the editor and waits for the htmx swap to settle by waiting
// on the POST response, then on the refreshed banner being present.
async function saveBudget(page: Page) {
  const saved = page.waitForResponse(
    (r) => r.url().includes('/budget') && r.request().method() === 'POST',
  );
  await page.getByTestId('budget-save').click();
  await saved;
  await expect(page.getByTestId('budget-balance-banner')).toBeVisible();
}

test('A budget is set, shows its residual and a balanced banner, and persists across a reload', async ({
  page,
}) => {
  resetBudget();
  await page.goto('/budget');
  await expect(page.getByTestId('budget-page')).toBeVisible();

  await page.getByTestId('budget-income').fill('5000');
  await page.getByTestId('budget-savings').fill('1000');
  await addFoodCategory(page);
  await foodLimitInput(page).fill('600');

  // The residual recomputes live on the client, before any save round-trip:
  // everything else = 5000 - 600 (limit) - 1000 (savings) = 3400.
  await expect(page.getByTestId('budget-residual')).toContainText('$3,400.00');

  await saveBudget(page);

  // The saved render shows the same residual and a balanced banner.
  await expect(page.getByTestId('budget-residual')).toContainText('$3,400.00');
  await expect(page.getByTestId('budget-balance-banner')).toContainText('Balanced');

  // The plan persists: a fresh load reads the saved targets and limit back.
  await page.reload();
  await expect(page.getByTestId('budget-income')).toHaveValue('5000.00');
  await expect(page.getByTestId('budget-savings')).toHaveValue('1000.00');
  await expect(foodLimitInput(page)).toHaveValue('600.00');
  await expect(page.getByTestId('budget-residual')).toContainText('$3,400.00');
});

test('An over-allocated plan still saves and shows the over-allocated banner', async ({ page }) => {
  resetBudget();
  await page.goto('/budget');
  await expect(page.getByTestId('budget-page')).toBeVisible();

  // 500 (limit) + 800 (savings) = 1300 > 1000 income -> over-allocated.
  await page.getByTestId('budget-income').fill('1000');
  await page.getByTestId('budget-savings').fill('800');
  await addFoodCategory(page);
  await foodLimitInput(page).fill('500');
  await saveBudget(page);

  await expect(page.getByTestId('budget-balance-banner')).toContainText('Over-allocated');

  // Over-allocated is surfaced, not blocked: the plan still persists.
  await page.reload();
  await expect(page.getByTestId('budget-income')).toHaveValue('1000.00');
  await expect(foodLimitInput(page)).toHaveValue('500.00');
  await expect(page.getByTestId('budget-balance-banner')).toContainText('Over-allocated');
});
