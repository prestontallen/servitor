---
name: ticket-flow
description: Workflow for every ticket — own branch, own worktree, PR-based merge. No work ever lands on main directly.
---

# Ticket flow: branch per ticket, merge by PR

Main is integration-only. No agent commits to main, no agent pushes to
main, and the pre-commit guard already refuses commits in the canonical
checkout. Every piece of work travels the same path:

```
ticket → worktree+branch → commits → push branch → PR → human merges
```

## 1. Start a ticket

```bash
servitor set <ref> --status active          # claim the card — the cross-machine mutex
REPO=$(git -C <canonical> rev-parse --show-toplevel)
TREE="$(dirname $REPO)/$(basename $REPO)-worktrees/<ticket-slug>"
BRANCH="agent/<agentname>/<ticket-slug>"
git -C "$REPO" fetch origin
git -C "$REPO" worktree add -b "$BRANCH" "$TREE" origin/main
cd "$TREE"
printf '%s\n' "pid=$$ branch=$BRANCH" > .claim
```

One branch per ticket, named for the ticket slug — not for the task
du jour. If the work outgrows the ticket, split the ticket, then split
the branch. Branch and worktree die together at merge time.

## 2. Work

All commits happen in the worktree, on the branch. Small commits, honest
messages. If you fix something unrelated, it goes in its own branch off
main with its own PR — never rides along (the a1e127d lesson).

## 3. Open the PR — do not push to main

```bash
git fetch origin && git rebase origin/main   # in YOUR tree, before every push
git push origin "$BRANCH"                    # branch push only, never main
gh pr create --base main --head "$BRANCH" \
  --title "<ticket-slug>: <one-line what>" \
  --body "Ticket: <ULID/slug>. What changed, how it was verified, what to eyeball."
```

The PR is the presentation: it is what the human reviews and merges.
Work summaries in chat do not replace the PR, and PRs do not merge
themselves — merging is a human act (or an agent's, only on explicit
human instruction naming the PR).

## 4. Merge and clean up

After the human merges:

```bash
git -C "$REPO" fetch origin && git -C "$REPO" worktree remove --force "$TREE"
git -C "$REPO" branch -D "$BRANCH"
servitor log <ref> note "PR #N merged; branch + worktree released."
```

Local main never carries exclusive work: if main ever has commits that
are not on origin/main, that is a defect in process — they belong in a
branch and a PR. Get them onto one immediately.

## Exceptions

- **Trivial hotfix with human approval**: still a branch + PR. There is
  no tier small enough to skip it — the guard enforces the worktree
  part, and review enforces the rest.
- **SERVITOR_ALLOW_CANONICAL=1**: exists for human-directed exceptions
  only (e.g. repairing this workflow itself). Using it without a human
  explicitly directing it is a violation, not a shortcut.

## Cross-machine notes

Other agents work from other clones. The only shared ground is
origin/main; `.claim` files are invisible across machines. Card
claiming (step 1) is the only mutex they can see. Push rejection means
rebase your branch onto origin/main and push the branch again — never
force-push, never rewrite others' commits.
