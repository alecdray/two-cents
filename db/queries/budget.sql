-- name: GetBudget :one
SELECT * FROM budget
WHERE id = ?;

-- name: UpsertBudget :one
INSERT INTO budget (
    id,
    income_target,
    savings_target
) VALUES (
    ?, ?, ?
)
ON CONFLICT(id) DO UPDATE SET
    income_target  = excluded.income_target,
    savings_target = excluded.savings_target,
    updated_at     = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListCategoryLimits :many
SELECT * FROM budget_category_limits
ORDER BY category_id;

-- name: DeleteAllCategoryLimits :exec
DELETE FROM budget_category_limits;

-- name: CreateCategoryLimit :exec
INSERT INTO budget_category_limits (
    category_id,
    limit_amount
) VALUES (
    ?, ?
);
