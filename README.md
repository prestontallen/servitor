# servitor

Work tracking as an append-only event ledger. Nothing is edited, nothing is
deleted; corrections are new events. The store is the system of record.

## Components

- `servitord` — HTTP API daemon (`:8181`). Stateless; the store is the only backend.
- `servitor` — CLI. Thin client: holds no state, applies no rules.
- `servitor-mcp` — same Service exposed as MCP tools over stdio.
- `web/` — Svelte GUI (read-only).

The Go layer owns the projection; the schema intentionally has no apply
trigger. One transaction per `append_event`.

## Model

- Every ticket and sub-item has a ULID. Sub-items are addressed by prefix.
- A **slug** is a mutable alias, unique case-insensitively across all history.
- Five statuses: `queued`, `active`, `blocked` (requires `blocked_on`),
  `done`, `dropped`. "Archived" is a query, not a status.
- **Gates** replace phase ladders: `contract_approved` (human only),
  `presented`, `shipped`. A card word (shaping/building/checking/shipping)
  derives from the latest gate — never set by hand.
- Events carry one actor of record: `human:`, `agent:`, or `system`.
  Judgment events carry human actors; the store enforces it.

## Usage

Agents interact only via the CLI or MCP — never the database, never the
files. Core verbs:

```
servitor hook [ref]       SessionStart hook: preflight header + ctx; always exits 0
servitor ctx [ref]        ticket aggregate, JSON only; always exits 0
servitor board            queued/active/blocked, rank-ordered
servitor arcs             arcs (tickets with members) with derived rollups
servitor new --slug S     -> ticket ULID
servitor set <ref> ...    status / PR / free field=value pairs
servitor log <ref> note   append an event
servitor gate <ref> <g>   pass a gate
servitor history <ref>    full event timeline
```

Arcs group tickets: an arc is any ticket another ticket points at via
`servitor set <ref> parent=<arc-ulid>` (create the arc as a plain ticket
first). Rollup status, member count and last activity are derived, never
set by hand. `source`/`source_ref`/`depends`/`area` are free fields set
the same way; history events carry a derived `class` (signal|transition).

Intended workflow: orient on `board`, classify the work at intake, work
tickets with notes as you go, pass gates at handoffs, and record blockers
as `blocked --on <party>` — never fake done.

## Install

```
./install.sh [--check] [--tone|--no-tone] [--dsn URL] [--token TOKEN]
```

Builds the binaries, installs them to `~/.local/bin`, restarts the
`servitord` systemd unit, and links the skill into detected agent skill
directories. `--check` reports drift. `--tone` additionally links the
optional reporting-register skill.

The daemon reads its secrets from `~/.config/servitor/servitord.env`
(mode 0600), which install.sh creates on first run — from `--dsn` when
given, otherwise by migrating the DSN out of an older unit. Reruns
without `--dsn` keep the existing file. `servitord apply-schema` runs
against that DSN before the restart, so a pending migration never
leaves the daemon 500ing on new columns; if apply-schema fails, install
aborts without restarting. `servitor-mcp` picks up `SERVITOR_DSN` /
`SERVITOR_TOKEN` by sourcing the same env file.

## Environment

| Variable | Purpose | Default |
|---|---|---|
| `SERVITOR_API` | CLI target | `http://localhost:8181` |
| `SERVITOR_DSN` | Postgres/TimescaleDSN (daemon, MCP) | local `servitor` DB |
| `SERVITOR_ADDR` | daemon listen address | `:8181` |
| `SERVITOR_TOKEN` | bearer-token auth for the API | off |
| `SERVITOR_ACTOR` | actor of record | `agent:cli` |
| `SERVITOR_TICKET` | hook focus ticket | — |

## Database bootstrap (one-time)

The `servitor` DB role must own the schema — servitord, servitor-mcp and
`servitord apply-schema` run under it, and later migrations need ALTER
rights. On a fresh DB, as a superuser (once, idempotent):

```
psql -U postgres -d servitor -f deploy/grants.sql
```

Then `SERVITOR_DSN` points at the `servitor` role; the daemon never needs
admin credentials.

---

++OBSERVATION NOTED. FUNCTION CONTINUES.++
