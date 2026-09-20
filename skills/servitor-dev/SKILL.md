---
name: servitor-dev
description: Servitor repo dev process — which target a change is demonstrated against (running service, staging database, or the long-lived dev database), the staging lifecycle, and the schema migration order for agents working in this repo.
---

# Servitor dev process: demo targets, staging, schema discipline

The production ledger (`servitor` database, `servitord.service` on :8181)
is the system of record. Agents never connect to its database. Where a
change is demonstrated depends on what the change touches:

| Change touches | Demo target | Database |
|---|---|---|
| `web/` only (GUI) | the running service on :8181, through Vite's proxy | none |
| `cmd/`, `internal/` (daemon, CLI, MCP, API) | the worktree's built daemon | per-worktree staging DB, seeded from prod |
| the schema (`apply-schema` migrations) | the worktree's built daemon | the long-lived dev database `servitor_dev` |

Pick the row by the diff, not by habit. A diff that touches Go is a
binary change even if it started as a GUI ticket. A GUI change that
writes is also a binary-class demo, because through the proxy its
writes would land in the prod ledger.

## GUI-only changes: hit the running service

No database, no daemon, nothing to seed or tear down.

```bash
cd <worktree>/web
npm ci
npx vite --port <worktree-port> --strictPort
```

`vite.config.js` proxies `/api` to `http://localhost:8181`, so the new
GUI reads the real ledger. Demo URL: `http://localhost:<port>/#/...`.
Teardown is killing Vite. The built GUI is embedded into the daemon at
build time, so the change reaches :8181 only after merge and redeploy.

## Binary changes: per-worktree staging database

One staging DB per worktree, named for the ticket slug.

### One-time setup (done once per host, by the human or install.sh)

- Postgres role `servitor_staging` with login + password, granted rights
  only on `servitor_staging_*` and `servitor_dev` databases. Never the
  prod `servitor` DB.
- Credentials in `~/.config/servitor/staging.env` (outside the repo,
  never committed): `SERVITOR_STAGING_PASSWORD`.

### Lifecycle

```bash
source ~/.config/servitor/staging.env   # SERVITOR_STAGING_PASSWORD
```

On this host Postgres runs in the `timescaledb` container; `psql`/`pg_dump`
run inside it (host has no postgres client). Generic form:

```bash
STAGE_DB="servitor_staging_<slug>"
docker exec timescaledb psql -U postgres -c "DROP DATABASE IF EXISTS $STAGE_DB WITH (FORCE)"
docker exec timescaledb psql -U postgres -c "CREATE DATABASE $STAGE_DB"
docker exec timescaledb psql -U postgres -d "$STAGE_DB" -c "CREATE EXTENSION IF NOT EXISTS timescaledb"

# seed from prod snapshot so demos show real shape
docker exec timescaledb pg_dump -U postgres servitor \
  | docker exec -i timescaledb psql -U postgres -d "$STAGE_DB"

# staging rights: transfer ownership of DB + restored tables to the role
docker exec timescaledb psql -U postgres -c "ALTER DATABASE $STAGE_DB OWNER TO servitor_staging"
docker exec timescaledb psql -U postgres -d "$STAGE_DB" -c "DO \$\$ DECLARE r record; BEGIN \
  FOR r IN SELECT tablename FROM pg_tables WHERE schemaname='public' LOOP \
    EXECUTE format('ALTER TABLE public.%I OWNER TO servitor_staging', r.tablename); END LOOP; \
  FOR r IN SELECT sequencename FROM pg_sequences WHERE schemaname='public' LOOP \
    EXECUTE format('ALTER SEQUENCE %I OWNER TO servitor_staging', r.sequencename); END LOOP; \
END \$\$;"
# VERIFY ownership actually moved — a DO block can succeed while changing
# nothing (seen 2026-09: unqualified tablename left tables owned by the
# dump owner and the daemon 500'd on permission denied):
docker exec timescaledb psql -U postgres -d "$STAGE_DB" -c \
  "SELECT tablename, tableowner FROM pg_tables WHERE schemaname='public'"
```

The staging DSN is
`postgres://servitor_staging:<pw>@localhost:5432/$STAGE_DB?sslmode=disable`.

Seeding from prod is the default. Do not put secrets into tickets/events
that you would not want copied into staging (staging is same-host, same
trust domain — this is about hygiene, not security).

### Staging daemon

```bash
cd <worktree>
go build -o /tmp/servitord-<slug> ./cmd/servitord
SERVITOR_DSN="postgres://servitor_staging:<pw>@localhost:5432/$STAGE_DB?sslmode=disable" \
SERVITOR_ADDR=":<worktree-port>" \
  /tmp/servitord-<slug> &
```

- The staging daemon serves the worktree's embedded GUI on its own port,
  so a binary change that also has GUI work is demoed there, not through
  Vite. Bookmark `http://<host>:<port>/` on the iPad, hard-refresh.
- CLI/MCP changes: `SERVITOR_API=http://localhost:<port>` and exercise
  the new verbs against the staging daemon.
- The daemon's env is set per-invocation, as above; the prod daemon's env
  lives only in its systemd unit. Never export a prod DSN into a shell.

## Schema migrations: the long-lived dev database

Migrations are rehearsed on `servitor_dev`, one database that persists
across tickets and receives every migration in the order prod will.
A fresh restore per ticket only proves a migration survives a snapshot;
the dev database also proves it survives the migrations before it.

Create it once with the staging lifecycle above and `STAGE_DB=servitor_dev`,
then keep it. Re-seed from a prod snapshot only deliberately: when prod
has moved far past it, or when a rehearsal left it in a state prod will
never be in. Re-seeding discards its migration history, so say so in the
ticket. Its DSN is
`postgres://servitor_staging:<pw>@localhost:5432/servitor_dev?sslmode=disable`.

Gotchas from creating it the first time (2026-09, ticket
dev-database-setup):

- The prod dump's COPY for `ledger` restores 0 rows — the data lives in
  TimescaleDB chunks and pg_dump's hypertable COPY comes out empty against
  a fresh dev DB. After the normal restore, copy it explicitly:
  `COPY (SELECT ... FROM ledger ORDER BY id) TO STDOUT` on prod, then a
  plain `COPY ... FROM STDIN` on dev (from a file; a `-c` that mixes
  other statements breaks the COPY protocol). Row counts must match
  before calling the seed done.
- Ownership transfer needs all three, or apply-schema dies on
  `permission denied for schema public`: `ALTER DATABASE`, plus
  `ALTER SCHEMA public OWNER TO servitor_staging`, plus the
  tables/sequences DO loops. The `cagg_%` materialized views must also
  move (`ALTER MATERIALIZED VIEW`), or ownership is silently split.

`apply-schema` takes its DSN from the environment:

```bash
SERVITOR_DSN="postgres://servitor_staging:<pw>@localhost:5432/servitor_dev?sslmode=disable" \
  /tmp/servitord-<slug> apply-schema      # NEVER against prod; see below
```

### Migration order (hard sequence)

1. Fixture tests pass (`go test`) — proves the migration is *correct*.
2. `apply-schema` against `servitor_dev` — proves the migration survives
   real data and the migrations already applied before it.
3. Exercise the migrated schema through a daemon on the dev DSN (CLI + GUI).
4. Demo to the human on that daemon. `presented` evidence = its URL or a
   CLI transcript, not test output alone.
5. Only after gate acceptance: merge to main, redeploy via install.sh,
   and run `apply-schema` against prod (env from the systemd unit).

A migration that fails at step 2 or 3 is not "mostly done" — it is a
defect, fix before demoing.

## Teardown

- GUI-only: kill Vite. Nothing else exists.
- Binary: when the worktree is released, drop its staging database:
  ```bash
  docker exec timescaledb psql -U postgres -c "DROP DATABASE IF EXISTS $STAGE_DB WITH (FORCE)"
  ```
  Staging DBs are disposable by design; anything valuable lives in the
  ledger or the code, never only in staging.
- Migrations: never drop `servitor_dev` as part of a ticket. It is
  long-lived on purpose.
