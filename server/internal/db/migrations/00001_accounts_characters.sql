-- +goose Up
CREATE TABLE accounts (
    id            BIGSERIAL PRIMARY KEY,
    -- Stored lowercase; uniqueness is case-insensitive by construction.
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE characters (
    id         BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    model      TEXT NOT NULL,
    zone_id    TEXT NOT NULL,
    pos_x      DOUBLE PRECISION NOT NULL,
    pos_y      DOUBLE PRECISION NOT NULL DEFAULT 0,
    pos_z      DOUBLE PRECISION NOT NULL,
    yaw        DOUBLE PRECISION NOT NULL DEFAULT 0,
    level      INTEGER NOT NULL DEFAULT 1,
    exp        BIGINT NOT NULL DEFAULT 0,
    hp         INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX characters_name_ci ON characters (lower(name));
CREATE INDEX characters_account ON characters (account_id);

-- +goose Down
DROP TABLE characters;
DROP TABLE accounts;
