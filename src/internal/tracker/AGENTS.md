# tracker — utility module

Rules: ../../../docs/architecture/archetypes/utility.md

Pure current-month Tracker projection: given the month's per-Category spend, income, savings, the Budget config, and days-left, it derives per-Category remaining + pace targets, the everything-else residual, the total pace, and income/savings progress.

Module-specific notes:
- Pure leaf: no `Service`, no `repo.go`, no persistence; imports no domain package. Inputs and outputs are local structs in signed integer cents; the composing `home` module fetches the data, fills these structs, and joins Category names afterward.
- Budget-relative: unlike `reporting` (actuals-only), the Tracker compares actuals against the Budget config the composer passes in; with no budget it reports actuals only (`NeedsBudget`). It carries no Surplus — that is a closed-month figure owned by `reporting`/the wrap ([glossary](../../../docs/domain/README.md)).
