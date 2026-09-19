import test from 'node:test';
import assert from 'node:assert/strict';
import { attention, sparkline, laneOf } from './inbox.js';

const NOW = new Date('2026-09-19T12:00:00Z').getTime();
const ago = (h) => new Date(NOW - h * 3600000).toISOString();
let id = 0;
const ev = (ticket, kind, payload, extra = {}) => ({ id: ++id, ticket, kind, payload, ts: ago(1), actor: 'agent:cli', actor_type: 'agent', ...extra });
const card = (o) => ({ ulid: o.slug + '-ulid', title: '', blocked_on: null, blocked_since: null, card_word: null, active_by: null, updated_at: ago(1), ...o });

test('each rule fires on the right card and carries the CLI action', () => {
  const cards = [
    card({ slug: 'b', status: 'blocked', blocked_on: 'human', blocked_since: ago(120) }),
    card({ slug: 'b2', status: 'blocked', blocked_on: 'adirondack', blocked_since: ago(200) }),
    card({ slug: 'p', status: 'active', card_word: 'checking', updated_at: ago(2) }),
    card({ slug: 's', status: 'active', card_word: 'shipping', updated_at: ago(3) }),
    card({ slug: 'c', status: 'active', card_word: null, updated_at: ago(4) }),
    card({ slug: 'q', status: 'queued', updated_at: ago(500) }),
    card({ slug: 'w', status: 'active', card_word: 'building', updated_at: ago(5) })
  ];
  const events = [
    ev('p-ulid', 'field.set', { field: 'pr', v: 'https://github.com/x/y/pull/7' }),
    ev('p-ulid', 'gate', { gate: 'presented' }, { ts: ago(10) }),
    ev('c-ulid', 'note', { v: 'CONTRACT (draft): Intent: x. In: y.' }, { ts: ago(6) })
  ];
  const rows = attention(cards, events, NOW);
  assert.deepEqual(rows.map((r) => r.rule), ['blocked', 'presented', 'contract', 'shipped'], 'oldest wait first; other-party block, queued and building excluded');
  assert.equal(rows[0].action, 'servitor set b --status queued');
  assert.equal(rows[1].why, 'presented · x/y/pull/7 open');
  assert.equal(rows[1].since, ago(10), 'presented wait starts at the gate, not the last event');
  assert.equal(rows[2].action, 'SERVITOR_ACTOR=human:preston servitor gate c contract_approved');
  assert.equal(rows[3].action, 'servitor set s --status done');
});

test('a silent active ticket is stale after three days; the rules degrade to the board alone', () => {
  const rows = attention([card({ slug: 'z', status: 'active', card_word: 'building', updated_at: ago(80) })], [], NOW);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].rule, 'stale');
  assert.equal(rows[0].why, 'active but silent 3d');
  assert.deepEqual(attention([card({ slug: 'z', status: 'active', card_word: 'building', updated_at: ago(70) })], [], NOW), []);
});

test('sparkline buckets seven days oldest first and marks human days; no events is flat', () => {
  const events = [
    ev('t', 'note', {}, { ts: ago(2) }),
    ev('t', 'note', {}, { ts: ago(3), actor_type: 'human' }),
    ev('t', 'note', {}, { ts: ago(30) }),
    ev('t', 'note', {}, { ts: ago(24 * 8) }),
    ev('u', 'note', {}, { ts: ago(1) })
  ];
  const s = sparkline(events, 't', NOW);
  assert.deepEqual(s.counts, [0, 0, 0, 0, 0, 1, 2]);
  assert.deepEqual(s.human, [false, false, false, false, false, false, true]);
  assert.equal(s.total, 3);
  assert.equal(sparkline(events, 'nobody', NOW).total, 0);
});

test('lane placement: blocked to the rail, queued left, active by card word, no word means shaping', () => {
  assert.equal(laneOf({ status: 'blocked', card_word: 'building' }), 'blocked');
  assert.equal(laneOf({ status: 'queued', card_word: 'checking' }), 'queued');
  assert.equal(laneOf({ status: 'active', card_word: 'checking' }), 'checking');
  assert.equal(laneOf({ status: 'active', card_word: null }), 'shaping');
});
