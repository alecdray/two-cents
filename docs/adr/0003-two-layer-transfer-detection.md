# Two-layer transfer detection

Transfers (money moved between the user's own accounts, excluded from spending and income) are detected in two layers: (1) **classification** from the primary level of the bank's `personal_finance_category` on a single transaction, no pairing; (2) **destination + subtype** (savings contribution / credit-card payment / plain transfer) by pairing the inflow leg on another connected account — exact amount, ±3-day window.

The second layer exists because the provider's schema gives the category and a raw `counterparty` but **no reference to the destination account** — so we cheaply know *that* a transaction is a transfer, never *where it went*, and where it went is what decides a savings contribution. Pairing is deliberately conservative (exact amount, tight window, ambiguous matches left unmatched), because a false pair silently hides real spending — worse than a missed pair the user can correct. A transfer to an account not connected to Two Cents stays unresolved (one leg only) until the user marks it, and a manual override always wins over both layers.

Rejected: pure pairing to detect transfers (fragile, and unneeded once the category classifies most); trusting `counterparty` alone (unstructured, no account identity).
