# Servitor — agent orientation

This repo is managed by servitor (worklog/task system, daemon on :8181).
A SessionStart hook injects `servitor ctx` output into every new session.

Rules:

1. Read the injected ctx output before planning work. It names the focused
   ticket (SERVITOR_TICKET) and lists open cards when nothing is focused.
2. No focus and no applicable card? Run `servitor board` before picking up
   work — never invent a ticket that servitor doesn't know about.
3. Log load-bearing discoveries and decisions as events on the ticket
   (`servitor log <ref> note "..."`), not in commit messages or chat.
4. Gates: never claim a human approval that didn't happen. Human gates use
   `SERVITOR_ACTOR=human:<name>`.
5. Multi-agent isolation is enforced, not advisory: this checkout refuses
   agent commits. Before editing, claim a worktree (skills/devops/worktrees:
   `git worktree add` + `.claim`), `servitor claim <ref>` the ticket, and
   release both at the end. A pre-commit hook rejects canonical-checkout
   commits; SERVITOR_ALLOW_CANONICAL=1 is the loud human exception.
6. No commit before the human has seen a work summary; no push without an
   explicit prompt naming what and where.

Full process: `servitor` skill (Hermes: ~/.hermes/skills/servitor,
Claude: ~/.claude/skills).
