// Pure math for the Time view (cadence, window and flow over a shared
// brush), so `node --test` covers the layout and only the drawing lives
// in the component. Bucket and segment shapes are the /api/timeline JSON
// as served (snake_case keys).

const DAY = 86400000;

// ---- ledger kinds (drawing order and hues for the lattice) -----------------
// 'event' is the kind of a count that arrives without one (the timeline
// endpoint's actor buckets); it draws in ink like a note.
export const KIND_ORDER = ['decision', 'feedback', 'note', 'event'];
export const KIND_COLOR = { decision: 'var(--k-decision)', feedback: 'var(--k-feedback)', note: 'var(--text)', event: 'var(--text)' };

// the timeline endpoint's hourly {hour, by_actor} buckets expanded into
// package events {t, group, side}: every count is kind 'event', human on
// the human side, agent and system on the agent side.
export function eventsFromHours(hours) {
  const out = [];
  for (const h of hours) {
    const t = new Date(h.hour).getTime();
    for (const [actor, n] of Object.entries(h.by_actor || {})) {
      const who = actor === 'human' ? 'human' : 'agent';
      for (let i = 0; i < n; i++) out.push({ t, group: 'event', side: who });
    }
  }
  return out;
}


const t = (iso) => new Date(iso).getTime();

// ---- window <-> hash ------------------------------------------------------

// #/time?from=<ms>&to=<ms> so a moment is a link. Absent edges default
// to the last seven days. Garbage values fall back to the default — the
// hash is a link, not an API contract.
export function parseTimeHash(hash, now = Date.now()) {
  const q = (hash || '').split('?')[1] || '';
  const p = new URLSearchParams(q);
  let from = Number(p.get('from'));
  let to = Number(p.get('to'));
  if (!Number.isFinite(from) || from <= 0) from = now - 7 * DAY;
  if (!Number.isFinite(to) || to <= from) to = Math.min(now, from + 7 * DAY);
  if (to <= from) to = from + 3600000; // from in the future: keep a non-empty span
  return { from, to };
}

export function timeHash(from, to) {
  return `#/time?from=${Math.round(from)}&to=${Math.round(to)}`;
}

// ---- Flow: segments -> per-ticket bars -------------------------------------

// One lane per ticket that has a segment inside the window: board cards
// first in board order, then tickets the board no longer lists (done or
// dropped inside the window) in the order they first appear. Each segment
// becomes a bar clipped to the window; segments outside it are dropped.
// Returns lanes of {ulid, slug, title, bars:[{x0, x1, phase}]} in
// window-fraction coordinates (0..1) — the component multiplies by width.
export function flowLanes(segments, cards, from, to) {
  const byId = new Map(cards.map((c) => [c.ulid, c]));
  const lanes = new Map();
  for (const c of cards) lanes.set(c.ulid, { ulid: c.ulid, slug: c.slug || c.ulid, title: c.title || c.slug || c.ulid, bars: [] });
  for (const s of segments) {
    const ulid = s.ticket_ulid;
    if (!lanes.has(ulid)) {
      const c = byId.get(ulid);
      lanes.set(ulid, { ulid, slug: s.slug || c?.slug || ulid, title: c?.title || s.slug || ulid, bars: [] });
    }
    const start = Math.max(t(s.from), from);
    const end = Math.min(t(s.to), to);
    if (end - start <= 0) continue;
    lanes.get(ulid).bars.push({ x0: (start - from) / (to - from), x1: (end - from) / (to - from), phase: s.phase });
  }
  return [...lanes.values()].filter((l) => l.bars.length);
}

// ---- Cadence: hourly buckets -> per-day totals ------------------------------

// Buckets arrive hourly ordered; fold to per-day totals keyed by the
// local calendar day (YYYY-MM-DD) with by-actor split kept for coloring.
export function cadenceDays(buckets) {
  const days = [];
  const idx = new Map();
  for (const b of buckets) {
    const key = dayKey(b.hour);
    if (!idx.has(key)) {
      idx.set(key, { day: key, total: 0, byActor: {} });
      days.push(idx.get(key));
    }
    const d = idx.get(key);
    for (const [actor, n] of Object.entries(b.by_actor || {})) {
      d.total += n;
      d.byActor[actor] = (d.byActor[actor] || 0) + n;
    }
  }
  return days;
}

// Local calendar day of an ISO/hourly timestamp.
export function dayKey(ts) {
  const d = new Date(ts);
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// The calendar days the brush selection touches, oldest first — for the
// Day view's prev/next stepping.
export function daysIn(from, to) {
  const out = [];
  const d = new Date(from);
  d.setHours(0, 0, 0, 0);
  while (d.getTime() <= to) {
    out.push(dayKey(d));
    d.setDate(d.getDate() + 1);
  }
  return out;
}

// ---- durations (the text chain under the drawing) ---------------------------

export function fmtDur(ms) {
  if (ms == null) return '';
  const m = Math.round(ms / 60000);
  if (m < 1) return '<1m';
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 48) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
  const dd = Math.floor(h / 24);
  return h % 24 ? `${dd}d ${h % 24}h` : `${dd}d`;
}

// ---- the ledger itself: /api/events -> dots ---------------------------------
// The Time view draws the ledger one dot per event. Each event folds to a
// dot {t, id, kind, group, side, ticket, slug, signal}; `group` is the hue
// key and is rewritten by hueBy() for the mode the reader picked, `side`
// is human or agent (system counts as the machine), `signal` is the
// store's own class so the signals-only toggle agrees with the ledger.

// kind -> hue group. Contract and review are one group (the signed forms);
// anything the store classes as a transition draws dim.
export const LEDGER_ORDER = ['decision', 'gate', 'feedback', 'contract', 'note', 'transition'];
export const LEDGER_COLOR = {
  decision: 'var(--k-decision)', gate: 'var(--k-gate)', feedback: 'var(--k-feedback)',
  contract: 'var(--k-contract)', note: 'var(--k-note)', transition: 'var(--k-transition)'
};
const KIND_GROUP = { decision: 'decision', gate: 'gate', feedback: 'feedback', contract: 'contract', review: 'contract', note: 'note' };
export function kindGroup(kind) {
  return KIND_GROUP[kind] || 'transition';
}

export const ACTOR_ORDER = ['human', 'agent'];
export const ACTOR_COLOR = { human: 'var(--phos)', agent: 'var(--phos-dim)' };

// six hues for the busiest tickets, the rest go dim; more than six is noise
export const TICKET_PALETTE = ['var(--t1)', 'var(--t2)', 'var(--t3)', 'var(--t4)', 'var(--t5)', 'var(--t6)'];
export const TICKET_OTHER = 'var(--k-transition)';
export const TICKET_TOP = 6;

// slugOf: ticket ulid -> slug (falls back to the ulid's first eight chars)
export function foldLedger(events, slugOf = () => null) {
  return events.map((e) => ({
    t: t(e.ts), id: e.id, kind: e.kind, group: kindGroup(e.kind),
    side: e.actor_type === 'human' ? 'human' : 'agent',
    ticket: e.ticket_ulid, slug: slugOf(e.ticket_ulid) || (e.ticket_ulid || '').slice(0, 8),
    signal: e.class === 'signal', actor: e.actor, payload: e.payload || {}
  }));
}

// the busiest tickets in a set of dots, busiest first: [{ticket, slug, n}]
export function topTickets(dots, n = TICKET_TOP) {
  const count = new Map();
  for (const d of dots) {
    const c = count.get(d.ticket) || { ticket: d.ticket, slug: d.slug, n: 0 };
    c.n++;
    count.set(d.ticket, c);
  }
  return [...count.values()].sort((a, b) => b.n - a.n || a.slug.localeCompare(b.slug)).slice(0, n);
}

// mode: 'kind' | 'actor' | 'ticket'. Returns the lattice's group order +
// colors and the dots with `group` set for that mode. `top` (ticket mode)
// is the set the hues are assigned to; pass the window's busiest so the
// overview strip and the window agree on who is who.
export function hueBy(dots, mode, top = []) {
  if (mode === 'actor') {
    return { groups: ACTOR_ORDER.map((name) => ({ name, color: ACTOR_COLOR[name] })),
             dots: dots.map((d) => ({ ...d, group: d.side })) };
  }
  if (mode === 'ticket') {
    const hue = new Map(top.map((x, i) => [x.ticket, TICKET_PALETTE[i % TICKET_PALETTE.length]]));
    const groups = top.map((x, i) => ({ name: x.slug, color: TICKET_PALETTE[i % TICKET_PALETTE.length] }));
    groups.push({ name: 'other', color: TICKET_OTHER });
    return { groups, dots: dots.map((d) => ({ ...d, group: hue.has(d.ticket) ? d.slug : 'other' })) };
  }
  return { groups: LEDGER_ORDER.map((name) => ({ name, color: LEDGER_COLOR[name] })),
           dots: dots.map((d) => ({ ...d, group: kindGroup(d.kind) })) };
}

// the signals-only toggle and the legend filter, as one pass
export function filterDots(dots, { signalsOnly = false, only = null } = {}) {
  return dots.filter((d) => (!signalsOnly || d.signal) && (!only || d.group === only));
}

// what one column holds: counts by group, the tickets in it (busiest
// first) and the newest event id, which is where the journal opens
export function columnSummary(dots, t0, t1) {
  const inCol = dots.filter((d) => d.t >= t0 && d.t < t1);
  const byGroup = {};
  let newest = 0;
  for (const d of inCol) {
    byGroup[d.group] = (byGroup[d.group] || 0) + 1;
    if (d.id > newest) newest = d.id;
  }
  return { total: inCol.length, byGroup, tickets: topTickets(inCol, 4), newestId: newest || null };
}

// the column under an x offset, in the lattice's own bucketing: cols of
// `cell` px across `width`, so the tooltip and the click agree with the
// dots the lib drew
export function columnAt(x, width, span, cell = 8) {
  const cols = Math.max(1, Math.floor(width / cell));
  const i = Math.min(cols - 1, Math.max(0, Math.floor(x / cell)));
  const ms = (span.max - span.min) / cols;
  return { i, t0: span.min + i * ms, t1: span.min + (i + 1) * ms };
}

// per-day totals from dots, oldest first, for the chain under the overview
export function dayTotals(dots) {
  const days = new Map();
  for (const d of dots) {
    const key = dayKey(d.t);
    const row = days.get(key) || { day: key, total: 0, human: 0 };
    row.total++;
    if (d.side === 'human') row.human++;
    days.set(key, row);
  }
  return [...days.values()].sort((a, b) => a.day.localeCompare(b.day));
}

// ---- the range fields ----------------------------------------------------------
// Two native datetime-local fields set the window to the minute, local
// time: 'YYYY-MM-DDTHH:MM'. A bare date still works (from at midnight, to
// at the end of its day). Seconds are not a control here: finer than a
// minute is more than the ledger needs to be read at. A reversed or empty
// pair is an error the caller reports and does not navigate on.
const LOCAL = /^(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2}))?$/;
function parseLocal(s, endOfDay) {
  const m = LOCAL.exec(s || '');
  if (!m) return NaN;
  const [, y, mo, d, h, mi] = m;
  if (h == null) return endOfDay ? new Date(+y, mo - 1, +d + 1).getTime() - 1 : new Date(+y, mo - 1, +d).getTime();
  return new Date(+y, mo - 1, +d, +h, +mi).getTime();
}
export function rangeFromLocal(fromStr, toStr) {
  if (!fromStr || !toStr) return { error: 'both ends are needed' };
  const from = parseLocal(fromStr, false), to = parseLocal(toStr, true);
  if (!Number.isFinite(from) || !Number.isFinite(to)) return { error: 'that is not a date' };
  if (to <= from) return { error: 'the range runs backwards' };
  return { from, to };
}

// a timestamp as the field's own format, local time to the minute
export function minuteKey(ms) {
  const d = new Date(ms);
  const p = (n) => String(n).padStart(2, '0');
  return `${dayKey(ms)}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

// the arrows: step the window by its own length, either way
export function stepWindow(from, to, dir) {
  const len = to - from;
  return { from: from + dir * len, to: to + dir * len };
}
