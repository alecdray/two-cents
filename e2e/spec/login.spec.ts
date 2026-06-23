import { test, expect } from '@playwright/test';
import { TEST_PASSWORD } from '../helpers/auth';

// Scenarios from e2e/feat/login.feature

// This spec exercises the gate itself, so it opts out of the shared
// authenticated session the rest of the suite inherits — every test here starts
// signed out. The login password is the one global setup seeded via
// bin/setpassword.
test.use({ storageState: { cookies: [], origins: [] } });

test('An unauthenticated visitor is sent to the login screen', async ({ page }) => {
  await page.goto('/accounts');
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByTestId('login-page')).toBeVisible();
});

test('The correct password signs me in', async ({ page }) => {
  await page.goto('/login');
  await page.getByTestId('login-password').fill(TEST_PASSWORD);
  await page.getByTestId('login-submit').click();

  // The app navbar renders only on authenticated pages, so its presence proves
  // we crossed the gate and landed inside the app.
  await expect(page.getByTestId('app-navbar')).toBeVisible();
  await expect(page).toHaveURL(/\/$/);
});

test('A wrong password is refused', async ({ page }) => {
  await page.goto('/login');
  await page.getByTestId('login-password').fill('not the password');
  await page.getByTestId('login-submit').click();

  await expect(page.getByTestId('login-error')).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByTestId('app-navbar')).toHaveCount(0);
});

test('Signing out returns to the login screen and re-locks the app', async ({ page }) => {
  // Start this test signed in by logging in through the form.
  await page.goto('/login');
  await page.getByTestId('login-password').fill(TEST_PASSWORD);
  await page.getByTestId('login-submit').click();
  await expect(page.getByTestId('app-navbar')).toBeVisible();

  await page.getByTestId('nav-more').click();
  await expect(page.getByTestId('more-sheet')).toBeVisible();
  await page.getByTestId('nav-logout').click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByTestId('login-page')).toBeVisible();

  // The session is gone: reopening a protected page bounces back to login.
  await page.goto('/transactions');
  await expect(page).toHaveURL(/\/login$/);
});
