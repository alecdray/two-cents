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
    required_checking,
    fixed_safety_margin,
    suggested_sweep,
    direction,
    reasons,
    timeline,
    computed_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
);
