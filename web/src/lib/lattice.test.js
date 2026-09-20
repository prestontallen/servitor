import test from 'node:test';
import assert from 'node:assert/strict';
import { bucketize, quantumFor, stack, rowsFor, placeLabels, KIND_ORDER } from './lattice.js';

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

test('stack orders kinds baseline outward and rounds over the side, not per kind', () => {
  const b = bucketize([ev(1, 'note'), ev(1, 'note'), ev(1, 'note'), ev(1, 'decision'), ev(1, 'feedback', 'human')], T0, T0 + 10 * H, 5);
  // 4 agent events at quantum 2 -> 2 dots, not 3: the decision fills half a dot and note completes it
  const agent = stack(b[0], 'agent', 2);
  assert.deepEqual(agent.map((d) => d.kind), ['decision', 'note']);
  assert.deepEqual(stack(b[0], 'human', 2), [{ kind: 'feedback' }]);
  assert.deepEqual(KIND_ORDER, ['decision', 'feedback', 'note']);
});

test('a side draws exactly ceil(total / quantum) dots whatever the kind mix', () => {
  const mix = [ev(1, 'decision'), ev(1, 'feedback'), ev(1, 'note'), ev(1, 'note'), ev(1, 'note'), ev(1, 'note'), ev(1, 'note')];
  const b = bucketize(mix, T0, T0 + 10 * H, 5);
  assert.equal(stack(b[0], 'agent', 3).length, Math.ceil(7 / 3));
});

test('rowsFor reports the tallest column per side', () => {
  const b = bucketize([ev(1, 'note'), ev(1, 'note'), ev(3, 'decision', 'human')], T0, T0 + 10 * H, 5);
  assert.deepEqual(rowsFor(b, 1), { human: 1, agent: 2 });
});

test('placeLabels keeps order, spreads collisions and marks displaced labels', () => {
  const w = 40;
  const out = placeLabels([{ cx: 20, w }, { cx: 200, w }, { cx: 396, w }, { cx: 398, w }, { cx: 399, w }], 400, 10);
  assert.equal(out[0].lx, 20);                       // fits where it is
  assert.equal(out[0].displaced, false);
  assert.equal(out[1].displaced, false);
  // the three at the right edge fan out leftwards from the edge, in order, gap kept
  assert.equal(out[4].left + out[4].w, 400);
  assert.equal(out[3].left + out[3].w + 10, out[4].left);
  assert.equal(out[2].left + out[2].w + 10, out[3].left);
  assert.ok(out[2].displaced && out[3].displaced);
  assert.ok(out.every((it, i) => i === 0 || it.left >= out[i - 1].left + out[i - 1].w + 10));
});

test('placeLabels clamps a lone label inside the width', () => {
  const [a] = placeLabels([{ cx: 0, w: 30 }], 200);
  assert.equal(a.left, 0);
  const [b] = placeLabels([{ cx: 200, w: 30 }], 200);
  assert.equal(b.left, 170);
});
