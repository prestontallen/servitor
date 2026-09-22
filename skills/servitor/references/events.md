# Plan and review events: the shapes the skills write and the GUI reads

`servitor.plan` and `servitor.review` write structure, not prose. These are
the payloads. The ticket page and the board read exactly these keys, so a
change here is a change to both skills and the GUI together.

Everything lands in ledger payload jsonb or `subitems.fields` jsonb. No
schema migration is needed for any of it.

## contract (ledger event)

The contract document, replacing the `Contract:` note convention. The
latest `contract` event on a ticket is the document; earlier ones are
versions and stay in the ledger.

```
servitor log <ref> contract '{
  "intent": "one sentence: what exists when this is done",
  "in":  ["thing we build", "..."],
  "out": ["adjacency we explicitly do not build", "..."],
  "verification": "how the hardest behaviour is proven, named",
  "risks": "the step that could kill the design, and any open question"
}'
```

- `intent` is required; the store rejects a contract without it.
- `in`, `out` are arrays of strings. `verification`, `risks` are strings.
- Acceptance criteria are NOT in the payload. They are `criterion`
  subitems (`servitor add <ref> criterion "when X, then Y — verified by Z"`)
  so each one can be passed or failed by identity.
- Tier and complexity stay in the intake note; the card reads them from
  there.

The Contract card shows source `contract event` when one exists, and falls
back to `Contract:` / `Intake:` notes for tickets that predate this.

## criterion evidence (subitem.set)

Passing or failing a criterion carries how it was proven:

```
servitor subitem <ref> <criterion-prefix> --state pass --evidence "go test ./internal/store -run TestCtxReadContract"
servitor subitem <ref> <criterion-prefix> --state fail --evidence "board card shows no verdict mark on staging"
```

`evidence` is stored in `subitems.fields.evidence` and surfaces as
`criteria[].evidence` in `servitor ctx`. The Contract card prints it beside
the check. Omitting it is allowed; the card then shows only who and when.

## review (ledger event)

One event per review run. The latest run is what `servitor ctx` returns;
earlier runs stay in the ledger with a run count.

```
servitor log <ref> review '{
  "verdict": "present",
  "summary": "3/3 criteria pass, 1 finding",
  "findings": [
    {"loc": "internal/store/read.go:L212", "tag": "scope", "what": "touches List, which the Out list excludes", "fix": "revert the List change or amend the contract"}
  ]
}'
```

- `verdict` is required and is `present` or `hold`. `present` means the
  work can go to the `presented` gate. `hold` means it cannot yet.
- `summary` is one line. For tickets with criteria it is the scorecard
  (`N/M criteria pass, K unverified`). For tier 0 it is pass or fail
  against the intake note's done-when line.
- `findings` is an array, possibly empty. Empty with `present` is the null
  result (`Lean already. Ship.`).
- Each finding: `loc` (`file:L12` or `file:L12-38`; may be a ticket-level
  location like `contract` or `branch`), `tag` (the skill's tag vocabulary,
  free text here), `what` (one clause), `fix` (one clause, required; a
  finding without a fix is not a finding).
- Criterion pass/fail is written with `subitem.set` before the review event,
  not inside it. The review event is the verdict; the criteria are the
  scorecard.

`servitor ctx` exposes the latest run as `review` with `actor`, `ts` and
`runs` (the count of review events on the ticket). That is the skill's
read-back; the GUI reserves no space for review (decided 2026-09-22) and
the ledger fold is where a human sees the runs.

Open direction, not yet built: once the review skill also triages external
PR review, each finding needs its own disposition (applied, rejected with a
why, escalated). That means `finding` subitems with a `source`, not an
array inside the run event. Settle it with the skill, not before.

## What the GUI reserves

- Ticket page: Contract and Plan sections always render. An empty one shows
  a single line naming the skill that fills it.
- Board card: one mark, criteria pass over total (`—` when none), read from
  `/api/board` fields `criteria_pass` and `criteria_total`.

## Classification

`contract` and `review` are signal events (like `note`, `decision`, `gate`,
`feedback`), not transitions. They count as human- or agent-chosen records
in every view that weights by class.
