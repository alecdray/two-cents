import { defineConfig, devices } from '@playwright/test';
import { config } from 'dotenv';

config(); // load .env so PORT / GOOSE_DBSTRING are available to helpers

const port = process.env.PORT || '4690';

export default defineConfig({
  testDir: './e2e/spec',
  fullyParallel: false,
  retries: 0,
  use: {
    baseURL: `http://127.0.0.1:${port}`,
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
