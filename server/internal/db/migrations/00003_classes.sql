-- +goose Up
-- Characters created before classes become Sword Cultivators.
ALTER TABLE characters ADD COLUMN class TEXT NOT NULL DEFAULT 'sword_cultivator';

-- +goose Down
ALTER TABLE characters DROP COLUMN class;
