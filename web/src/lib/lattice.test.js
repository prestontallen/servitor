import test from 'node:test';
import assert from 'node:assert/strict';
import { bucketize, quantumFor, stack, rowsFor, KIND_ORDER } from './lattice.js';

const T0 = Date.parse('2026-09-18T00:00:00Z');
const H = 3600000;
const ev = (h, kind, actor_type = 'agent') => ({ ts: new Date(T0 + h * H).toISOString(), kind, actor_type });

test('bucketize splits the span evenly and keeps the last edge', () => {
  const b = bucketize([ev(0, 'note'), ev(5, 'note'), ev(10, 'decision', 'human')], T0, T0 + 10 * H, 5);
  assert.equal(b.length, 5);
  assert.equal(b[0].agent, 1);
  assert.equal(b[2].agent, 1);
  assert.equal(b[4].human, 1); // t == max lands in the last column, not off the end
  assert.deepEqual(b[4].counts.decision, { human: 1, agent: 0 });
});

test('bucketize ignores events outside the span', () => {
  const b = bucketize([ev(-1, 'note'), ev(11, 'note')], T0, T0 + 10 * H, 5);
  assert.equal(b.reduce((n, x) => n + x.agent + x.human, 0), 0);
});

test('quantum grows so the tallest side fits maxRows', () => {
  const many = Array.from({ length: 25 }, () => ev(1, 'note'));
  const b = bucketize(many, T0, T0 + 10 * H, 5);
  assert.equal(quantumFor(b, 10), 3);
  assert.equal(quantumFor(bucketize([ev(1, 'note')], T0, T0 + 10 * H, 5), 10), 1);
});

test('stack orders kinds baseline outward and rounds partial quanta up', () => {
  const b = bucketize([ev(1, 'note'), ev(1, 'note'), ev(1, 'note'), ev(1, 'decision'), ev(1, 'feedback', 'human')], T0, T0 + 10 * H, 5);
  const agent = stack(b[0], 'agent', 2);
  assert.deepEqual(agent.map((d) => d.kind), ['decision', 'note', 'note']);
  assert.ok(agent.every((d) => !d.human));
  const human = stack(b[0], 'human', 2);
  assert.deepEqual(human, [{ kind: 'feedback', human: true }]);
  assert.deepEqual(KIND_ORDER, ['decision', 'feedback', 'note']);
});

test('rowsFor reports the tallest column per side', () => {
  const b = bucketize([ev(1, 'note'), ev(1, 'note'), ev(3, 'decision', 'human')], T0, T0 + 10 * H, 5);
  assert.deepEqual(rowsFor(b, 1), { human: 1, agent: 2 });
});
