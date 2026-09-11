-- name: GetLatestSweepRecommendation :one
SELECT * FROM sweep_recommendation
ORDER BY computed_at DESC, rowid DESC
LIMIT 1;

-- name: GetSweepRecommendationByID :one
SELECT * FROM sweep_recommendation
WHERE id = ?;

-- name: ListSweepRecommendations :many
SELECT * FROM sweep_recommendation
ORDER BY computed_at DESC, rowid DESC;

-- name: InsertSweepRecommendation :exec
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
    reasons,
    computed_at,
    card_balance
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
);
