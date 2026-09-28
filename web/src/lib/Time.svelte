<script>
  // Time view: the ledger itself, one dot per event. One GraphQL query per
  // window (timeview.js TIME_WINDOW_QUERY) brings the events inside it,
  // each with its ticket's slug, and the timeline segments for the same
  // bounds; the events fold client-side into dots for the one lattice.
  // Hue by kind, actor or ticket; signals-only drops the transitions; the
  // legend is a filter. A column opens the journal at its newest event, a
  // gate tick opens the dossier. Flow is one lane per ticket the window
  // touched, from the segments.
  import { gql } from './api.svelte.js';
  import { board, openTicket, say } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import DotLattice from './DotLattice.svelte';
  import Lanes from './Lanes.svelte';
  import { parseTimeHash, timeHash, flowLanes, touchedTickets, dayKey, daysIn, fmtDur } from './timeview.js';
  import { foldLedger, hueBy, filterDots, topTickets, columnSummary, columnAt, rangeFromLocal, minuteKey, stepWindow } from './timeview.js';
  import { TIME_WINDOW_QUERY, MAX_PAGES, windowVars, mergePages, needsRefetch } from './timeview.js';

  const DAY = 86400000, HOUR = 3600000;
  const CELL = 8;
  const SIDES = { human: 'up', agent: 'down' };
  const SIDE_OPACITY = { human: 1, agent: 0.7 };
  const now = () => Date.now();

  // ---- the window: the hash makes it linkable.
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

  // ---- the window read ---------------------------------------------------------
  // one query for the window, paged on next_before_id up to MAX_PAGES; the
  // response is kept only for the window it was asked for. Live changes
  // refetch only a window that can still gain events (needsRefetch) and
  // not more than once a while.
  let win = $state({ events: [], segments: [], truncated: false });
  let ledgerError = $state(null);
  let fetchedAt = 0;
  const REFRESH_MS = 20000;

  async function loadWindow(f, t) {
    try {
      const pages = [];
      let before = null;
      for (let i = 0; i < MAX_PAGES; i++) {
        const page = await gql(TIME_WINDOW_QUERY, windowVars(f, t, before));
        pages.push(page);
        before = page.events.next_before_id;
        if (before == null) break;
      }
      if (from !== f || to !== t) return;   // the window moved while we read
      win = mergePages(pages);
      ledgerError = null;
      fetchedAt = now();
    } catch (e) {
      if (from === f && to === t) ledgerError = e.message;
    }
  }
  $effect(() => { fetchedAt = 0; loadWindow(from, to); });
  $effect(() => {
    void live.changeCount;
    if (!fetchedAt || !needsRefetch(to, fetchedAt) || now() - fetchedAt < REFRESH_MS) return;
    loadWindow(from, to);
  });
  const dots = $derived(foldLedger(win.events));

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

  // ---- hue, filters, legend ---------------------------------------------------
  let hue = $state('kind');          // kind | actor | ticket
  let signalsOnly = $state(false);
  let only = $state(null);           // legend isolate: a group name or null

  const windowDots = $derived(dots.filter((d) => d.t >= from && d.t <= to));
  // ticket hues follow the window's busiest so both strips agree on who is who
  const top = $derived(hue === 'ticket' ? topTickets(filterDots(windowDots, { signalsOnly })) : []);
  const hued = $derived(hueBy(dots, hue, top));
  const groups = $derived(hued.groups);
  const windowDrawn = $derived(filterDots(hued.dots, { signalsOnly, only }).filter((d) => d.t >= from && d.t <= to));
  const counts = $derived.by(() => {
    const c = {};
    for (const d of filterDots(hued.dots.filter((x) => x.t >= from && x.t <= to), { signalsOnly })) c[d.group] = (c[d.group] || 0) + 1;
    return c;
  });
  function setHue(h) { hue = h; only = null; }
  function toggleOnly(name) { only = only === name ? null : name; }

  // ---- gates as ticks on the window lattice --------------------------------------
  // every gate in the window, in time order (the lib keeps that order and
  // labels each tick with its index, so a click maps back)
  const gateTicks = $derived(windowDrawn
    .filter((d) => d.kind === 'gate')
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

  let windowQuantum = $state(1);

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
      // dotlattice 0.2.0 labels each tick with its time-order index; the
      // DOM-order fallback covers 0.1.0 and goes with the v0.2.0 bump
      const i = tick.dataset.index != null ? Number(tick.dataset.index) : [...winEl.querySelectorAll('.dl-tick')].indexOf(tick);
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

  // ---- flow: only the tickets the window touched, drawn by the lib's Lanes ---------
  const lanes = $derived(flowLanes(win.segments, board.cards, from, to, touchedTickets(windowDots)));
  const laneRows = $derived(lanes.map((l) => ({
    id: l.ulid, label: l.slug, title: l.title,
    bars: l.bars.map((b) => ({ start: b.start, end: b.end, kind: b.phase }))
  })));
  const PHASES = ['queued', 'shaping', 'building', 'checking', 'shipping'];
  // the phases in the card-word tokens; blocked is the hatch the board uses
  const KINDS = {
    queued: 'var(--w-queued)', shaping: 'var(--w-shaping)', building: 'var(--w-building)',
    checking: 'var(--w-checking)', shipping: 'var(--w-shipping)',
    blocked: { color: 'var(--blood)', hatch: 'var(--blood-dim)' }
  };
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

    {#if win.truncated}
      <p class="muted">window truncated at {win.events.length} events — narrow it</p>
    {/if}

    <!-- ================= the window: one dot per event, gates as ticks ================= -->
    <div class="frame win" data-testid="window">
      <span class="cap">{windowIsDay ? `day · ${selectedDay}` : `window · ${fmtDur(to - from)}`}</span>
      <span class="cap right">hover a column · click it for the journal{#if gateTicks.length}{' · hover ✠ · click its label for the dossier'}{/if}</span>
      <div class="winwrap" bind:this={winEl} onclick={windowClick} role="presentation">
        <DotLattice span={windowSpan} events={windowDrawn} {groups} sides={SIDES} sideOpacity={SIDE_OPACITY} axis={windowAxis} ticks={gateTicks} tickLabels="hover" {tooltip} cell={CELL} maxRows={14} bind:quantum={windowQuantum} label="ledger events per column in the window, human above the line, agents below" />
      </div>
      <div class="chain">
        <span class="link">{windowDrawn.length} drawn<b>{windowHuman} by the human</b></span>
        <span class="link key">human above · agent below{#if windowQuantum > 1} · one dot is {windowQuantum} events{/if}</span>
      </div>
    </div>

    <!-- ================= flow: one lane per touched ticket, phase bars, blocked hatched ================= -->
    <div class="frame flow" data-testid="flow">
      <span class="cap">Flow · phase per card</span>
      <span class="cap right">{lanes.length} card{lanes.length === 1 ? '' : 's'} touched in the window · click a card for the dossier · hover a bar</span>
      {#if laneRows.length}
        <Lanes span={windowSpan} lanes={laneRows} kinds={KINDS} axis={windowAxis} cell={CELL} onLabel={(l) => openTicket(l.id)} label="phase per card across the window" />
      {:else}
        <p class="muted empty">no cards touched in this window</p>
      {/if}
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
  .winwrap { cursor: pointer; }
  .winwrap :global(.dl-tick) { cursor: pointer; }

  .flow :global(.dl-lanes) { margin-top: 4px; }
  .flow :global(.dl-lane-label) { color: var(--phos-dim); text-transform: none; letter-spacing: 0; }
  .flow :global(.dl-lane-label:hover) { color: var(--phos); }
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
  }
</style>
