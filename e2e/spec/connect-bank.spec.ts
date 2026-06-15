import { test, expect } from '@playwright/test';
import { resetAccounts } from '../helpers/db';

// Scenarios from e2e/feat/connect-bank.feature

// The app runs in fake-bank mode (BANK_PROVIDER=fake), so the connect control
// posts directly and the deterministic stand-in returns its fixed account set:
//   - Everyday Checking   (cash,   $1,200.00)
//   - High-Yield Savings  (cash,   $3,400.00)
//   - Travel Rewards Card (credit, $450.00)
// Net cash = (1200 + 3400) - 450 = 4150; total cash = 4600; total debt = 450.
const EXPECTED_TOTAL_CASH = '$4,600.00';
const EXPECTED_TOTAL_DEBT = '$450.00';
const EXPECTED_NET_CASH = '$4,150.00';

test('Linking a bank from the empty overview reveals its accounts', async ({ page }) => {
  resetAccounts();

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  // The empty overview offers the connect control as its primary CTA.
  await expect(page.getByTestId('accounts-overview-empty')).toBeVisible();
  const connect = page.getByTestId('accounts-overview-connect');
  await expect(connect).toBeVisible();

  // Link the fake bank: the form posts and the overview region swaps in place.
  await connect.getByRole('button').click();

  // The linked bank's cash accounts land in the cash group.
  const cashGroup = page.getByTestId('accounts-overview-cash');
  await expect(cashGroup).toBeVisible();
  await expect(cashGroup).toContainText('Everyday Checking');
  await expect(cashGroup).toContainText('High-Yield Savings');

  // The credit card lands in the credit group.
  const creditGroup = page.getByTestId('accounts-overview-credit');
  await expect(creditGroup).toBeVisible();
  await expect(creditGroup).toContainText('Travel Rewards Card');

  // The empty state is gone and the position reflects the linked accounts.
  await expect(page.getByTestId('accounts-overview-empty')).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-net-cash')).toHaveText(EXPECTED_NET_CASH);
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveText(EXPECTED_TOTAL_CASH);
  await expect(page.getByTestId('accounts-overview-total-debt')).toHaveText(EXPECTED_TOTAL_DEBT);

  // The connect control persists as the "add account" affordance.
  await expect(page.getByTestId('accounts-overview-connect')).toBeVisible();
});
