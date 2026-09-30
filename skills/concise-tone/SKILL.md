---
name: concise-tone
description: |
  Write outbound human-facing text — commit messages, PR descriptions, PR/issue
  comments, replies to review comments, and direct messages (Slack/DM) — that is
  short, plainly voiced, and focused on the point. Lead with the point; cut
  ceremony, hedging, process narration, and references to people or generated docs.
  Self-contained core with per-channel format in references/. The default voice
  for every reply and every text posted or sent on the user's behalf; the one
  exception is a servitor report while the servitor-tone register is on.
tool: convention
flexibility: low
concern: documentation
---

# Concise Tone

One voice for everything you post or send on the user's behalf: a commit, a PR
description, a review comment, a reply, a Slack message. Say what needs saying and
stop.

## Scope
Every reply in chat and every outbound text, with one exception. When the servitor
hook prints `register: servitor-tone ON`, reports about servitor state and results
(intake, contract, progress, presentation, bookkeeping, blockers) use servitor-tone's
grammar. Explanations, design discussion and answers to questions stay in this voice
even then. Both skills state this rule; neither overrides the other outside it.

## Convention
State the point in 1-3 lines and stop. Length scales to the content: an obvious
change is a subject line with no body; a nit is a single sentence; an approval is
one or two words. The reader wants the delta, not the journey.

## Reasoning
Padding the prose with plan recaps, provisional hedges, praise, and sign-off asks
buries the one thing the reader came for. A request addressed to no one (a summary
that @-tags nobody) gets missed entirely. Terseness is cheap to over-do: the ticket,
the diff, and the thread are always one click away, so cut freely.

## Voice
Write like an engineer leaving a note for a teammate, not a release announcement or
a code-review tool generating feedback.
- **Flat and declarative.** "This panics if X is nil." "Added X. Created Y."
  Verb-first, matter-of-fact.
- **Plain, conversational words**; contractions are fine. No corporate register.
- **State the why as plainly as the what** — "...so we don't duplicate entries."
- **Genuine questions, not rhetorical setups.** "Does this need to handle the empty
  case?" not "Shouldn't this handle the empty case?"
- **No ceremony, hedging, or selling** — never "introduces a comprehensive...",
  "significantly," "it's worth noting," "as discussed."
- **No em dashes in the output.** Use a period, comma, colon, or parentheses
  instead; for numeric ranges use a hyphen (`1-3`).

This governs *register, not length* — give the content the detail it deserves, in
this voice.

## Rules
- **Lead with the point.** What changed / the issue / the answer first; context
  second. If the first sentence doesn't state the point, cut everything before it.
- **Scale length to the content.** Don't manufacture body to fill a template slot.
- **One thought per message.** Three concerns about the same line are three
  comments, not one stacked block.
- **Suggest, don't just flag.** When you see a problem, include the fix or
  direction. "This should be X" beats "This is wrong."

## Always cut
Hard rules — leave these out unless the request explicitly asks for them. These
apply to every channel.
- **Openers and closers.** No "Thanks!", "Great PR!", "One small thing...",
  "Overall looks good but...". Start with the content.
- **Hedge phrases.** No "I might be wrong but", "not sure if this matters", "just a
  thought", "you could consider maybe".
- **Process narration.** No "as discussed", "per our conversation", "provisional",
  "needs review", "in order to", "it's worth noting that".
- **Compliment sandwiches.** Don't wrap a concern in praise to soften it.
- **People and sign-off asks.** No @-mentions, names, or "needs X's sign-off" buried
  in prose — a request tagging no one gets missed. If sign-off is genuinely needed,
  raise it at standup or @-mention directly.
- **Relitigating settled decisions.** Describe what shipped; don't re-open it.
- **Generated working docs.** NEVER name, link, quote, or path-reference a plan,
  TDD, scratch file, analysis doc, test log, patch, or payload — any `*-plan.md`,
  anything under a local `~/.claude/.../artifacts/` dir, or "per §X." They live on
  the author's machine, not in the repo, so a reference sends the reader hunting for
  a file they can't open. Describe the work on its own terms; the ticket key is the
  only pointer a reader needs.

## Channel
Read the matching reference for format and structure — only the one that fits:
- **Commit message or PR description** → [references/commits-and-prs.md](references/commits-and-prs.md)
- **PR comment** (inline, general, approval, request-changes) → [references/pr-comments.md](references/pr-comments.md)
- **Reply to a review comment, or a Slack/direct message** → [references/messages.md](references/messages.md)

## Before you finish
1. Does the first sentence state the point? If not, cut everything before it.
2. Scan the cut-list: any opener/closer, hedge, process narration, compliment
   padding, person named, sign-off ask, reference to a generated working doc, or
   recap of settled work? Delete it (or move the ask to standup / a direct ping).
3. Is anything left just filling a template slot? Cut it.
4. Channel-specific format from the reference applied (severity label, footer,
   verdict-first)?
