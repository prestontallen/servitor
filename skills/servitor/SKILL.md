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
servitor feedback [--since DATE] [--source human|self] [--limit N]
                                       feedback events across all tickets
```

- `ref` is a ULID or slug, case-insensitive.
- `set` maps flags to events: `--status active`, `--status blocked --on human`,
  `--pr https://...` (or `--pr -` to make it absent/NULL), free
  `field=value` pairs (JSON passthrough if the value looks like JSON).
- `set x priority=-` removes the field; `""` sets it to empty string.
  Absent vs empty are distinct everywhere (pr is the load-bearing case).
- `gate` needs a `human:` actor for `contract_approved` — set
  `SERVITOR_ACTOR=human:<name>` for the call (SERVITOR_HUMAN alone is
  currently rejected by the CLI; verified 2026-09).

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
3. Before presenting: run the `ponytail-review` skill on the working diff
   (it reviews diffs, so after code exists, not on the plan) and apply its
   cuts. Then: `servitor gate <ref> presented`, present the work, then
   `servitor set <ref> --status done` after human acceptance.
4. Blocked: `servitor set <ref> --status blocked --on human` — always say
   on WHOM and why in a note.
5. Abandoning: `--status dropped`. Never fake done.

## Process

Classify at intake and say so; log one intake note with the rating.

- **Tier** decides how much process applies: 0 trivial (one obvious edit),
  1 small (local change), 2 feature (multi-file / user-visible surface),
  3 major (cross-cutting or multi-session). When unsure, pick the higher
  one — downgrading mid-task is cheap, discovering missing process isn't.
- **Complexity** (low/medium/high) is uncertainty and blast radius, NOT
  size — a large mechanical change is low, a one-liner in auth is high.
  It throttles how much investigation the contract phase deserves.
- **Spikes** ("research X"): deliverable is an answer, not a change. No
  implementation code on a spike, ever.

**Contract** (tier 1+): present what will exist when the work is done —
what we build, what we explicitly won't, how we'll prove it — per
[references/contract.md](references/contract.md), then request the
`contract_approved` gate. **Do not write implementation code before the
gate passes** (tier 2+). Tier 0 skips the gate; the one-line done-when
goes in the intake note.

**Note-worthiness** — log an event when it changes the record, not to
narrate:

- note: a load-bearing discovery, a blocker's cause, why the plan changed
- decision: a real tradeoff was made (prefer API/MCP decision events; on
  the CLI, a note prefixed `DECISION:` and let the human gate it)
- never: restating tool output, progress chatter, or a re-read of state
  the ledger already holds

**Structure over prose** — the GUI's ticket page builds its cards from
structured events first and falls back to note conventions (`Intake:`,
`DECISION:`, `CORRECTION:`, "open question"). Convention-derived cards are
labelled as such; structured ones are the record. So when the contract is
approved, log its criteria and plan as subitems, not as one prose note:

```
servitor add <ref> criterion "when X, then Y — verified by Z"
servitor add <ref> plan "step 1: ..."
servitor subitem <ref> <ulid-prefix> --state pass|fail     # at presentation
servitor decide <ref> "<what>" --why "<why>"               # a real tradeoff
```

The intake note still carries tier, complexity and intent; the criteria
carry the scorecard.

**Feedback (friction capture)** — when the human corrects or redirects you,
or you catch your own miss, log it in the moment, one line, as a feedback
event on the ticket:

```
servitor log <ref> feedback '{"source":"human|self","finding":"one line"}'
```

- `finding` is REQUIRED (the store rejects feedback without it). `source:
  human` — the human corrected or redirected the agent; `self` — the agent
  caught its own miss. Extra payload fields (e.g. `tag`) are optional; add
  one only when a slice you actually use needs it — don't invent taxonomies
  ahead of use.
- Review what's already in the repo/code BEFORE proposing — the most common
  correction is proposing something that already exists. Checking first is
  cheaper than a correction event.
- The event is the receipt, not the fix. When the same correction happens a
  SECOND time, edit the skill in that session — the fix is the skill edit
  the next session loads, not a retrospective query of the feedback log.

**Hard checkpoints** — every tier, never scaled away, prior approval never
carries over:

1. No commit before the human has seen a work summary — approach and why,
   key decisions, core files, deviations. Not a diffstat.
2. No PR-comment reply without showing the exact text for approval first.
3. No push without an explicit prompt naming what and where. Approval to
   commit is not approval to push.

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
