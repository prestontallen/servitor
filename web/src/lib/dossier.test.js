import test from 'node:test';
import assert from 'node:assert/strict';
import { buildDossier, splitContract, decisions, notes } from './dossier.js';

let id = 0;
const ts = (h) => `2026-09-18T${String(h).padStart(2, '0')}:00:00Z`;
const ev = (kind, payload, extra = {}) => ({
  id: ++id, ts: ts(id % 24), actor: 'agent:cli', actor_type: 'agent', kind, payload, ...extra
});
const note = (v, extra) => ev('note', { v }, extra);
const bare = { criteria: [], plan: [], decisions: [], questions: [], links: [], feedback: [], gates: [] };

test('intake note becomes the contract, with tier and complexity pulled out', () => {
  const d = buildDossier(bare, [note('Intake: tier 2, complexity medium. Build the thing.'), note('progress chatter')]);
  assert.equal(d.contract.source, 'intake note');
  assert.equal(d.contract.tier, '2');
  assert.equal(d.contract.complexity, 'medium');
  assert.equal(d.contract.intent, 'tier 2, complexity medium. Build the thing.');
  assert.equal(d.contract.approved, null);
});

test('a contract note splits into intent, in and out; the newest one is the document', () => {
  const s = splitContract('Intent: cards not lists. In: contract form; plan track; ledger folded. Out: writes from the GUI; new endpoints. Verification: staging eyeball.');
  assert.equal(s.intent, 'cards not lists');
  assert.deepEqual(s.in, ['contract form', 'plan track', 'ledger folded']);
  assert.deepEqual(s.out, ['writes from the GUI', 'new endpoints']);
  assert.equal(s.verification, 'staging eyeball');
  const d = buildDossier(bare, [
    note('INTAKE (tier 2, medium complexity): rework the ledger page first'),
    note('CONTRACT (draft): Intent: v1. In: a. Out: b.'),
    note('Contract v2 approved by Preston: Intent: v2. In: a; c. Out: b.')
  ]);
  assert.equal(d.contract.source, 'contract note');
  assert.equal(d.contract.intent, 'v2');
  assert.deepEqual(d.contract.in, ['a', 'c']);
  assert.equal(d.contract.versions, 2);
  assert.equal(d.contract.tier, '2');
});

test('criteria carry state, evidence from the subitem.set that proved them, and a tally', () => {
  const doc = { ...bare, criteria: [{ ulid: '01AAAAAAAAAAAAAAAAAAAAAAAA', body: 'A', state: 'pass' }, { ulid: '01BBBB', body: 'B', state: null }],
    gates: [{ gate: 'contract_approved', actor: 'human:preston', ts: ts(1) }] };
  const h = [ev('subitem.set', { ulid: '01AAAAAAAA', state: 'pass' }, { actor: 'agent:claude', ts: ts(5) }), note('Amended after approval: scope grew, preston agreed', { ts: ts(6) })];
  const d = buildDossier(doc, h);
  assert.equal(d.contract.source, 'criteria subitems');
  assert.equal(d.contract.criteria[0].evidence.actor, 'agent:claude');
  assert.equal(d.contract.criteria[1].evidence, null);
  assert.deepEqual(d.contract.tally, { pass: 1, fail: 0, open: 1 });
  assert.equal(d.contract.approved.actor, 'human:preston');
  assert.equal(d.contract.amendments.length, 1);
});

test('plan steps get add/done times, durations and the current index', () => {
  const doc = { ...bare, plan: [{ ulid: '01P1XXXX', body: 'one', state: 'done' }, { ulid: '01P2XXXX', body: 'two', state: null }] };
  const h = [
    ev('subitem.add', { kind: 'plan', body: 'one' }, { ts: ts(1) }),
    ev('subitem.add', { kind: 'plan', body: 'two' }, { ts: ts(1) }),
    ev('subitem.set', { ulid: '01P1', state: 'done' }, { ts: ts(3), actor: 'agent:hermes' })
  ];
  const p = buildDossier(doc, h).plan;
  assert.equal(p.current, 1);
  assert.equal(p.done, 1);
  assert.equal(p.steps[0].took, 2 * 3600 * 1000);
  assert.equal(p.steps[0].done.actor, 'agent:hermes');
  assert.equal(p.steps[1].took, null);
});

test('decisions dedupe across event and subitem, split "X over Y because Z", and read DECISION: notes', () => {
  const doc = { ...bare, decisions: [{ what: 'SVG over echarts because it must measure labels', why: null }] };
  const h = [ev('decision', { what: 'SVG over echarts because it must measure labels' }, { actor: 'human:preston', actor_type: 'human' }),
    note('DECISION (chat, Preston): rebuild on a fresh branch')];
  const dd = decisions(doc, h, notes(h));
  assert.equal(dd.items.length, 2);
  assert.equal(dd.items[0].chosen, 'SVG');
  assert.equal(dd.items[0].rejected, 'echarts');
  assert.equal(dd.items[0].why, 'it must measure labels');
  assert.equal(dd.items[0].actor_type, 'human');
  assert.equal(dd.items[1].what, 'rebuild on a fresh branch');
  assert.equal(dd.source, 'decision events + DECISION: notes');
  const prose = decisions(bare, [], notes([note('DECISION: merged the plan combining both branches without committing, not over the wire but locally, rather than waiting for review because the review never came and the branch aged for a day while nobody looked at it at all')]));
  assert.equal(prose.items[0].rejected, null, 'long prose never splits into a fork');
});

test('questions are intervals: open until an ANSWER: note, with open-for time', () => {
  const h = [note('Built the widget. Open question from Preston: echarts instead of SVG? Recommendation: SVG.', { ts: ts(1) }), note('Question: does the board stay read-only?', { ts: ts(2) }), note('ANSWER: yes, by the current rule', { ts: ts(3), actor: 'human:preston' }),
    note('CORRECTION: the open question about the worktree is closed', { ts: ts(4) })];
  const q = buildDossier(bare, h, new Date(ts(5)).getTime()).questions;
  assert.equal(q.items.length, 2, 'the correction is not a question');
  assert.equal(q.items[0].body, 'Open question from Preston: echarts instead of SVG?');
  assert.equal(q.open, 1);
  assert.equal(q.items[0].answer, null);
  assert.equal(q.items[0].openFor, 4 * 3600 * 1000);
  assert.equal(q.items[1].answer.body, 'yes, by the current rule');
  assert.equal(q.items[1].openFor, 1 * 3600 * 1000);
});

test('feedback tallies by tag and source; corrections find the note they amend; fed ids are marked', () => {
  const h = [
    note('Checked from Iron Deck: repo clean, worktrees empty, question about uncommitted worktree stays open'),
    note('CORRECTION: Iron Deck IS adirondack, the machine the check ran from; the uncommitted worktree question is closed'),
    ev('feedback', { finding: 'look before proposing', source: 'human', tag: 'correction' }),
    ev('feedback', { finding: 'claimed verification early', source: 'self', tag: 'correction' }),
    ev('feedback', { finding: 'wall of text', source: 'human', tag: 'rework' })
  ];
  const d = buildDossier(bare, h);
  assert.deepEqual(d.feedback.byTag, { correction: 2, rework: 1 });
  assert.deepEqual(d.feedback.bySource, { human: 2, self: 1 });
  assert.equal(d.corrections.items[0].amends.id, h[0].id);
  assert.equal(d.fed.get(h[1].id), 'corrections');
  assert.equal(d.fed.has(h[0].id), false);
});

test('no structure and no conventions means an empty dossier with counts', () => {
  const d = buildDossier(bare, [ev('ticket.create', { slug: 'x' }), ev('status.set', { status: 'active' }), note('just working')]);
  assert.equal(d.empty, true);
  assert.deepEqual(d.counts, [['ticket.create', 1], ['status.set', 1], ['note', 1]]);
});

// ---- plan/review events (skills/servitor/references/events.md) ----------------

test('a contract event is the document ahead of contract notes, and counts its versions', () => {
  const h = [
    note('Intake: tier 2, complexity medium. Old way.'),
    note('Contract: Intent: from a note. In: a. Out: b.'),
    ev('contract', { intent: 'v1', in: ['a'], out: ['b'], verification: 'staging', risks: 'none' }),
    ev('contract', { intent: 'v2', in: ['a', 'c'], out: ['b'] }, { actor: 'agent:claude' })
  ];
  const c = buildDossier(bare, h).contract;
  assert.equal(c.source, 'contract event');
  assert.equal(c.intent, 'v2');
  assert.deepEqual(c.in, ['a', 'c']);
  assert.deepEqual(c.out, ['b']);
  assert.equal(c.verification, null);
  assert.equal(c.versions, 2);
  assert.equal(c.tier, '2');
  // the contract note is not consumed when the event is the document
  assert.equal(c.fed.length, 1);
});

test('criterion evidence text comes from ctx or from the subitem.set that proved it', () => {
  const doc = { ...bare, criteria: [
    { ulid: '01AAAAAAAAAAAAAAAAAAAAAAAA', body: 'A', state: 'pass', evidence: 'go test -run A' },
    { ulid: '01BBBBBBBBBBBBBBBBBBBBBBBB', body: 'B', state: 'fail', evidence: null },
    { ulid: '01CCCCCCCCCCCCCCCCCCCCCCCC', body: 'C', state: null, evidence: null }
  ] };
  const h = [
    ev('subitem.set', { ulid: '01AAAA', state: 'pass', evidence: 'go test -run A' }, { actor: 'agent:claude' }),
    ev('subitem.set', { ulid: '01BBBB', state: 'fail', evidence: 'board shows no mark' }, { actor: 'agent:hermes' })
  ];
  const c = buildDossier(doc, h).contract;
  assert.equal(c.criteria[0].evidence.text, 'go test -run A');
  assert.equal(c.criteria[0].evidence.actor, 'agent:claude');
  assert.equal(c.criteria[1].evidence.text, 'board shows no mark');
  assert.equal(c.criteria[2].evidence, null);
  assert.deepEqual(c.tally, { pass: 1, fail: 1, open: 1 });
});
