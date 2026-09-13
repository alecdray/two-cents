-- +goose Up
-- Records that this card's bank login will not serve billing-cycle detail — the
-- product was never authorized, or consent was withdrawn. It is a fact about the
-- card rather than a state of the connection: the login still serves balances
-- and transactions in full, and marking it broken would put a breakage badge on
-- a working connection (ADR-0026). The sweep simply keeps the card's worst-case
-- reading while this holds.
ALTER TABLE accounts ADD COLUMN statements_unavailable INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE accounts DROP COLUMN statements_unavailable;
