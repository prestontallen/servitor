# Commits and PR descriptions

Channel format for [concise-tone](../SKILL.md). Applies the core voice and cut-list;
this file adds structure.

A commit message or PR description states what this change does and why — for a
teammate who wasn't in the room — and nothing more.

## Commit format
- `type: Subject`, then an optional body, then the footer.
- `type` ∈ `feat` `fix` `refactor` `chore` `docs` `test`; add a `(domain)` when
  useful, e.g. `feat(auth)`.
- **Subject** — imperative ("Add", not "Added"), capitalized, no trailing period,
  ≤50 chars (72 max).
- **Body** (optional) — the *why*, wrapped at 72. Reserve a body for multi-file
  changes or a non-obvious decision worth recording. An obvious, single-purpose
  change is subject-only; don't manufacture a body to fill the slot.
- **Footer** — the ticket key when the repo uses one (e.g. `PROJ-123`) and, as
  the final line, any trailer the repo requires (e.g. a `Co-authored-by:` line).
- Run the repo's formatter before committing. Keep commits atomic — one logical
  change each.
- **State only this change.** No recap of prior commits, no "already fixed in
  X/Y/Z," no adjacent-work context.

## PR description format
- **Summary** — what + why, short.
- **Test plan** — how it was verified.
- **Footer** — the same trailer as the last line.

When the repo requires a trailer, it is not optional: keep it if present, **add it if
missing** — never assume the source already has it.

## Before → after
A PR summary, and the reviewer's reaction to it:

```
# before — names people, buries a sign-off ask, hedges a settled decision, cites the plan
The export shape (rows[].{id, label, count} + top-level generatedAt) is
provisional and needs Alice / Bob sign-off per PROJ-456 plan §10 step 1.
Locked in by schema invariants in export/format_test.go so any future
drift fails loudly at test time.
```

> Reviewer: "is this new since we reviewed the design? I almost missed it since
> I'm not tagged in the summary, and I'm sure the other reviewer hasn't seen it."

The summary made a settled decision look unresolved and addressed a sign-off request
to nobody. State what shipped and how it's enforced — and if sign-off is truly
outstanding, that's a standup item or a direct ping, not a line here.

```
# after — what shipped, and the test that pins it
Export shape: rows[].{id, label, count} with a top-level generatedAt. Pinned
by schema invariants in export/format_test.go, so any future drift fails at
test time.
```
