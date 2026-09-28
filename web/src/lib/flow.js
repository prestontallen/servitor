// Flowcharts from the ledger. Agents submit a whole-graph snapshot as a
// flow.set event; the ticket dossier renders the latest one. Pure functions
// only — no Svelte, no DOM — so `node --test` covers everything. The Svelte
// adapter (Flow.svelte) owns no logic.
//
// Event shape (kind "flow.set") — a system diagram, not a ticket lifecycle:
//   { nodes: [{ id, label, kind? }], edges: [{ from, to, label?, both? }] }
// kind (optional) is what the box is: edge|service|store|queue|client|
// external|other (default other); hues come from the GUI's existing ledger
// tokens. Edge label names what moves along it; both:true marks a round
// trip with a double-headed arrow.
// Ranks advance top-to-bottom (longest path); siblings in a rank sit
// side by side, left-to-right.

const KINDS = new Set(['edge', 'service', 'store', 'queue', 'client', 'external', 'other']);

// ---- fold: pick the latest snapshot, validate it totally --------------------

export function foldFlow(history = []) {
  const events = history.filter((e) => e.kind === 'flow.set').sort((a, b) => a.id - b.id);
  if (!events.length) return null;
  const last = events[events.length - 1];
  const p = last.payload;
  const fail = (error) => ({ ok: false, error, versions: events.length, actor: last.actor, ts: last.ts });
  if (!p || typeof p !== 'object' || !Array.isArray(p.nodes) || p.nodes.length === 0) {
    return fail('flow.set payload needs a non-empty nodes array');
  }
  const ids = new Set();
  for (const n of p.nodes) {
    if (!n || typeof n.id !== 'string' || !n.id) return fail('every node needs a non-empty string id');
    if (ids.has(n.id)) return fail(`duplicate node id ${n.id}`);
    ids.add(n.id);
  }
  const edges = Array.isArray(p.edges) ? p.edges : [];
  for (const e of edges) {
    if (!e || typeof e !== 'object') return fail('every edge must be an object');
    if (!ids.has(e.from) || !ids.has(e.to)) return fail(`edge references unknown node: ${e.from || '?'} → ${e.to || '?'}`);
    if (e.from === e.to) return fail(`self edge on ${e.from}`);
  }
  const nodes = p.nodes.map((n) => ({
    id: n.id,
    label: typeof n.label === 'string' && n.label ? n.label : n.id,
    kind: KINDS.has(n.kind) ? n.kind : 'other'
  }));
  return {
    ok: true, nodes,
    edges: edges.map((e) => ({
      from: e.from, to: e.to,
      label: typeof e.label === 'string' && e.label ? e.label : null,
      both: e.both === true
    })),
    versions: events.length, actor: last.actor, ts: last.ts
  };
}

// ---- layout: layered, ranks top-to-bottom, siblings left-to-right ----------
//
// Rank by longest path (DFS, cycles tolerated: a node being visited keeps
// the DFS from recursing forever and its back edge is simply not a rank
// constraint). Order within a rank by barycenter of already-placed
// neighbours, declaration order breaking ties. x/y are lattice cells.

export function layoutFlow(f) {
  if (!f || !f.ok) return null;
  const nodes = f.nodes.map((n) => ({ ...n }));
  const index = new Map(nodes.map((n, i) => [n.id, i]));
  const ins = nodes.map(() => []);
  for (const e of f.edges) {
    if (!index.has(e.from) || !index.has(e.to)) continue;
    ins[index.get(e.to)].push(index.get(e.from));
  }
  // longest-path rank FROM sources (rank 0 = top): rank = 1 + max rank
  // of predecessors. A BUSY predecessor is a back edge in the DFS tree and
  // is not a rank constraint, which is what makes cycles rankable.
  const UNSEEN = 0, DONE = 1, BUSY = 2;
  const mark = new Array(nodes.length).fill(UNSEEN);
  const rank = new Array(nodes.length).fill(0);
  const visit = (i) => {
    if (mark[i] !== UNSEEN) return rank[i];
    mark[i] = BUSY;
    let r = 0;
    for (const j of ins[i]) if (mark[j] !== BUSY) r = Math.max(r, visit(j) + 1);
    mark[i] = DONE;
    rank[i] = r;
    return r;
  };
  for (let i = 0; i < nodes.length; i++) visit(i);

  // ranks: row lists in declaration order
  const maxRank = Math.max(...rank);
  const rows = Array.from({ length: maxRank + 1 }, () => []);
  for (let i = 0; i < nodes.length; i++) rows[rank[i]].push(i);

  // order within each row by barycenter of already-placed neighbours
  // (downward sweep using sources; declaration order breaks ties)
  const placed = new Set();
  const order = new Array(nodes.length).fill(0);
  for (const row of rows) {
    const keyed = row.map((i, position) => {
      const sources = ins[i].filter((j) => placed.has(j)).map((j) => order[j]);
      const bary = sources.length ? sources.reduce((a, b) => a + b, 0) / sources.length : position;
      return { i, bary, position };
    });
    keyed.sort((u, v) => u.bary - v.bary || u.position - v.position);
    keyed.forEach((k, position) => (order[k.i] = position));
    for (const k of keyed) placed.add(k.i);
  }

  return {
    nodes: nodes.map((n, i) => ({ ...n, x: order[i], y: rank[i] })),
    edges: f.edges.map((e) => ({ ...e }))
  };
}

// ---- render: one SVG string, pixel boxes on the lattice pitch ---------------
//
// Pure: layout in, string out, nothing read from the DOM or the environment.
// The Svelte adapter injects it with {@html} and styles it with classes —
// the theme comes from the same --line/--ok/--warn/--fail/--accent tokens the
// rest of the GUI uses, so no colors live here.

const PITCH_X = 132;  // column pitch in px: box width 104 + corridor 28
const PITCH_Y = 82;   // row pitch: box height 34 + corridor 48 for labels and heads
const BOX_W = 104;
const BOX_H = 34;
const PAD = 14;       // lattice margin
const MAX_LABEL = 15;      // ~15 chars at 11px fits 96px of box
const MAX_ELABEL = 18;     // edge labels truncate to about one column pitch

function esc(s) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

const clip = (s, max) => (s.length > max ? s.slice(0, max - 1).trimEnd() + '…' : s);

// Arrowhead marker; its stroke is themed by Flow.svelte's .fl-head rule,
// since var() is not reliable in an SVG presentation attribute.
const DEFS = '<defs><marker id="fl-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path class="fl-head" d="M0 0 L8 4 L0 8" fill="none"/></marker></defs>';

export function svgFlow(l) {
  if (!l) return '';
  const cols = Math.max(0, ...l.nodes.map((n) => n.x)) + 1;
  const rows = Math.max(0, ...l.nodes.map((n) => n.y)) + 1;
  let W = PAD * 2 + cols * PITCH_X - (PITCH_X - BOX_W);
  let H = PAD * 2 + rows * PITCH_Y - (PITCH_Y - BOX_H);
  const at = (n) => ({ cx: PAD + n.x * PITCH_X + BOX_W / 2, cy: PAD + n.y * PITCH_Y + BOX_H / 2 });
  const CORR = PITCH_Y - BOX_H;          // row corridor height
  const GUT = (PITCH_X - BOX_W) / 2;     // half the column gutter
  const into = new Map();                // labels already stacked on a target

  const edges = l.edges.map((e) => {
    const u = l.nodes.find((n) => n.id === e.from);
    const v = l.nodes.find((n) => n.id === e.to);
    if (!u || !v) return '';
    const a = at(u), b = at(v);
    // lattice-stepped between box edges, so heads land in the corridor, not
    // under a box. Adjacent rows: down, across the corridor, down. Longer
    // spans and same-row loops step out into a column gutter, which no box
    // occupies, and back in along the corridor next to the target.
    const dir = Math.sign(b.cy - a.cy) || 1;
    const sy = a.cy + dir * BOX_H / 2;
    const ty = a.cy === b.cy ? b.cy + BOX_H / 2 : b.cy - dir * BOX_H / 2;
    const tm = ty - (a.cy === b.cy ? -1 : dir) * CORR / 2; // corridor line beside the target
    let d;
    if (Math.abs(u.y - v.y) === 1) {
      d = `M ${a.cx} ${sy} V ${tm} H ${b.cx} V ${ty}`;
    } else {
      const sm = sy + dir * CORR / 2;
      const gx = u.x === v.x
        ? PAD + u.x * PITCH_X + BOX_W + GUT
        : PAD + Math.max(u.x, v.x) * PITCH_X - GUT;
      W = Math.max(W, gx + PAD);
      d = `M ${a.cx} ${sy} V ${sm} H ${gx} V ${tm} H ${b.cx} V ${ty}`;
    }
    H = Math.max(H, tm + PAD);
    const heads = e.both ? ' marker-start="url(#fl-arrow)" marker-end="url(#fl-arrow)"' : ' marker-end="url(#fl-arrow)"';
    // label beside the target's last leg, in the half of the corridor the
    // crossing lines do not use; several edges into one target stack away
    // from it. Haloed in the card colour; path, label and tooltip share one
    // <g> so the title is a real tooltip.
    let label = '';
    if (e.label) {
      const side = a.cy === b.cy ? 1 : -dir;       // which way is away from the target
      const k = into.get(v.id) || 0;
      into.set(v.id, k + 1);
      const ly = ty + side * (8 + k * 11) + (side > 0 ? 7 : 0);
      label = `<text class="fl-elabel" x="${b.cx + 5}" y="${ly}">${esc(clip(e.label, MAX_ELABEL))}</text>`;
    }
    const tip = e.label && e.label.length > MAX_ELABEL ? `<title>${esc(e.label)}</title>` : '';
    return `<g class="fl-e"><path class="fl-edge" d="${d}" stroke-dasharray="2 4"${heads}/>${label}${tip}</g>`;
  }).join('');

  const nodes = l.nodes.map((n) => {
    const x = PAD + n.x * PITCH_X;
    const y = PAD + n.y * PITCH_Y;
    const cls = `fl-node k-${n.kind}`; // foldFlow normalized kind
    const title = n.label.length > MAX_LABEL ? `<title>${esc(n.label)}</title>` : '';
    return `<g class="${cls}">${title}<rect x="${x}" y="${y}" width="${BOX_W}" height="${BOX_H}" rx="2"/><text x="${x + BOX_W / 2}" y="${y + BOX_H / 2 + 4}" text-anchor="middle">${esc(clip(n.label, MAX_LABEL))}</text></g>`;
  }).join('');

  return `<svg class="fl-svg" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="img">${DEFS}${edges}${nodes}</svg>`;
}
