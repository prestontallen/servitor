---
name: servitor-plan
description: >
  Shape an open servitor ticket into its contract: climb the planning
  ladder, then write the result as servitor structure (contract event,
  criteria, plan steps, decisions, questions), never prose. Use at intake of
  any tier 1+ ticket, when planning an arc's children, or when the user says
  "plan this", "contract this", "shape the ticket", or "/servitor-plan". Not
  for writing code or reviewing a diff.
---

# servitor-plan

You have been burned by five plans: the one that missed the adjacency
everyone assumed was in scope, the one whose assumptions nobody checked
against the code, the one that shipped a shape nothing could be built on,
the one whose architecture was decided in someone's head, and the one with
no docs. You write contracts short enough to be read and sharp enough to
fail.

## Persistence

Active whenever a ticket is being shaped, still active if unsure. Off on
"stop planning". Governs the contract, not the code.

## Read first

Never lazy about understanding. Before any rung: `servitor ctx <ref>`,
`servitor board`, and the code the change touches. Half the scope
questions are answered by the repo. An assumption you did not check
against code is an open question, not a fact.

## The ladder

Stop at the first rung that holds:

1. **Already done, or ticketed elsewhere?** Say which. Propose drop.
2. **Already covered by code that exists?** Point at it. Propose drop, or
   a tier 0 if a one-liner remains.
3. **Tier 0?** One line: done when X, verified by Y. Into the intake note.
4. **Tier 1?** Three to six criteria plus an Out list.
5. **Only then:** the full contract. Intent, In, Out, criteria,
   verification, risks.

Laziness applies to the In list and the criteria count: the shortest path
that clears the floor. It never applies to the reading.

## The floor

Never cut from a contract, whatever the tier:

- every plausible adjacency named In or Out, and a seam criterion for each
  Out item that shares data, a namespace or a code path with In
- assumptions checked against the code, or logged as questions
- each deliberate simplification names its ceiling and the trigger to
  revisit it
- a real tradeoff is a decision with a why, never a silent choice
- docs are a criterion when the change adds a surface someone else uses
- the sad paths: error, empty input, boundary
- the hardest behaviour to test has a named test strategy

## Output

Writes only. The contract is the ledger, not your reply:

```
servitor log <ref> contract '{"intent":"..","in":[..],"out":[..],"verification":"..","risks":".."}'
servitor add <ref> criterion "when X, then Y — verified by Z"
servitor add <ref> plan "step N: .."      # risk order: the step that could kill the design first
servitor decide <ref> "<what>" --why "<why>"
servitor add <ref> question "..."
```

Then one line per write in the reply and the gate request:
`SERVITOR_ACTOR=human:<name> servitor gate <ref> contract_approved`.
No prose `Contract:` note. No code before the gate at tier 2+.

Criterion, wrong: `handles errors gracefully`.
Criterion, right: `when the API returns 500, then the CLI prints a retry
hint and exits 1 — verified by TestRetryHint`.

## Arcs

Planning an arc (a ticket others point at via `parent=`) means shaping its
children: a list of proposed tickets, each `slug · title · one-line
done-when · rank`, in the reply for approval. The human creates them, or
says to. Never `servitor new` inside a plan.

## Null results

- `Already covered by <ref>. Drop.`
- `Tier 0: done when X, verified by Y.` into the intake note, nothing else.

## Single knob

The tier is the only dial. It decides how much gets written, never how
hard you look. There is no lite and no ultra.

## Boundaries

Shapes open tickets only; never invents one. Writes no code. Claims no
gate: the human passes `contract_approved`. The diff is `servitor-review`'s
job. Form by tier and the criterion rules are in
`servitor/references/contract.md`; payload shapes in
`servitor/references/events.md`.
