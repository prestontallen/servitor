# PR comments

Channel format for [concise-tone](../SKILL.md). Applies the core voice and cut-list;
this file adds structure.

A PR comment says exactly what needs saying and stops.

## Inline (line-level) comments
- State the issue, question, or suggestion directly. No preamble.
- **Say blocker or nit, not nothing.** If it's optional, prefix `nit:`. If it must
  be addressed before merge, say "Blocking:" or "This needs to change before merge."
- One thought per comment. Don't stack multiple concerns.

## General (top-level) comments
- Verdict first — "Looks good.", "Two blockers below.", "One question."
- If listing items, use a flat bullet list. No headers unless there are four or more
  distinct categories. List by severity: blockers first, then nits.

## Approvals
- "LGTM." or one sentence stating what you verified, then "LGTM."
- Never: "Great work!", "Thanks for the PR!", or any opener that delays the verdict.

## Request changes
- Start with the blocker, not a compliment softener.
- List items by severity if there are multiple: blockers first, then nits.

## Before → after

```
# before — hedged, padded, delayed
I might be missing context here, but I noticed that this function doesn't
seem to handle the case where the slice is empty. Not sure if that can
happen in practice, but it might be worth adding a check? Just a thought.
```

```
# after — direct, states the risk
This panics on an empty slice. Add a length guard or return early.
```

---

```
# before — opener delays the verdict
Overall this looks really solid and the approach makes sense! Just one small
nit: the variable name `tmp` is a bit generic.
```

```
# after — nit labeled, no opener
nit: `tmp` is too generic here. `pending` or `unsorted` would be clearer.
```
