import { execSync } from 'node:child_process';

// The accounts overview suite needs to position the real DB into specific
// connection/account shapes (mixed kinds, an unknown balance, a
// needs-reconnect connection) that no public API on the running app exposes —
// the app only writes accounts via a live Plaid enrollment, which the suite
// deliberately never touches. Direct sqlite3 shell-out keeps the helper
// dependency-free and honours the suite's "real backend, no mocks" rule: we
// seed the same SQLite file the app reads from.

const DB_PATH = process.env.GOOSE_DBSTRING ?? './tmp/db.sql';

function execSql(sql: string): string {
  // The running app holds the same SQLite file open, and Playwright runs spec
  // files across parallel workers, so a seeder's write can briefly collide with
  // an app write. A busy timeout makes the sqlite3 CLI wait for the lock to
  // clear instead of failing instantly with "database is locked".
  return execSync(`sqlite3 ${DB_PATH} "PRAGMA busy_timeout=5000;" ${JSON.stringify(sql)}`, {
    encoding: 'utf8',
  });
}

// resetAccounts wipes every account and connection, leaving the overview in the
// "no accounts linked" shape that drives the empty state. Accounts go first to
// respect the connection_id foreign key.
export function resetAccounts() {
  execSql(`DELETE FROM accounts; DELETE FROM connections;`);
}

// resetActivity wipes every transaction, sync cursor, account, and connection —
// the fully clean slate the transactions surface starts each scenario from.
// Transactions and cursors go before accounts/connections they reference.
export function resetActivity() {
  execSql(`DELETE FROM transactions; DELETE FROM transaction_sync_state;`);
  resetAccounts();
}

// resetCategorization clears the user-created categorization state — every Rule
// and every custom (non-built-in) Category — leaving the seeded built-in taxonomy
// intact. The shared DB persists across runs, so a categorization scenario resets
// this first to start from the known built-in-only baseline.
export function resetCategorization() {
  execSql(`DELETE FROM rules; DELETE FROM categories WHERE builtin = 0;`);
}

// resetBudget clears the single rolling budget config and every per-Category
// limit, leaving the budget editor in its "no budget set" baseline (all-zero
// targets, no limits). The shared DB persists across runs, so a budget scenario
// resets this first to start from a known empty plan.
export function resetBudget() {
  execSql(`DELETE FROM budget_category_limits; DELETE FROM budget;`);
}

// seedConnectionWithoutActivity resets everything then inserts one active
// connection with a single cash account and no transactions — the
// connected-but-nothing-synced shape that drives the "nothing synced yet" empty
// state. The dummy access token is fine here: the transactions page reads the
// account list and stored transactions only, and never decrypts the token.
export function seedConnectionWithoutActivity() {
  resetActivity();
  seedConnection('conn-active', 'active');
  seedAccount('acct-0', 'conn-active', {
    name: 'Everyday Checking',
    bankType: 'checking',
    kind: 'cash',
    balanceKnown: true,
    amount: 1200,
    connection: 'active',
  });
}

export type SeedAccount = {
  name: string;
  bankType: string;
  kind: 'cash' | 'credit' | 'other';
  // balanceKnown=false renders the row's balance as an em dash and excludes it
  // from the overview totals (never counted as zero).
  balanceKnown: boolean;
  amount: number;
  // Which connection this account hangs off — drives the needs-reconnect badge.
  connection: 'active' | 'reconnect';
  // Account display lifecycle; defaults to 'active'. Non-active accounts are
  // dropped from the overview entirely.
  state?: 'active' | 'hidden' | 'closed';
  // Whether the account is counted as savings; defaults to false. Drives the
  // overview's counts-as-savings toggle state (cash/other rows only).
  countsAsSavings?: boolean;
};

// seedConnection inserts one connection row in the given state. The encrypted
// access_token column is a non-null dummy — the overview page never decrypts
// it.
function seedConnection(id: string, state: 'active' | 'needs_reconnect') {
  execSql(
    `INSERT INTO connections (id, item_id, access_token, state)` +
      ` VALUES ('${id}', 'item-${id}', 'dummy-encrypted-token', '${state}');`,
  );
}

// seedAccount inserts one account row under the named connection.
function seedAccount(id: string, connectionId: string, a: SeedAccount) {
  execSql(
    `INSERT INTO accounts (` +
      `id, connection_id, provider_account_id, name, bank_type, kind,` +
      ` kind_overridden, counts_as_savings, savings_overridden,` +
      ` balance_amount, balance_currency, balance_known, state` +
      `) VALUES (` +
      `'${id}', '${connectionId}', 'prov-${id}', '${a.name}', '${a.bankType}', '${a.kind}',` +
      ` 0, ${a.countsAsSavings ? 1 : 0}, 0,` +
      ` ${a.amount}, 'USD', ${a.balanceKnown ? 1 : 0}, '${a.state ?? 'active'}'` +
      `);`,
  );
}

// markConnectionsNeedsReconnect flips every connection to the needs_reconnect
// state in place, leaving its stored (encrypted) access_token untouched. Use it
// after linking the fake bank through the UI so the connection's token stays a
// real, decryptable value — the reconnect flow decrypts it to call the provider,
// so a dummy token would break it. (Contrast seedConnection, whose dummy token
// is fine only for the overview page, which never decrypts.)
export function markConnectionsNeedsReconnect() {
  execSql(`UPDATE connections SET state = 'needs_reconnect';`);
}

// seedUnpairedTransfer inserts one posted outflow Transfer on the linked fake
// checking account with no matching inflow leg, so the auto-pairing pass leaves
// its destination unknown — the row the mark/correct flow targets. It must run
// after the fake bank is linked (the account it hangs off must already exist); the
// account is resolved by the fake provider_account_id. The amount and date are
// deliberately unlike any canned transfer so nothing accidentally pairs to it.
// classification is set to 'transfer' directly (the row is never in a sync pull,
// so the on-sync auto-categorize never revisits it).
export function seedUnpairedTransfer(opts: {
  id: string;
  merchant: string;
  amount: number;
  date: string;
}) {
  execSql(
    `INSERT INTO transactions (` +
      `id, account_id, date, amount_amount, amount_currency, merchant, counterparty,` +
      ` category_primary, category_detailed, status, classification,` +
      ` transfer_subtype, transfer_destination_overridden` +
      `) VALUES (` +
      `'${opts.id}', (SELECT id FROM accounts WHERE provider_account_id = 'fake-checking'),` +
      ` '${opts.date}', ${opts.amount}, 'USD', '${opts.merchant}', '${opts.merchant}',` +
      ` 'TRANSFER_OUT', 'TRANSFER_OUT_WITHDRAWAL', 'posted', 'transfer',` +
      ` 'plain', 0` +
      `);`,
  );
}

// seedTransfer inserts one posted outflow Transfer on a given account id — the
// directly-seeded analogue of seedUnpairedTransfer that needs no fake bank (so no
// token decryption). It hangs the leg off the account id the caller seeded via
// seedOverview (e.g. 'acct-0'), classified 'transfer' with a 'plain' subtype and
// no destination override, so the transactions surface renders it and its editor
// opens the transfer-destination picker. Amount is a positive outflow.
export function seedTransfer(opts: {
  id: string;
  accountId: string;
  merchant: string;
  amount: number;
  date: string;
}) {
  execSql(
    `INSERT INTO transactions (` +
      `id, account_id, date, amount_amount, amount_currency, merchant, counterparty,` +
      ` category_primary, category_detailed, status, classification,` +
      ` transfer_subtype, transfer_destination_overridden` +
      `) VALUES (` +
      `'${opts.id}', '${opts.accountId}',` +
      ` '${opts.date}', ${opts.amount}, 'USD', '${opts.merchant}', '${opts.merchant}',` +
      ` 'TRANSFER_OUT', 'TRANSFER_OUT_WITHDRAWAL', 'posted', 'transfer',` +
      ` 'plain', 0` +
      `);`,
  );
}

export type SeedRule = {
  id: string;
  merchantSubstring: string;
  classification: 'spending' | 'income' | 'transfer';
  // The built-in Category id a spending rule targets (e.g. 'food_and_drink');
  // omitted for an income/transfer rule, which carries none.
  categoryId?: string;
};

// seedRules clears the user categorization state then inserts the given Rules
// directly, so an edit / delete scenario starts from a known Rule without driving
// the create modal first. The seeded built-in taxonomy is left intact, so a spending
// rule can reference a built-in Category id. Pass an empty list for the no-rules
// baseline the empty state and a fresh create start from.
export function seedRules(rules: SeedRule[]) {
  resetCategorization();
  rules.forEach((r) => {
    const category = r.categoryId ? `'${r.categoryId}'` : 'NULL';
    execSql(
      `INSERT INTO rules (id, merchant_substring, classification, category_id)` +
        ` VALUES ('${r.id}', '${r.merchantSubstring}', '${r.classification}', ${category});`,
    );
  });
}

// seedTransactions resets activity then seeds one active connection + cash account
// and the given uncategorized transactions (empty classification — the transient
// pre-categorization state). Creating a Rule whose substring matches one then
// re-categorizes it, so the modal save surfaces a real "N transactions
// re-categorized" count. The merchant is stored verbatim as both merchant and
// counterparty; the amount is a positive outflow.
export function seedTransactions(txns: { id: string; merchant: string; amount: number }[]) {
  resetActivity();
  seedConnection('conn-rules', 'active');
  seedAccount('acct-rules', 'conn-rules', {
    name: 'Everyday Checking',
    bankType: 'checking',
    kind: 'cash',
    balanceKnown: true,
    amount: 1000,
    connection: 'active',
  });
  txns.forEach((t) => {
    execSql(
      `INSERT INTO transactions (` +
        `id, account_id, date, amount_amount, amount_currency, merchant, counterparty,` +
        ` category_primary, category_detailed, status, classification` +
        `) VALUES (` +
        `'${t.id}', 'acct-rules', '2026-06-01', ${t.amount}, 'USD', '${t.merchant}', '${t.merchant}',` +
        ` '', '', 'posted', ''` +
        `);`,
    );
  });
}

// The application timezone (ADR-0004, core/app default) that decides which
// calendar month the app treats as "now". The month-navigation rail and the
// current-month wrap redirect are reckoned in this zone, so the month slugs the
// specs build (and the prior-month seed dates) must be computed in it too — not
// in the test runner's local zone, which can disagree at a month boundary.
const APP_TIMEZONE = 'America/New_York';

// monthYM returns the YYYY-MM slug of the month `offset` calendar months from the
// current month, reckoned in the app timezone. offset 0 is the current month
// (the one the app's `/` Tracker owns); -1 is the prior month (which has a real
// wrap page). Computing in the app zone keeps the slug in lockstep with the
// server's `timex.CurrentMonth`, so a current-month URL redirects and a
// prior-month URL renders its wrap regardless of the runner's local zone.
export function monthYM(offset: number): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: APP_TIMEZONE,
    year: 'numeric',
    month: '2-digit',
  }).formatToParts(new Date());
  let year = Number(parts.find((p) => p.type === 'year')!.value);
  let month = Number(parts.find((p) => p.type === 'month')!.value) + offset; // 1-based
  while (month < 1) {
    month += 12;
    year -= 1;
  }
  while (month > 12) {
    month -= 12;
    year += 1;
  }
  return `${year}-${String(month).padStart(2, '0')}`;
}

// currentMonthYM / priorMonthYM name the two months the wrap suite navigates: the
// current month (its wrap address redirects to `/`) and the prior month (a real
// wrap page seeded by seedPriorMonthWrap).
export function currentMonthYM(): string {
  return monthYM(0);
}
export function priorMonthYM(): string {
  return monthYM(-1);
}

// PriorMonthWrap is the deterministic figure set seedPriorMonthWrap plants, so a
// spec asserts against these exact rendered strings rather than re-deriving them.
export type PriorMonthWrap = {
  ym: string;
  grossIncome: string;
  spending: string;
  netIncome: string;
  savingsContributed: string;
  surplus: string;
  generalMerchandise: string;
  foodAndDrink: string;
  monthRowCount: number;
  // grossIncomeAfterSidegig is the gross income once the seeded needs-review
  // inflow is re-categorized to Income (the edit-refresh scenario).
  grossIncomeAfterSidegig: string;
};

// priorWrapRow inserts one fully-classified transaction dated in the prior month
// on the seeded wrap account. Unlike the current-month seeders (which leave
// classification blank for the on-sync ladder to resolve), a wrap's figures need
// resolved classifications up front — this row is never in a sync pull, so
// nothing re-categorizes it. categoryId is a built-in Category id or null;
// subtype is '' unless the row is a transfer leg.
function priorWrapRow(opts: {
  id: string;
  date: string;
  amount: number;
  merchant: string;
  primary: string;
  detailed: string;
  status: 'posted' | 'pending';
  classification: string;
  categoryId: string | null;
  subtype: string;
}) {
  const categoryId = opts.categoryId ? `'${opts.categoryId}'` : 'NULL';
  // The app stores transaction dates as full timestamps at UTC midnight
  // ("YYYY-MM-DD 00:00:00+00:00", the go-sqlite3 text form). The month-range
  // queries compare t.date against that same textual form, so a bare "YYYY-MM-DD"
  // sorts BEFORE the range start at the 1st-of-month boundary and the row drops.
  // Match the stored format exactly so every seeded day is bucketed correctly.
  const date = `${opts.date} 00:00:00+00:00`;
  execSql(
    `INSERT INTO transactions (` +
      `id, account_id, date, amount_amount, amount_currency, merchant, counterparty,` +
      ` category_primary, category_detailed, status, classification, category_id,` +
      ` transfer_subtype, transfer_destination_overridden` +
      `) VALUES (` +
      `'${opts.id}', 'acct-wrap', '${date}', ${opts.amount}, 'USD', '${opts.merchant}', '${opts.merchant}',` +
      ` '${opts.primary}', '${opts.detailed}', '${opts.status}', '${opts.classification}', ${categoryId},` +
      ` '${opts.subtype}', 0` +
      `);`,
  );
}

// seedPriorMonthWrap plants a small, deterministic, fully-classified transaction
// set dated in the PRIOR calendar month, so a real wrap page renders there (the
// current month no longer has one — its wrap address redirects to `/`). It resets
// activity and user categorization first, seeds one active connection + cash
// account, then inserts: a $2,000 paycheck (Income), a $120 grocery outflow
// (Spending / General Merchandise), a $30 pending coffee outflow (Spending / Food
// & Drink — the pending row makes the wrap settling), a $300 savings-contribution
// source leg plus its plain mirror inflow (only the source counts as savings), and
// a $150 needs-review inflow (the row the edit-refresh scenario re-categorizes to
// Income). Being the only month with transactions, the prior month sits at the
// backfill edge, so its wrap is partial and the rail spans prior→current (two
// chips). Returns the exact rendered figure strings for assertions.
export function seedPriorMonthWrap(): PriorMonthWrap {
  const ym = priorMonthYM();
  resetActivity();
  resetCategorization();
  seedConnection('conn-wrap', 'active');
  seedAccount('acct-wrap', 'conn-wrap', {
    name: 'Everyday Checking',
    bankType: 'checking',
    kind: 'cash',
    balanceKnown: true,
    amount: 1000,
    connection: 'active',
  });

  priorWrapRow({
    id: 'wrap-groceries',
    date: `${ym}-01`,
    amount: 120.0,
    merchant: 'Whole Foods',
    primary: 'GENERAL_MERCHANDISE',
    detailed: 'GENERAL_MERCHANDISE_SUPERSTORES',
    status: 'posted',
    classification: 'spending',
    categoryId: 'general_merchandise',
    subtype: '',
  });
  priorWrapRow({
    id: 'wrap-paycheck',
    date: `${ym}-02`,
    amount: -2000.0,
    merchant: 'Acme Payroll',
    primary: 'INCOME',
    detailed: 'INCOME_WAGES',
    status: 'posted',
    classification: 'income',
    categoryId: null,
    subtype: '',
  });
  priorWrapRow({
    id: 'wrap-coffee',
    date: `${ym}-03`,
    amount: 30.0,
    merchant: 'Blue Bottle Coffee',
    primary: 'FOOD_AND_DRINK',
    detailed: 'FOOD_AND_DRINK_COFFEE',
    status: 'pending',
    classification: 'spending',
    categoryId: 'food_and_drink',
    subtype: '',
  });
  priorWrapRow({
    id: 'wrap-savings',
    date: `${ym}-04`,
    amount: 300.0,
    merchant: 'Rainy Day Savings',
    primary: 'TRANSFER_OUT',
    detailed: 'TRANSFER_OUT_SAVINGS',
    status: 'posted',
    classification: 'transfer',
    categoryId: null,
    subtype: 'savings_contribution',
  });
  priorWrapRow({
    id: 'wrap-savings-mirror',
    date: `${ym}-04`,
    amount: -300.0,
    merchant: 'Transfer from Checking',
    primary: 'TRANSFER_IN',
    detailed: 'TRANSFER_IN_ACCOUNT_TRANSFER',
    status: 'posted',
    classification: 'transfer',
    categoryId: null,
    subtype: 'plain',
  });
  priorWrapRow({
    id: 'wrap-sidegig',
    date: `${ym}-05`,
    amount: -150.0,
    merchant: 'Side Hustle Co',
    primary: '',
    detailed: '',
    status: 'posted',
    classification: 'needs_review',
    categoryId: null,
    subtype: '',
  });

  return {
    ym,
    // Gross income = the $2,000 paycheck alone.
    grossIncome: '$2,000.00',
    // Total spending = $120 General Merchandise + $30 Food & Drink.
    spending: '$150.00',
    // Net income = $2,000 income - ($120 + $30) spending.
    netIncome: '$1,850.00',
    savingsContributed: '$300.00',
    // Surplus = net income ($1,850) - savings contributed ($300).
    surplus: '$1,550.00',
    generalMerchandise: '$120.00',
    foodAndDrink: '$30.00',
    monthRowCount: 6,
    // Re-categorizing the $150 side-gig inflow to Income lifts gross income to $2,150.
    grossIncomeAfterSidegig: '$2,150.00',
  };
}

// seedOverview resets the DB then seeds two connections (one active, one
// needs_reconnect) and the given accounts. Accounts on the 'reconnect'
// connection inherit its needs-reconnect state, surfacing the badge on the
// overview.
export function seedOverview(accounts: SeedAccount[]) {
  resetAccounts();
  seedConnection('conn-active', 'active');
  seedConnection('conn-reconnect', 'needs_reconnect');
  accounts.forEach((a, i) => {
    const connectionId = a.connection === 'reconnect' ? 'conn-reconnect' : 'conn-active';
    seedAccount(`acct-${i}`, connectionId, a);
  });
}
