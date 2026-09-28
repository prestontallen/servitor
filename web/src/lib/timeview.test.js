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
  // the clipped bounds in ms too, for a renderer that takes timestamps
  assert.equal(bar1.start, from);
  assert.equal(bar1.end, from + 3600000);
  assert.equal(bar2.end, to);
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

test('flowLanes: a touched set keeps only tickets the ledger wrote to inside the window', () => {
  const from = NOW - 2 * 86400000;
  const to = NOW;
  const cards = [{ ulid: 'a', slug: 'a' }, { ulid: 'b', slug: 'b' }];
  const segments = [
    // a: sat blocked across the whole window, nothing written — no lane
    { ticket_ulid: 'a', phase: 'blocked', from: iso(from - 86400000), to: iso(to + 86400000) },
    // b: same shape but touched — lane, bars clipped to the window
    { ticket_ulid: 'b', phase: 'building', from: iso(from - 86400000), to: iso(from + 3600000) },
    { ticket_ulid: 'b', phase: 'checking', from: iso(from + 3600000), to: iso(to + 86400000) },
    // z: not on the board, touched — lane after the board cards
    { ticket_ulid: 'z', slug: 'z-done', phase: 'shipping', from: iso(from + 3600000), to: iso(from + 7200000) },
  ];
  const lanes = flowLanes(segments, cards, from, to, new Set(['b', 'z']));
  assert.deepEqual(lanes.map((l) => l.ulid), ['b', 'z']);
  assert.deepEqual(lanes[0].bars.map((b) => [b.phase, b.x0, b.x1]), [['building', 0, 3600000 / (to - from)], ['checking', 3600000 / (to - from), 1]]);
  // null is no filter: the old behaviour
  assert.deepEqual(flowLanes(segments, cards, from, to, null).map((l) => l.ulid), ['a', 'b', 'z']);
  assert.deepEqual(flowLanes(segments, cards, from, to).map((l) => l.ulid), ['a', 'b', 'z']);
  // an empty touched set is a filter that keeps nothing
  assert.deepEqual(flowLanes(segments, cards, from, to, new Set()), []);
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

test('foldLedger: an event carrying ticket.slug uses it before slugOf; touchedTickets is the set of ulids', () => {
  const d = foldLedger([{ ...EV[4], ticket: { slug: 'gamma' } }, EV[0]], slugOf);
  assert.deepEqual(d.map((x) => x.slug), ['gamma', 'alpha']);
  assert.deepEqual([...touchedTickets(foldLedger(EV, slugOf))].sort(), ['T1', 'T2', 'T3']);
  assert.deepEqual([...touchedTickets([])], []);
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

// ---- the window read ----------------------------------------------------------
import { TIME_WINDOW_QUERY, MAX_PAGES, windowVars, mergePages, needsRefetch, touchedTickets } from './timeview.js';

test('windowVars: since is from, until is to + 1 ms (half-open server bound, inclusive window), before passes through', () => {
  const from = Date.parse('2026-09-21T20:17:00Z'), to = Date.parse('2026-09-28T20:17:00.000Z');
  assert.deepEqual(windowVars(from, to), { since: '2026-09-21T20:17:00.000Z', until: '2026-09-28T20:17:00.001Z', before: null });
  assert.equal(windowVars(from, to, 4711).before, 4711);
  assert.ok(TIME_WINDOW_QUERY.includes('events(since: $since, until: $until, before_id: $before, limit: 10000)'));
  assert.ok(TIME_WINDOW_QUERY.includes('timeline(since: $since, until: $until)'));
});

test('mergePages: newest first, deduped by id, segments from the first page, truncated only past MAX_PAGES with more left', () => {
  const page = (ids, next, segs = []) => ({ events: { events: ids.map((id) => ({ id })), next_before_id: next }, timeline: { segments: segs } });
  const one = mergePages([page([9, 8, 7], null, [{ phase: 'building' }])]);
  assert.deepEqual(one.events.map((e) => e.id), [9, 8, 7]);
  assert.deepEqual(one.segments, [{ phase: 'building' }]);
  assert.equal(one.truncated, false);

  // a page boundary moved under a live append: 7 arrives twice, once
  const two = mergePages([page([9, 8, 7], 7), page([7, 6, 5], null)]);
  assert.deepEqual(two.events.map((e) => e.id), [9, 8, 7, 6, 5]);
  assert.equal(two.truncated, false);

  // MAX_PAGES pages and the last one still points older: truncated
  const full = Array.from({ length: MAX_PAGES }, (_, i) => page([100 - i], 100 - i));
  assert.equal(mergePages(full).truncated, true);
  // MAX_PAGES pages but the last is the end: not truncated
  full[MAX_PAGES - 1] = page([1], null);
  assert.equal(mergePages(full).truncated, false);
  assert.deepEqual(mergePages([page([], null)]), { events: [], segments: [], truncated: false });
});

test('needsRefetch: a window that ended before the last fetch never refetches; one that reaches it does', () => {
  assert.equal(needsRefetch(1000, 2000), false);   // window closed before the fetch: append-only, nothing new
  assert.equal(needsRefetch(3000, 2000), true);    // window runs past the fetch: may have gained events
  assert.equal(needsRefetch(2000, 2000), true);    // boundary: the fetch could have raced an append at to
});
