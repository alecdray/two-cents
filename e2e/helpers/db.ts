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
  return execSync(`sqlite3 ${DB_PATH} ${JSON.stringify(sql)}`, { encoding: 'utf8' });
}

// resetAccounts wipes every account and connection, leaving the overview in the
// "no accounts linked" shape that drives the empty state. Accounts go first to
// respect the connection_id foreign key.
export function resetAccounts() {
  execSql(`DELETE FROM accounts; DELETE FROM connections;`);
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
      ` 0, 0, 0,` +
      ` ${a.amount}, 'USD', ${a.balanceKnown ? 1 : 0}, '${a.state ?? 'active'}'` +
      `);`,
  );
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
