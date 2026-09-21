// Dot lattice: the ledger drawn one dot per event on a fixed grid.
// Pure functions so the layout is testable without a DOM. The strip
// component owns pixels; this owns the counting.

// stacking order within a column, baseline outward. 'event' is the kind
// of a count that arrives without one (the timeline endpoint's actor
// buckets); it draws in ink like a note.
export const KIND_ORDER = ['decision', 'feedback', 'note', 'event'];
export const KIND_COLOR = { decision: 'var(--k-decision)', feedback: 'var(--k-feedback)', note: 'var(--text)', event: 'var(--text)' };

const side = (e) => (e.actor_type === 'human' ? 'human' : 'agent');

// bucket events into `cols` equal time slots over [min, max]. Each bucket
// carries counts per kind per side plus totals. Events outside the span
// are ignored; the last edge is inclusive so the newest event lands.
export function bucketize(events, min, max, cols) {
  const out = Array.from({ length: cols }, (_, i) => ({
    i, start: min + ((max - min) * i) / cols, counts: {}, human: 0, agent: 0
  }));
  if (cols <= 0 || max <= min) return out;
  for (const e of events) {
    const t = new Date(e.ts).getTime();
    if (t < min || t > max) continue;
    const i = Math.min(cols - 1, Math.floor(((t - min) / (max - min)) * cols));
    const b = out[i];
    const who = side(e);
    (b.counts[e.kind] ??= { human: 0, agent: 0 })[who]++;
    b[who]++;
  }
  return out;
}

// the timeline endpoint's hourly {hour, by_actor} buckets folded into
// `cols` lattice columns over [min, max]: every count is kind 'event',
// human on the human side, agent and system on the agent side.
export function fromActorBuckets(hours, min, max, cols) {
  const out = Array.from({ length: cols }, (_, i) => ({
    i, start: min + ((max - min) * i) / cols, counts: {}, human: 0, agent: 0
  }));
  if (cols <= 0 || max <= min) return out;
  for (const h of hours) {
    const t = new Date(h.hour).getTime();
    if (t < min || t > max) continue;
    const b = out[Math.min(cols - 1, Math.floor(((t - min) / (max - min)) * cols))];
    for (const [actor, n] of Object.entries(h.by_actor || {})) {
      const who = actor === 'human' ? 'human' : 'agent';
      (b.counts.event ??= { human: 0, agent: 0 })[who] += n;
      b[who] += n;
    }
  }
  return out;
}

// how many events one dot stands for, so the tallest column fits maxRows.
export function quantumFor(buckets, maxRows) {
  let peak = 0;
  for (const b of buckets) peak = Math.max(peak, b.human, b.agent);
  return Math.max(1, Math.ceil(peak / maxRows));
}

// the dots for one side of one column, baseline outward: [{kind}].
// Rounding runs over the side's cumulative count, not per kind, so the
// side always draws ceil(total / quantum) dots and the legend stays true.
export function stack(bucket, who, quantum) {
  const dots = [];
  let cum = 0;
  for (const k of KIND_ORDER) {
    const c = bucket.counts[k];
    if (!c || !c[who]) continue;
    const before = Math.ceil(cum / quantum);
    cum += c[who];
    for (let n = Math.ceil(cum / quantum) - before; n > 0; n--) dots.push({ kind: k });
  }
  return dots;
}

// rows needed on each side, given the quantum
export function rowsFor(buckets, quantum) {
  let human = 0, agent = 0;
  for (const b of buckets) {
    human = Math.max(human, stack(b, 'human', quantum).length);
    agent = Math.max(agent, stack(b, 'agent', quantum).length);
  }
  return { human, agent };
}

// milestone labels along a width: each stays centred on its own line
// (clamped inside the edges); one that would touch an earlier label takes
// the lowest free row, so coincident labels pile straight down. Items are
// {cx, w} and must arrive in nondecreasing cx order (time order), since
// each row only remembers its right edge; the result adds row, tx and anchor.
export function stackLabels(items, width, gap = 10) {
  const rowsRight = [];
  return items.map((it) => {
    let left = it.cx - it.w / 2, anchor = 'middle';
    if (left < 0) { left = 0; anchor = 'start'; }
    else if (left + it.w > width) { left = width - it.w; anchor = 'end'; }
    let row = 0;
    while (row < rowsRight.length && left < rowsRight[row] + gap) row++;
    rowsRight[row] = left + it.w;
    return { ...it, row, anchor, tx: anchor === 'middle' ? it.cx : anchor === 'start' ? 0 : width };
  });
}
