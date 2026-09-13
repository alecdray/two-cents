import { test, expect } from '@playwright/test';
import { seedOverview, resetAccounts, type SeedAccount } from '../helpers/db';

// Scenarios from e2e/feat/accounts-overview.feature

// The seeded fixture spans every property the overview must demonstrate:
//   - one spendable cash account with a known balance   -> 1200
//   - one cash account flagged counts-as-savings        -> 3300 (the total savings)
//   - one cash account with an UNKNOWN balance          -> shown as em dash, excluded
//   - one credit account on the reconnect conn          -> total debt 450, badge shown
//   - one OTHER account (brokerage)                      -> shown but excluded
// Total cash = 1200 + 3300 = 4500; net cash = 4500 - 450 = 4050; total savings =
// 3300; free cash = net cash - total savings = 4050 - 3300 = 750.
const SEED: SeedAccount[] = [
  { name: 'Everyday Checking', bankType: 'checking', kind: 'cash', balanceKnown: true, amount: 1200, connection: 'active' },
  { name: 'High-Yield Savings', bankType: 'savings', kind: 'cash', balanceKnown: true, amount: 3300, connection: 'active', countsAsSavings: true },
  { name: 'Mystery Wallet', bankType: 'checking', kind: 'cash', balanceKnown: false, amount: 0, connection: 'active' },
  { name: 'Travel Rewards Card', bankType: 'credit card', kind: 'credit', balanceKnown: true, amount: 450, connection: 'reconnect' },
  { name: 'Brokerage', bankType: 'brokerage', kind: 'other', balanceKnown: true, amount: 9999, connection: 'active' },
];

const EXPECTED_TOTAL_CASH = '$4,500.00';
const EXPECTED_TOTAL_DEBT = '$450.00';
const EXPECTED_NET_CASH = '$4,050.00';
const EXPECTED_TOTAL_SAVINGS = '$3,300.00';
const EXPECTED_FREE_CASH = '$750.00';

test('Seeded overview', async ({ page }) => {
  seedOverview(SEED);

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  // PC1: rendered totals match the service's derivation (cash − credit;
  // other + unknown excluded). Free cash headlines (net cash − total savings);
  // net cash, total savings, total cash, and total debt support it.
  await expect(page.getByTestId('accounts-overview-free-cash')).toHaveText(EXPECTED_FREE_CASH);
  await expect(page.getByTestId('accounts-overview-net-cash')).toHaveText(EXPECTED_NET_CASH);
  await expect(page.getByTestId('accounts-overview-total-savings')).toHaveText(EXPECTED_TOTAL_SAVINGS);
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

test('Hide and unhide an account', async ({ page }) => {
  seedOverview(SEED);

  await page.goto('/accounts');
  await expect(page.getByTestId('accounts-overview-page')).toBeVisible();

  const cashGroup = page.getByTestId('accounts-overview-cash');
  const hiddenSection = page.getByTestId('accounts-overview-hidden');

  // Baseline: three cash rows, nothing hidden, total cash $4,500.
  await expect(cashGroup.getByTestId('accounts-overview-account-row')).toHaveCount(3);
  await expect(hiddenSection).toHaveCount(0);
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveText(EXPECTED_TOTAL_CASH);

  // Hide the first active account — Everyday Checking ($1,200), the first cash
  // row in seed order. The HTMX swap re-renders the region.
  await page.getByTestId('accounts-overview-account-hide').first().click();

  // It leaves the cash group for the Hidden section, and total cash drops its
  // $1,200 (4,500 - 1,200 = 3,300).
  await expect(hiddenSection).toBeVisible();
  await expect(hiddenSection.getByTestId('accounts-overview-hidden-row')).toHaveCount(1);
  await expect(cashGroup.getByTestId('accounts-overview-account-row')).toHaveCount(2);
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveText('$3,300.00');

  // Unhide it: the only unhide control returns it to the cash group, the Hidden
  // section disappears, and total cash is whole again.
  await page.getByTestId('accounts-overview-account-unhide').click();

  await expect(page.getByTestId('accounts-overview-hidden')).toHaveCount(0);
  await expect(cashGroup.getByTestId('accounts-overview-account-row')).toHaveCount(3);
  await expect(page.getByTestId('accounts-overview-total-cash')).toHaveText(EXPECTED_TOTAL_CASH);
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

// A card whose bank reported a statement: billed 500 of a 840 balance, issued
// five days ago, payment due twelve days out.
const CARD_WITH_STATEMENT: SeedAccount = {
  name: 'Sapphire Card',
  bankType: 'credit card',
  kind: 'credit',
  balanceKnown: true,
  amount: 840,
  connection: 'active',
  statement: { billed: 500, issuedDaysFromNow: -5, dueDaysFromNow: 12 },
};

test('A card shows what its statement takes and when', async ({ page }) => {
  resetAccounts();
  seedOverview([CARD_WITH_STATEMENT]);

  await page.goto('/accounts');

  // The billed figure, not the 840 balance: the rest was spent this cycle and
  // has no due date yet.
  await expect(page.getByTestId('accounts-overview-card-payment-due')).toContainText('$500.00');
  await expect(page.getByTestId('accounts-overview-card-statement-unavailable')).toHaveCount(0);
});

test('Choosing when a card is paid', async ({ page }) => {
  resetAccounts();
  seedOverview([CARD_WITH_STATEMENT]);

  // Both dates come from the server's own rendering and are compared against
  // each other, never against a calendar recomputed in Node: the runner's
  // timezone need not match the app's, and a run crossing local midnight would
  // otherwise compare against a different day than was seeded.
  const paymentDue = () => page.getByTestId('accounts-overview-card-payment-due').innerText();

  // "$500.00 due Sep 8" -> a Date in a fixed year, so two rendered dates can be
  // differenced. A wrap (Dec -> Jan) shows up as a negative span and is undone.
  const renderedDay = (text: string): Date => {
    const m = text.match(/due ([A-Z][a-z]{2}) (\d{1,2})$/);
    if (!m) throw new Error(`payment-due text not in the expected shape: ${text}`);
    return new Date(`${m[1]} ${m[2]}, 2000 00:00:00`);
  };
  const daysBetween = (from: string, to: string): number => {
    const span = (renderedDay(to).getTime() - renderedDay(from).getTime()) / 86400000;
    return span < 0 ? span + 365 : span;
  };

  await page.goto('/accounts');
  // On the due date to begin with: twelve days out, not reckoned from the
  // statement at all.
  const onDueDate = await paymentDue();

  // Paying a fixed number of days after the statement issues, rather than on
  // the due date — autopay pulls when it is configured to.
  // Wait on the settle belonging to *this* swap, not on any settle: the offset
  // input enters the DOM a moment before HTMX binds its change trigger, and a
  // fill landing in that window posts nothing at all. A bare counter would also
  // be satisfied by an unrelated settle, so the promise is armed for the next
  // one and awaited straight after the action that causes it.
  const modeSettled = page.evaluate(
    () => new Promise((resolve) => document.body.addEventListener('htmx:afterSettle', resolve, { once: true })),
  );
  await page.getByTestId('accounts-overview-card-payment-mode').selectOption('statement_plus_days');
  await modeSettled;

  // The offset starts at zero, so the payment now lands on the issue date —
  // five days before the run, and necessarily different from the due date it
  // started on.
  await expect(page.getByTestId('accounts-overview-card-payment-due')).not.toHaveText(onDueDate);
  const atIssueDate = await paymentDue();
  // The seeded statement issued five days before the due date is twelve days
  // out — seventeen days apart, which is what switching off the due date reveals.
  expect(daysBetween(atIssueDate, onDueDate)).toBe(17);

  // Tie the interaction to its request rather than to the DOM alone: the swap
  // above puts the input in the document a moment before HTMX binds its change
  // trigger, so a fill that lands in that window silently posts nothing. Waiting
  // on the response makes that failure loud and immediate instead of surfacing
  // later as a stale date.
  const saved = page.waitForResponse(
    (r) => r.url().includes('/payment-schedule') && r.request().method() === 'POST',
  );
  const offset = page.getByTestId('accounts-overview-card-payment-offset');
  await offset.fill('3');
  await offset.blur();
  await saved;

  // The statement issued five days ago, so paying three days after it issued
  // re-dates the payment to two days ago — already due, and nothing like the
  // reported due date it started on.
  await expect(page.getByTestId('accounts-overview-card-payment-offset')).toHaveValue('3');
  // Exactly three days past the statement's issue date — the offset applied,
  // not merely some other date.
  await expect(page.getByTestId('accounts-overview-card-payment-due')).not.toHaveText(atIssueDate);
  expect(daysBetween(atIssueDate, await paymentDue())).toBe(3);
});

test('A bank that will not share statement detail says so on the card', async ({ page }) => {
  resetAccounts();
  seedOverview([
    { ...CARD_WITH_STATEMENT, statement: undefined, statementsUnavailable: true },
  ]);

  await page.goto('/accounts');

  await expect(page.getByTestId('accounts-overview-card-statement-unavailable')).toBeVisible();
  // The gap is actionable where it appears, not just explained.
  await expect(page.getByTestId('accounts-overview-card-reauthorize')).toBeVisible();
  // The login still works: no reconnect badge on a connection that served
  // everything else it was asked for.
  await expect(page.getByTestId('accounts-overview-needs-reconnect')).toHaveCount(0);
});
