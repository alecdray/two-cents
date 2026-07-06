import { test, expect } from '@playwright/test';

// Scenarios from e2e/feat/more-nav.feature
// Inherits the shared authenticated session from global setup.

test('The primary destinations are reachable from the bottom bar', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  await page.getByTestId('nav-transactions').click();
  await expect(page).toHaveURL(/\/transactions$/);

  await page.getByTestId('nav-budget').click();
  await expect(page).toHaveURL(/\/budget$/);

  await page.getByTestId('nav-accounts').click();
  await expect(page).toHaveURL(/\/accounts$/);

  await page.getByTestId('nav-spending').click();
  await expect(page).toHaveURL(/\/$/);
});

test('The More sheet reaches the secondary destinations', async ({ page }) => {
  await page.goto('/');

  await page.getByTestId('nav-more').click();
  await expect(page.getByTestId('more-sheet')).toBeVisible();
  await page.getByTestId('nav-categories').click();
  await expect(page).toHaveURL(/\/categories$/);

  await page.getByTestId('nav-more').click();
  await page.getByTestId('nav-rules').click();
  await expect(page).toHaveURL(/\/rules$/);
});

test('The active destination is highlighted', async ({ page }) => {
  await page.goto('/transactions');
  await expect(page.getByTestId('nav-transactions')).toHaveClass(/text-primary/);
  await expect(page.getByTestId('nav-spending')).toHaveClass(/text-muted/);

  // A secondary destination highlights the More control instead.
  await page.goto('/rules');
  await expect(page.getByTestId('nav-more')).toHaveClass(/text-primary/);
});
