// Dot lattice: the ledger drawn one dot per event on a fixed grid.
// Pure functions so the layout is testable without a DOM. The strip
// component owns pixels; this owns the counting.

// stacking order within a column, baseline outward
export const KIND_ORDER = ['decision', 'feedback', 'note'];

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
