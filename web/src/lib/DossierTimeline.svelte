<script>
  // Per-ticket timeline (VIZ 1). One SVG drawn in measured pixels, so
  // nothing overlaps or clamps: a phase band coloured by card word,
  // blocked spans hatched over it, signal events as a density strip above,
  // milestone ticks below with labels that step down a row when they would
  // collide. The chain under the drawing prints every duration in text, so
  // the numbers survive any width.
  import { fmtTs } from './state.svelte.js';

  let { doc, history = [] } = $props();

  let width = $state(0);
  let now = $state(Date.now());
  $effect(() => {
    const id = setInterval(() => (now = Date.now()), 30000);
    return () => clearInterval(id);
  });

  const WORD_COLOR = {
    shaping: 'var(--text-dim)',
    building: 'var(--accent)',
    checking: 'var(--warn)',
    shipping: 'var(--ok)'
  };
  // card word that holds after each milestone (derived from the latest gate)
  const WORD_AFTER = { created: 'shaping', contract: 'building', presented: 'checking', shipped: 'shipping' };
  const TERMINAL = new Set(['done', 'dropped']);
  const SIGNAL_KINDS = new Set(['note', 'decision', 'feedback']);

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

  // phase segments between consecutive ticks; the open one runs to now
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

  // ---- layout (px) ------------------------------------------------------
  const STRIP_Y = 0, STRIP_H = 10;        // signal density strip
  const BAND_Y = 14, BAND_H = 12;         // phase band
  const TICK_Y0 = BAND_Y - 3;             // tick line starts just above the band
  const LABEL_Y = 40, ROW_H = 12;         // first label baseline, row step
  const LABEL_PX = 6.1;                   // ~px per uppercase 10px char
  const GAP = 8;

  // tick labels: greedy left-to-right, drop a row on collision
  const ticks = $derived.by(() => {
    if (!span || !width) return [];
    const items = milestones.map((m) => ({ ...m, label: m.key, cx: x(m.t) }));
    if (!terminal) items.push({ key: 'now', label: 'now', t: now, cx: width });
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

  const rows = $derived(ticks.reduce((n, tk) => Math.max(n, tk.row + 1), 1));
  const height = $derived(LABEL_Y + (rows - 1) * ROW_H + 4);

  function tickColor(key) {
    if (key === 'now') return 'var(--accent)';
    if (key === 'done') return 'var(--ok)';
    if (key === 'dropped') return 'var(--fail)';
    return 'var(--text-dim)';
  }

  function segLabel(s) {
    const px = x(s.end) - x(s.start);
    const label = fmtDur(s.end - s.start);
    return px > label.length * LABEL_PX + 10 ? label : '';
  }
</script>

{#if span}
  <div class="dtl" bind:clientWidth={width} data-testid="dossier-timeline">
    {#if width > 0}
      <svg {width} {height} aria-label="ticket timeline">
        <defs>
          <pattern id="dtl-hatch" patternUnits="userSpaceOnUse" width="6" height="6" patternTransform="rotate(45)">
            <rect width="2.5" height="6" fill="var(--fail)" />
          </pattern>
        </defs>

        <!-- signal density: each event a translucent bar; bursts stack darker -->
        {#each signals as e (e.id)}
          <rect class="sig" class:human={e.actor_type === 'human'}
            x={x(t(e.ts)) - 1} y={STRIP_Y} width="2" height={STRIP_H} />
        {/each}

        <!-- phase band -->
        <rect x="0" y={BAND_Y} {width} height={BAND_H} class="band-bg" />
        {#each segments as s}
          {#if s.word}
            <rect x={x(s.start)} y={BAND_Y} width={Math.max(x(s.end) - x(s.start), 1)} height={BAND_H}
              fill={WORD_COLOR[s.word]} class="seg">
              <title>{s.word}: {s.from} → {s.to}, {fmtDur(s.end - s.start)}</title>
            </rect>
            {#if segLabel(s)}
              <text class="dur" x={(x(s.start) + x(s.end)) / 2} y={BAND_Y + BAND_H - 3} text-anchor="middle">{segLabel(s)}</text>
            {/if}
          {/if}
        {/each}
        {#each blocked as b}
          <rect x={x(b.start)} y={BAND_Y} width={Math.max(x(b.end) - x(b.start), 2)} height={BAND_H} fill="url(#dtl-hatch)">
            <title>blocked on {b.on}: {fmtDur(b.end - b.start)}, from {fmtTs(new Date(b.start).toISOString())}</title>
          </rect>
        {/each}

        <!-- milestone ticks -->
        {#each ticks as tk (tk.key)}
          <g class="tick" style:color={tickColor(tk.key)}>
            <line x1={tk.cx} x2={tk.cx} y1={TICK_Y0} y2={LABEL_Y + tk.row * ROW_H - 9} />
            <text x={tk.tx} y={LABEL_Y + tk.row * ROW_H} text-anchor={tk.anchor}>{tk.label.toUpperCase()}</text>
            <title>{tk.label}{tk.who ? ' by ' + tk.who : ''}: {fmtTs(new Date(tk.t).toISOString())}</title>
          </g>
        {/each}
      </svg>
    {/if}

    <!-- the numbers, in text, whatever the width -->
    <div class="chain">
      {#each segments as s, i}
        <span class="link" title="{s.from} → {s.to}">
          {#if s.word}<i style:background={WORD_COLOR[s.word]}></i>{s.word}{:else}{s.from} → {s.to}{/if}
          <b>{fmtDur(s.end - s.start)}</b>
        </span>
      {/each}
      {#each blocked as b}
        <span class="link blocked"><i></i>blocked on {b.on} <b>{fmtDur(b.end - b.start)}</b></span>
      {/each}
      {#if signals.length}
        <span class="link muted">{Object.entries(counts).map(([k, n]) => `${n} ${k}${n === 1 ? '' : 's'}`).join(', ')}</span>
      {/if}
    </div>
  </div>
{/if}

<style>
  .dtl { margin: 6px 0 8px; }
  svg { display: block; overflow: visible; }
  .band-bg { fill: var(--bg-inset); }
  .seg { opacity: 0.8; }
  .sig { fill: var(--text-dim); opacity: 0.45; }
  .sig.human { fill: var(--accent); opacity: 0.9; }
  .dur {
    font-size: 9px; fill: var(--bg); font-weight: 600;
    pointer-events: none;
  }
  .tick line { stroke: currentColor; stroke-width: 1.5; }
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
    display: inline-block; width: 8px; height: 8px; border-radius: 2px;
    margin-right: 5px; vertical-align: middle;
  }
  .link.blocked { color: var(--fail); }
  .link.blocked i {
    background: repeating-linear-gradient(45deg, var(--fail), var(--fail) 2px, transparent 2px, transparent 4px);
  }
</style>
