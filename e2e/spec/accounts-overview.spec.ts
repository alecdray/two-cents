import { test, expect } from '@playwright/test';
import { seedOverview, resetAccounts, type SeedAccount } from '../helpers/db';

// Scenarios from e2e/feat/accounts-overview.feature

// The seeded fixture spans every property the overview must demonstrate:
//   - two cash accounts with known balances    -> total cash 1200 + 3300 = 4500
//   - one cash account with an UNKNOWN balance  -> shown as em dash, excluded
//   - one credit account on the reconnect conn  -> total debt 450, badge shown
//   - one OTHER account (brokerage)             -> shown but excluded
// Net cash = total cash - total debt = 4500 - 450 = 4050.
const SEED: SeedAccount[] = [
  { name: 'Everyday Checking', bankType: 'checking', kind: 'cash', balanceKnown: true, amount: 1200, connection: 'active' },
  { name: 'High-Yield Savings', bankType: 'savings', kind: 'cash', balanceKnown: true, amount: 3300, connection: 'active' },
  { name: 'Mystery Wallet', bankType: 'checking', kind: 'cash', balanceKnown: false, amount: 0, connection: 'active' },
  { name: 'Travel Rewards Card', bankType: 'credit card', kind: 'credit', balanceKnown: true, amount: 450, connection: 'reconnect' },
  { name: 'Brokerage', bankType: 'brokerage', kind: 'other', balanceKnown: true, amount: 9999, connection: 'active' },
];

const EXPECTED_TOTAL_CASH = '$4,500.00';
const EXPECTED_TOTAL_DEBT = '$450.00';
const EXPECTED_NET_CASH = '$4,050.00';

test('Seeded overview', async ({ page }) => {
  seedOverview(SEED);

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  // PC1: rendered totals match the service's derivation (cash − credit;
  // other + unknown excluded).
  await expect(page.getByTestId('accounts-overview-net-cash')).toHaveText(EXPECTED_NET_CASH);
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveText(EXPECTED_TOTAL_CASH);
  await expect(page.getByTestId('accounts-overview-total-debt')).toHaveText(EXPECTED_TOTAL_DEBT);

  // Each kind group renders. Cash holds its three accounts (two known + the
  // unknown), credit holds the one card.
  const cashGroup = page.getByTestId('accounts-overview-cash');
  const creditGroup = page.getByTestId('accounts-overview-credit');
  await expect(cashGroup).toBeVisible();
  await expect(creditGroup).toBeVisible();
  await expect(cashGroup.getByTestId('accounts-overview-account-row')).toHaveCount(3);
  await expect(creditGroup.getByTestId('accounts-overview-account-row')).toHaveCount(1);

  // The other section is the excluded one — present, labelled, holding the
  // brokerage account, and explicitly outside the net cash position.
  const otherSection = page.getByTestId('accounts-overview-other');
  await expect(otherSection).toBeVisible();
  await expect(otherSection.getByTestId('accounts-overview-account-row')).toHaveCount(1);
  await expect(otherSection).toContainText('Excluded from net cash');

  // The unknown-balance account renders an em dash, never $0. Its balance cell
  // lives in the cash group (it is a cash account whose balance is unreported).
  const cashBalances = cashGroup.getByTestId('accounts-overview-account-balance');
  const cashBalanceTexts = await cashBalances.allInnerTexts();
  expect(cashBalanceTexts, 'unknown-balance cash account must show an em dash').toContain('—');
  for (const t of cashBalanceTexts) {
    expect(t.trim(), 'an unknown balance must never render as $0.00').not.toBe('$0.00');
  }

  // The needs-reconnect badge is shown (the credit card hangs off the
  // needs_reconnect connection). Exactly one account is on that connection.
  const badges = page.getByTestId('accounts-overview-needs-reconnect');
  await expect(badges).toHaveCount(1);
  await expect(badges.first()).toBeVisible();

  // Save a full-page screenshot of the seeded overview for review (PC1 proof).
  await page.screenshot({ path: 'tmp/overview.png', fullPage: true });
});

test('Empty state', async ({ page }) => {
  resetAccounts();

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  // PC3: the empty state stands in cleanly — no headline/totals chrome, no
  // zeroed-out group sections.
  await expect(page.getByTestId('accounts-overview-empty')).toBeVisible();
  await expect(page.getByTestId('accounts-overview-headline')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-net-cash')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-total-debt')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-cash')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-credit')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-other')).toHaveCount(0);
});
