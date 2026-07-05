# reporting — utility module

Rules: ../../../docs/architecture/archetypes/utility.md

Pure month-wrap projection: given a month's transaction rows it derives net income, gross income, savings contributed, spend-by-Category, and the settling/final state. **Actuals only** — it never reads or compares against a Budget.

Module-specific notes:
- Pure leaf: no `Service`, no `repo.go`, no persistence; imports no domain package. Inputs are local structs with raw Category ids (`*string`; nil = uncategorized) and money as signed integer cents; the composing `home` module fetches the rows, fills these structs, and joins Category names afterward.
- Money sign (inherited from `banking`): outflow positive, inflow negative. Spending is summed **signed** (a refund reduces spend); income legs are negated to a positive total; savings-contribution source legs are positive outflows. Gross income is the income legs alone; net income subtracts spending. Transfers are excluded from both income and spending.
- **Surplus** ([glossary](../../../docs/domain/README.md)) is net income minus savings contributed (i.e. income − spend − savings) — the month's income left unallocated after spending and saving.
