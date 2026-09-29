-- Author: every event names who wrote it (actor), from which machine (host)
-- and in which harness session (session, a 001 column that finally gets
-- filled). Nullable, no default: existing rows are never rewritten (the
-- ledger is append-only), so legacy events read host = NULL. Authors are a
-- query over these columns, not a table.

ALTER TABLE ledger ADD COLUMN host text;
