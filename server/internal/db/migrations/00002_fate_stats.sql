-- +goose Up
-- Fate proof of concept. The inventory is a JSONB map item_key -> quantity;
-- it moves to its own table with an item ledger when trading arrives (M4).
ALTER TABLE characters
    ADD COLUMN luck      INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN karma     INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN inventory JSONB   NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE characters
    DROP COLUMN luck,
    DROP COLUMN karma,
    DROP COLUMN inventory;
