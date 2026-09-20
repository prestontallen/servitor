<script>
  // The dot lattice, shared by every time drawing. One SVG in measured
  // pixels over a span: columns of dots on a fixed grid, the human's above
  // the baseline and the agents' below, hue by kind, stacked baseline
  // outward. Overlays, all optional: dashed annotation lines with labels
  // that stack rows on collision (ticks), runs of red dots on the baseline
  // (runs), a translucent window (highlight), small labels along the top
  // (axis). The caller supplies the columns through bucketsFor(cols), so
  // it decides what an event is; this component only decides where it goes.
  import { quantumFor, stack, rowsFor, stackLabels, KIND_ORDER, KIND_COLOR } from './lattice.js';

  let {
    span,                    // {min, max} in ms
    bucketsFor,              // (cols) => lattice buckets, see lattice.js
    cell = 8,                // grid pitch (px)
    r = 2.5,                 // dot radius (px)
    maxRows = 12,            // rows a side before a dot becomes a quantum
    ticks = [],              // [{key, t, label, sig?, color?, title?}]
    runs = [],               // [{start, end, title, color?}]
    highlight = null,        // {from, to}
    axis = [],               // [{t, label}]
    columnTitle = null,      // (bucket, bucketMs) => string
    quantum = $bindable(1),  // read back by the caller for its chain
    label = 'dot lattice'
  } = $props();

  let width = $state(0);

  const cols = $derived(Math.max(1, Math.floor(width / cell)));
  const buckets = $derived(span && width ? bucketsFor(cols) : []);
  const q = $derived(quantumFor(buckets, maxRows));
  $effect(() => { quantum = q; });
  const rows = $derived.by(() => {
    const rr = rowsFor(buckets, q);
    // keep a row on each side so the baseline never sits on an edge
    return { human: Math.max(1, rr.human), agent: Math.max(1, rr.agent) };
  });
  const bucketMs = $derived(span ? (span.max - span.min) / cols : 0);

  const x = (ms) => ((Math.min(Math.max(ms, span.min), span.max) - span.min) / (span.max - span.min)) * width;
  const snap = (px) => Math.min(cols - 1, Math.floor(px / cell)) * cell + cell / 2;
  const colOf = (ms) => Math.min(cols - 1, Math.floor(x(ms) / cell));

  // ---- layout (px) ------------------------------------------------------
  const TOP = $derived(axis.length ? 14 : 2);
  const y0 = $derived(TOP + rows.human * cell);                 // baseline
  const lowY = $derived(y0 + rows.agent * cell);                // bottom of agent dots
  const LABEL_PX = 6.1, SIG_PX = 5.2;     // ~px per char: uppercase label, signature
  const GAP = 10, ROW_H = 11;
  const labelY = $derived(lowY + 16);     // first label baseline

  // labels: centred on their own line; a collision takes the lowest free row
  const placed = $derived.by(() => {
    if (!span || !width) return [];
    const items = ticks.map((tk) => ({ ...tk, sig: tk.sig || '', cx: snap(x(tk.t)) }));
    return stackLabels(items.map((it) => ({ ...it, w: it.label.length * LABEL_PX + it.sig.length * SIG_PX })), width, GAP);
  });
  const labelRows = $derived(placed.reduce((n, tk) => Math.max(n, tk.row + 1), 0));
  const height = $derived(placed.length ? labelY + (labelRows - 1) * ROW_H + 4 : lowY + 4);

  const fmtStamp = (ms) => new Date(ms).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  function fmtDur(ms) {
    if (ms < 60000) return '<1m';
    const m = Math.round(ms / 60000);
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    if (h < 48) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
    const d = Math.floor(h / 24);
    return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
  }
  function defaultTitle(b) {
    const parts = KIND_ORDER.filter((k) => b.counts[k]).map((k) => {
      const c = b.counts[k].human + b.counts[k].agent;
      return `${c} ${k}${c === 1 ? '' : 's'}`;
    });
    return `${fmtStamp(b.start)} +${fmtDur(bucketMs)}\n${parts.join(', ')}${b.human ? ` · ${b.human} by the human` : ''}`;
  }
  const titleOf = (b) => (columnTitle ? columnTitle(b, bucketMs) : defaultTitle(b));
</script>

{#if span}
  <div class="lattice" bind:clientWidth={width} data-testid="dot-lattice">
    {#if width > 0}
      <svg {width} {height} aria-label={label}>
        <!-- axis labels along the top -->
        {#each axis as a (a.t)}
          <text class="axis" x={x(a.t)} y="9">{a.label}</text>
        {/each}

        <!-- the window, under everything -->
        {#if highlight}
          <rect class="highlight" x={x(highlight.from)} y={TOP} width={Math.max(x(highlight.to) - x(highlight.from), 2)} height={lowY - TOP} />
        {/if}

        <!-- ticks: dashed annotation lines through the lattice, under the dots -->
        {#each placed as tk (tk.key)}
          <g class="tick" style:color={tk.color || 'var(--text-dim)'}>
            <line class="dash" x1={tk.cx} x2={tk.cx} y1={TOP} y2={labelY + tk.row * ROW_H - 9} />
            <text x={tk.tx} y={labelY + tk.row * ROW_H} text-anchor={tk.anchor}>{tk.label}<tspan class="sig">{tk.sig}</tspan></text>
            {#if tk.title}<title>{tk.title}</title>{/if}
          </g>
        {/each}

        <!-- baseline, with runs of dots on it -->
        <line class="base" x1="0" x2={width} y1={y0} y2={y0} />
        {#each runs as run}
          {@const c0 = colOf(run.start)}
          {@const c1 = colOf(run.end)}
          <g class="run" style:color={run.color || 'var(--fail)'}>
            {#each { length: c1 - c0 + 1 } as _, i}
              <circle cx={(c0 + i) * cell + cell / 2} cy={y0} r={r} />
            {/each}
            {#if run.title}<title>{run.title}</title>{/if}
          </g>
        {/each}

        <!-- the lattice: one dot per event (or per quantum), human up, agent down -->
        {#each buckets as b (b.i)}
          {#if b.human + b.agent}
            {@const cx = b.i * cell + cell / 2}
            <g class="col">
              {#each stack(b, 'human', q) as d, i}
                <circle cx={cx} cy={y0 - 1 - (i * cell + cell / 2)} r={r} fill={KIND_COLOR[d.kind]} class="dot human" />
              {/each}
              {#each stack(b, 'agent', q) as d, i}
                <circle cx={cx} cy={y0 + 1 + (i * cell + cell / 2)} r={r} fill={KIND_COLOR[d.kind]} class="dot" />
              {/each}
              <rect class="hit" x={b.i * cell} y={TOP} width={cell} height={lowY - TOP}>
                <title>{titleOf(b)}</title>
              </rect>
            </g>
          {/if}
        {/each}
      </svg>
    {/if}
  </div>
{/if}

<style>
  .lattice { position: relative; }
  svg { display: block; overflow: visible; }
  .axis { fill: var(--text-dim); font-size: 9px; letter-spacing: 0.06em; }
  .highlight { fill: var(--accent); opacity: 0.12; pointer-events: none; }
  .base { stroke: var(--line-strong); stroke-width: 1; }
  .run circle { fill: currentColor; opacity: 0.85; }
  .dot { opacity: 0.55; }
  .dot.human { opacity: 1; }
  .hit { fill: transparent; }
  .col:hover .hit { fill: var(--text); fill-opacity: 0.07; }
  .tick line { stroke: currentColor; stroke-width: 1; opacity: 0.8; }
  .tick line.dash { stroke-dasharray: 2 3; }
  .tick text { fill: currentColor; font-size: 9px; letter-spacing: 0.06em; }
  .tick .sig { fill: var(--text-dim); letter-spacing: 0; }
</style>
