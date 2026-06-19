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
//      starts authenticated and never repeats the login.
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
  await browser.close();
}

export default globalSetup;
