<script>
  // Time view: the ledger itself, one dot per event. One request to
  // /api/events is folded client-side (timeview.js) into dots; the
  // thirty-day overview strip carries the brush and the window lattice
  // below it is the zoomed view, both from the same dots and one legend.
  // Hue by kind, actor or ticket; signals-only drops the transitions; the
  // legend is a filter. A column opens the journal at its newest event, a
  // gate tick opens the dossier. Flow is one lane per ticket that moved in
  // the window, from /api/timeline's segments, which are still the right
  // shape for it.
  import { get } from './api.svelte.js';
  import { board, arcs, openTicket, say } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import DotLattice from './DotLattice.svelte';
  import { parseTimeHash, timeHash, flowLanes, dayKey, daysIn, fmtDur } from './timeview.js';
  import { foldLedger, hueBy, filterDots, topTickets, columnSummary, columnAt, dayTotals, rangeFromLocal, minuteKey, stepWindow } from './timeview.js';

  const OUTER_DAYS = 30;
  const DAY = 86400000, HOUR = 3600000;
  const CELL = 8;
  const BAND_H = 9;
  const SIDES = { human: 'up', agent: 'down' };
  const SIDE_OPACITY = { human: 1, agent: 0.7 };

  // ---- the ledger ------------------------------------------------------------
  // one request, newest first, the whole ledger the endpoint will give
  // (cap 10000, no time filter); refreshed on live changes but not more
  // than once a while
  let raw = $state([]);
  let ledgerError = $state(null);
  let fetchedAt = 0;
  const REFRESH_MS = 20000;
  const now = () => Date.now();

  async function loadLedger() {
    try {
      raw = await get('/api/events?limit=10000');
      ledgerError = null;
      fetchedAt = now();
    } catch (e) {
      ledgerError = e.message;
    }
  }
  $effect(() => {
    void live.changeCount;
    if (now() - fetchedAt < REFRESH_MS) return;
    loadLedger();
  });

  // ticket ulid -> slug: board, arcs and their members, then the flow segments
  const slugMap = $derived.by(() => {
    const m = new Map();
    for (const c of board.cards) m.set(c.ulid, c.slug);
    for (const a of arcs.list) { m.set(a.ulid, a.slug); for (const x of a.members) m.set(x.ulid, x.slug); }
    for (const s of inner.segments) m.set(s.ticket_ulid, s.slug);
    return m;
  });
  const dots = $derived(foldLedger(raw, (u) => slugMap.get(u)));

  // ---- the window: the shared brush selection; the hash makes it linkable.
  // Read once at construction and then only on hashchange: a bare #/time
  // defaults to a window ending now, so an effect that both re-read the
  // hash and wrote the window would chase the clock forever.
  const initial = parseTimeHash(location.hash);
  let from = $state(initial.from);
  let to = $state(initial.to);
  $effect(() => {
    const apply = () => {
      const w = parseTimeHash(location.hash);
      if (w.from !== from || w.to !== to) { from = w.from; to = w.to; }
    };
    window.addEventListener('hashchange', apply);
    return () => window.removeEventListener('hashchange', apply);
  });
  const windowSpan = $derived({ min: from, max: to });
  // the overview strip spans thirty days or the chosen range, whichever is longer
  const outerSpan = $derived({ min: Math.min(now() - OUTER_DAYS * DAY, from), max: Math.max(now(), to) });

  // ---- the header row: the window as two local times, to the minute ---------------
  let fromAt = $state(minuteKey(initial.from));
  let toAt = $state(minuteKey(initial.to));
  $effect(() => { fromAt = minuteKey(from); toAt = minuteKey(to); });   // brush and presets reflect
  function applyRange() {
    const r = rangeFromLocal(fromAt, toAt);
    if (r.error) { say(`range: ${r.error}`); return; }
    if (r.from === from && r.to === to) return;
    location.hash = timeHash(r.from, r.to);
    say(`window ${stamp(r.from)} → ${stamp(r.to)}`);
  }
  function step(dir) {
    const w = stepWindow(from, to, dir);
    location.hash = timeHash(w.from, w.to);
  }

  // flow segments for the window, refetched when the brush moves
  let inner = $state({ segments: [], error: null });
  $effect(() => {
    if (!to) return;
    const f = from, t = to;
    get(`/api/timeline?since=${new Date(f).toISOString()}&until=${new Date(t).toISOString()}`)
      .then((d) => { if (from === f && to === t) inner = { segments: d.segments, error: null }; })
      .catch((e) => (inner = { ...inner, error: e.message }));
  });

  // ---- hue, filters, legend ---------------------------------------------------
  let hue = $state('kind');          // kind | actor | ticket
  let signalsOnly = $state(false);
  let only = $state(null);           // legend isolate: a group name or null

  const windowDots = $derived(dots.filter((d) => d.t >= from && d.t <= to));
  // ticket hues follow the window's busiest so both strips agree on who is who
  const top = $derived(hue === 'ticket' ? topTickets(filterDots(windowDots, { signalsOnly })) : []);
  const hued = $derived(hueBy(dots, hue, top));
  const groups = $derived(hued.groups);
  const outerDrawn = $derived(filterDots(hued.dots, { signalsOnly, only }));
  const windowDrawn = $derived(outerDrawn.filter((d) => d.t >= from && d.t <= to));
  const counts = $derived.by(() => {
    const c = {};
    for (const d of filterDots(hued.dots.filter((x) => x.t >= from && x.t <= to), { signalsOnly })) c[d.group] = (c[d.group] || 0) + 1;
    return c;
  });
  function setHue(h) { hue = h; only = null; }
  function toggleOnly(name) { only = only === name ? null : name; }

  // ---- gates as ticks on the window lattice --------------------------------------
  // scaled to the window so the labels stay readable: every gate across a
  // day or two, only the human's contract approvals across a fortnight,
  // none beyond that (the readout still counts them)
  const tickGate = (d, len) => len <= 2 * DAY ? true : len <= 14 * DAY ? d.payload.gate === 'contract_approved' : false;
  const gateTicks = $derived(windowDrawn
    .filter((d) => d.kind === 'gate' && tickGate(d, to - from))
    .sort((a, b) => a.t - b.t)
    .map((d) => ({
      t: d.t, ticket: d.ticket, label: '✠ ' + String(d.payload.gate || 'gate').replace('_approved', ''), sig: d.slug,
      color: d.payload.gate === 'contract_approved' ? 'var(--phos)' : 'var(--phos-dim)',
      title: `${d.payload.gate} · ${d.slug} · ${d.actor}`
    })));

  // ---- axes ----------------------------------------------------------------------
  const dayLabel = (ms) => new Date(ms).toLocaleDateString([], { month: 'short', day: 'numeric' }).toUpperCase();
  const hourLabel = (ms) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const midnight = (day) => { const [y, m, d] = day.split('-').map(Number); return new Date(y, m - 1, d).getTime(); };
  const stamp = (ms) => new Date(ms).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });

  const cadenceEvery = typeof window !== 'undefined' && window.innerWidth < 700 ? 10 : 5;
  const cadenceAxis = $derived(daysIn(outerSpan.min, outerSpan.max).filter((_, i) => i % cadenceEvery === 0)
    .map((d) => ({ t: midnight(d), label: dayLabel(midnight(d)) })).filter((a) => a.t >= outerSpan.min));
  const windowAxis = $derived.by(() => {
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

  // ---- brush: a capture layer over the overview strip ----------------------------
  let capEl = $state(null);
  let dragging = $state(false);
  let dragCur = $state(0);
  let anchor = $state(0);
  function brushT(e) {
    const rect = capEl.getBoundingClientRect();
    return outerSpan.min + ((e.clientX - rect.left) / rect.width) * (outerSpan.max - outerSpan.min);
  }
  function brushDown(e) {
    dragging = true;
    dragCur = brushT(e);
    anchor = Math.abs(dragCur - from) < Math.abs(dragCur - to) ? from : to;   // nearest window edge
    capEl.setPointerCapture(e.pointerId);
  }
  function brushMove(e) { if (dragging) dragCur = brushT(e); }
  function brushUp() {
    if (!dragging) return;
    dragging = false;
    const lo = Math.min(anchor, dragCur), hi = Math.max(anchor, dragCur);
    if (hi - lo < HOUR) pinDay(dayKey(dragCur));   // a click pins the clicked day
    else location.hash = timeHash(lo, hi);
  }
  const highlight = $derived(dragging ? { from: Math.min(anchor, dragCur), to: Math.max(anchor, dragCur) } : { from, to });

  function pinDay(day) { const s = midnight(day); location.hash = timeHash(s, s + DAY); }
  function preset(days) { const n = now(); location.hash = timeHash(n - days * DAY, n); }
  const selectedDay = $derived(dayKey(Math.max(from, to - 60000)));
  const windowIsDay = $derived(to - from <= DAY + 60000);

  // ---- the window lattice as a way in: tooltip, column click, tick click ---------
  let winEl = $state(null);
  function tooltip(bucket, bucketMs) {
    const s = columnSummary(windowDrawn, bucket.start, bucket.start + bucketMs);
    const kinds = Object.entries(s.byGroup).map(([g, n]) => `${n} ${g}`).join(', ');
    const tickets = s.tickets.map((x) => `${x.slug} ${x.n}`).join(' · ');
    return `${stamp(bucket.start)} +${fmtDur(bucketMs)}\n${kinds}\n${tickets}\nclick: journal at this column`;
  }
  function windowClick(e) {
    const tick = e.target.closest?.('.dl-tick');
    if (tick) {
      const i = [...winEl.querySelectorAll('.dl-tick')].indexOf(tick);   // the lib draws ticks in time order
      const g = gateTicks[i];
      if (g) { openTicket(g.ticket); return; }
    }
    const rect = winEl.getBoundingClientRect();
    const col = columnAt(e.clientX - rect.left, rect.width, windowSpan, CELL);
    const s = columnSummary(windowDrawn, col.t0, col.t1);
    if (!s.newestId) { say(`nothing in that column`); return; }
    say(`journal from ${stamp(col.t0)} · ${s.total} event${s.total === 1 ? '' : 's'}`);
    location.hash = `#/journal?before=${s.newestId + 1}`;
  }

  // ---- readout ---------------------------------------------------------------------
  const windowSignals = $derived(windowDots.filter((d) => d.signal).length);
  const windowHuman = $derived(windowDots.filter((d) => d.side === 'human').length);
  const windowGates = $derived(windowDots.filter((d) => d.kind === 'gate').length);
  const blockedNow = $derived(board.cards.filter((c) => c.status === 'blocked').length);
  const tail = $derived(dayTotals(outerDrawn).slice(-5));

  // ---- flow ------------------------------------------------------------------------
  const lanes = $derived(flowLanes(inner.segments, board.cards, from, to));
  let laneW = $state(0);
  const RUN_STEP = 6;
  const PHASES = ['queued', 'shaping', 'building', 'checking', 'shipping'];
  function laneDur(l) {
    return l.bars.map((b) => ({ phase: b.phase, dur: fmtDur((b.x1 - b.x0) * (to - from)) }));
  }
</script>

<section class="time">
  <header class="rowhead">
    <h2 class="goth">Time</h2>
    <span class="range">
      <label class="date"><span class="label">from</span><input id="range-from" type="datetime-local" step="60" bind:value={fromAt} onchange={applyRange} max={toAt}></label>
      <span class="arrow">→</span>
      <label class="date"><span class="label">to</span><input id="range-to" type="datetime-local" step="60" bind:value={toAt} onchange={applyRange} min={fromAt}></label>
    </span>
    <span class="window">{fmtDur(to - from)}</span>
    <span class="presets">
      <button class:active={windowIsDay} onclick={() => pinDay(dayKey(now()))}>day</button>
      <button onclick={() => preset(7)}>week</button>
      <button onclick={() => preset(30)}>month</button>
      <button class="step" id="step-back" onclick={() => step(-1)} title="back one window">←</button>
      <button class="step" id="step-fwd" onclick={() => step(1)} title="forward one window">→</button>
    </span>
  </header>

  {#if ledgerError}
    <p class="muted">ledger unreachable: {ledgerError}</p>
  {:else}
    <div class="readout">
      <div><div class="k">signals in window</div><div class="v">{windowSignals}<small>of {windowDots.length}</small></div></div>
      <div><div class="k">by human hand</div><div class="v">{windowHuman}</div></div>
      <div><div class="k">gates passed</div><div class="v">{windowGates}</div></div>
      <div><div class="k">blocked now</div><div class="v" class:blood={blockedNow > 0}>{blockedNow}</div></div>
    </div>

    <!-- ================= controls + the one legend ================= -->
    <div class="controls">
      <span class="ctl">
        <span class="label">hue</span>
        {#each ['kind', 'actor', 'ticket'] as h (h)}
          <button class="hue" class:active={hue === h} onclick={() => setHue(h)} data-hue={h}>{h}</button>
        {/each}
      </span>
      <button class="ctl signals" class:active={signalsOnly} onclick={() => (signalsOnly = !signalsOnly)} aria-pressed={signalsOnly} id="signals-only">
        {signalsOnly ? '■' : '□'} signals only
      </button>
      <span class="legend" data-testid="legend">
        {#each groups as g (g.name)}
          <button class="lg" class:isolated={only === g.name} class:off={only && only !== g.name} onclick={() => toggleOnly(g.name)} data-group={g.name} title={only === g.name ? 'click to restore all' : 'click to isolate'}>
            <i style:background={g.color}></i>{g.name}<b>{counts[g.name] || 0}</b>
          </button>
        {/each}
      </span>
    </div>

    <!-- ================= overview: thirty days, the window as a highlight, drag to move it ================= -->
    <div class="frame cad" data-testid="cadence">
      <span class="cap">Overview · {Math.round((outerSpan.max - outerSpan.min) / DAY)} days</span>
      <div class="brushwrap">
        <DotLattice span={outerSpan} events={outerDrawn} {groups} sides={SIDES} sideOpacity={SIDE_OPACITY} axis={cadenceAxis} {highlight} maxRows={14} bind:quantum={cadenceQuantum} label="ledger events per column over thirty days, human above the line, agents below" />
        <div class="cap-layer" bind:this={capEl} onpointerdown={brushDown} onpointermove={brushMove} onpointerup={brushUp} onpointercancel={brushUp} data-testid="cadence-brush"></div>
      </div>
      <div class="chain">
        {#each tail as d (d.day)}
          <span class="link">{d.day.slice(5)} <b>{d.total}</b></span>
        {/each}
        <span class="link key">human above · agent below{#if cadenceQuantum > 1} · one dot is {cadenceQuantum} events{/if}</span>
      </div>
    </div>

    <!-- ================= the window: same dots, finer bucket, gates as ticks ================= -->
    <div class="frame win" data-testid="window">
      <span class="cap">{windowIsDay ? `day · ${selectedDay}` : `window · ${fmtDur(to - from)}`}</span>
      <span class="cap right">hover a column · click it for the journal{#if gateTicks.length}{' · click ✠ for the dossier'}{/if}</span>
      <div class="winwrap" bind:this={winEl} onclick={windowClick} role="presentation">
        <DotLattice span={windowSpan} events={windowDrawn} {groups} sides={SIDES} sideOpacity={SIDE_OPACITY} axis={windowAxis} ticks={gateTicks} {tooltip} cell={CELL} maxRows={14} bind:quantum={windowQuantum} label="ledger events per column in the window, human above the line, agents below" />
      </div>
      <div class="chain">
        <span class="link">{windowDrawn.length} drawn<b>{windowHuman} by the human</b></span>
        <span class="link key">human above · agent below{#if windowQuantum > 1} · one dot is {windowQuantum} events{/if}</span>
      </div>
    </div>

    <!-- ================= flow: one lane per ticket, phase as the ramp, blocked as red dots ================= -->
    <div class="frame flow" data-testid="flow">
      <span class="cap">Flow · phase per card</span>
      <span class="cap right">{lanes.length} card{lanes.length === 1 ? '' : 's'} moved in the window</span>
      {#if inner.error}<p class="muted">timeline unreachable: {inner.error}</p>{/if}
      {#each lanes as l (l.ulid)}
        <div class="lane">
          <button class="slug" onclick={() => openTicket(l.ulid)} title={l.title}>{l.slug}</button>
          <div class="bandwrap" bind:clientWidth={laneW}>
            {#if laneW > 0}
              <svg width={laneW} height={BAND_H}>
                {#each l.bars as b}
                  {@const x0 = b.x0 * laneW}
                  {@const w = Math.max(b.x1 * laneW - x0, 1)}
                  {#if b.phase === 'blocked'}
                    <g class="blocked">
                      {#each { length: Math.max(1, Math.floor(w / RUN_STEP)) } as _, i}
                        <rect x={x0 + i * RUN_STEP + 1} y={BAND_H / 2 - 2} width="4" height="4" />
                      {/each}
                      <title>blocked: {fmtDur((b.x1 - b.x0) * (to - from))}</title>
                    </g>
                  {:else}
                    <rect x={x0} y="0" width={w} height={BAND_H} class="seg p-{b.phase}">
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
            <span class="link" class:blocked={b.phase === 'blocked'}><i class="p-{b.phase}"></i>{b.phase} <b>{b.dur}</b></span>
          {/each}
        </div>
      {:else}
        <p class="muted empty">no cards moved in this window</p>
      {/each}
      <div class="plegend">
        {#each PHASES as p (p)}<span><i class="p-{p}"></i>{p}</span>{/each}
        <span><i class="p-blocked"></i>blocked</span>
      </div>
    </div>
  {/if}
</section>

<style>
  .time { max-width: 1240px; margin: 0 auto; display: flex; flex-direction: column; gap: 16px; }
  .rowhead { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
  .range { display: inline-flex; align-items: center; gap: 8px; }
  .date { display: inline-flex; align-items: center; gap: 6px; }
  .date input {
    font: inherit; font-size: 11px; color: var(--bone); background: var(--iron-2);
    border: 1px solid var(--rust-2); padding: 2px 6px; color-scheme: inherit; min-width: 0;
  }
  .date input:focus { border-color: var(--phos); }
  .arrow { color: var(--phos-dim); }
  .window { font-size: 11px; color: var(--bone-dim); letter-spacing: 0.06em; }
  .presets { display: inline-flex; gap: 4px; margin-left: auto; }
  .presets button { font-size: 10px; padding: 2px 8px; min-height: 0; }

  .readout { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 10px; }
  .readout > div { border: 1px solid var(--rust); padding: 6px 8px; background: var(--iron-2); }
  .readout .k { font-size: 9px; letter-spacing: 0.24em; color: var(--bone-dim); text-transform: uppercase; }
  .readout .v { font-size: 20px; color: var(--phos); line-height: 1.1; margin-top: 2px; }
  .readout .v small { font-size: 11px; color: var(--bone-dim); margin-left: 4px; }
  .readout .v.blood { color: var(--blood); }

  /* ---- controls + legend: one row, wraps on a phone ---- */
  .controls { display: flex; align-items: center; gap: 8px 18px; flex-wrap: wrap; font-size: 11px; }
  .ctl { display: inline-flex; align-items: center; gap: 4px; }
  .ctl .label { margin-right: 4px; }
  .hue, .signals { font-size: 10px; padding: 2px 8px; min-height: 0; }
  .legend { display: inline-flex; flex-wrap: wrap; gap: 4px 12px; margin-left: auto; }
  .lg {
    border: none; padding: 2px 0; min-height: 0; min-width: 0;
    font-size: 10px; letter-spacing: 0.14em; text-transform: uppercase; color: var(--bone-dim);
    display: inline-flex; align-items: center; gap: 5px;
  }
  .lg i { display: inline-block; width: 7px; height: 7px; border-radius: 50%; }
  .lg b { font-weight: 400; color: var(--bone); letter-spacing: 0; }
  .lg:hover { color: var(--bone); }
  .lg.isolated { color: var(--phos); border-bottom: 1px solid var(--phos); }
  .lg.off { opacity: 0.4; }

  .frame { padding: 14px 12px 10px; }
  .brushwrap { position: relative; }
  .cap-layer { position: absolute; inset: 0; cursor: crosshair; touch-action: none; }
  .winwrap { cursor: pointer; }
  .winwrap :global(.dl-tick) { cursor: pointer; }

  .seg.p-queued { fill: var(--w-queued); }
  .seg.p-shaping { fill: var(--w-shaping); }
  .seg.p-building { fill: var(--w-building); }
  .seg.p-checking { fill: var(--w-checking); }
  .seg.p-shipping { fill: var(--w-shipping); }
  .blocked rect { fill: var(--blood); }
  .lane { display: flex; gap: 10px; align-items: center; margin-top: 6px; }
  .slug {
    width: 150px; flex: none; text-align: left; font-size: 11px; border: none; background: none;
    color: var(--phos-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 0; min-height: 0; min-width: 0;
    text-transform: none; letter-spacing: 0;
  }
  .slug:hover { color: var(--phos); }
  .bandwrap { flex: 1; min-width: 0; background: repeating-linear-gradient(90deg, var(--rust) 0 1px, transparent 1px 8px); }
  .lanechain { margin: 2px 0 0 160px; }
  .chain { display: flex; flex-wrap: wrap; gap: 4px 14px; margin-top: 8px; font-size: 11px; color: var(--bone-dim); }
  .link { white-space: nowrap; }
  .link b { color: var(--bone); font-weight: 500; margin-left: 4px; }
  .link i, .plegend i { display: inline-block; width: 7px; height: 7px; margin-right: 5px; vertical-align: middle; }
  .link.blocked { color: var(--blood); }
  .key { font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase; }
  .plegend { display: flex; flex-wrap: wrap; gap: 6px 16px; font-size: 10px; letter-spacing: 0.14em; color: var(--bone-dim); text-transform: uppercase; margin-top: 12px; }
  i.p-queued { background: var(--w-queued); }
  i.p-shaping { background: var(--w-shaping); }
  i.p-building { background: var(--w-building); }
  i.p-checking { background: var(--w-checking); }
  i.p-shipping { background: var(--w-shipping); }
  i.p-blocked { background: var(--hatch); }
  .empty { margin: 4px 0; }
  @media (max-width: 700px) {
    .range { flex-wrap: wrap; width: 100%; }
    .date { flex: 1 1 100%; }
    .date input { flex: 1; }
    .arrow { display: none; }
    .presets { margin-left: 0; }
    .legend { margin-left: 0; }
    .slug { width: 90px; font-size: 10px; }
    .lanechain { margin-left: 100px; }
  }
</style>
