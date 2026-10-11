-- +goose Up
-- The sweep's safety margin moves out of FIXED_SAFETY_MARGIN (a boot-time env
-- var) and into the database, editable at runtime (ADR-0032). A single row,
-- always addressed by the fixed literal id 'default' (the singleton-row
-- pattern this schema already used for sweep_recommendation before it became a
-- snapshot history), holds the one value this table will ever need.
--
-- Seeded at 500 -- the env var's own former default. A migration is plain SQL
-- with no access to the process environment, so a deployment that had
-- customized FIXED_SAFETY_MARGIN away from 500 resets to 500 here; accepted,
-- since the UI this same change adds is exactly where that value is now set
-- and seen.
-- +goose StatementBegin
CREATE TABLE sweep_settings (
    id                  TEXT PRIMARY KEY,
    fixed_safety_margin REAL NOT NULL,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO sweep_settings (id, fixed_safety_margin) VALUES ('default', 500);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sweep_settings;
-- +goose StatementEnd
