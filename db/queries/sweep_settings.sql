-- name: GetSweepSettings :one
SELECT * FROM sweep_settings WHERE id = 'default';

-- name: UpdateSweepSafetyMargin :exec
UPDATE sweep_settings SET fixed_safety_margin = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 'default';
