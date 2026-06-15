# home — composing module

Rules: ../../../docs/architecture/archetypes/domain-module.md (domain-module-shaped, but owns **no tables and no repo**).

The dashboard's composition root: it turns the already-built primitives into the rendered read-side. Domain authority: [`docs/domain/README.md`](../../../docs/domain/README.md) §"Derived projections"; design: [`docs/superpowers/specs/2026-06-15-budget-tracker-wrap-design.md`](../../../docs/superpowers/specs/2026-06-15-budget-tracker-wrap-design.md) §5.

Module-specific notes:
- **It is the only module that imports multiple domain services** — `budget` + `transactions` + `categorization` + `accounts` — plus the pure `tracker` / `reporting` projections, `core/timex`, and `cfg.AppTimezone`. This is the legitimate dashboard composition root; the isolation test enforces that **nothing imports `home` except `server`**, and that `home` imports **no provider client** (it reaches the bank only transitively through the services it composes).
- **Owns no persistence.** No `repo.go`, no sqlc, no tables. It fetches through the services, fills the pure projections' local input structs, and joins Category **names** back onto the results — money is converted to dollars at the view boundary.
- **Pages:** `GET /{$}` the current-month Tracker (the landing page), `GET /wraps` the month list, `GET /wraps/{ym}` (`ym` = `YYYY-MM`) a single month wrap. The accounts overview moved to `/accounts`; `home` owns `/`.
- **Time basis (ADR-0004):** the app timezone decides which calendar month "now" is and how many days remain (`timex.CurrentMonth` / `DaysLeftInclusive`); the month-range filter boundaries are UTC midnight (`timex.MonthRange`). **Transaction-date months are read from the stored calendar date directly (UTC), never re-zoned** — re-zoning a 1st-of-month UTC-midnight row would mis-bucket it (audit M1). The connect month for the partial-edge flag, by contrast, is a real instant and IS reckoned in the app tz.
- **Signed cents (audit M2):** net spend is summed signed (`cents()` preserves sign, refunds reduce spend); income legs (inflows, negative) are negated to a positive total; savings is the sum of `savings_contribution` source legs. The no-budget fallback uses `budget.IsNoBudget`.
- **Partial flag:** a month is partial when there is at least one connection and the month is at or before the earliest connection's month; with no connections nothing is partial.
