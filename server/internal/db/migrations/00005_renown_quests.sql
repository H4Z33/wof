-- +goose Up
ALTER TABLE characters
    ADD COLUMN renown INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN title  TEXT    NOT NULL DEFAULT '',
    -- Quest state: {"bandit_trouble": {"state": "active", "progress": [3]}}
    ADD COLUMN quests JSONB   NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE characters
    DROP COLUMN renown,
    DROP COLUMN title,
    DROP COLUMN quests;
