---
name: ticket-flow
description: Workflow for every ticket — own branch, own worktree, PR-based merge. No work ever lands on main directly.
---

# Ticket flow: branch per ticket, merge by PR

Main is integration-only: no agent pushes to main, ever. CI runs on every
PR and a red check means the PR does not merge. (Server-side branch
protection needs a public repo or a paid plan; it lands with the
main-branch-protection ticket once the repo can go public.) Every piece
of work travels the same path:

```
ticket → worktree+branch → commits → push branch → PR → human merges
```

## 0. First five minutes

Every session starts with `servitor hook` output (Claude: the SessionStart
hook install.sh registers; Hermes: run it yourself). Read it, then check:

1. `SERVITOR_ACTOR` is `agent:<you>`, not the default `agent:cli`.
2. Where you are. `canonical checkout` means create a worktree before
   editing; the commands are in the output. In a worktree, the branch
   must be your ticket's. `canonical checkout on <anything but main>`
   means someone checked a branch out there: restore it first (the hook
   prints how), because every agent on this host shares that checkout.
3. Behind origin/main? Fetch and rebase before you push, not mid-thought.
4. The focused card `active by` someone else? Do not start it.
   Coordinate or pick another card.

## 1. Start a ticket

Look at `servitor board` first. A card that is already `active` shows
`active_by` and `active_since`: do not start a card active under another
actor — coordinate or pick another card. That is the only mutex agents
on different machines can see.

```bash
export SERVITOR_ACTOR=agent:<agentname>
servitor set <ref> --status active
git fetch origin
git worktree add -b agent/<agentname>/<ticket-slug> ../servitor-worktrees/<ticket-slug> origin/main
servitor set <ref> branch=agent/<agentname>/<ticket-slug> worktree=../servitor-worktrees/<ticket-slug> pushed=false next="contract"
cd ../servitor-worktrees/<ticket-slug>
```

Claude Code sessions: `cd` does not move the session, every tool call
resets to the launch directory. After `git worktree add`, call
`EnterWorktree` with `path=../servitor-worktrees/<ticket-slug>` so the
session itself runs in the worktree. Hermes: `cd` is enough, but never
`git checkout` a branch in the canonical checkout.

One branch per ticket, named for the ticket slug — not for the task
du jour. If the work outgrows the ticket, split the ticket, then split
the branch. Branch and worktree die together at merge time. The
canonical checkout stays on main and takes no edits.

## 2. Work

The stash stack is shared by the canonical checkout and every worktree,
and another session can pop it. Never bare `git stash` / `git stash pop`;
set work aside with a WIP commit on your branch and squash it later.

All commits happen in the worktree, on the branch. Small commits, honest
messages. Every commit updates the handoff record in the same breath:
`servitor set <ref> head=<sha> pushed=false next="..."`. Stage files by name, never `git add -A` or `git add .`: a
half-committed change (a state file without its consumers, a stray test
file riding along) is exactly what that shortcut produces. If you fix
something unrelated, it goes in its own branch off main with its own PR.

## 3. Open the PR — do not push to main

```bash
git fetch origin && git rebase origin/main   # in YOUR tree, before every push
git push origin agent/<agentname>/<ticket-slug>
servitor set <ref> head=<sha> pushed=true checkpoint=push-ok next="open the PR"
```

Open the PR against main with `gh pr create` or on GitHub: title
`<ticket-slug>: <one-line what>`, body naming the ticket, what changed,
how it was verified, and what to eyeball. CI runs vite build, go build,
go vet and go test on every PR; a red check blocks the merge.

The PR is the presentation: it is what the human reviews and merges.
Work summaries in chat do not replace the PR, and PRs do not merge
themselves — merging is a human act (or an agent's, only on explicit
human instruction naming the PR). Push rejected because origin/main
moved? Rebase your branch and push again; never force-push.

## 4. Merge and clean up

After the human merges:

```bash
servitor gate <ref> shipped                   # merged on origin/main IS shipped
git -C <canonical> fetch origin
git -C <canonical> worktree remove --force ../servitor-worktrees/<ticket-slug>
git -C <canonical> branch -D agent/<agentname>/<ticket-slug>
git push origin --delete agent/<agentname>/<ticket-slug>
servitor set <ref> staging=- next="none, done"   # staging torn down with the worktree
servitor log <ref> note "PR #N merged; branch + worktree released."
```

Local main never carries exclusive work: if main ever has commits that
are not on origin/main, that is a defect in process — they belong in a
branch and a PR. Get them onto one immediately.

## Definition of done

A ticket is DONE when its PR is MERGED and the changes are on main in
the REMOTE repository (origin/main). An approved work summary, a pushed
branch, or an open PR is not done. Contracts and acceptance criteria
should state it exactly that way: "merged into main and on the remote
repository." Only after the merge lands on origin/main: set the ticket
done, release the worktree and branch, and log the merge.

## Exceptions

Trivial hotfix with human approval: still a branch + PR. There is no
tier small enough to skip it — the PR is where review happens.
