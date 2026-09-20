// Pure math for the Time view (cadence, window and flow over a shared
// brush), so `node --test` covers the layout and only the drawing lives
// in the component. Bucket and segment shapes are the /api/timeline JSON
// as served (snake_case keys).

const DAY = 86400000;

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
