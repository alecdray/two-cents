import { defineConfig, devices } from '@playwright/test';
import { config } from 'dotenv';
import { STORAGE_STATE } from './e2e/helpers/auth';

config(); // load .env so PORT / GOOSE_DBSTRING are available to helpers

const port = process.env.PORT || '4690';

export default defineConfig({
  testDir: './e2e/spec',
  // Auth gates the whole app (ADR-0007). Global setup seeds the login and logs
  // in once; every spec inherits that session via storageState, so tests start
  // authenticated and seed only their own data. The login spec opts out with an
  // empty storageState to exercise the gate itself.
  globalSetup: './e2e/helpers/global-setup.ts',
  fullyParallel: false,
  // Every spec seeds the one shared SQLite file the running app reads, with no
  // per-test isolation, so specs must run serially — parallel workers would
  // stomp on each other's seeded state. fullyParallel:false only serialises
  // within a file; a single worker serialises across files too.
  workers: 1,
  retries: 0,
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    storageState: STORAGE_STATE,
    trace: 'on-first-retry',
    testIdAttribute: 'data-testid',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
