# Sweep reserve — scope

## Goal

Provide a sweep number, on demand, telling the user how much money to move **into or out of
savings** to maintain a safe checking balance while maximizing interest earned in savings.

## Entities to consider

Each attribute notes where it comes from, and flags anything we do not hold today.

- **Checking account** — the single active cash account not marked counts-as-savings
  - Current balance (synced; may be unknown, may be stale)
  - Month-to-date spending that left this account
  - Month-to-date savings contributions made from it
  - Month-to-date income received — today the ledger reads income **across all
    accounts only**; there is no per-account income read

- **Savings account** — the single active cash account marked counts-as-savings
  - Current balance (synced; may be unknown or stale — today this does not block)
  - Interest rate / APY — **not held today**, and the goal names interest explicitly
  - Minimum balance or withdrawal limits — **not held today**

- **Credit cards** — every active credit account, summed
  - Current balance = amount owed right now (synced)
  - Statement balance = what the last closed cycle billed — **not held today** (liabilities)
  - Payment due date — **not held today** (liabilities)
  - Minimum payment — **not held today** (liabilities)
  - Autopay setting and amount — **not available from the provider at all**

- **Budget** — one rolling config, no per-month history
  - Monthly income target
  - Monthly savings target
  - Total spending budget (= income − savings)
  - Per-category spending limits

- **Transactions** — the ledger behind the month-to-date figures
  - Spending (by account, or across all accounts)
  - Income
  - Transfers, including savings contributions
  - Card payments — **not distinguishable from any other transfer today**

- **Time**
  - The run instant (a snapshot can be produced at any moment)
  - Current month boundaries, in the configured app timezone
  - Pay dates — **not held today**

- **Configuration**
  - Fixed safety margin (default $500)

## Not decided here

The formula, the horizon it covers, which entities are obligations, and how income enters.
Those come next, derived against this inventory.
