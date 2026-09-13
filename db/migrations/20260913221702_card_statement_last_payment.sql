-- +goose Up
-- The payment a card's bank reports against its last statement. This is what
-- says how much of the billed figure is still owed, so the sweep never has to
-- read the card's current balance as a stand-in for having paid — the balance
-- survives only as a ceiling on what the card can claim (ADR-0028).
--
-- Nullable like every other reported statement figure: an unknown must survive
-- into the domain as an unknown, because the sweep turns it into the worst case
-- (subtract nothing, reserve the whole statement), and a defaulted zero would
-- read as a real "nothing has been paid".
--
-- Stored as reported and never netted into statement_balance. Whether the
-- payment counts against *this* cycle is a comparison against
-- statement_issued_at that the sweep makes, and pre-netting would destroy the
-- inputs it needs to make it.
ALTER TABLE accounts ADD COLUMN statement_last_payment_amount REAL;
ALTER TABLE accounts ADD COLUMN statement_last_payment_at TIMESTAMP;

-- +goose Down
ALTER TABLE accounts DROP COLUMN statement_last_payment_at;
ALTER TABLE accounts DROP COLUMN statement_last_payment_amount;
