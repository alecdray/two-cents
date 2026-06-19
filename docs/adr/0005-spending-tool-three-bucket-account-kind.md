# Two Cents is a spending tool — three-bucket account kind

Two Cents is a spending / cash-flow tool, not a net-worth tracker. Every Account carries a **kind** — `cash`, `credit`, or `other` — seeded from the provider's reported type (depository → `cash`, credit → `credit`, everything else → `other`) and user-overridable. **Net cash = Σ cash balances (savings included) − Σ credit balances owed**, excluding `other` accounts (and hidden, closed, or unknown-balance ones).

A single bank login routinely surfaces a 401(k), brokerage, car loan, or mortgage. Forcing those into `cash` or `credit` would make the headline net-cash figure lie — inflating it with money that can't be spent, or counting a debt as a card balance. The third bucket lets us store and list those accounts honestly (the user connected a bank and expects to see everything) while holding them out of the cash-flow position. Because kind is overridable, a user who treats a particular `other` account as spendable can move it to `cash`.

Rejected: a `cash | credit` binary that drops every other account (loses accounts the user can plainly see in their bank); folding investments/loans into net cash (turns a spending tool into a net-worth tracker); a per-account "count in net cash?" boolean instead of a typed kind (a flag carries no vocabulary about *why* an account is excluded).
