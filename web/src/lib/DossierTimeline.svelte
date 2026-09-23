<script>
  // Per-ticket timeline: the dot lattice from created to now (or the
  // terminal milestone), milestones as dashed lines, blocked spans as
  // runs on the baseline, and the numbers in a text chain underneath so
  // they survive any width. The drawing itself is DotLattice.
  import { fmtTs } from './state.svelte.js';
  import DotLattice from './DotLattice.svelte';
  import { KIND_ORDER, KIND_COLOR } from './timeview.js';

  let { doc, history = [] } = $props();

  let now = $state(Date.now());
  $effect(() => {
    const id = setInterval(() => (now = Date.now()), 30000);
    return () => clearInterval(id);
  });

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

  // next gate for an in-flight ticket, from the gates already passed
  const nextGate = $derived.by(() => {
    if (terminal) return null;
    const have = new Set(milestones.map((m) => m.key));
    return ['contract', 'presented', 'shipped'].find((g) => !have.has(g)) || null;
  });

  // ---- what the lattice draws --------------------------------------------
  let quantum = $state(1);

  // milestone hue: the human's gates and now in the accent, done in ok, the rest ink
  function tickColor(key, who) {
    if (key === 'now' || who?.startsWith('human:')) return 'var(--accent)';
    if (key === 'done') return 'var(--ok)';
    if (key === 'dropped') return 'var(--fail)';
    return 'var(--text-dim)';
  }
  // the human's approval is the one signature worth showing on the drawing
  const sigOf = (m) => (m.key === 'contract' && m.who?.startsWith('human:') ? ' · ' + m.who.replace(/^\w+:/, '') : '');

  const ticks = $derived.by(() => {
    const out = milestones.map((m) => ({
      key: m.key, t: m.t, label: m.key.toUpperCase(), sig: sigOf(m), color: tickColor(m.key, m.who),
      title: `${m.key}${m.who ? ' by ' + m.who : ''}: ${fmtTs(new Date(m.t).toISOString())}`
    }));
    if (!terminal && span) out.push({ key: 'now', t: span.max, label: 'NOW', color: tickColor('now'), title: `now: ${fmtTs(new Date(now).toISOString())}` });
    return out;
  });
  const runs = $derived(blocked.map((b) => ({
    start: b.start, end: b.end,
    title: `blocked on ${b.on}: ${fmtDur(b.end - b.start)}, from ${fmtTs(new Date(b.start).toISOString())}`
  })));
  // the package consumes events {t, group, side}; signals carry kind + actor
  const events = $derived(signals.map((e) => ({
    t: t(e.ts),
    group: e.kind,
    side: e.actor_type === 'human' ? 'human' : 'agent',
  })));
  const LATTICE = {
    groups: KIND_ORDER.map((k) => ({ name: k, color: KIND_COLOR[k] })),
    sides: { human: 'up', agent: 'down' },
    sideOpacity: { human: 1, agent: 0.55 },
  };
</script>

{#if span}
  <div class="dtl" data-testid="dossier-timeline">
    <DotLattice {span} {events} {...LATTICE} {ticks} {runs} bind:quantum label="ticket timeline: signals per column, human above the line, agents below" />

    <!-- the numbers, in text, whatever the width -->
    <div class="chain">
      {#each segments as s, i}
        <span class="link" title="{s.from} → {s.to}">
          {#if s.word}{s.word}{:else}{s.from} → {s.to}{/if}
          <b>{fmtDur(s.end - s.start)}{#if !terminal && i === segments.length - 1} so far{/if}</b>
        </span>
      {/each}
      {#if nextGate}<span class="link next">next gate <b>{nextGate}</b></span>{/if}
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
  .dtl { margin: 8px 0 8px; }
  .link.next { color: var(--accent); }
  .link.next b { color: var(--accent); }
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
