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
- Arcs group tickets: an arc is any ticket another points at via
  `parent=<arc-ulid>`. Rollup status, member count and last activity are
  derived, never set by hand.
  `source`/`source_ref`/`depends`/`area` are free fields;
  history events carry a derived `class` (signal|transition).

## Usage

Agents interact only via the CLI or MCP — never the database, never the
files.

```
servitor hook [ref]       SessionStart hook: preflight header + ctx; always exits 0
servitor ctx [ref]        ticket aggregate, JSON only; always exits 0
servitor board            queued/active/blocked, rank-ordered
servitor arcs             arcs with derived rollups
servitor new --slug S     -> ticket ULID
servitor set <ref> ...    status / PR / free field=value pairs
servitor log <ref> note   append an event
servitor gate <ref> <g>   pass a gate
servitor history <ref>    full event timeline
```

Orient on `board`, classify at intake, log as you go, pass gates at
handoffs, record blockers as `blocked --on <party>`. Never fake done.

Agents carrying the optional `servitor-tone` skill (`--tone` at install)
report in a fixed grammar — state, result, obstruction, one line each,
`no change.` when there is none:

```
skill-link-verify · done · shipped
done: install.sh links no skill the tree does not carry
evidence: 43 assertions; --check exits 0
next: nothing. Branch and worktree released.
```

## Install

```
./install.sh [--check] [--from-source] [--version TAG]
             [--tone|--no-tone] [--dsn URL] [--token TOKEN]
```

Downloads the release matching this host, verifies its checksum, and
installs into `~/.local/bin`; `--from-source` builds from the checkout
instead and `--version` pins a tag. Then restarts the `servitord` unit and
links the skills into detected agent skill directories — skipping, with a
warning, any the tree does not carry. `--check` reports drift.

Secrets live in `~/.config/servitor/servitord.env` (mode 0600), created on
first run from `--dsn` or by migrating the DSN out of an older unit; reruns
without `--dsn` keep it. `servitord apply-schema` runs against that DSN
before the restart, so a pending migration never leaves the daemon 500ing
on new columns; if it fails, install aborts without restarting.
`servitor-mcp` sources the same file.

## Environment

| Variable | Purpose | Default |
|---|---|---|
| `SERVITOR_API` | CLI target | `http://localhost:8181` |
| `SERVITOR_DSN` | Postgres/Timescale DSN (daemon, MCP) | local `servitor` DB |
| `SERVITOR_ADDR` | daemon listen address | `:8181` |
| `SERVITOR_TOKEN` | bearer-token auth for the API | off |
| `SERVITOR_ACTOR` | actor of record | `agent:cli` |
| `SERVITOR_TICKET` | hook focus ticket | — |

## Database bootstrap (one-time)

The `servitor` role must own the schema — servitord, servitor-mcp and
`apply-schema` run under it, and migrations need ALTER rights. On a fresh
DB, as a superuser (once, idempotent):

```
psql -U postgres -d servitor -f deploy/grants.sql
```

`SERVITOR_DSN` then points at the `servitor` role; the daemon never needs
admin credentials.

---

++OBSERVATION NOTED. FUNCTION CONTINUES.++
