// Flowcharts from the ledger. Agents submit a whole-graph snapshot as a
// flow.set event; the ticket dossier renders the latest one. Pure functions
// only — no Svelte, no DOM — so `node --test` covers everything. The Svelte
// adapter (Flow.svelte) owns no logic.
//
// Event shape (kind "flow.set"):
//   { nodes: [{ id, label, state }], edges: [{ from, to }] }
// state maps to the servitor ticket statuses: queued|active|blocked|done.
// Ranks advance left-to-right (longest path); siblings in a rank stack
// top-to-bottom.

const STATES = new Set(['queued', 'active', 'blocked', 'done']);

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
    state: STATES.has(n.state) ? n.state : null
  }));
  return {
    ok: true, nodes, edges: edges.map((e) => ({ from: e.from, to: e.to })),
    versions: events.length, actor: last.actor, ts: last.ts
  };
}

// ---- layout: layered, ranks left-to-right, siblings top-to-bottom ----------
//
// Rank by longest path (DFS, cycles tolerated: a node being visited keeps
// the DFS from recursing forever and its back edge is simply not a rank
// constraint). Order within a rank by barycenter of already-placed
// neighbours, declaration order breaking ties. x/y are lattice cells.

export function layoutFlow(f) {
  if (!f || !f.ok) return null;
  const nodes = f.nodes.map((n) => ({ ...n }));
  const index = new Map(nodes.map((n, i) => [n.id, i]));
  const outs = nodes.map(() => []);
  const ins = nodes.map(() => []);
  for (const e of f.edges) {
    if (!index.has(e.from) || !index.has(e.to)) continue;
    outs[index.get(e.from)].push(index.get(e.to));
    ins[index.get(e.to)].push(index.get(e.from));
  }
  // longest-path rank FROM sources (rank 0 = leftmost): rank = 1 + max rank
  // of predecessors. A BUSY predecessor is a back edge in the DFS tree and
  // is not a rank constraint, which is what makes cycles rankable.
  const UNSEEN = 0, DONE = 1, BUSY = 2;
  const state = new Array(nodes.length).fill(UNSEEN);
  const rank = new Array(nodes.length).fill(0);
  const visit = (i) => {
    if (state[i] !== UNSEEN) return rank[i];
    state[i] = BUSY;
    let r = 0;
    for (const j of ins[i]) if (state[j] !== BUSY) r = Math.max(r, visit(j) + 1);
    state[i] = DONE;
    rank[i] = r;
    return r;
  };
  for (let i = 0; i < nodes.length; i++) visit(i);

  // ranks: column lists in declaration order
  const maxRank = Math.max(...rank);
  const columns = Array.from({ length: maxRank + 1 }, () => []);
  for (let i = 0; i < nodes.length; i++) columns[rank[i]].push(i);

  // order within each column by barycenter of already-placed neighbours
  // (left sweep using sources; declaration order breaks ties)
  const placed = new Set();
  const y = new Array(nodes.length).fill(0);
  for (const col of columns) {
    const keyed = col.map((i, position) => {
      const sources = ins[i].filter((j) => placed.has(j)).map((j) => y[j]);
      const bary = sources.length ? sources.reduce((a, b) => a + b, 0) / sources.length : position;
      return { i, bary, position };
    });
    keyed.sort((u, v) => u.bary - v.bary || u.position - v.position);
    keyed.forEach((k, position) => (y[k.i] = position));
    for (const k of keyed) placed.add(k.i);
  }

  return {
    nodes: nodes.map((n, i) => ({ ...n, x: rank[i], y: y[i] })),
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
const PITCH_Y = 52;   // row pitch: box height 34 + corridor 18
const BOX_W = 104;
const BOX_H = 34;
const PAD = 14;       // lattice margin
const MAX_LABEL = 15; // ~15 chars at 11px fits 96px of box

function esc(s) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

const clip = (s, max) => (s.length > max ? s.slice(0, max - 1).trimEnd() + '…' : s);

export function svgFlow(l) {
  if (!l) return '';
  const xs = l.nodes.map((n) => n.x);
  const ys = l.nodes.map((n) => n.y);
  const cols = Math.max(0, ...xs) + 1;
  const rows = Math.max(0, ...ys) + 1;
  const W = PAD * 2 + cols * PITCH_X - (PITCH_X - BOX_W);
  const H = PAD * 2 + rows * PITCH_Y - (PITCH_Y - BOX_H);
  const at = (n) => ({ cx: PAD + n.x * PITCH_X + BOX_W / 2, cy: PAD + n.y * PITCH_Y + BOX_H / 2 });

  const edges = l.edges.map((e) => {
    const u = l.nodes.find((n) => n.id === e.from);
    const v = l.nodes.find((n) => n.id === e.to);
    if (!u || !v) return '';
    const a = at(u), b = at(v);
    // lattice-stepped: horizontal out of the source, vertical in the
    // corridor between the two columns, horizontal into the target
    const mid = (a.cx + b.cx) / 2;
    const d = `M ${a.cx} ${a.cy} H ${mid} V ${b.cy} H ${b.cx}`;
    return `<path class="fl-edge${v.state === 'blocked' ? ' to-blocked' : ''}" d="${d}" stroke-dasharray="2 4"/>`;
  }).join('');

  const nodes = l.nodes.map((n) => {
    const x = PAD + n.x * PITCH_X;
    const y = PAD + n.y * PITCH_Y;
    const cls = n.state ? `fl-node st-${n.state}` : 'fl-node';
    const title = n.label.length > MAX_LABEL ? `<title>${esc(n.label)}</title>` : '';
    return `<g class="${cls}">${title}<rect x="${x}" y="${y}" width="${BOX_W}" height="${BOX_H}" rx="2"/><text x="${x + BOX_W / 2}" y="${y + BOX_H / 2 + 4}" text-anchor="middle">${esc(clip(n.label, MAX_LABEL))}</text></g>`;
  }).join('');

  return `<svg class="fl-svg" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="img">${edges}${nodes}</svg>`;
}
