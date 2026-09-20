<script>
  // Per-ticket timeline as a dot lattice. One SVG in measured pixels: a
  // bare baseline from created to now (or the terminal milestone); every
  // signal event a dot snapped to a fixed grid, the human's above the
  // line and the agents' below, hue by kind, stacked baseline outward.
  // Milestones are thin dashed annotation lines through the lattice,
  // blocked spans a run of red dots on the baseline. Labels step down a
  // row when they would collide.
  // The chain under the drawing prints every duration in text, so the
  // numbers survive any width.
  import { fmtTs } from './state.svelte.js';
  import { bucketize, quantumFor, stack, rowsFor, KIND_ORDER } from './lattice.js';

  let { doc, history = [] } = $props();

  let width = $state(0);
  let now = $state(Date.now());
  $effect(() => {
    const id = setInterval(() => (now = Date.now()), 30000);
    return () => clearInterval(id);
  });

  // card word that holds after each milestone (derived from the latest gate)
  const WORD_AFTER = { created: 'shaping', contract: 'building', presented: 'checking', shipped: 'shipping' };
  const TERMINAL = new Set(['done', 'dropped']);
  const SIGNAL_KINDS = new Set(KIND_ORDER);
  const KIND_COLOR = { decision: 'var(--k-decision)', feedback: 'var(--k-feedback)', note: 'var(--text)' };

  const t = (iso) => new Date(iso).getTime();

  function fmtDur(ms) {
    if (ms < 60000) return '<1m';
    const m = Math.round(ms / 60000);
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    if (h < 48) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
    const d = Math.floor(h / 24);
    return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
  }

  const sorted = $derived([...history].sort((a, b) => a.id - b.id));

  // milestones in time order: created, each gate, then done/dropped
  const milestones = $derived.by(() => {
    if (!sorted.length) return [];
    const create = sorted.find((e) => e.kind === 'ticket.create') || sorted[0];
    const ms = [{ key: 'created', t: t(create.ts) }];
    for (const g of doc?.gates || []) ms.push({ key: g.gate.replace('_approved', ''), t: t(g.ts), who: g.actor });
    const end = sorted.filter((e) => e.kind === 'status.set' && TERMINAL.has(e.payload?.status)).pop();
    if (end) ms.push({ key: end.payload.status, t: t(end.ts), who: end.actor });
    return ms.sort((a, b) => a.t - b.t);
  });

  const terminal = $derived(milestones.find((m) => TERMINAL.has(m.key)));

  // the axis: created .. (last milestone | now)
  const span = $derived.by(() => {
    if (!milestones.length) return null;
    const min = milestones[0].t;
    let max = milestones[milestones.length - 1].t;
    if (!terminal) max = Math.max(max, now);
    if (max <= min) max = min + 1;
    return { min, max };
  });

  const x = (ms) => ((Math.min(Math.max(ms, span.min), span.max) - span.min) / (span.max - span.min)) * width;

  // phase segments between consecutive milestones, for the text chain
  const segments = $derived.by(() => {
    if (!span) return [];
    const out = [];
    let word = 'shaping';
    for (let i = 0; i < milestones.length; i++) {
      const m = milestones[i];
      word = TERMINAL.has(m.key) ? null : WORD_AFTER[m.key] || word;
      const next = milestones[i + 1];
      const end = next ? next.t : terminal ? null : now;
      if (end === null) break;
      out.push({ from: m.key, to: next ? next.key : 'now', start: m.t, end, word });
    }
    return out;
  });

  // blocked: status.set blocked -> next status.set (or now)
  const blocked = $derived.by(() => {
    const out = [];
    let open = null;
    for (const e of sorted) {
      if (e.kind !== 'status.set') continue;
      const st = e.payload?.status;
      if (st === 'blocked' && !open) open = { start: t(e.ts), on: e.payload?.on || '?' };
      else if (open && st !== 'blocked') { out.push({ ...open, end: t(e.ts) }); open = null; }
    }
    if (open) out.push({ ...open, end: now });
    return out;
  });

  const signals = $derived(sorted.filter((e) => SIGNAL_KINDS.has(e.kind)));

  const counts = $derived.by(() => {
    const c = {};
    for (const e of signals) c[e.kind] = (c[e.kind] || 0) + 1;
    return c;
  });

  // ---- lattice --------------------------------------------------------
  const CELL = 6, R = 2;                  // grid pitch and dot radius (px)
  const MAX_ROWS = 8;                     // per side; beyond this a dot is a quantum
  const cols = $derived(Math.max(1, Math.floor(width / CELL)));
  const buckets = $derived(span && width ? bucketize(signals, span.min, span.max, cols) : []);
  const quantum = $derived(quantumFor(buckets, MAX_ROWS));
  const rows = $derived.by(() => {
    const r = rowsFor(buckets, quantum);
    // keep a row on each side so the baseline never sits on an edge
    return { human: Math.max(1, r.human), agent: Math.max(1, r.agent) };
  });
  const bucketMs = $derived(span ? (span.max - span.min) / cols : 0);

  function columnTitle(b) {
    const n = b.human + b.agent;
    if (!n) return '';
    const parts = KIND_ORDER.filter((k) => b.counts[k]).map((k) => {
      const c = b.counts[k].human + b.counts[k].agent;
      return `${c} ${k}${c === 1 ? '' : 's'}`;
    });
    const from = fmtTs(new Date(b.start).toISOString());
    return `${from} +${fmtDur(bucketMs)}\n${parts.join(', ')}${b.human ? ` · ${b.human} by the human` : ''}`;
  }

  // ---- layout (px) ------------------------------------------------------
  const TOP = 2;
  const y0 = $derived(TOP + rows.human * CELL);                 // baseline
  const lowY = $derived(y0 + rows.agent * CELL);                // bottom of agent dots
  const LABEL_PX = 6.1;                   // ~px per uppercase 10px char
  const ROW_H = 12;
  const GAP = 8;
  const labelY = $derived(lowY + 14);     // first label baseline

  // tick labels: greedy left-to-right, drop a row on collision
  const ticks = $derived.by(() => {
    if (!span || !width) return [];
    const snap = (px) => Math.min(cols - 1, Math.floor(px / CELL)) * CELL + CELL / 2;
    const items = milestones.map((m) => ({ ...m, label: m.key, cx: snap(x(m.t)) }));
    if (!terminal) items.push({ key: 'now', label: 'now', t: now, cx: snap(width) });
    const rowsRight = [];
    return items.map((it) => {
      const w = it.label.length * LABEL_PX;
      let left = it.cx - w / 2, anchor = 'middle';
      if (left < 0) { left = 0; anchor = 'start'; }
      else if (left + w > width) { left = width - w; anchor = 'end'; }
      let row = 0;
      while (row < rowsRight.length && left < rowsRight[row] + GAP) row++;
      rowsRight[row] = left + w;
      const tx = anchor === 'middle' ? it.cx : anchor === 'start' ? 0 : width;
      return { ...it, row, tx, anchor };
    });
  });

  const labelRows = $derived(ticks.reduce((n, tk) => Math.max(n, tk.row + 1), 1));
  const height = $derived(labelY + (labelRows - 1) * ROW_H + 4);

  // milestone hue: the human's gates and now in the accent, done in ok, the rest ink
  function tickColor(key, who) {
    if (key === 'now' || who?.startsWith('human:')) return 'var(--accent)';
    if (key === 'done') return 'var(--ok)';
    if (key === 'dropped') return 'var(--fail)';
    return 'var(--text-dim)';
  }
</script>

{#if span}
  <div class="dtl" bind:clientWidth={width} data-testid="dossier-timeline">
    {#if width > 0}
      <svg {width} {height} aria-label="ticket timeline: signals per column, human above the line, agents below">
        <!-- milestones: dashed annotation lines through the lattice, under the signal dots -->
        {#each ticks as tk (tk.key)}
          <g class="tick" style:color={tickColor(tk.key, tk.who)}>
            <line x1={tk.cx} x2={tk.cx} y1={TOP} y2={labelY + tk.row * ROW_H - 9} />
            <text x={tk.tx} y={labelY + tk.row * ROW_H} text-anchor={tk.anchor}>{tk.label.toUpperCase()}</text>
            <title>{tk.label}{tk.who ? ' by ' + tk.who : ''}: {fmtTs(new Date(tk.t).toISOString())}</title>
          </g>
        {/each}

        <!-- baseline, with blocked spans as a run of dots on it -->
        <line class="base" x1="0" x2={width} y1={y0} y2={y0} />
        {#each blocked as b}
          {@const c0 = Math.min(cols - 1, Math.floor(x(b.start) / CELL))}
          {@const c1 = Math.min(cols - 1, Math.floor(x(b.end) / CELL))}
          <g class="blocked">
            {#each { length: c1 - c0 + 1 } as _, i}
              <circle cx={(c0 + i) * CELL + CELL / 2} cy={y0} r={R} />
            {/each}
            <title>blocked on {b.on}: {fmtDur(b.end - b.start)}, from {fmtTs(new Date(b.start).toISOString())}</title>
          </g>
        {/each}

        <!-- the lattice: one dot per signal (or per quantum), human up, agent down -->
        {#each buckets as b (b.i)}
          {#if b.human + b.agent}
            {@const cx = b.i * CELL + CELL / 2}
            <g class="col">
              {#each stack(b, 'human', quantum) as d, r}
                <circle cx={cx} cy={y0 - 1 - (r * CELL + CELL / 2)} r={R} fill={KIND_COLOR[d.kind]} class="dot human" />
              {/each}
              {#each stack(b, 'agent', quantum) as d, r}
                <circle cx={cx} cy={y0 + 1 + (r * CELL + CELL / 2)} r={R} fill={KIND_COLOR[d.kind]} class="dot" />
              {/each}
              <rect class="hit" x={b.i * CELL} y={TOP} width={CELL} height={lowY - TOP}>
                <title>{columnTitle(b)}</title>
              </rect>
            </g>
          {/if}
        {/each}
      </svg>
    {/if}

    <!-- the numbers, in text, whatever the width -->
    <div class="chain">
      {#each segments as s}
        <span class="link" title="{s.from} → {s.to}">
          {#if s.word}{s.word}{:else}{s.from} → {s.to}{/if}
          <b>{fmtDur(s.end - s.start)}</b>
        </span>
      {/each}
      {#each blocked as b}
        <span class="link blocked"><i></i>blocked on {b.on} <b>{fmtDur(b.end - b.start)}</b></span>
      {/each}
      {#if signals.length}
        {#each KIND_ORDER.filter((k) => counts[k]) as k (k)}
          <span class="link muted"><i style:background={KIND_COLOR[k]}></i>{counts[k]} {k}{counts[k] === 1 ? '' : 's'}</span>
        {/each}
        <span class="link muted key">human above · agent below{#if quantum > 1} · one dot is {quantum} events{/if}</span>
      {/if}
    </div>
  </div>
{/if}

<style>
  .dtl { margin: 6px 0 8px; }
  svg { display: block; overflow: visible; }
  .base { stroke: var(--line-strong); stroke-width: 1; }
  .blocked circle { fill: var(--fail); opacity: 0.85; }
  .dot { opacity: 0.55; }
  .dot.human { opacity: 1; }
  .hit { fill: transparent; }
  .col:hover .hit { fill: var(--text); fill-opacity: 0.07; }
  .tick line { stroke: currentColor; stroke-width: 1; stroke-dasharray: 2 3; opacity: 0.8; }
  .tick text {
    fill: currentColor; font-size: 9px; letter-spacing: 0.06em;
  }
  .chain {
    display: flex; flex-wrap: wrap; gap: 4px 14px; margin-top: 6px;
    font-size: 11px; color: var(--text-dim);
  }
  .link { white-space: nowrap; }
  .link b { color: var(--text); font-weight: 500; margin-left: 4px; }
  .link i {
    display: inline-block; width: 7px; height: 7px; border-radius: 50%;
    margin-right: 5px; vertical-align: middle;
  }
  .link.blocked { color: var(--fail); }
  .link.blocked i { background: var(--fail); border-radius: 1px; height: 3px; }
  .key { font-size: 10px; }
</style>
