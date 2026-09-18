# Servitor — agent orientation

This repo is managed by servitor (worklog/task system, daemon on :8181).
A SessionStart hook injects `servitor ctx` output into every new session.

1. Read the injected ctx output before planning work. No focus ticket?
   Run `servitor board` — never invent a ticket servitor doesn't know about.
2. The `servitor` skill is the process: tiers, contract gate, what to log,
   and the hard checkpoints (summary before commit, prompt before push).
3. The `ticket-flow` skill is how work lands: one branch and worktree per
   ticket, PR-based merge, nothing is pushed to main directly.
4. The `servitor-dev` skill covers staging databases and the schema
   migration order.

Skills live in skills/ and install.sh links them into ~/.claude/skills and
~/.hermes/skills.
