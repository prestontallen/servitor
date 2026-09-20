<script>
  // Time view: one window over the ledger, drawn as the dot lattice.
  // Cadence is the outer span (thirty days) with the window as a highlight
  // and a brush to move it; the window itself is the same lattice at a
  // finer bucket, so a narrow window is the day view; Flow is one lane
  // per ticket that moved in the window, phase as accent lightness and
  // blocked as a run of dots. Data: /api/timeline twice, outer and window.
  import { get } from './api.svelte.js';
  import { board, openTicket } from './state.svelte.js';
  import DotLattice from './DotLattice.svelte';
  import { fromActorBuckets } from './lattice.js';
  import { parseTimeHash, timeHash, flowLanes, cadenceDays, dayKey, daysIn, fmtDur } from './timeview.js';

  const OUTER_DAYS = 30;
  const DAY = 86400000, HOUR = 3600000;
  // phase as a lightness ramp of the accent, not four hues
  const PHASE_ALPHA = { queued: 0.15, shaping: 0.3, building: 0.55, checking: 0.8, shipping: 1 };
  const BAND_H = 10;

  let outer = $state({ since: 0, until: 0, buckets: [], error: null });
  let inner = $state({ segments: [], buckets: [], error: null });

  // window: the shared brush selection; the hash makes it linkable
  let from = $state(0);
  let to = $state(0);

  $effect(() => {
    const apply = () => {
      const w = parseTimeHash(location.hash);
      if (w.from !== from || w.to !== to) { from = w.from; to = w.to; }
    };
    apply();
    window.addEventListener('hashchange', apply);
    return () => window.removeEventListener('hashchange', apply);
  });

  // outer span: fixed lookback for cadence and the brush
  $effect(() => {
    get(`/api/timeline?days=${OUTER_DAYS}`)
      .then((d) => (outer = { since: Date.parse(d.since), until: Date.parse(d.until), buckets: d.buckets, error: null }))
      .catch((e) => (outer = { ...outer, error: e.message }));
  });

  // window: refetched whenever the brush moves
  $effect(() => {
    if (!to) return;
    const f = from, t = to;
    get(`/api/timeline?since=${new Date(f).toISOString()}&until=${new Date(t).toISOString()}`)
      .then((d) => {
        if (from !== f || to !== t) return; // the brush moved on; response is stale
        inner = { segments: d.segments, buckets: d.buckets, error: null };
      })
      .catch((e) => (inner = { ...inner, error: e.message }));
  });

  // ---- spans, buckets, axes ----------------------------------------------
  const outerSpan = $derived(outer.until ? { min: outer.since, max: outer.until } : null);
  const windowSpan = $derived(to ? { min: from, max: to } : null);
  const cadenceBuckets = (cols) => fromActorBuckets(outer.buckets, outerSpan.min, outerSpan.max, cols);
  const windowBuckets = (cols) => fromActorBuckets(inner.buckets, from, to, cols);

  const dayLabel = (ms) => new Date(ms).toLocaleDateString([], { month: 'short', day: 'numeric' }).toUpperCase();
  const hourLabel = (ms) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const midnight = (day) => { const [y, m, d] = day.split('-').map(Number); return new Date(y, m - 1, d).getTime(); };

  // cadence: every fifth day; window: hours when it is short, days otherwise
  const cadenceAxis = $derived(outerSpan
    ? daysIn(outerSpan.min, outerSpan.max).filter((_, i) => i % 5 === 0).map((d) => ({ t: midnight(d), label: dayLabel(midnight(d)) })).filter((a) => a.t >= outerSpan.min)
    : []);
  const windowAxis = $derived.by(() => {
    if (!windowSpan) return [];
    const len = to - from;
    if (len <= 2 * DAY) {
      const step = len <= 12 * HOUR ? HOUR : 6 * HOUR;
      const first = Math.ceil(from / step) * step;
      const out = [];
      for (let t = first; t < to; t += step) out.push({ t, label: hourLabel(t) });
      return out;
    }
    const every = len > 14 * DAY ? 5 : len > 7 * DAY ? 2 : 1;
    return daysIn(from, to).filter((_, i) => i % every === 0).map((d) => ({ t: midnight(d), label: dayLabel(midnight(d)) })).filter((a) => a.t >= from);
  });

  let cadenceQuantum = $state(1);
  let windowQuantum = $state(1);

  // ---- brush: a capture layer over the cadence lattice ---------------------
  let capEl = $state(null);
  let dragging = $state(false);
  let dragCur = $state(0);
  let anchor = $state(0);

  function brushT(e) {
    const rect = capEl.getBoundingClientRect();
    return outerSpan.min + ((e.clientX - rect.left) / rect.width) * (outerSpan.max - outerSpan.min);
  }
  function brushDown(e) {
    if (!outerSpan) return;
    dragging = true;
    dragCur = brushT(e);
    // the drag anchors at the window edge nearest the pointer
    anchor = Math.abs(dragCur - from) < Math.abs(dragCur - to) ? from : to;
    capEl.setPointerCapture(e.pointerId);
  }
  function brushMove(e) {
    if (dragging) dragCur = brushT(e);
  }
  function brushUp() {
    if (!dragging) return;
    dragging = false;
    const lo = Math.min(anchor, dragCur), hi = Math.max(anchor, dragCur);
    if (hi - lo < HOUR) pinDay(dayKey(dragCur)); // a click pins the clicked day
    else location.hash = timeHash(lo, hi);
  }
  const highlight = $derived(dragging ? { from: Math.min(anchor, dragCur), to: Math.max(anchor, dragCur) } : { from, to });

  function pinDay(day) {
    const start = midnight(day);
    location.hash = timeHash(start, start + DAY);
  }
  function stepDay(dir) {
    const start = midnight(selectedDay) + dir * DAY;
    location.hash = timeHash(start, start + DAY);
  }
  function preset(days) {
    const now = Date.now();
    location.hash = timeHash(now - days * DAY, now);
  }
  const selectedDay = $derived(dayKey(Math.max(from, to - 60000)));
  const windowIsDay = $derived(to - from <= DAY + 60000);

  // ---- chains --------------------------------------------------------------
  const cadenceTail = $derived(cadenceDays(outer.buckets).slice(-5));
  const windowTotal = $derived(inner.buckets.reduce((n, b) => n + Object.values(b.by_actor || {}).reduce((a, c) => a + c, 0), 0));
  const windowHuman = $derived(inner.buckets.reduce((n, b) => n + ((b.by_actor || {}).human || 0), 0));

  // ---- flow ------------------------------------------------------------------
  const lanes = $derived(flowLanes(inner.segments, board.cards, from, to));
  let laneW = $state(0);
  const RUN_STEP = 6;
  function laneDur(l) {
    return l.bars.map((b) => ({ phase: b.phase, dur: fmtDur((b.x1 - b.x0) * (to - from)) }));
  }
</script>

<section class="time">
  <header class="rowhead">
    <h2>Time</h2>
    <span class="muted window">{new Date(from).toLocaleDateString()} → {new Date(to).toLocaleDateString()} · {fmtDur(to - from)}</span>
    <span class="presets">
      <button class:active={windowIsDay} onclick={() => pinDay(dayKey(Date.now()))}>day</button>
      <button onclick={() => preset(7)}>week</button>
      <button onclick={() => preset(30)}>month</button>
    </span>
    {#if windowIsDay}
      <button class="step" onclick={() => stepDay(-1)} title="previous day">← day</button>
      <button class="step" onclick={() => stepDay(1)} title="next day">day →</button>
    {/if}
  </header>

  {#if outer.error}
    <p class="muted">timeline unreachable: {outer.error}</p>
  {:else}
    <!-- ================= cadence: thirty days, the window as a highlight, drag to move it ================= -->
    <div class="panel cad" data-testid="cadence">
      <h3>cadence · {OUTER_DAYS} days · drag to choose the window, click a day to pin it</h3>
      {#if outerSpan}
        <div class="brushwrap">
          <DotLattice span={outerSpan} bucketsFor={cadenceBuckets} axis={cadenceAxis} {highlight} maxRows={16} bind:quantum={cadenceQuantum} label="ledger events per column over thirty days, human above the line, agents below" />
          <div class="cap" bind:this={capEl} onpointerdown={brushDown} onpointermove={brushMove} onpointerup={brushUp} onpointercancel={brushUp} data-testid="cadence-brush"></div>
        </div>
        <div class="chain">
          {#each cadenceTail as d (d.day)}
            <span class="link">{d.day.slice(5)} <b>{d.total}</b></span>
          {/each}
          <span class="link muted key">human above · agent below{#if cadenceQuantum > 1} · one dot is {cadenceQuantum} events{/if}</span>
        </div>
      {/if}
    </div>

    <!-- ================= the window: same lattice, finer bucket ================= -->
    <div class="panel win" data-testid="window">
      <h3>{windowIsDay ? `day · ${selectedDay}` : `window · ${fmtDur(to - from)}`}</h3>
      {#if inner.error}
        <p class="muted">unreachable: {inner.error}</p>
      {:else if windowSpan}
        <DotLattice span={windowSpan} bucketsFor={windowBuckets} axis={windowAxis} maxRows={14} bind:quantum={windowQuantum} label="ledger events per column in the window, human above the line, agents below" />
        <div class="chain">
          <span class="link">{windowTotal} event{windowTotal === 1 ? '' : 's'}<b>{windowHuman} by the human</b></span>
          <span class="link muted key">human above · agent below{#if windowQuantum > 1} · one dot is {windowQuantum} events{/if}</span>
        </div>
      {/if}
    </div>

    <!-- ================= flow: one lane per ticket, phase as lightness, blocked as dots ================= -->
    <div class="panel flow" data-testid="flow">
      <h3>flow · {lanes.length} ticket{lanes.length === 1 ? '' : 's'} in the window</h3>
      {#each lanes as l (l.ulid)}
        <div class="lane">
          <button class="slug" onclick={() => openTicket(l.ulid)} title={l.title}>{l.slug}</button>
          <div class="bandwrap" bind:clientWidth={laneW}>
            {#if laneW > 0}
              <svg width={laneW} height={BAND_H}>
                <rect x="0" y="0" width={laneW} height={BAND_H} class="band-bg" />
                {#each l.bars as b}
                  {@const x0 = b.x0 * laneW}
                  {@const w = Math.max(b.x1 * laneW - x0, 1)}
                  {#if b.phase === 'blocked'}
                    <g class="blocked">
                      {#each { length: Math.max(1, Math.floor(w / RUN_STEP)) } as _, i}
                        <circle cx={x0 + i * RUN_STEP + RUN_STEP / 2} cy={BAND_H / 2} r="2" />
                      {/each}
                      <title>blocked: {fmtDur((b.x1 - b.x0) * (to - from))}</title>
                    </g>
                  {:else}
                    <rect x={x0} y="0" width={w} height={BAND_H} class="seg" style:opacity={PHASE_ALPHA[b.phase] ?? 0.3} onclick={() => openTicket(l.ulid)}>
                      <title>{b.phase}: {fmtDur((b.x1 - b.x0) * (to - from))}</title>
                    </rect>
                  {/if}
                {/each}
              </svg>
            {/if}
          </div>
        </div>
        <div class="chain lanechain">
          {#each laneDur(l) as b}
            <span class="link" class:blocked={b.phase === 'blocked'}><i style:opacity={b.phase === 'blocked' ? 1 : PHASE_ALPHA[b.phase] ?? 0.3}></i>{b.phase} <b>{b.dur}</b></span>
          {/each}
        </div>
      {:else}
        <p class="muted empty">no tickets moved in this window</p>
      {/each}
    </div>
  {/if}
</section>

<style>
  .time { max-width: 1240px; margin: 0 auto; display: flex; flex-direction: column; gap: 14px; }
  h2 { font-size: 14px; margin: 0; }
  h3 {
    font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em;
    color: var(--text-dim); margin: 0 0 8px; font-weight: 500;
  }
  .rowhead { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .presets { display: inline-flex; gap: 4px; }
  .presets button, .step { font-size: 11px; padding: 2px 8px; min-height: 0; }
  .panel { padding: 10px 14px; }
  .brushwrap { position: relative; }
  .cap { position: absolute; inset: 0; cursor: crosshair; touch-action: none; }
  .band-bg { fill: var(--bg-inset); }
  .seg { fill: var(--accent); cursor: pointer; }
  .blocked circle { fill: var(--fail); opacity: 0.85; }
  .lane { display: flex; gap: 10px; align-items: center; margin-top: 8px; }
  .slug {
    width: 110px; flex: none; text-align: left; font-size: 11px; border: none; background: none;
    color: var(--accent); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 0; min-height: 0;
  }
  .bandwrap { flex: 1; }
  .lanechain { margin: 2px 0 0 120px; }
  .chain {
    display: flex; flex-wrap: wrap; gap: 4px 14px; margin-top: 6px;
    font-size: 11px; color: var(--text-dim);
  }
  .link { white-space: nowrap; }
  .link b { color: var(--text); font-weight: 500; margin-left: 4px; }
  .link i {
    display: inline-block; width: 8px; height: 8px; border-radius: 2px;
    margin-right: 5px; vertical-align: middle; background: var(--accent);
  }
  .link.blocked { color: var(--fail); }
  .link.blocked i { background: var(--fail); height: 3px; border-radius: 1px; }
  .key { font-size: 10px; }
  .empty { margin: 4px 0; }
  @media (max-width: 700px) {
    .slug { width: 80px; font-size: 10px; }
    .lanechain { margin-left: 90px; }
  }
</style>
