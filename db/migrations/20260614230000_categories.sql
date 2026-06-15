-- +goose Up
-- +goose StatementBegin
CREATE TABLE categories (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    builtin    INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- Seed the twelve built-in spending Categories with stable string ids, so the
-- in-code PFC-primary → Category-id map is a static table, not data to migrate.
-- +goose StatementBegin
INSERT INTO categories (id, name, builtin) VALUES
    ('food_and_drink',           'Food & Drink',           1),
    ('general_merchandise',      'General Merchandise',     1),
    ('transportation',           'Transportation',          1),
    ('travel',                   'Travel',                  1),
    ('rent_and_utilities',       'Rent & Utilities',        1),
    ('medical',                  'Medical',                 1),
    ('personal_care',            'Personal Care',           1),
    ('general_services',         'General Services',         1),
    ('entertainment',            'Entertainment',           1),
    ('home_improvement',         'Home Improvement',        1),
    ('bank_fees',                'Bank Fees',               1),
    ('government_and_non_profit','Government & Non-Profit',  1);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE categories;
-- +goose StatementEnd
