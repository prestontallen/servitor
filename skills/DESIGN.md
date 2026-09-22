# Skill design notes: what ponytail gets right, and how to reuse it

Source: [DietrichGebert/ponytail](https://github.com/dietrichgebert/ponytail)
v4.10.0, `skills/ponytail/SKILL.md` and `skills/ponytail-review/SKILL.md`.
The installed copies (Claude plugin cache, `~/.hermes/skills`) are
byte-identical to upstream as of 2026-09-22. Nothing here is a local fork.

Purpose: extract why those two skills work so well, isolate the process they
encode, and turn that into a reusable pattern for two servitor skills that do
not exist yet: a **review skill** and a **planning skill**. Both write their
output back to servitor as events and get reserved space in the GUI board
view. Sections 5 and 6 carry the decisions.

---

## 1. Why the skills work

The one-line thesis: **they are written as constraints with a test, not as
values.** "Prefer simplicity" is a value the model can agree with and then
ignore. "Stop at the first rung that holds" is a procedure with a stopping
rule, and every rule under it can be checked against something concrete.
Neither file contains a vague adjective the model could satisfy by tone.

The mechanisms, in rough order of how much they carry:

1. **A persona with a stake.** "Lazy senior dev who got paged at 3am." Not a
   checklist, an evaluator. It resolves ambiguous cases the way that person
   would, instead of averaging over every possible reviewer. The persona is
   chosen so its incentives point at the goal: this person suffers from
   complexity personally, so they cut it.

2. **An ordered decision ladder with a stop rule.** Seven rungs, each a
   yes/no question against one specific place to look. Order encodes
   priority, so the model never weighs trade-offs. It climbs until a rung
   holds and stops.

3. **It pre-empts its own failure mode.** The most valuable text in the
   builder skill is the part that says when not to be lazy: "never lazy about
   understanding the problem", "the smallest change in the wrong place is a
   second bug", "grep every caller before you edit". A minimalism prompt
   without that section produces confident wrong fixes. The author watched
   the skill misfire (upstream issues #245, #217) and wrote the guard.

4. **Deferral must name a trigger.** `[code] → skipped: [X], add when [Y].`
   You cannot skip something silently. Every cut carries the condition that
   reverses it. This is what makes terse output trustworthy instead of
   negligent.

5. **Shortcuts become greppable.** The `ponytail:` comment convention
   requires a ceiling and an upgrade path. A judgment call becomes a line
   another tool can harvest. `ponytail-debt` closes the loop.

6. **A hard safety floor.** Trust-boundary validation, data-loss error
   handling, security, accessibility, explicit requests, hardware
   calibration knobs, and one runnable check are exempt from cutting. The
   model never has to decide whether minimalism outranks those.

7. **Calibration by example, not adjective.** Three intensity levels shown on
   one identical task. The review skill shows a bad finding beside a good
   one. Examples set the register more reliably than "concise" does.

8. **Drift resistance.** "Active every response, still active if unsure",
   with an explicit off phrase. Long sessions decay back to default
   verbosity; this clause fights it.

9. **The skill practices what it preaches.** Builder ~6.6 KB, review
   ~2.4 KB. They fit in context without diluting whatever else is loaded.

The review skill adds three of its own:

10. **Single-axis scope.** Complexity only. Correctness, security, and
    performance are named and routed elsewhere. Narrow scope means no
    hedging and no diluted findings.

11. **A rigid finding grammar.** `L<line>: <tag> <what>. <replacement>.`
    The replacement is mandatory, so "have you considered" findings are
    structurally impossible.

12. **Permission to find nothing.** `Lean already. Ship.` is a legal output.
    Invented findings are the main way review prompts go wrong, and a valid
    null result is the cheapest defense. One score line, `net: -N lines
    possible.`, gives a single metric with a fixed direction.

---

## 2. The process, isolated

The themes the two skills encode, in the order they fire:

| # | Step | What it does |
|---|------|--------------|
| 1 | Comprehension gate | Read the task and every file it touches, trace the flow, grep callers. Runs before any rung. |
| 2 | Existence check | Does this need to exist at all. Speculative need is skipped and named in one line. |
| 3 | Reuse hierarchy | Repo helper, then stdlib, then native platform, then already-installed dependency. New deps last. |
| 4 | Abstraction prohibitions | No one-impl interface, one-product factory, config for a constant, scaffolding for later, wrapper that only delegates. |
| 5 | Root-cause rule | Fix once where all callers route through, never at the reported symptom. |
| 6 | Compression | One line if possible. Deletion over addition, boring over clever, fewest files. |
| 7 | Safety floor | The never-simplify list. Correctness on edge cases wins a tie between equal-size options. |
| 8 | Minimum verification | One runnable assert-based check for any branch, loop, parser, money or security path. No frameworks. |
| 9 | Explicit deferral | Skipped X, add when Y. A `ponytail:` marker for any corner cut with a known ceiling. |
| 10 | Output contract | Code first, at most three lines after. Requested explanation is exempt. |
| 11 | Mode control | Three levels, persistent until changed, explicit off switch. Governs what you build, not how you talk. |

The review skill's five tags map onto steps 2 through 6:

| Tag | Step | Replacement type |
|-----|------|------------------|
| `delete:` | Existence check | nothing |
| `stdlib:` | Reuse hierarchy | a named function |
| `native:` | Reuse hierarchy | a named platform feature |
| `yagni:` | Abstraction prohibitions | inline it |
| `shrink:` | Compression | the shorter form, shown |

The sibling skills reuse this exactly. `ponytail-audit` is the review grammar
applied repo-wide and ranked by cut size. `ponytail-debt` greps the markers
the builder plants. The two core skills are the whole system; the others are
different scopes over the same taxonomy.

---

## 3. The surfaces it inspects

Each rung is a lookup against one specific surface, which is why the ladder
is cheap to run.

| Surface | What is checked | Rule |
|---------|-----------------|------|
| The requirement itself | Real need or speculative | step 2, `delete` |
| The local codebase | Existing helper, util, type, pattern | step 3 |
| Callers of the touched function | Whether the fix belongs in the shared path | step 5 |
| Language standard library | A shipped function for the hand-rolled thing | step 3, `stdlib` |
| Platform natives | HTML inputs, CSS, DB constraints, Intl | step 3, `native` |
| Installed dependency manifest | An already-present package that covers it | step 3 |
| Structural shape of the code | Single-impl interfaces, one-caller layers, unset config | `yagni` |
| Diff line numbers | Anchors every review finding | review format |
| Comment markers | `ponytail:` ceiling and upgrade path | debt convention |
| Trust boundaries and hardware | The exempt set that must not be cut | safety floor |

---

## 4. The portable pattern

Strip the persona and the subject matter and this is what remains. Use it as
the authoring checklist for any new skill.

1. **Persona with a stake.** Name a person whose own incentives point at the
   goal. Say what they have suffered. Do not describe their virtues.
2. **Ordered procedure with a stop rule.** A ladder or a fixed step list.
   Each step is a yes/no check against one named surface. Order is priority.
3. **Guard against the skill's own failure mode.** Write down how a model
   would misapply this instruction, then forbid that explicitly. This
   section is usually the most valuable one and is usually written last,
   after the skill has misfired once.
4. **Mandatory trigger on every deferral.** Nothing is skipped silently.
   Every "not now" carries a "when".
5. **Greppable markers for judgment calls.** A fixed comment or note prefix
   with a fixed shape, so a later tool can harvest them.
6. **Safety floor.** The short list of things the skill may never trade
   away, stated as nouns, not principles.
7. **Output contract with a template and examples.** A literal pattern for
   the output. One bad example beside one good one. If the skill has
   intensity levels, show all levels on the same input.
8. **A legal null result.** Give the exact phrase to emit when there is
   nothing to report. It is the cheapest defense against invented findings.
9. **Single-axis scope with explicit routing.** Name what is out of scope
   and where it goes instead.
10. **Drift clause and off switch.** "Active every response, still active if
    unsure", plus the literal phrase that turns it off.
11. **Composition boundary.** Say what the skill governs and what it does
    not, so it stacks with other skills without fighting them.
12. **Size budget.** Under ~7 KB for a mode skill, under ~3 KB for a
    one-shot skill. If the guidance is longer than that, it is two skills.

---

## 5. Target: a servitor review skill

### What exists today

The `servitor` skill already tells agents to run `ponytail-review` on the
working diff before the `presented` gate. That covers complexity. It does
not cover the things a servitor presentation actually gets bounced for.

### What it should cover that ponytail-review does not

Ponytail-review reviews the diff against the language. A servitor review
reviews the diff against the **ticket**. Candidate surfaces:

| Surface | Question | Candidate tag |
|---------|----------|---------------|
| Contract criteria subitems | Does the diff satisfy each `criterion`, with the verification it names | `criterion:` pass / fail / unverified |
| The Out list | Does the diff touch anything the contract said it would not | `scope:` |
| Seam criteria | Is the out-of-scope thing still unreachable from new code | `seam:` |
| Ticket slug vs branch | Is this the ticket's work, or the task du jour | `branch:` |
| Minimum check | Is the one runnable check present for non-trivial logic | `check:` |
| `ponytail:` markers | Does every corner cut have a ceiling and a trigger | `marker:` |
| Migrations, per `servitor-dev` | Is the schema order respected, is the demo target right | `migration:` |
| Staged files | Anything riding along that is not the ticket's | `stray:` |

### Strawman shape

- Persona: the human who has to merge this at the end of the day and has
  been burned by a PR that passed review and failed the ticket.
- Procedure: walk the contract's criteria first, then the Out list, then the
  ticket-flow mechanics. Contract conformance outranks code quality.
- Format: `<criterion ulid-prefix>: pass|fail|unverified. <evidence>.` for
  criteria, then `<file>:L<line>: <tag> <what>. <fix>.` for the rest.
- Null result: `Contract met. Present.`
- Score line: `<N>/<M> criteria pass, <K> unverified.`
- Boundaries: complexity goes to ponytail-review, correctness to a normal
  review pass. Lists, does not apply. Does not gate; the human gates.

### Decisions (Preston, 2026-09-22)

- **It replaces the ponytail-review call** in the servitor workflow. One
  pass. The complexity tags (`delete`, `stdlib`, `native`, `yagni`,
  `shrink`) fold in as a section of the servitor review rather than a
  separate skill invocation.
- **Naming.** The skills are part of servitor, not a ponytail fork. Leaning:
  `servitor.plan` and `servitor.review`. Exact packaging (sibling skill
  directories vs `references/` under the servitor skill) is open.

- **It writes back.** Every finding is a servitor event: `criterion`
  subitems get `--state pass|fail`, the rest land as structured events the
  GUI can card. The review is the scorecard, not a transcript of one.
- **Tier 0** reviews against the intake note's done-when line.
- **The GUI reserves space for it.** Plan and review are first-class cards
  on the ticket page and the board view, not conventions parsed out of
  notes. The skill's output shape and the GUI's card shape are the same
  contract; change one, change both.

---

## 6. Target: a planning skill

### The persona problem

This is the open question the user raised. The lazy dev is the right
persona for cutting, and the wrong one for originating. A tired dev at 3am
does not get up to design a feature. So the planning skill cannot inherit
the ponytail persona directly. Three ways through:

1. **Keep the incentive, change the pain.** The ponytail persona hurts from
   complexity. A planning persona should hurt from a specific planning
   failure. Candidates: the engineer who has been handed a plan that was
   never read, the one who has shipped to a criterion nobody could verify,
   the one who found out at demo time that the "obvious" adjacency was in
   scope after all. Each of those people plans lean but plans.
2. **Split the stance by phase.** Lazy about scope, never lazy about
   comprehension. Ponytail already draws this line ("the ladder shortens the
   solution, never the reading"). Planning is the reading. The laziness only
   applies to what ends up in the In list.
3. **Make the ladder about the ticket, not the code.** The existence check
   still fires first, but the question is "does this ticket need to exist"
   and the surfaces are servitor's, not the language's.

Recommendation: option 1 with option 2 as the guard section. The persona is
someone who has been burned by bad contracts, and the "when not to be lazy"
section says comprehension is never cut.

### Decisions (Preston, 2026-09-22)

**The persona's scars.** The planning failures the persona has suffered,
which become both the persona's motivation and the skill's safety floor:

| Failure | Ponytail analogue | What the skill must never cut |
|---------|-------------------|-------------------------------|
| Missing scope | the Out list | Every plausible adjacency named In or Out. Seam criteria for shared paths. |
| Assumptions vs actual code | comprehension gate | Read the repo before drafting. An assumption not checked against code is logged as an open question, not resolved silently. |
| Building something we can't expand later | `ponytail:` ceiling and upgrade path | Each deliberate simplification names its ceiling and the upgrade path. Lean now, extendable later. |
| Bad or no architecture decisions | `DECISION:` events | A real tradeoff is a `servitor decide` with a why, not an unstated choice. |
| Bad or no docs | none, ponytail is anti-prose | Docs are a criterion when the change adds a surface someone else has to use. |

**Where laziness applies.** Everywhere: the lazy dev finds the shortest path
to the goal, and that includes the In list and the criteria count. It is
balanced by the five scars above. Shortest path that still clears the floor.

**Originate or shape.** Shape only. The skill plans open tickets; it does not
invent them. The one exception is an arc: planning an arc ticket produces
the proposed child tickets (title, one-line done-when, rank) so the arc's
shape is visible before any child starts. An arc is a plain ticket that
children point at via `parent=<ulid>`, so the output is a list of `servitor
new` calls the human approves, not tickets the skill creates itself.

**Markers.** No comment-prefix convention. Assumptions, decisions and open
questions are already servitor events; the ledger is the harvest.
`servitor.plan` writes structure (`criterion`, `plan`, `decide`, open
questions) and lets the GUI build the cards.

**Naming.** Part of servitor. Leaning `servitor.plan` and `servitor.review`.

### A planning ladder, strawman

Stop at the first rung that holds:

1. **Already done or already ticketed?** Check `servitor board` and history.
   Duplicate or superseded → say so, propose drop.
2. **Already covered by existing code?** Read the repo. Half the scope
   questions are answerable there (this is contract.md's own rule).
3. **Can it be a tier 0?** One sentence done-when. If yes, that is the plan.
4. **Tier 1 mini-contract?** Three to six done-when lines plus Not doing.
5. **Only then:** full contract. Intent, In/Out, criteria with verification,
   definition of done, risks and open questions.

### Surfaces

| Surface | What is checked |
|---------|-----------------|
| `servitor board` and `history` | Duplicate, superseded, or already active elsewhere |
| The repo | What already exists, what the change actually touches |
| The ticket title and intake note | Tier, complexity, intent |
| Adjacent code paths | Everything plausible-but-not-requested → the Out list |
| Shared data, namespaces, code paths | Each Out item that shares one → a seam criterion |
| The hardest behavior to test | The concrete test strategy, named |
| Unresolved choices | Open questions, surfaced not silently resolved |

### Output contract

Contract.md already defines the form by tier. The skill's job is to enforce
it with the ponytail mechanisms: a literal template per tier, one bad
criterion beside one good one ("handles errors gracefully" vs "when the API
returns 500, the CLI prints a retry hint and exits 1"), and a null result
(`Already covered by <ref>. Drop.` or `Tier 0: done when X, verified by Y.`).

Then log the result as structure, not prose: `servitor add <ref> criterion`
and `servitor add <ref> plan` per line, one intake note for tier and
complexity.

**Single knob.** The tier sets how much gets written down. The lazy stance
is always on at full strength, balanced by the floor. No lite/full/ultra. A
session that wants harder cutting on one ticket says so in words.

**Floor, complete.** The five scars above plus, from contract.md, sad-path
criteria and a named test strategy for the hardest-to-test behavior. Seven
things the plan may never trade away for brevity.

**It writes back, all of it.** Criteria, plan steps, decisions, open
questions and the proposed child tickets of an arc are servitor events, not
prose in the reply. The GUI reserves space for the plan the same way it
does for the review. Same rule: output shape and card shape are one
contract.

---

## 7. Next steps

1. Define the event shapes the GUI will card for plan and review (which
   kinds, which payload fields), since both skills write to them. This is
   the seam between the skills and the board view and should be one ticket.
2. Write `servitor.review` against the checklist in section 4. Under 3 KB.
   It absorbs the ponytail-review tags and replaces that call in the
   servitor workflow.
3. Write `servitor.plan` against the same checklist. Under 7 KB.
4. Decide packaging (sibling skill dirs vs `references/` under the servitor
   skill) and wire `install.sh` accordingly.
5. Run each on a real ticket once before adjusting; write the "when not to"
   section from what misfires, not from what might.
