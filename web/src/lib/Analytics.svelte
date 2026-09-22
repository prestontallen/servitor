<script>
  // Analytics: the ledger drawn as the dot lattice (one dot per event,
  // stacked by kind) and handoff latency as text. There is exactly one
  // chart library in the GUI — the dotlattice package.
  import { get } from './api.svelte.js';
  import { live } from './live.svelte.js';
  import DotLattice from './DotLattice.svelte';
  import { KIND_ORDER, KIND_COLOR } from './timeview.js';

  let days = $state(30);
  let buckets = $state([]);
  let error = $state(null);

  // handoff latency
  let handoffs = $state([]);
  let handoffError = $state(null);

  function fmtDur(secs) {
    if (secs == null) return '—';
    if (secs < 60) return `${Math.round(secs)}s`;
    if (secs < 3600) return `${Math.round(secs / 60)}m`;
    if (secs < 86400) return `${(secs / 3600).toFixed(1)}h`;
    return `${(secs / 86400).toFixed(1)}d`;
  }

  $effect(() => {
    // reload when the range changes or the ledger grows
    void live.changeCount;
    get(`/api/analytics?days=${days}`)
      .then((b) => {
        buckets = b;
        error = null;
      })
      .catch((e) => (error = e.message));
    get('/api/analytics/handoffs')
      .then((r) => {
        handoffs = r;
        handoffError = null;
      })
      .catch((e) => (handoffError = e.message));
  });

  // ---- the lattice ---------------------------------------------------------
  // daily {day, by_kind} buckets expanded into package events; kinds without
  // a dedicated hue (gates, status changes) draw in palette/ink colors.
  const events = $derived.by(() => {
    const out = [];
    for (const b of buckets) {
      const t = Date.parse(b.day);
      for (const [kind, n] of Object.entries(b.by_kind || {})) {
        for (let i = 0; i < n; i++) out.push({ t, group: kind, side: 'up' });
      }
    }
    return out;
  });
  const groups = KIND_ORDER.map((k) => ({ name: k, color: KIND_COLOR[k] }));
  const kindCounts = $derived.by(() => {
    const c = {};
    for (const e of events) c[e.group] = (c[e.group] || 0) + 1;
    return c;
  });
  const total = $derived(events.length);

  // ---- weekly handoff waits (text, not a chart) ----------------------------
  // weekly average of each wait, keyed on the week the wait ENDED
  // (human waits: week of presented; agent waits: week of approval).
  const weeklyWaits = $derived.by(() => {
    const wk = (ts) => {
      const d = new Date(ts);
      const day = (d.getUTCDay() + 6) % 7; // monday-start
      d.setUTCDate(d.getUTCDate() - day);
      return d.toISOString().slice(0, 10);
    };
    const acc = {};
    for (const r of handoffs) {
      if (r.human_wait_secs != null && r.presented_at) {
        const k = wk(r.presented_at);
        (acc[k] ??= { h: [], a: [] }).h.push(r.human_wait_secs);
      }
      if (r.agent_wait_secs != null && r.approved_at) {
        const k = wk(r.approved_at);
        (acc[k] ??= { h: [], a: [] }).a.push(r.agent_wait_secs);
      }
    }
    const weeks = Object.keys(acc).sort();
    const avg = (a) => (a.length ? a.reduce((x, y) => x + y, 0) / a.length : null);
    return weeks.map((w) => ({ week: w, human: avg(acc[w].h), agent: avg(acc[w].a) }));
  });
</script>

<section class="panel analytics">
  <div class="head">
    <h2>Ledger activity</h2>
    <div class="range">
      {#each [30, 90, 180] as d}
        <button class:active={days === d} onclick={() => (days = d)}>{d}d</button>
      {/each}
    </div>
  </div>
  {#if error}
    <p class="muted">unreachable: {error}</p>
  {:else if buckets.length === 0}
    <p class="muted">no activity in range</p>
  {:else}
    <DotLattice {events} {groups} autoAxis="day" maxRows={14} label="ledger events per day, stacked by kind" />
    <div class="chain">
      <span class="link">{total} event{total === 1 ? '' : 's'} over {days} days</span>
      {#each Object.keys(kindCounts) as k (k)}
        <span class="link muted"><i style:background={KIND_COLOR[k] ?? 'var(--text-dim)'}></i>{kindCounts[k]} {k}{kindCounts[k] === 1 ? '' : 's'}</span>
      {/each}
    </div>
  {/if}
</section>

<section class="panel analytics">
  <div class="head"><h2>Handoff latency</h2></div>
  {#if handoffError}
    <p class="muted">unreachable: {handoffError}</p>
  {:else if handoffs.length === 0}
    <p class="muted">no presented tickets yet</p>
  {:else}
    <div class="chain weeks">
      {#each weeklyWaits as w (w.week)}
        <span class="link">{w.week.slice(5)}<b>human {fmtDur(w.human)} · agent {fmtDur(w.agent)}</b></span>
      {/each}
    </div>
    <div class="rows" role="table" aria-label="handoff latency per ticket">
      {#each handoffs.slice(0, 30) as r (r.ulid)}
        <a class="row" href={`#/ticket/${r.ulid}`}>
          <span class="slug">{r.slug}</span>
          <span class="wait human">on human {fmtDur(r.human_wait_secs)}</span>
          <span class="wait agent">on agent {fmtDur(r.agent_wait_secs)}</span>
          <span class="when">{r.presented_at ? r.presented_at.slice(0, 10) : ''}</span>
        </a>
      {/each}
    </div>
  {/if}
</section>

<style>
  .analytics { max-width: 980px; margin: 0 auto; padding: 14px 18px; }
  .head { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 10px; }
  h2 { margin: 0; font-size: 15px; }
  .range { display: flex; gap: 4px; }
  .chain {
    display: flex; flex-wrap: wrap; gap: 4px 14px; margin-top: 6px;
    font-size: 11px; color: var(--text-dim);
  }
  .chain.weeks { margin: 0 0 10px; }
  .link { white-space: nowrap; }
  .link b { color: var(--text); font-weight: 500; margin-left: 4px; }
  .link i {
    display: inline-block; width: 7px; height: 7px; border-radius: 50%;
    margin-right: 5px; vertical-align: middle;
  }
  .rows { display: flex; flex-direction: column; }
  .row {
    display: grid;
    grid-template-columns: minmax(10rem, 1.4fr) 1fr 1fr auto;
    gap: 8px;
    align-items: baseline;
    min-height: 44px;
    padding: 6px 4px;
    border-bottom: 1px solid var(--line);
    text-decoration: none;
    color: var(--text);
    font-size: 13px;
  }
  .slug { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .wait.human { color: var(--warn); }
  .wait.agent { color: var(--accent); }
  .when { color: var(--text-dim); font-size: 11px; }
  @media (max-width: 640px) {
    .row { grid-template-columns: 1fr auto; }
    .wait.agent, .when { display: none; }
  }
</style>
