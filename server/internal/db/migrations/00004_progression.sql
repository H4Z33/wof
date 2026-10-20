-- +goose Up
-- Discipline experience, attribute ranks and the chosen specialization:
-- {"xp": {"strength": 120}, "ranks": {"physique": 3}, "spec": "blade_saint"}
ALTER TABLE characters ADD COLUMN progression JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE characters DROP COLUMN progression;
