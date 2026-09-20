import test from 'node:test';
import assert from 'node:assert/strict';
import { parseTimeHash, timeHash, flowLanes, cadenceDays, dayKey, dayColumns, daysIn, fmtDur } from './timeview.js';

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
    { ticket: 'a', phase: 'building', from: iso(from - 86400000), to: iso(from + 3600000) },
    { ticket: 'a', phase: 'checking', from: iso(from + 3600000), to: iso(to + 86400000) },
    // b: entirely before the window — dropped, lane gone
    { ticket: 'b', phase: 'queued', from: iso(from - 86400000 * 3), to: iso(from - 86400000 * 2) },
  ];
  const lanes = flowLanes(segments, cards, from, to);
  assert.deepEqual(lanes.map((l) => l.ulid), ['a']);
  assert.deepEqual(lanes[0].bars.map((b) => b.phase), ['building', 'checking']);
  const [bar1, bar2] = lanes[0].bars;
  assert.equal(bar1.x0, 0); // clipped at the window edge
  assert.ok(Math.abs(bar1.x1 - 3600000 / (to - from)) < 1e-9);
  assert.equal(bar2.x1, 1); // clipped at the window end
});

test('cadenceDays folds hourly buckets to local days in order', () => {
  // two hours on one day, one on the next
  const buckets = [
    { hour: '2026-09-19T10:00:00', byActor: { agent: 3, human: 1 } },
    { hour: '2026-09-19T11:00:00', byActor: { agent: 2 } },
    { hour: '2026-09-20T01:00:00', byActor: { human: 4 } },
  ];
  const days = cadenceDays(buckets);
  assert.deepEqual(days.map((d) => d.day), ['2026-09-19', '2026-09-20']);
  assert.equal(days[0].total, 6);
  assert.equal(days[1].total, 4);
  assert.deepEqual(days[0].byActor, { agent: 5, human: 1 });
});

test('dayColumns: 24 slots, empty hours zeroed, foreign days ignored', () => {
  const day = '2026-09-19';
  const buckets = [
    { hour: '2026-09-19T10:00:00', byActor: { agent: 3 } },
    { hour: '2026-09-20T10:00:00', byActor: { agent: 9 } },
  ];
  const cols = dayColumns(buckets, day);
  assert.equal(cols.length, 24);
  assert.equal(cols[10].events, 3);
  assert.equal(cols[9].events, 0);
  // 10:00 local means hour index 10 only if the bucket is local-naive;
  // the fixture is parsed as local time, so index follows getHours()
  assert.equal(cols[new Date('2026-09-19T10:00:00').getHours()].events, 3);
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
