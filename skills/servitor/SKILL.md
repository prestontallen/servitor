---
name: servitor
description: |
  Use at the start of EVERY session to orient on active work (the SessionStart
  hook injects `servitor ctx` output); also whenever tickets are added, started,
  updated, or completed, and for any question about work state, decisions, or
  history. Servitor is Preston's worklog/task system: a Go daemon on
  TimescaleDB, accessed by agents via the servitor CLI and/or MCP tools.
tool: workflow
concern: process
---

# Servitor (greenfield)

Servitor is the system of record for work tracking. The store is a
TimescaleDB database; `servitord` is the API daemon; agents interact ONLY
through the CLI (`servitor`) or MCP tools — never by touching files or the
database directly.

## Orientation (every session)

The SessionStart hook runs `servitor ctx` for the focused ticket and always
exits 0. If you see "servitor: unavailable", the daemon is down — say so and
carry on without blocking; do NOT retry in a loop.

With no focus ticket, run `servitor board` to see queued/active/blocked work.

## The model

- **Identity**: every ticket and sub-item has a ULID. Sub-items are addressed
  by ULID prefix (use enough characters to be unique; same-millisecond ULIDs
  share leading chars). Position is never identity — ordering is a rank.
- **Slug**: a mutable alias, unique case-insensitively across ALL history,
  including archived/dropped. `SERVITOR_TICKET` env or the board shows slugs.
- **Statuses** (exactly five): `queued`, `active`, `blocked` (requires
  `--on human|<party>`), `done`, `dropped`. "Archived" is a query, not a
  status: done before a date.
- **Gates** replace phase ladders: `contract_approved` (HUMAN actor only),
  `presented`, `shipped`. A card word (shaping/building/checking/shipping)
  derives from the latest gate — never set it by hand.
- **Everything is an event** — notes, decisions, status changes, field diffs,
  hook chatter. The ledger is append-only; nothing is ever edited or deleted.
  Corrections are new events.
- **Events have actors.** Judgement events (decisions, gate approvals) carry
  `human:` actors; mechanical events carry `agent:`/`system`. Never claim a
  human actor unless the human actually approved — the store enforces this
  for `contract_approved` and will reject you.

## CLI verbs

```
servitor ctx [ref]     whole ticket aggregate JSON (hook). ALWAYS exits 0.
servitor board         queued/active/blocked cards, rank-ordered
servitor new --slug S [--title T] [--rank N]     -> prints ticket ULID
servitor set <ref> [--status S [--on WHO]] [--pr V|-] [field=value ...]
servitor log <ref> <kind> [text]                 e.g. note; or any ledger kind
servitor gate <ref> <contract_approved|presented|shipped>
servitor history <ref>                 full event timeline
```

- `ref` is a ULID or slug, case-insensitive.
- `set` maps flags to events: `--status active`, `--status blocked --on human`,
  `--pr https://...` (or `--pr -` to make it absent/NULL), free
  `field=value` pairs (JSON passthrough if the value looks like JSON).
- `set x priority=-` removes the field; `""` sets it to empty string.
  Absent vs empty are distinct everywhere (pr is the load-bearing case).
- `gate` needs `SERVITOR_HUMAN` set (your human identity) for
  `contract_approved` when the actor isn't already `human:`.

## MCP tools (when registered)

`servitor_ctx(ref)`, `servitor_board()`, `servitor_history(ref, limit)`,
`servitor_append(kind, payload, actor, ticket?)`. The append tool is the full
write path; `ticket.create` generates the ticket ULID server-side. Domain
violations surface as tool errors with stable codes:
`stale_write`, `slug_claimed`, `blocked_requires_on`, `human_gate_required`,
`gate_already_passed`, `ambiguous_prefix`, `unknown_ticket`, `no_live_slug`.
Key on codes; read messages for detail.

## Workflow

1. Orient: hook output or `servitor board`.
2. Working a ticket: `set <ref> --status active`, log notes as you go
   (`servitor log <ref> note "..."`). Decisions are events: prefer
   `servitor log <ref> decision "..."` style payloads only via API/MCP —
   on the CLI record decisions in notes and let the human gate them.
3. Done: `servitor gate <ref> presented`, present the work, then
   `servitor set <ref> --status done` after human acceptance.
4. Blocked: `servitor set <ref> --status blocked --on human` — always say
   on WHOM and why in a note.
5. Abandoning: `--status dropped`. Never fake done.

## Environment

`SERVITOR_API` (default http://localhost:8181), `SERVITOR_ACTOR`
(default `agent:cli` — set per agent, e.g. `agent:hermes`),
`SERVITOR_HUMAN` (Preston), `SERVITOR_TICKET` (hook focus),
`SERVITOR_DSN` (daemon/MCP only).

## Hard rules

1. Never write to the database or read-model files directly; the read model
   is projection-only and the ledger is append-only — the store enforces it.
2. Never claim human approval that didn't happen.
3. The GUI (port 8181) is read-only for now; all writes via CLI/MCP/API.
4. One actor of record per event — don't forge sessions.
