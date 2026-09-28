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

// ---- the ledger fold -------------------------------------------------------
import { foldLedger, hueBy, filterDots, topTickets, columnSummary, columnAt, dayTotals, kindGroup, LEDGER_ORDER } from './timeview.js';

const EV = [
  { id: 1, ts: '2026-09-28T10:00:00Z', ticket_ulid: 'T1', actor: 'human:preston', actor_type: 'human', kind: 'gate', class: 'signal', payload: { gate: 'contract_approved' } },
  { id: 2, ts: '2026-09-28T10:10:00Z', ticket_ulid: 'T1', actor: 'agent:claude', actor_type: 'agent', kind: 'field.set', class: 'transition', payload: {} },
  { id: 3, ts: '2026-09-28T10:20:00Z', ticket_ulid: 'T2', actor: 'agent:claude', actor_type: 'agent', kind: 'note', class: 'signal', payload: {} },
  { id: 4, ts: '2026-09-28T11:00:00Z', ticket_ulid: 'T2', actor: 'system', actor_type: 'system', kind: 'review', class: 'signal', payload: {} },
  { id: 5, ts: '2026-09-28T11:30:00Z', ticket_ulid: 'T3', actor: 'agent:cli', actor_type: 'agent', kind: 'somecustomkind', class: 'transition', payload: {} },
];
const slugOf = (u) => ({ T1: 'alpha', T2: 'beta' }[u] || null);

test('foldLedger: one dot per event, kind grouped, system on the agent side, slug falls back to the ulid', () => {
  const d = foldLedger(EV, slugOf);
  assert.equal(d.length, 5);
  assert.deepEqual(d.map((x) => x.group), ['gate', 'transition', 'note', 'contract', 'transition']);
  assert.deepEqual(d.map((x) => x.side), ['human', 'agent', 'agent', 'agent', 'agent']);
  assert.equal(d[4].slug, 'T3');
  assert.deepEqual(d.map((x) => x.signal), [true, false, true, true, false]);
  assert.equal(kindGroup('review'), 'contract');
  assert.equal(kindGroup('anything-else'), 'transition');
});

test('hueBy: kind is the ledger order, actor is the side, ticket hues the busiest and dims the rest', () => {
  const d = foldLedger(EV, slugOf);
  assert.deepEqual(hueBy(d, 'kind').groups.map((g) => g.name), LEDGER_ORDER);
  assert.deepEqual(hueBy(d, 'actor').dots.map((x) => x.group), ['human', 'agent', 'agent', 'agent', 'agent']);
  const top = topTickets(d, 1);
  assert.deepEqual(top.map((x) => x.slug), ['alpha']);          // T1 and T2 tie on 2; alpha sorts first
  const tk = hueBy(d, 'ticket', top);
  assert.deepEqual(tk.groups.map((g) => g.name), ['alpha', 'other']);
  assert.deepEqual(tk.dots.map((x) => x.group), ['alpha', 'alpha', 'other', 'other', 'other']);
});

test('filterDots: signals only drops transitions; only isolates one group; both compose', () => {
  const d = hueBy(foldLedger(EV, slugOf), 'kind').dots;
  assert.equal(filterDots(d, { signalsOnly: true }).length, 3);
  assert.deepEqual(filterDots(d, { only: 'transition' }).map((x) => x.id), [2, 5]);
  assert.deepEqual(filterDots(d, { signalsOnly: true, only: 'transition' }), []);
  assert.equal(filterDots(d).length, 5);
});

test('columnAt and columnSummary agree with the lattice bucketing and name the newest id', () => {
  const span = { min: Date.parse('2026-09-28T10:00:00Z'), max: Date.parse('2026-09-28T12:00:00Z') };
  const col = columnAt(15, 80, span, 8);                    // 10 columns of 12 minutes; x=15 is column 1
  assert.equal(col.i, 1);
  assert.equal(col.t0, span.min + 12 * 60000);
  const d = hueBy(foldLedger(EV, slugOf), 'kind').dots;
  const s = columnSummary(d, span.min, span.min + 30 * 60000); // 10:00..10:30 holds ids 1,2,3
  assert.equal(s.total, 3);
  assert.deepEqual(s.byGroup, { gate: 1, transition: 1, note: 1 });
  assert.equal(s.newestId, 3);
  assert.deepEqual(s.tickets.map((x) => x.slug), ['alpha', 'beta']);
  assert.equal(columnSummary(d, span.max, span.max + 1).newestId, null);
  assert.equal(columnAt(-5, 80, span, 8).i, 0);
  assert.equal(columnAt(500, 80, span, 8).i, 9);
});

test('dayTotals folds dots to local days with the human count kept', () => {
  const rows = dayTotals(foldLedger(EV, slugOf));
  assert.equal(rows.length, 1);
  assert.equal(rows[0].total, 5);
  assert.equal(rows[0].human, 1);
});

// ---- the date range fields ---------------------------------------------------
import { rangeFromLocal, minuteKey, stepWindow } from './timeview.js';

test('rangeFromLocal: to the minute, local time; a bare date is midnight to end of day; bad pairs are errors', () => {
  const r = rangeFromLocal('2026-09-21T09:30', '2026-09-21T17:45');
  assert.equal(r.from, new Date(2026, 8, 21, 9, 30).getTime());
  assert.equal(r.to, new Date(2026, 8, 21, 17, 45).getTime());
  const d = rangeFromLocal('2026-09-21', '2026-09-28');
  assert.equal(d.from, new Date(2026, 8, 21).getTime());
  assert.equal(d.to, new Date(2026, 8, 29).getTime() - 1);
  assert.equal(rangeFromLocal('2026-09-21T10:00', '2026-09-21T10:00').error, 'the range runs backwards');
  assert.equal(rangeFromLocal('', '2026-09-21T10:00').error, 'both ends are needed');
  assert.equal(rangeFromLocal('soon', '2026-09-21T10:00').error, 'that is not a date');
  assert.equal(rangeFromLocal('2026-09-21T10:00:30', '2026-09-21T11:00').error, 'that is not a date');   // seconds are not a control
});

test('minuteKey round-trips through rangeFromLocal', () => {
  const t = new Date(2026, 8, 21, 9, 30).getTime();
  assert.equal(minuteKey(t), '2026-09-21T09:30');
  assert.equal(rangeFromLocal(minuteKey(t), minuteKey(t + 60000)).from, t);
});

test('stepWindow moves the window by its own length either way', () => {
  assert.deepEqual(stepWindow(100, 200, 1), { from: 200, to: 300 });
  assert.deepEqual(stepWindow(100, 200, -1), { from: 0, to: 100 });
});
