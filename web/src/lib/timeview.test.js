import test from 'node:test';
import assert from 'node:assert/strict';
import { parseTimeHash, timeHash, flowLanes, cadenceDays, dayKey, daysIn, fmtDur } from './timeview.js';

const NOW = new Date('2026-09-20T12:00:00').getTime();
const iso = (ms) => new Date(ms).toISOString();

test('parseTimeHash: defaults, garbage fallback, roundtrip through timeHash', () => {
  const d = parseTimeHash('#/time', NOW);
  assert.equal(d.to, NOW);
  assert.equal(d.from, NOW - 7 * 86400000);

  // garbage is a default, never a NaN on the axis
  const g = parseTimeHash('#/time?from=abc&to=0', NOW);
  assert.equal(g.from, NOW - 7 * 86400000);
  assert.equal(g.to, NOW);

  // from/to roundtrip; to before from is repaired
  const w = { from: NOW - 2 * 86400000, to: NOW };
  assert.deepEqual(parseTimeHash(timeHash(w.from, w.to), NOW), w);
  const bad = parseTimeHash(timeHash(NOW, NOW - 1000), NOW);
  assert.ok(bad.to > bad.from);
});

test('flowLanes: clips segments to the window, keeps board order, drops empty lanes', () => {
  const from = NOW - 2 * 86400000;
  const to = NOW;
  const cards = [{ ulid: 'a' }, { ulid: 'b' }];
  const segments = [
    // a: straddles the window start
    { ticket_ulid: 'a', phase: 'building', from: iso(from - 86400000), to: iso(from + 3600000) },
    { ticket_ulid: 'a', phase: 'checking', from: iso(from + 3600000), to: iso(to + 86400000) },
    // b: entirely before the window — dropped, lane gone
    { ticket_ulid: 'b', phase: 'queued', from: iso(from - 86400000 * 3), to: iso(from - 86400000 * 2) },
  ];
  const lanes = flowLanes(segments, cards, from, to);
  assert.deepEqual(lanes.map((l) => l.ulid), ['a']);
  assert.deepEqual(lanes[0].bars.map((b) => b.phase), ['building', 'checking']);
  const [bar1, bar2] = lanes[0].bars;
  assert.equal(bar1.x0, 0); // clipped at the window edge
  assert.ok(Math.abs(bar1.x1 - 3600000 / (to - from)) < 1e-9);
  assert.equal(bar2.x1, 1); // clipped at the window end
});

test('flowLanes: a ticket that finished inside the window still gets a lane, after the board cards', () => {
  const from = NOW - 2 * 86400000;
  const to = NOW;
  const cards = [{ ulid: 'a', slug: 'a-slug' }];
  const segments = [
    { ticket_ulid: 'z', slug: 'z-done', phase: 'shipping', from: iso(from + 3600000), to: iso(from + 7200000) },
    { ticket_ulid: 'a', phase: 'building', from: iso(from), to: iso(to) }
  ];
  const lanes = flowLanes(segments, cards, from, to);
  assert.deepEqual(lanes.map((l) => [l.ulid, l.slug]), [['a', 'a-slug'], ['z', 'z-done']]);
});

test('cadenceDays folds hourly buckets to local days in order', () => {
  // two hours on one day, one on the next
  const buckets = [
    { hour: '2026-09-19T10:00:00', by_actor: { agent: 3, human: 1 } },
    { hour: '2026-09-19T11:00:00', by_actor: { agent: 2 } },
    { hour: '2026-09-20T01:00:00', by_actor: { human: 4 } },
  ];
  const days = cadenceDays(buckets);
  assert.deepEqual(days.map((d) => d.day), ['2026-09-19', '2026-09-20']);
  assert.equal(days[0].total, 6);
  assert.equal(days[1].total, 4);
  assert.deepEqual(days[0].byActor, { agent: 5, human: 1 });
});

test('daysIn lists every calendar day of the selection, oldest first', () => {
  const days = daysIn(new Date('2026-09-19T15:00:00').getTime(), new Date('2026-09-21T01:00:00').getTime());
  assert.deepEqual(days, ['2026-09-19', '2026-09-20', '2026-09-21']);
});

test('dayKey and fmtDur match the house formats', () => {
  assert.equal(dayKey(new Date(2026, 8, 4, 5, 6).getTime()), '2026-09-04');
  assert.equal(fmtDur(45000000), '12h 30m');
  assert.equal(fmtDur(86400000 * 2), '2d');
  assert.equal(fmtDur(15000), '<1m'); // rounds under a minute
});
