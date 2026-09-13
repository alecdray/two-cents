-- +goose Up
-- Billing-cycle detail a credit card's bank reports, and the user's statement
-- about when the card is paid. Each reported figure is independently nullable:
-- an unknown must survive into the domain as an unknown, because the sweep
-- turns it into the worst case, and a defaulted zero or today would quietly
-- make the answer less conservative instead of more (ADR-0026).
--
-- The statement carries no timestamp of its own; it shares the row's
-- last_synced_at, which is what keeps one staleness rule rather than two.
ALTER TABLE accounts ADD COLUMN statement_balance REAL;
ALTER TABLE accounts ADD COLUMN statement_issued_at TIMESTAMP;
ALTER TABLE accounts ADD COLUMN statement_due_at TIMESTAMP;

-- The payment schedule defaults to the reported due date, so an account that
-- has never been configured needs no backfill.
ALTER TABLE accounts ADD COLUMN payment_schedule_mode TEXT NOT NULL DEFAULT 'due_date'
    CHECK (payment_schedule_mode IN ('due_date', 'statement_plus_days'));
ALTER TABLE accounts ADD COLUMN payment_schedule_offset_days INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE accounts DROP COLUMN payment_schedule_offset_days;
ALTER TABLE accounts DROP COLUMN payment_schedule_mode;
ALTER TABLE accounts DROP COLUMN statement_due_at;
ALTER TABLE accounts DROP COLUMN statement_issued_at;
ALTER TABLE accounts DROP COLUMN statement_balance;
