-- Servitor greenfield: TimescaleDB schema (ledger-first hybrid CQRS).
-- Ledger = system of record, stored as a hypertable.
-- The PROJECTION IS APPLIED IN GO: this schema intentionally has NO
-- apply trigger. Go owns the write path (one append_event transaction).
-- What remains DB-enforced: identity/shape constraints, read-model
-- protection, ledger immutability.

CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TYPE ticket_status AS ENUM ('queued','active','blocked','done','dropped');
CREATE TYPE gate_kind       AS ENUM ('contract_approved','presented','shipped');
CREATE TYPE actor_type      AS ENUM ('human','agent','system');
CREATE TYPE card_word       AS ENUM ('shaping','building','checking','shipping');

CREATE TABLE tickets (
    ulid          text          PRIMARY KEY,
    slug          text          NOT NULL,
    title         text          NOT NULL DEFAULT '',
    status        ticket_status NOT NULL DEFAULT 'queued',
    rank          bigint        NOT NULL DEFAULT 0,     -- ordering; never identity
    blocked_on    text,                                 -- required iff blocked
    blocked_since timestamptz,
    pr            text,                                 -- NULL = absent, '' = empty
    fields        jsonb         NOT NULL DEFAULT '{}',  -- unknown keys verbatim
    card_word     card_word,                            -- derived from latest gate
    last_gate_ts  timestamptz,
    created_at    timestamptz   NOT NULL,
    updated_at    timestamptz   NOT NULL
);
CREATE UNIQUE INDEX tickets_slug_ci_uidx ON tickets (lower(slug));

CREATE TABLE slug_history (
    slug_lower  text        NOT NULL PRIMARY KEY,
    ticket_ulid text        NOT NULL REFERENCES tickets(ulid) DEFERRABLE INITIALLY DEFERRED,
    claimed_at  timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz                           -- kept forever: history-wide uniqueness
);

CREATE TABLE subitems (
    ulid        text        PRIMARY KEY,
    ticket_ulid text        NOT NULL REFERENCES tickets(ulid) ON DELETE CASCADE,
    kind        text        NOT NULL,     -- criterion|plan|decision|question|link|note
    rank        bigint      NOT NULL,
    body        text        NOT NULL DEFAULT '',
    state       text,                     -- criterion: pass|fail|NULL ; link: url
    fields      jsonb       NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);
CREATE INDEX subitems_ticket_rank_idx ON subitems (ticket_ulid, kind, rank);
CREATE UNIQUE INDEX subitems_decision_dedupe_uidx
    ON subitems (ticket_ulid, md5(body), md5(coalesce(fields->>'why','')))
    WHERE kind = 'decision';

CREATE TABLE ledger (
    id          bigserial,
    ulid        text        NOT NULL,
    ticket_ulid text        NOT NULL,
    ts          timestamptz NOT NULL DEFAULT now(),
    actor       text        NOT NULL,
    actor_type  actor_type  NOT NULL,
    session     text,
    kind        text        NOT NULL CHECK (kind <> ''),
    payload     jsonb       NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, ts)                  -- hypertable: unique must include ts
);
CREATE UNIQUE INDEX ledger_ulid_uidx ON ledger (ulid, ts);
CREATE INDEX ledger_ticket_id_idx    ON ledger (ticket_ulid, id);
CREATE INDEX ledger_kind_ts_idx      ON ledger (kind, ts DESC);

SELECT create_hypertable('ledger', 'ts', chunk_time_interval => INTERVAL '30 days', migrate_data => true);

CREATE TABLE gate_events (
    id          bigserial   PRIMARY KEY,
    ticket_ulid text        NOT NULL REFERENCES tickets(ulid),
    gate        gate_kind   NOT NULL,
    actor       text        NOT NULL,
    actor_type  actor_type  NOT NULL,
    ts          timestamptz NOT NULL DEFAULT now(),
    ledger_id   bigint      NOT NULL,   -- no FK: FKs onto hypertables are forbidden
    CONSTRAINT gate_events_one_per_ticket UNIQUE (ticket_ulid, gate)
);

CREATE TABLE ledger_watermark (
    id          int PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    last_id     bigint NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now()
);
INSERT INTO ledger_watermark (id) VALUES (1);

-- Time-series rollups (real-time continuous aggregates): the GUI charts read
-- these. Realtime mode unions recent raw data, so today's events appear
-- without a refresh; the materialization catches up in the background.
CREATE MATERIALIZED VIEW cagg_events_per_day
WITH (timescaledb.continuous) AS
SELECT time_bucket('1 day', ts) AS day, count(*) AS events
FROM ledger GROUP BY 1
WITH NO DATA;

CREATE MATERIALIZED VIEW cagg_events_kind_day
WITH (timescaledb.continuous) AS
SELECT time_bucket('1 day', ts) AS day, kind, count(*) AS events
FROM ledger GROUP BY 1, 2
WITH NO DATA;

-- Hourly refresh keeps the rollups warm for when the analytics read swaps
-- from direct GROUP BY to the caggs (see api.Analytics). NOTE: an empty
-- refresh sets a watermark that hides later rows from the realtime union,
-- so Analytics does NOT read the caggs until volume justifies the swap.
SELECT add_continuous_aggregate_policy('cagg_events_per_day',
    start_offset => INTERVAL '120 days', end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');
SELECT add_continuous_aggregate_policy('cagg_events_kind_day',
    start_offset => INTERVAL '120 days', end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');

-- Read model is projection-only: writes come from Go inside the append txn,
-- which sets `SET LOCAL servitor.write = 'on'`. Everything else is rejected.
CREATE OR REPLACE FUNCTION block_direct_write() RETURNS trigger AS $fn$
BEGIN
    IF current_setting('servitor.write', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'read model is projection-only; append events instead (%)', TG_TABLE_NAME;
    END IF;
    RETURN COALESCE(NEW, OLD);
END $fn$ LANGUAGE plpgsql;
CREATE TRIGGER tickets_readonly    BEFORE INSERT OR UPDATE OR DELETE ON tickets     FOR EACH ROW EXECUTE FUNCTION block_direct_write();
CREATE TRIGGER subitems_readonly   BEFORE INSERT OR UPDATE OR DELETE ON subitems    FOR EACH ROW EXECUTE FUNCTION block_direct_write();
CREATE TRIGGER gate_events_readonly BEFORE INSERT OR UPDATE OR DELETE ON gate_events FOR EACH ROW EXECUTE FUNCTION block_direct_write();

-- Ledger is append-only, forever.
CREATE OR REPLACE FUNCTION block_ledger_mutation() RETURNS trigger AS $fn$
BEGIN
    RAISE EXCEPTION 'ledger is append-only';
END $fn$ LANGUAGE plpgsql;
CREATE TRIGGER ledger_no_mutation BEFORE UPDATE OR DELETE ON ledger
    FOR EACH ROW EXECUTE FUNCTION block_ledger_mutation();
