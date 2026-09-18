# The contract: what "done" means, written down before the work

A contract is the agreement presented at the `contract_approved` gate. It
answers three questions no code review can: *what are we building, what are
we explicitly not building, and how will we prove it's done?*

Two layers, kept distinct:

- **Acceptance criteria** — task-specific. "Did we build the right thing?"
- **Definition of done** — the standing project bar, same every task.

## Writing acceptance criteria

1. **Observable from outside.** "Handles errors gracefully" is not a
   criterion. "When the API returns 500, the CLI prints a retry hint and
   exits 1" is — you can run it and watch.
2. **Every criterion carries its verification.** A command, a test name, or
   a concrete manual check. If you can't say how it'll be verified, the
   criterion isn't done being written.
3. **Behavioral form.** Prefer "when/given X, then Y".
4. **Cover the sad paths.** Error handling, empty inputs, and boundaries
   are criteria, not afterthoughts.
5. **Few and sharp beats many and mushy.** If a criterion doesn't change
   what you'd build or test, cut it.
6. **Guard the seams.** Every Out-of-scope item that shares data, a
   namespace, or a code path with in-scope work gets one criterion
   asserting the boundary holds. "X is out of scope" and "X is unreachable
   from the new code" are different claims.

## Drafting

- **Investigate before drafting.** Read the relevant code; half the scope
  questions are answerable from the repo.
- **The Out list is worth more than the In list.** Put every
  plausible-but-not-requested adjacency there explicitly.
- **Surface open questions** instead of silently resolving them — each is
  either answered at contract review or logged as an assumption.
- **Test strategy must be concrete:** name how the hardest-to-test behavior
  gets proven. "Unit tests" is not an answer.
- **Risk order:** the step that could kill the design runs early, not last.

## Form by tier

- Tier 0: one sentence — "Done when ⟨observable result⟩, verified by
  ⟨check⟩."
- Tier 1: inline mini-contract — 3–6 done-when lines plus explicit Not
  doing.
- Tier 2+: full contract — Intent, Scope (In/Out), Acceptance criteria,
  Definition of done, Risks & open questions.

Spikes (investigation-first work) are scoped by a question, not a change,
at tier-1 weight; the deliverable is an answer. A fix the research reveals
is a proposed ticket, never a diff.

## Lifecycle

`draft → approved (contract_approved gate) → in-progress → fulfilled`.
Changes after approval are amendments: log a new note stating what changed,
why, and that the human agreed — the ledger's append-only rule is the
amendments log. At presentation time the contract becomes the scorecard:
every criterion with pass/fail and the evidence for it.
