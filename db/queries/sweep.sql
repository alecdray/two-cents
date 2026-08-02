-- name: GetLatestSweepRecommendation :one
SELECT * FROM sweep_recommendation
WHERE id = ?;

-- name: UpsertSweepRecommendation :exec
INSERT INTO sweep_recommendation (
    id,
    kind,
    current_checking,
    current_savings,
    savings_unknown,
    total_spending_budget,
    mtd_spending,
    savings_target,
    mtd_savings_contributed,
    reserve,
    fixed_safety_margin,
    suggested_sweep,
    direction,
    reasons
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(id) DO UPDATE SET
    kind                    = excluded.kind,
    current_checking        = excluded.current_checking,
    current_savings         = excluded.current_savings,
    savings_unknown         = excluded.savings_unknown,
    total_spending_budget   = excluded.total_spending_budget,
    mtd_spending            = excluded.mtd_spending,
    savings_target          = excluded.savings_target,
    mtd_savings_contributed = excluded.mtd_savings_contributed,
    reserve                 = excluded.reserve,
    fixed_safety_margin     = excluded.fixed_safety_margin,
    suggested_sweep         = excluded.suggested_sweep,
    direction               = excluded.direction,
    reasons                 = excluded.reasons,
    updated_at              = CURRENT_TIMESTAMP;
