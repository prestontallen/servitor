import test from 'node:test';
import assert from 'node:assert/strict';
import { foldFlow, layoutFlow, svgFlow } from './flow.js';

let id = 0;
const ts = (h) => `2026-09-23T${String(h).padStart(2, '0')}:00:00Z`;
const ev = (kind, payload, extra = {}) => ({
  id: ++id, ts: ts(id % 24), actor: 'agent:cli', actor_type: 'agent', kind, payload, ...extra
});
const flow = (nodes, edges, extra = {}) => ev('flow.set', { nodes, edges }, extra);

// ---- foldFlow: latest snapshot wins, validation is total --------------------

test('no flow.set events yields null', () => {
  assert.equal(foldFlow([ev('note', { v: 'hi' }), ev('status.set', { status: 'active' })]), null);
});

test('the latest flow.set is the document; earlier ones are versions', () => {
  const h = [
    flow([{ id: 'a', label: 'old' }], []),
    flow([{ id: 'a', label: 'new' }, { id: 'b', label: 'two' }], [], { actor: 'agent:hermes', ts: ts(5) })
  ];
  const f = foldFlow(h);
  assert.equal(f.ok, true);
  assert.equal(f.versions, 2);
  assert.equal(f.actor, 'agent:hermes');
  assert.equal(f.nodes.length, 2);
  assert.equal(f.nodes[0].label, 'new');
});

test('a malformed payload folds to a one-line error, not a throw', () => {
  const bad = foldFlow([ev('flow.set', { nope: true })]);
  assert.equal(bad.ok, false);
  assert.match(bad.error, /nodes/);
});

test('dangling edge references are rejected', () => {
  const bad = foldFlow([flow([{ id: 'a', label: 'a' }], [{ from: 'a', to: 'ghost' }])]);
  assert.equal(bad.ok, false);
  assert.match(bad.error, /ghost/);
});

// ---- layoutFlow: ranks left-to-right, siblings top-to-bottom ---------------

const G = (nodes, edges) => ({ ok: true, nodes: nodes.map((n) => typeof n === 'string' ? { id: n, label: n, state: null } : n), edges });

test('three ranks advance left-to-right by longest path', () => {
  // a -> b -> d, a -> c -> d: d sits on rank 2, the longest path
  const l = layoutFlow(G(['a', 'b', 'c', 'd'], [{ from: 'a', to: 'b' }, { from: 'a', to: 'c' }, { from: 'b', to: 'd' }, { from: 'c', to: 'd' }]));
  const x = Object.fromEntries(l.nodes.map((n) => [n.id, n.x]));
  assert.ok(x.a < x.b && x.a < x.c, 'sources left of their targets');
  assert.ok(x.b < x.d && x.c < x.d, 'longest path advances right');
  assert.equal(new Set(l.nodes.map((n) => n.x)).size, 3, 'three distinct rank columns');
});

test('siblings in the same rank stack top-to-bottom', () => {
  const l = layoutFlow(G(['a', 'b', 'c'], [{ from: 'a', to: 'b' }, { from: 'a', to: 'c' }]));
  const p = Object.fromEntries(l.nodes.map((n) => [n.id, [n.x, n.y]]));
  assert.equal(p.b[0], p.c[0], 'same rank');
  assert.ok(p.b[1] !== p.c[1], 'stacked apart');
  const ids = [p.b, p.c].sort((u, v) => u[1] - v[1]).map((q) => q === p.b ? 'b' : 'c');
  assert.equal(ids[0], 'b', 'declaration order wins ties top-to-bottom');
});

test('cycles: back edges are ignored for ranking, still drawn, no hang', () => {
  const l = layoutFlow(G(['a', 'b', 'c'], [{ from: 'a', to: 'b' }, { from: 'b', to: 'c' }, { from: 'c', to: 'a' }]));
  const x = Object.fromEntries(l.nodes.map((n) => [n.id, n.x]));
  assert.equal(l.edges.length, 3, 'the back edge survives');
  // a 3-cycle cannot rank acyclically: exactly one edge is a back edge,
  // the other two advance left-to-right
  const back = l.edges.filter((e) => x[e.from] >= x[e.to]);
  assert.equal(back.length, 1, 'exactly one back edge');
});

test('deterministic: same graph, byte-identical layout', () => {
  const g = G(['a', 'b', 'c', 'd', 'e'], [{ from: 'a', to: 'b' }, { from: 'a', to: 'c' }, { from: 'b', to: 'd' }, { from: 'c', to: 'd' }, { from: 'd', to: 'e' }]);
  assert.deepEqual(JSON.stringify(layoutFlow(g)), JSON.stringify(layoutFlow(g)));
});

test('no two node boxes overlap', () => {
  const l = layoutFlow(G(['a', 'b', 'c', 'd', 'e', 'f'], [
    { from: 'a', to: 'b' }, { from: 'a', to: 'c' }, { from: 'a', to: 'd' },
    { from: 'b', to: 'e' }, { from: 'c', to: 'e' }, { from: 'd', to: 'f' }
  ]));
  for (let i = 0; i < l.nodes.length; i++) {
    for (let j = i + 1; j < l.nodes.length; j++) {
      const u = l.nodes[i], v = l.nodes[j];
      const overlap = u.x === v.x && u.y === v.y;
      assert.ok(!overlap, `nodes ${u.id} and ${v.id} share no cell`);
    }
  }
});

test('barycenter: connected siblings order to cut crossings', () => {
  // two independent chains a->x, b->y; ranking both x,y on rank 1.
  // barycenter pulls x under a, y under b (their sources), not reversed.
  const l = layoutFlow(G(['a', 'b', 'x', 'y'], [{ from: 'a', to: 'x' }, { from: 'b', to: 'y' }]));
  const p = Object.fromEntries(l.nodes.map((n) => [n.id, n.y]));
  // order preservation across columns: targets keep their sources' order
  assert.equal(Math.sign(p.a - p.b), Math.sign(p.x - p.y), 'targets keep source order');
});

// ---- svgFlow: themed SVG string from a layout ------------------------------

test('svgFlow emits one box per node and one path per edge', () => {
  const l = layoutFlow(G(['a', 'b'], [{ from: 'a', to: 'b' }]));
  const svg = svgFlow(l);
  assert.match(svg, /^<svg/);
  assert.match(svg, /<\/svg>$/);
  assert.equal((svg.match(/<rect/g) || []).length, 2);
  assert.equal((svg.match(/<path/g) || []).length, 1);
  assert.match(svg, />a</);
  assert.match(svg, />b</);
});

test('node state maps to the servitor status palette classes', () => {
  const l = layoutFlow(G([
    { id: 'q', label: 'q', state: 'queued' },
    { id: 'r', label: 'r', state: 'active' },
    { id: 's', label: 's', state: 'blocked' },
    { id: 't', label: 't', state: 'done' },
    { id: 'u', label: 'u', state: null }
  ], []));
  const svg = svgFlow(l);
  assert.match(svg, /class="fl-node st-queued"/);
  assert.match(svg, /class="fl-node st-active"/);
  assert.match(svg, /class="fl-node st-blocked"/);
  assert.match(svg, /class="fl-node st-done"/);
  assert.match(svg, /class="fl-node"/); // plain stateless node
});

test('edges are dotted, lattice-stepped lines between node centers', () => {
  const l = layoutFlow(G(['a', 'b', 'c'], [{ from: 'a', to: 'b' }, { from: 'a', to: 'c' }]));
  const svg = svgFlow(l);
  assert.match(svg, /stroke-dasharray/);
  assert.match(svg, /class="fl-edge"/);
});

test('svgFlow is pure: same layout in, byte-identical string out', () => {
  const l = layoutFlow(G(['a', 'b', 'c'], [{ from: 'a', to: 'b' }, { from: 'b', to: 'c' }]));
  assert.equal(svgFlow(l), svgFlow(l));
});

test('long labels are truncated in text, full label kept as the title tooltip', () => {
  const l = layoutFlow(G([{ id: 'a', label: 'a very long node label that cannot fit', state: null }, 'b'], [{ from: 'a', to: 'b' }]));
  const svg = svgFlow(l);
  const texts = [...svg.matchAll(/<text[^>]*>(.*?)<\/text>/g)].map((m) => m[1]);
  assert.ok(texts.every((t) => !t.includes('a very long node label')), 'no text node carries the untruncated label');
  assert.ok(texts.some((t) => t.endsWith('…')), 'the clipped text ends in an ellipsis');
  assert.match(svg, /<title>a very long node label that cannot fit<\/title>/);
});
