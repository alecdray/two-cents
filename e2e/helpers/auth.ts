// Shared constants for the single-local-login e2e auth path (ADR-0007). The
// global setup seeds this password and logs in once; the login spec reuses the
// password to drive the form directly.

// The password the global setup seeds via `bin/setpassword` and logs in with.
export const TEST_PASSWORD = 'e2e-secret-password';

// Where the authenticated session is persisted. Every spec inherits it through
// the Playwright config's `use.storageState`, so tests start logged in.
export const STORAGE_STATE = 'e2e/.auth/state.json';
