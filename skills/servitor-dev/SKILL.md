---
name: servitor-dev
description: Servitor repo dev process — staging databases, demo daemons, and the schema migration order for agents working in this repo.
---

# Servitor dev process: staging, demos, schema discipline

The production ledger (`servitor` database, `servitord.service` on :8181)
is the system of record. Agents never connect to it. All development,
testing, and demos run against per-worktree staging databases. Use
together with the `worktrees` skill — this skill assumes you have claimed
a worktree named `<slug>` with a deterministic port offset.

## One-time setup (done once per host, by the human or install.sh)

- Postgres role `servitor_staging` with login + password, granted rights
  only on `servitor_staging_*` databases. Never the prod `servitor` DB.
- Credentials in `~/.config/servitor/staging.env` (outside the repo,
  never committed): `SERVITOR_STAGING_PASSWORD`.

## Staging database lifecycle (per worktree)

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

# seed from prod snapshot — demos show real shape, and migrations get
# exercised against real data before prod ever sees them
docker exec timescaledb pg_dump -U postgres servitor \
  | docker exec -i timescaledb psql -U postgres -d "$STAGE_DB"

# staging rights: transfer ownership of DB + restored tables to the role
docker exec timescaledb psql -U postgres -c "ALTER DATABASE $DB OWNER TO servitor_staging"
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

The role/password live in `~/.config/servitor/staging.env`. The staging
DSN is therefore:
`postgres://servitor_staging:<pw>@localhost:5432/$STAGE_DB?sslmode=disable`.

Seeding from prod is the default. Do not put secrets into tickets/events
that you would not want copied into staging (staging is same-host, same
trust domain — this is about hygiene, not security).

## Staging daemon (per worktree)

```bash
cd <worktree>
go build -o /tmp/servitord-<slug> ./cmd/servitord
SERVITOR_DSN="postgres://servitor_staging:<pw>@localhost:5432/$STAGE_DB?sslmode=disable" \
SERVITOR_ADDR=":<worktree-port>" \
  /tmp/servitord-<slug> &
/home/preston/.local/bin/servitord apply-schema   # NEVER against prod; see below
```

- The staging daemon serves the worktree's embedded GUI on its own port —
  this is the demo URL for the iPad (better than Vite for demos: no proxy,
  production build).
- `apply-schema` takes its DSN from the environment. Rule: the staging
  daemon's env is set per-invocation (as above); the prod daemon's env
  lives only in its systemd unit. Never export a prod DSN into a shell.

## Schema migration order (hard sequence)

1. Fixture tests pass (`go test`) — proves the migration is *correct*.
2. Restore prod snapshot into staging, run `apply-schema` — proves the
   migration *survives real data* (this catches what fixtures miss).
3. Exercise the migrated schema through the staging daemon (CLI + GUI).
4. Demo to the human on staging. `presented` evidence = staging URL or
   CLI transcript, not test output alone.
5. Only after gate acceptance: merge to main, redeploy via install.sh,
   and run `apply-schema` against prod (env from the systemd unit).

A migration that fails at step 2 or 3 is not "mostly done" — it is a
defect, fix before demoing.

## Demo recipes

- **GUI change**: staging daemon's port serves the built GUI. Bookmark
  `http://<host>:<port>/` on the iPad, hard-refresh.
- **Service/CLI change**: point the CLI at staging with
  `SERVITOR_API=http://localhost:<port>` and exercise the new verbs.
- **Schema change**: the migration order above IS the demo.

## Teardown

When the task's worktree is released, drop its staging database:

```bash
psql -h localhost -U postgres -c "DROP DATABASE IF EXISTS $STAGE_DB (FORCE)"
```

Staging DBs are disposable by design; anything valuable lives in the
ledger or the code, never only in staging.
