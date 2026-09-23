# Plan and review events: the shapes the skills write and the GUI reads

`servitor-plan` and `servitor-review` write structure, not prose. These are
the payloads. The ticket page reads exactly these keys, so a change here is
a change to the skills and the GUI together.

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
servitor subitem <ref> <criterion-prefix> --state fail --evidence "board card shows no plan mark on staging"
```

`evidence` is stored in `subitems.fields.evidence` and surfaces as
`criteria[].evidence` in `servitor ctx`. The Contract card prints it beside
the check. Omitting it is allowed; the card then shows only who and when.

## finding (subitem)

One subitem per review finding, so each can be closed by identity later.
Written by `servitor-review` in both phases.

```
servitor add <ref> finding "src=self internal/store/read.go:L212: scope: touches List, which Out excludes. Revert it or amend the contract."
servitor add <ref> finding "src=octocat https://github.com/o/r/pull/9#discussion_r1 cmd/x.go:L40: check: no runnable check for the parser. Add one assert-based test."
servitor subitem <ref> <finding-prefix> --state applied
```

- Body grammar: `src=<self|reviewer handle> [<comment url>] <loc>: <tag>
  <what>. <fix>.` The source and URL ride in the body for now; `servitor
  add` cannot set fields. Ceiling: when a view needs to filter findings by
  source, add a field flag to `servitor add` and move them.
- `loc` is `file:L12`, `file:L12-38`, or a ticket location such as
  `contract` or `branch`.
- Tags: `criterion`, `scope`, `seam`, `branch`, `stray`, `check`, `marker`,
  `migration`, then ponytail-review's `delete`, `stdlib`, `native`, `yagni`,
  `shrink`. A finding without a fix is not a finding.
- `state` is null while open, then `applied`, `rejected` or `escalated`.
  A rejection is also a `servitor decide` with a why. An accepted external
  finding the agent should have caught is also a `feedback` event with
  `source: human`.

## review (ledger event)

One event per review run. The latest run is what `servitor ctx` returns;
earlier runs stay in the ledger with a run count.

```
servitor log <ref> review '{
  "verdict": "present",
  "summary": "3/3 criteria pass, 0 unverified, 1 finding",
  "phase": "self"
}'
```

- `verdict` is required and is `present` or `hold`. `present` means the
  work can go to the `presented` gate. `hold` means it cannot yet.
- `summary` is one line: the scorecard. For tickets with criteria it is
  `N/M criteria pass, K unverified, F findings`. For tier 0 it is pass or
  fail against the intake note's done-when line.
- `phase` is `self` (the agent's own diff, before presenting) or
  `external` (after triaging reviewer comments on the PR).
- Findings are NOT in the payload. They are `finding` subitems, above.
  Criterion pass/fail is written with `subitem.set` before the review
  event. The review event is the verdict; the criteria and findings are
  the scorecard.

`servitor ctx` exposes the latest run as `review` with `actor`, `ts` and
`runs` (the count of review events on the ticket). That is the skill's
read-back; the GUI reserves no space for review (decided 2026-09-22) and
the ledger fold is where a human sees the runs.

## What the GUI reserves

- Ticket page: Contract and Plan sections always render. An empty one shows
  a single line naming the skill that fills it.
- Board card: one mark, criteria pass over total (`—` when none), read from
  `/api/board` fields `criteria_pass` and `criteria_total`.

## Classification

`contract` and `review` are signal events (like `note`, `decision`, `gate`,
`feedback`), not transitions. They count as human- or agent-chosen records
in every view that weights by class.
