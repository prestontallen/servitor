-- grants.sql — one-time bootstrap: give the servitor role ownership of the
-- servitor schema so servitord / servitor-mcp / apply-schema run without
-- admin (postgres) credentials. Idempotent: safe to rerun.
--
-- Run ONCE as a superuser:
--   psql -U postgres -d servitor -f deploy/grants.sql
--
-- Ownership (not mere GRANTs) so later migrations can ALTER TABLE / CREATE
-- without admin intervention. Objects are listed explicitly on purpose: the
-- public schema also holds TimescaleDB's own functions, which must stay
-- owned by postgres.

BEGIN;

-- Schema: USAGE + CREATE, and own it outright.
ALTER SCHEMA public OWNER TO servitor;
GRANT ALL ON SCHEMA public TO servitor;

-- Tables (001 + apply-schema's migrations table).
ALTER TABLE tickets          OWNER TO servitor;
ALTER TABLE slug_history     OWNER TO servitor;
ALTER TABLE subitems         OWNER TO servitor;
ALTER TABLE ledger           OWNER TO servitor;  -- hypertable: chunks follow owner
ALTER TABLE gate_events      OWNER TO servitor;
ALTER TABLE ledger_watermark OWNER TO servitor;
ALTER TABLE schema_migrations OWNER TO servitor;

-- Continuous-aggregate views (001 creates cagg_events_per_day /
-- cagg_events_kind_day as MATERIALIZED VIEWs; they appear as views + a
-- _timescaledb_internal matview). Reassign the user-facing views; the
-- internal storage follows the hypertable machinery.
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT c.relname, c.relkind
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relname LIKE 'cagg_%'
      AND c.relowner <> 'servitor'::regrole
  LOOP
    -- Continuous aggregates register as views but only accept
    -- ALTER MATERIALIZED VIEW.
    EXECUTE format('ALTER MATERIALIZED VIEW %I OWNER TO servitor', r.relname);
  END LOOP;
END $$;

-- Enum types.
ALTER TYPE ticket_status OWNER TO servitor;
ALTER TYPE gate_kind    OWNER TO servitor;
ALTER TYPE actor_type   OWNER TO servitor;
ALTER TYPE card_word    OWNER TO servitor;

-- Trigger functions guarding direct writes.
ALTER FUNCTION block_direct_write()     OWNER TO servitor;
ALTER FUNCTION block_ledger_mutation()  OWNER TO servitor;

-- Every sequence in public (ledger_id_seq, gate_events_id_seq, ...).
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT sequencename FROM pg_sequences
    WHERE schemaname = 'public' AND sequenceowner <> 'servitor'
  LOOP
    EXECUTE format('ALTER SEQUENCE %I OWNER TO servitor', r.sequencename);
  END LOOP;
END $$;

-- Future objects created by the servitor role are fine by default; anything
-- an admin creates later must be handed over explicitly.

COMMIT;
