-- user-auth (01M6M197): built-in users, sessions and per-actor API tokens.
-- Credentials live OUTSIDE the ledger: they are not events, they are the
-- thing that authorizes writing events. Passwords are argon2id PHC strings;
-- session and token values are stored only as sha256 hex hashes.

CREATE TABLE users (
    id        bigserial   PRIMARY KEY,
    username  text        NOT NULL,
    actor     text        NOT NULL UNIQUE,          -- human:<name>, bound forever
    pass_hash text        NOT NULL,                 -- argon2id PHC string
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_username_ci_uidx ON users (lower(username));

CREATE TABLE sessions (
    token_hash text        PRIMARY KEY,              -- sha256 hex of the cookie value
    user_id    bigint      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_tokens (
    token_hash text        PRIMARY KEY,              -- sha256 hex of the raw token
    actor      text        NOT NULL,                 -- agent:<name> or human:<name>
    label      text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
