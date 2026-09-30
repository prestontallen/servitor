# Replies and direct messages

Channel format for [concise-tone](../SKILL.md). Applies the core voice and cut-list;
this file adds structure for replies to review comments and Slack/direct messages.

These are more conversational than a commit or a PR body, but the register is the
same: lead with the answer, cut the ceremony.

## Replies to review comments
- **Explain the fix.** When code resolves a review comment (from a human, bot, or
  tool), reply stating what changed and where. Never resolve silently.
- Lead with the resolution — "Fixed: added a length guard in `parse()`." — then a
  clause of why only if it isn't obvious.
- If you disagree, say so plainly and give the reason. Don't soften with praise or
  hedge with "I might be wrong but".
- If the comment can't be addressed in this PR, say that and point to where it's
  tracked (a ticket key), not a promise.

## Direct and Slack messages
- **Open with the answer**, not a greeting or a windup. "Yes, that's deploying now."
  not "Hey! So I was looking into this and..."
- One message, one point. If you have three things, three short lines or a bullet
  list — not a wall of prose.
- A genuine question is fine; a rhetorical one isn't.
- No status narration ("just wanted to circle back", "quick update"): give the
  status.
- Skip sign-offs and pleasantry padding. A plain answer isn't curt in this channel.

## Before → after

```
# before — greeting, windup, buries the answer
Hey! Thanks for flagging this. I went back and looked at the handler and I
think you were right that there was an issue there. Anyway, I've pushed a fix
now so it should hopefully be good — let me know if anything else comes up!
```

```
# after — answer first, states the fix
Fixed: the handler now guards the empty case, pushed in a1b2c3d. Let me know if
you hit anything else.
```

---

```
# before — review reply that resolves nothing
Good catch, updated!
```

```
# after — says what changed and where
Added the nil check in `resolveUser()` and a test for the empty-token case.
```
