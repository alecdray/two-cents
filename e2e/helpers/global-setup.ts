import { chromium, expect } from '@playwright/test';
import { config as loadEnv } from 'dotenv';
import { execSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname } from 'node:path';

import { STORAGE_STATE, TEST_PASSWORD } from './auth';

loadEnv(); // PORT, GOOSE_DBSTRING, ENCRYPTION_KEY etc. for the setpassword run

// Global setup establishes the single shared login (ADR-0007). It runs once
// before the suite, against the already-running app:
//   1. seed the login password through the SAME command an operator uses
//      (`bin/setpassword`), so the e2e path exercises the real bootstrap rather
//      than a fixture hash that could drift from the hashing implementation;
//   2. log in once through the real form and persist the session, so every spec
//      starts authenticated and never repeats the login;
//   3. refuse to run at all unless the app is on the deterministic `fake`
//      provider (see assertFakeBankProvider).
// The running app and the setpassword command share the SQLite file; the DB's
// busy timeout absorbs any brief write contention.
async function globalSetup() {
  const port = process.env.PORT || '4690';
  const baseURL = `http://127.0.0.1:${port}`;

  execSync('go run ./src/cmd/setpassword', {
    stdio: 'inherit',
    env: { ...process.env, AUTH_PASSWORD: TEST_PASSWORD },
  });

  mkdirSync(dirname(STORAGE_STATE), { recursive: true });

  const browser = await chromium.launch();
  const page = await browser.newPage({ baseURL });
  await page.goto('/login');
  await page.getByTestId('login-password').fill(TEST_PASSWORD);
  await page.getByTestId('login-submit').click();
  // The app navbar only renders on authenticated pages, so its presence is the
  // signal the login succeeded and we landed inside the app.
  await expect(page.getByTestId('app-navbar')).toBeVisible();
  await page.context().storageState({ path: STORAGE_STATE });

  await assertFakeBankProvider(page);

  await browser.close();
}

// assertFakeBankProvider aborts the whole suite unless the running app is using
// the deterministic in-process bank stand-in (ADR-0006, ADR-0025).
//
// This is a safety interlock, not a tidiness check. A dozen specs click the
// connect control, which in `real` mode opens live Plaid Link against whatever
// PLAID_ENV the server was started with — and an operator's own .env is exactly
// where a real production one lives. The app runs as a separate process by
// design (the Playwright config has no webServer), so the suite cannot choose
// the provider; refusing to proceed is the only move it has.
//
// It reads the mode the server itself rendered rather than this process's
// BANK_PROVIDER, because the env the test process sees is not necessarily the
// env the server was launched with — which is exactly how the two drift.
async function assertFakeBankProvider(page: import('@playwright/test').Page) {
  await page.goto('/accounts');

  // The control renders in both overview shapes (accounts present or not), so
  // its absence means the page did not render at all — abort rather than read a
  // null mode as "not real".
  const control = page.getByTestId('accounts-overview-connect');
  await expect(
    control,
    'REFUSING TO RUN: could not read the bank provider mode from /accounts. ' +
      'The app must be running and rendering before the suite can verify it is safe to drive.',
  ).toBeVisible();

  const mode = await control.getAttribute('data-bank-mode');

  expect(
    mode,
    'REFUSING TO RUN: the app under test is not using the fake bank provider ' +
      `(data-bank-mode="${mode}"). The suite drives the connect control, which in ` +
      "real mode opens live Plaid Link against the server's PLAID_ENV and can reach " +
      'real bank logins. Restart the app with BANK_PROVIDER=fake ' +
      '(`task dev/e2e`, or `BANK_PROVIDER=fake ./bin/app`) and re-run.',
  ).toBe('fake');
}

export default globalSetup;
