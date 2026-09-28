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

const G = (nodes, edges) => ({ ok: true, nodes: nodes.map((n) => typeof n === 'string' ? { id: n, label: n, kind: 'other' } : n), edges });

test('three ranks advance top-to-bottom by longest path', () => {
  // a -> b -> d, a -> c -> d: d sits on rank 2, the longest path
  const l = layoutFlow(G(['a', 'b', 'c', 'd'], [{ from: 'a', to: 'b' }, { from: 'a', to: 'c' }, { from: 'b', to: 'd' }, { from: 'c', to: 'd' }]));
  const y = Object.fromEntries(l.nodes.map((n) => [n.id, n.y]));
  assert.ok(y.a < y.b && y.a < y.c, 'sources above their targets');
  assert.ok(y.b < y.d && y.c < y.d, 'longest path advances down');
  assert.equal(new Set(l.nodes.map((n) => n.y)).size, 3, 'three distinct rank rows');
});

test('siblings in the same rank sit side by side, left-to-right', () => {
  const l = layoutFlow(G(['a', 'b', 'c'], [{ from: 'a', to: 'b' }, { from: 'a', to: 'c' }]));
  const p = Object.fromEntries(l.nodes.map((n) => [n.id, [n.x, n.y]]));
  assert.equal(p.b[1], p.c[1], 'same rank');
  assert.ok(p.b[0] < p.c[0], 'declaration order wins ties left-to-right');
});

test('cycles: back edges are ignored for ranking, still drawn, no hang', () => {
  const l = layoutFlow(G(['a', 'b', 'c'], [{ from: 'a', to: 'b' }, { from: 'b', to: 'c' }, { from: 'c', to: 'a' }]));
  const y = Object.fromEntries(l.nodes.map((n) => [n.id, n.y]));
  assert.equal(l.edges.length, 3, 'the back edge survives');
  // a 3-cycle cannot rank acyclically: exactly one edge is a back edge,
  // the other two advance top-to-bottom
  const back = l.edges.filter((e) => y[e.from] >= y[e.to]);
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
  // barycenter puts x under a, y under b (their sources), not reversed.
  const l = layoutFlow(G(['a', 'b', 'y', 'x'], [{ from: 'a', to: 'x' }, { from: 'b', to: 'y' }]));
  const p = Object.fromEntries(l.nodes.map((n) => [n.id, n.x]));
  // order preservation across rows: targets keep their sources' order
  assert.equal(Math.sign(p.a - p.b), Math.sign(p.x - p.y), 'targets keep source order');
});

// ---- svgFlow: themed SVG string from a layout ------------------------------

test('svgFlow emits one box per node and one path per edge', () => {
  const l = layoutFlow(G(['a', 'b'], [{ from: 'a', to: 'b' }]));
  const svg = svgFlow(l);
  assert.match(svg, /^<svg/);
  assert.match(svg, /<\/svg>$/);
  assert.equal((svg.match(/<rect/g) || []).length, 2);
  assert.equal((svg.match(/class="fl-edge"/g) || []).length, 1);
  assert.match(svg, />a</);
  assert.match(svg, />b</);
});

test('node kind maps to the ledger hue classes; unknown and missing kinds default to other', () => {
  const l = layoutFlow(foldFlow([ev('flow.set', { edges: [], nodes: [
    { id: 'e', label: 'e', kind: 'edge' },
    { id: 's', label: 's', kind: 'service' },
    { id: 'd', label: 'd', kind: 'store' },
    { id: 'q', label: 'q', kind: 'queue' },
    { id: 'c', label: 'c', kind: 'client' },
    { id: 'x', label: 'x', kind: 'external' },
    { id: 'o', label: 'o', kind: 'other' },
    { id: 'n', label: 'n' },
    { id: 'z', label: 'z', kind: 'widget' }
  ] })]));
  const svg = svgFlow(l);
  assert.match(svg, /class="fl-node k-edge"/);
  assert.match(svg, /class="fl-node k-service"/);
  assert.match(svg, /class="fl-node k-store"/);
  assert.match(svg, /class="fl-node k-queue"/);
  assert.match(svg, /class="fl-node k-client"/);
  assert.match(svg, /class="fl-node k-external"/);
  assert.match(svg, /class="fl-node k-other"/);
  assert.equal((svg.match(/k-other"/g) || []).length, 3, 'missing, other and unknown kinds all render as other');
});

test('edge labels render on the path; long ones truncate with a title tooltip', () => {
  const l = layoutFlow(G(['a', 'b'], [{ from: 'a', to: 'b', label: 'publishes user.created' }]));
  const svg = svgFlow(l);
  assert.match(svg, /class="fl-elabel"/);
  const texts = [...svg.matchAll(/<text[^>]*>(.*?)<\/text>/g)].map((m) => m[1]);
  assert.ok(texts.some((t) => t.endsWith('…')), 'the long edge label is clipped');
  assert.match(svg, /<title>publishes user\.created<\/title>/);
});

test('both:true renders a double-headed edge; a plain edge gets one head', () => {
  const l = layoutFlow(G(['a', 'b', 'c'], [
    { from: 'a', to: 'b', both: true },
    { from: 'b', to: 'c' }
  ]));
  const svg = svgFlow(l);
  assert.equal((svg.match(/marker-start=/g) || []).length, 1, 'only the both edge has a start head');
  assert.equal((svg.match(/marker-end=/g) || []).length, 2, 'every edge has an end head');
});

test('the old payload shape (state field) still renders: state ignored, defaults applied', () => {
  const old = foldFlow([ev('flow.set', { nodes: [{ id: 'a', label: 'a', state: 'blocked' }], edges: [] })]);
  assert.equal(old.ok, true);
  assert.equal(old.nodes[0].kind, 'other');
  const l = layoutFlow(old);
  assert.match(svgFlow(l), /class="fl-node k-other"/);
  assert.doesNotMatch(svgFlow(l), /st-|to-blocked/);
});

test('no ticket-status vocabulary survives in the rendered output', () => {
  const l = layoutFlow(G(['a', 'b'], [{ from: 'a', to: 'b' }]));
  assert.doesNotMatch(svgFlow(l), /st-queued|st-active|st-blocked|st-done|to-blocked/);
});

test('edges are dotted, lattice-stepped lines between box edges, heads outside the boxes', () => {
  const l = layoutFlow(G(['a', 'b'], [{ from: 'a', to: 'b' }]));
  const svg = svgFlow(l);
  assert.match(svg, /stroke-dasharray/);
  // a sits on row 0, b on row 1: the path leaves a's bottom edge and ends
  // on b's top edge, so the end head is drawn in the corridor, not under b
  const d = svg.match(/class="fl-edge" d="M (\S+) (\S+) V (\S+) H (\S+) V (\S+)"/);
  assert.ok(d, 'stepped path: vertical, horizontal, vertical');
  const [sy, ty] = [Number(d[2]), Number(d[5])];
  const rects = [...svg.matchAll(/<rect x="\S+" y="(\S+)" width="\S+" height="(\S+)"/g)].map((m) => [Number(m[1]), Number(m[2])]);
  assert.equal(sy, rects[0][0] + rects[0][1], 'starts on the source bottom edge');
  assert.equal(ty, rects[1][0], 'ends on the target top edge');
});

test('a same-row edge loops below both boxes and stays inside the viewBox', () => {
  const l = { nodes: [{ id: 'a', label: 'a', kind: 'other', x: 0, y: 0 }, { id: 'b', label: 'b', kind: 'other', x: 1, y: 0 }], edges: [{ from: 'a', to: 'b', label: 'peer' }] };
  const svg = svgFlow(l);
  const H = Number(svg.match(/viewBox="0 0 \S+ (\S+)"/)[1]);
  const mid = Number(svg.match(/V (\S+) H/)[1]);
  assert.ok(mid < H, 'the loop is not clipped');
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

test('no edge segment crosses a box it does not connect', () => {
  // the v3 shape that failed by eye: a long gui -> api edge ran straight
  // down through the human box
  const f = foldFlow([ev('flow.set', {
    nodes: ['skill', 'agent', 'human', 'mcp', 'api', 'db', 'gui'].map((id) => ({ id })),
    edges: [
      { from: 'skill', to: 'agent' }, { from: 'agent', to: 'human', both: true },
      { from: 'agent', to: 'mcp' }, { from: 'mcp', to: 'api' }, { from: 'api', to: 'db' },
      { from: 'gui', to: 'api', both: true }, { from: 'gui', to: 'human' }
    ]
  })]);
  const svg = svgFlow(layoutFlow(f));
  const boxes = [...svg.matchAll(/<rect x="(\S+)" y="(\S+)" width="(\S+)" height="(\S+)"/g)]
    .map((m) => m.slice(1).map(Number)).map(([x, y, w, h]) => ({ x0: x, x1: x + w, y0: y, y1: y + h }));
  for (const [, d] of svg.matchAll(/class="fl-edge" d="([^"]+)"/g)) {
    const t = d.split(' ');
    let x = Number(t[1]), y = Number(t[2]);
    for (let i = 3; i < t.length; i += 2) {
      const [nx, ny] = t[i] === 'H' ? [Number(t[i + 1]), y] : [x, Number(t[i + 1])];
      const [lx, hx, ly, hy] = [Math.min(x, nx), Math.max(x, nx), Math.min(y, ny), Math.max(y, ny)];
      for (const b of boxes) {
        // strictly inside: touching a box edge is how an edge attaches
        const crosses = lx < b.x1 && hx > b.x0 && ly < b.y1 && hy > b.y0;
        assert.ok(!crosses, `segment ${d} crosses a box at ${b.x0},${b.y0}`);
      }
      x = nx; y = ny;
    }
  }
});
