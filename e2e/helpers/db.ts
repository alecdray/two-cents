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
