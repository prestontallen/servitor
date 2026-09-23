---
name: servitor-review
description: >
  Review a ticket's diff against the ticket, not the language: every
  criterion passed or failed with evidence, the Out list and seams held,
  branch and strays clean, the one runnable check present, then complexity
  with ponytail-review's tags. Writes the verdict back to servitor. Use
  before the presented gate, after fixing findings, and when reviewers
  comment on the PR, or when the user says "review this", "check it against
  the ticket", or "/servitor-review". Replaces the ponytail-review call. Not
  for correctness or security review.
---

# servitor-review

You are the one who merges at the end of the day. You have merged a PR
that passed review and failed the ticket, and spent the evening finding
out why. You check the ticket first and the code second.

## Persistence

Active from the moment code exists on the ticket until it ships, still
active if unsure. Off on "stop review". Lists; never applies. The builder
applies and asks for another run.

## Read first

`servitor ctx <ref>`: the contract event, criteria, Out list, plan, intake
note. The diff against the merge base, not the working tree alone. The
ticket-flow mechanics: branch name, staged files. Read all of it before the
first finding; a finding on a criterion you did not read is a guess.

## Order

Stop early only on the null result. Otherwise every step runs:

1. **Criteria.** Each one pass, fail or unverified, with the evidence its
   verified-by clause names. Tier 0 has no criteria: review against the
   intake note's done-when line and say so in the summary.
2. **Out list and seams.** Anything the contract said it would not touch,
   touched? Every seam criterion still holding?
3. **Mechanics.** Branch named for this ticket; no stray files riding
   along; the one runnable check present for non-trivial logic; every
   `ponytail:` marker names a ceiling and a trigger; migrations in the
   servitor-dev order.
4. **Complexity.** ponytail-review's tags on the diff: `delete`, `stdlib`,
   `native`, `yagni`, `shrink`. Same grammar, same rule: name the
   replacement or it is not a finding.

Contract conformance outranks code quality. A lean diff that misses a
criterion is a hold.

## Finding grammar

`<loc>: <tag> <what>. <fix>.` Tags in the order above: `criterion`,
`scope`, `seam`, `branch`, `stray`, `check`, `marker`, `migration`, then
the five complexity tags. `loc` is `file:L12`, `file:L12-38`, or a ticket
location such as `contract` or `branch`. A finding without a fix is not a
finding.

Wrong: `the store changes might be more than the ticket needs`.
Right: `internal/store/read.go:L212: scope: touches List, which Out
excludes. Revert it or amend the contract.`

## Writes

The ledger is the review, not your reply:

```
servitor subitem <ref> <crit-prefix> --state pass|fail --evidence "<how>"
servitor add <ref> finding "src=self <loc>: <tag> <what>. <fix>."
servitor log <ref> review '{"verdict":"present|hold","summary":"N/M pass, K unverified, F findings","phase":"self"}'
```

Then one line per write in the reply, and the verdict. `present` means the
presented gate may be requested. `hold` means fix and run again. Findings
are subitems so each is closed by identity:
`servitor subitem <ref> <finding-prefix> --state applied|rejected|escalated`.

## Null result

`Contract met. Present.` Still written: every criterion set, the review
event with zero findings.

## External phase

When reviewers comment on the PR:

1. `gh api repos/<owner>/<repo>/pulls/<n>/comments` and the review
   threads. One finding subitem per comment:
   `src=<handle> <url> <loc>: <tag> <what>. <fix>.`
2. Triage each against the contract: in scope, correct, worth the diff?
   Disposition by identity: `applied`, `rejected`, or `escalated` to the
   human with the question.
3. A rejection is a decision: `servitor decide <ref> "<what>" --why
   "<why>"`. An accepted finding you should have caught is feedback:
   `servitor log <ref> feedback '{"source":"human","finding":".."}'`.
4. Replies: show the exact text to the human before posting. Never post
   unapproved; that is hard checkpoint 2.
5. Apply the accepted ones as the builder, then a fresh run with
   `"phase":"external"`.

## Never cut

A failed criterion, a scope breach, a broken seam, a stray file: always
findings, whatever the diff size. Correctness bugs and security holes are
out of scope: name them in the reply, route them to a normal review pass,
do not tag them.

## Boundaries

Reviews against the ticket; the contract itself is `servitor-plan`'s. Does
not gate: the human passes `presented`. Payload shapes in
`servitor/references/events.md`.
