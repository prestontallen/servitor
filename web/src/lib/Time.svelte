<script>
  // Time view (VIZ, house style per DossierTimeline): one shared window
  // drives Flow (phase bars per ticket), Day (hourly actor columns) and
  // Cadence (per-day rhythm). Hand-rolled SVG in measured pixels; the
  // numbers print in text chains under each drawing. Data: /api/timeline
  // twice — an outer span for the brush and the selection for the views.
  import { get } from './api.svelte.js';
  import { board, openTicket } from './state.svelte.js';
  import { parseTimeHash, timeHash, flowLanes, cadenceDays, dayKey, dayColumns, fmtDur } from './timeview.js';

  const WORD_COLOR = {
    shaping: 'var(--text-dim)',
    building: 'var(--accent)',
    checking: 'var(--warn)',
    shipping: 'var(--ok)'
  };
  const ACTOR_COLOR = { human: 'var(--accent)', agent: 'var(--text-dim)', system: 'var(--line-strong)' };

  const OUTER_DAYS = 30;
  const BAND_H = 12;
  const ROW_GAP = 6;

  let outer = $state({ buckets: [], days: [], error: null });
  let inner = $state({ segments: [], buckets: [], error: null });

  // window: the shared brush selection; the hash makes it linkable
  let from = $state(0);
  let to = $state(0);

  $effect(() => {
    const apply = () => {
      const w = parseTimeHash(location.hash);
      if (w.from !== from || w.to !== to) {
        from = w.from;
        to = w.to;
      }
    };
    apply();
    window.addEventListener('hashchange', apply);
    return () => window.removeEventListener('hashchange', apply);
  });

  // outer span: fixed lookback for the brush and Cadence
  $effect(() => {
    get(`/api/timeline?days=${OUTER_DAYS}`)
      .then((d) => (outer = { buckets: d.buckets, days: cadenceDays(d.buckets), error: null }))
      .catch((e) => (outer = { ...outer, error: e.message }));
  });

  // selection: refetched whenever the brush moves
  $effect(() => {
    if (!to) return;
    const f = from, t = to;
    get(`/api/timeline?from=${new Date(f).toISOString()}&until=${new Date(t).toISOString()}`)
      .then((d) => {
        if (from !== f || to !== t) return; // the brush moved on; response is stale
        inner = { segments: d.segments, buckets: d.buckets, error: null };
      })
      .catch((e) => (inner = { ...inner, error: e.message }));
  });

  // ---- brush (pointer events) ---------------------------------------------
  let width = $state(0);
  let dragging = $state(false);
  let dragCur = $state(0);
  let anchor = $state(0);

  const span = $derived.by(() => {
    const now = Date.now();
    const start = now - OUTER_DAYS * 86400000;
    return { start, end: now, total: now - start };
  });

  const xOuter = (ms) => ((Math.min(Math.max(ms, span.start), span.end) - span.start) / span.total) * width;

  function brushDown(e) {
    dragging = true;
    dragCur = brushX(e);
    // the drag anchors at the window edge nearest the pointer
    anchor = Math.abs(dragCur - from) < Math.abs(dragCur - to) ? from : to;
    e.currentTarget.setPointerCapture(e.pointerId);
  }
  function brushX(e) {
    const rect = e.currentTarget.closest('svg').getBoundingClientRect();
    return span.start + ((e.clientX - rect.left) / rect.width) * span.total;
  }
  function brushMove(e) {
    if (dragging) dragCur = brushX(e);
  }
  function brushUp() {
    if (!dragging) return;
    dragging = false;
    const lo = Math.min(anchor, dragCur);
    const hi = Math.max(anchor, dragCur);
    if (hi - lo < 3600000) {
      pinDay(dayKey(dragCur)); // a click pins the clicked day
    } else {
      location.hash = timeHash(lo, hi);
    }
  }
  function pinDay(day) {
    const [y, m, d] = day.split('-').map(Number);
    const start = new Date(y, m - 1, d).getTime();
    location.hash = timeHash(start, start + 86400000);
  }
  function stepDay(dir) {
    const [y, m, d] = selectedDay.split('-').map(Number);
    const start = new Date(y, m - 1, d + dir).getTime();
    location.hash = timeHash(start, start + 86400000);
  }

  // ---- views ---------------------------------------------------------------
  const lanes = $derived(flowLanes(inner.segments, board.cards, from, to));
  const selectedDay = $derived(dayKey(Math.max(from, to - 60000)));
  const cols = $derived(dayColumns(inner.buckets, selectedDay));
  const dayMax = $derived(Math.max(1, ...cols.map((c) => c.events)));
  const cadMax = $derived(Math.max(1, ...outer.days.map((d) => d.total)));

  // stacked segments per hour column (agent under human), y measured bottom-up
  const dayStacks = $derived(
    cols.map((c) => {
      let y = 44;
      const segs = [];
      for (const [actor, n] of Object.entries(c.byActor)) {
        const h = Math.max(1, (n / dayMax) * 40);
        y -= h;
        segs.push({ y, h, actor, n });
      }
      return { hour: c.hour, events: c.events, segs };
    })
  );

  function laneDur(l) {
    return l.bars.map((b) => ({ phase: b.phase, dur: fmtDur((b.x1 - b.x0) * (to - from)) }));
  }
</script>

<section class="time">
  <header class="rowhead">
    <h2>Time</h2>
    <span class="muted window">{new Date(from).toLocaleDateString()} → {new Date(to).toLocaleDateString()} · {fmtDur(to - from)}</span>
    <button class="step" onclick={() => stepDay(-1)} title="previous day">← day</button>
    <button class="step" onclick={() => stepDay(1)} title="next day">day →</button>
  </header>

  {#if outer.error}
    <p class="muted">timeline unreachable: {outer.error}</p>
  {:else}
    <!-- ================= Cadence + brush: per-day bars, drag to select ================= -->
    <div class="panel cad" data-testid="cadence">
      <h3>cadence · drag to choose the window, click a day to pin it</h3>
      <div class="strip" bind:clientWidth={width}>
        {#if width > 0}
          <svg width={width} height="30" data-testid="cadence-strip">
            {#each outer.days as d (d.day)}
              {@const x = xOuter(new Date(d.day + 'T12:00:00').getTime())}
              {@const h = Math.max(1, (d.total / cadMax) * 18)}
              <rect class="cbar" class:inside={x >= xOuter(from) && x <= xOuter(to)}
                x={x - 3} y={22 - h} width="6" height={h}>
                <title>{d.day}: {d.total} events</title>
              </rect>
            {/each}
            <line class="now" x1={width - 0.5} x2={width - 0.5} y1="0" y2="24" />
            {#if dragging}
              <rect class="sel" x={Math.min(xOuter(anchor), xOuter(dragCur))} y="0"
                width={Math.max(Math.abs(xOuter(dragCur) - xOuter(anchor)), 2)} height="24" />
            {:else}
              <rect class="sel" x={xOuter(from)} y="0" width={Math.max(xOuter(to) - xOuter(from), 2)} height="24" />
            {/if}
            <rect class="cap" x="0" y="0" {width} height="24"
              onpointerdown={brushDown} onpointermove={brushMove} onpointerup={brushUp} />
          </svg>
        {/if}
        <div class="chain">
          {#each outer.days.slice(-5) as d (d.day)}
            <span class="link">{d.day.slice(5)} <b>{d.total}</b></span>
          {/each}
        </div>
      </div>
    </div>

    <!-- ================= Flow: phase bars per ticket ================= -->
    <div class="panel flow" data-testid="flow">
      <h3>flow · {lanes.length} ticket{lanes.length === 1 ? '' : 's'} in the window</h3>
      {#each lanes as l (l.ulid)}
        <div class="lane">
          <button class="slug" onclick={() => openTicket(l.ulid)}>{l.slug}</button>
          <div class="bandwrap">
            <svg width="100%" height={BAND_H} viewBox="0 0 100 {BAND_H}" preserveAspectRatio="none">
              <rect x="0" y="0" width="100" height={BAND_H} class="band-bg" vector-effect="non-scaling-stroke" />
              {#each l.bars as b}
                {@const x0 = b.x0 * 100}
                {@const w = Math.max(b.x1 * 100 - x0, 0.4)}
                <rect x={x0} y="0" width={w} height={BAND_H} fill={WORD_COLOR[b.phase]} class="seg"
                  onclick={() => openTicket(l.ulid)}>
                  <title>{b.phase}: {fmtDur((b.x1 - b.x0) * (to - from))}</title>
                </rect>
              {/each}
            </svg>
          </div>
        </div>
        <div class="chain lanechain">
          {#each laneDur(l) as b}
            <span class="link"><i style:background={WORD_COLOR[b.phase]}></i>{b.phase} <b>{b.dur}</b></span>
          {/each}
        </div>
      {:else}
        <p class="muted empty">no tickets moved in this window</p>
      {/each}
    </div>

    <!-- ================= Day: one calendar day, hourly actor columns ================= -->
    <div class="panel day" data-testid="day">
      <h3>day · {selectedDay}</h3>
      <div class="daycols">
        {#each dayStacks as c (c.hour)}
          <div class="col">
            <svg width="10" height="44">
              {#each c.segs as s}
                <rect x="0" y={s.y} width="10" height={s.h} fill={ACTOR_COLOR[s.actor] || 'var(--line-strong)'}>
                  <title>{selectedDay} {String(c.hour).padStart(2, '0')}h {s.actor}: {s.n}</title>
                </rect>
              {/each}
            </svg>
            {#if c.hour % 3 === 0}<span class="hl">{String(c.hour).padStart(2, '0')}</span>{/if}
          </div>
        {/each}
      </div>
      <div class="chain">
        <span class="link">{cols.reduce((n, c) => n + c.events, 0)} events on {selectedDay}</span>
        {#each cols.filter((c) => c.events) as c (c.hour)}
          <span class="link">{String(c.hour).padStart(2, '0')}h <b>{c.events}</b></span>
        {/each}
      </div>
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
  .rowhead { display: flex; align-items: baseline; gap: 12px; }
  .step { font-size: 11px; }
  .panel { padding: 10px 14px; }
  svg { display: block; overflow: visible; }
  .cbar { fill: var(--text-dim); opacity: 0.5; }
  .cbar:hover { opacity: 0.9; }
  .cbar.inside { fill: var(--accent); opacity: 0.85; }
  .now { stroke: var(--accent); stroke-width: 1.5; }
  .sel { fill: var(--accent); opacity: 0.12; pointer-events: none; }
  .cap { fill: transparent; cursor: crosshair; }
  .band-bg { fill: var(--bg-inset); }
  .seg { opacity: 0.8; cursor: pointer; }
  .lane { display: flex; gap: 10px; align-items: center; margin-top: 8px; }
  .slug {
    width: 110px; flex: none; text-align: left; font-size: 11px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 0;
  }
  .bandwrap { flex: 1; }
  .lanechain { margin: 2px 0 0 120px; }
  .daycols { display: flex; gap: 3px; align-items: flex-end; }
  .col { display: flex; flex-direction: column; align-items: center; gap: 2px; min-height: 52px; justify-content: flex-end; }
  .hl { font-size: 8px; color: var(--text-dim); }
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
  .empty { margin: 4px 0; }
  @media (max-width: 700px) {
    .slug { width: 80px; font-size: 10px; }
    .lanechain { margin-left: 90px; }
  }
</style>
