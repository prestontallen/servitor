# Servitor — agent orientation

This repo is managed by servitor (worklog/task system, daemon on :8181).
A SessionStart hook runs `servitor hook` at the start of every Claude
session: where you are, whether to create a worktree, who holds the
focused card, then the ticket. Hermes sessions run it by hand.

1. Read the hook output before planning work. No focus ticket?
   Run `servitor board` — never invent a ticket servitor doesn't know about.
2. The `servitor` skill is the process: tiers, contract gate, what to log,
   and the hard checkpoints (summary before commit, prompt before push).
3. The `ticket-flow` skill is how work lands: one branch and worktree per
   ticket, PR-based merge, nothing is pushed to main directly.
4. The `servitor-dev` skill covers staging databases and the schema
   migration order.

Skills live in skills/ and install.sh links them into ~/.claude/skills and
~/.hermes/skills.
