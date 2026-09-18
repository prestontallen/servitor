<script>
  import { arcs, view, openTicket, show, kindGlyph } from './state.svelte.js';
  import { get } from './api.svelte.js';

  let lanes = $state([]); // [{member, events:[...]}]
  let error = $state(null);
  let loading = $state(false);

  const arc = $derived(arcs.list.find((a) => a.ulid === view.arcRef));

  $effect(() => {
    if (view.name !== 'timeline' || !view.arcRef) return;
    load(view.arcRef);
  });

  async function load(arcUlid) {
    loading = true;
    error = null;
    try {
      const summary = arcs.list.find((a) => a.ulid === arcUlid)
        || await get('/api/arcs').then((all) => all.find((a) => a.ulid === arcUlid));
      if (!summary) throw new Error('arc not found');
      const members = [{ ulid: summary.ulid, slug: summary.slug, title: summary.title, status: summary.status },
                       ...summary.members];
      const out = await Promise.all(
        members.map(async (m) => ({ member: m, events: await get(`/api/ticket/${m.ulid}/history?limit=1000`) }))
      );
      lanes = out;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  // span: min..max ts across every event in the arc
  const span = $derived.by(() => {
    let min = Infinity, max = -Infinity;
    for (const l of lanes) for (const e of l.events) {
      const t = new Date(e.ts).getTime();
      if (t < min) min = t;
      if (t > max) max = t;
    }
    if (!isFinite(min)) return null;
    if (max === min) max = min + 1;
    return { min, max };
  });

  const DAY = 86400000;
  // status periods per member: walk events chronologically, split at status.set
  function periods(events) {
    const sorted = [...events].sort((a, b) => a.id - b.id);
    const out = [];
    let cur = { start: null, status: 'queued' };
    for (const e of sorted) {
      if (e.kind === 'status.set') {
        if (cur.start !== null) out.push({ ...cur, end: new Date(e.ts).getTime() });
        cur = { start: new Date(e.ts).getTime(), status: e.payload?.status || 'queued' };
      } else if (cur.start === null && e.kind === 'ticket.create') {
        cur.start = new Date(e.ts).getTime();
      }
    }
    if (cur.start !== null) out.push({ ...cur, end: Infinity });
    return out;
  }

  function pct(t) {
    return ((t - span.min) / (span.max - span.min)) * 100;
  }

  const STATUS_COLOR = { queued: 'var(--text-dim)', active: 'var(--accent)', blocked: 'var(--fail)', done: 'var(--ok)' };
  const SIGNAL_KINDS = ['note', 'decision', 'gate', 'feedback'];
</script>

<section class="wrap">
  <div class="head">
    <button class="back" onclick={() => show('arcs')}>&larr; arcs</button>
    <h2>{arc ? (arc.title || arc.slug) : 'arc'}</h2>
  </div>
  {#if error}<p class="muted">{error}</p>{/if}
  {#if loading}<p class="muted">loading…</p>{/if}

  {#if span && lanes.length}
    <div class="timeline panel">
      <div class="axis muted">
        <span>{new Date(span.min).toLocaleDateString()}</span>
        <span>{Math.max(1, Math.round((span.max - span.min) / DAY))} day{(span.max - span.min) > DAY ? 's' : ''}</span>
        <span>now</span>
      </div>
      {#each lanes as lane (lane.member.ulid)}
        <div class="lane">
          <button class="who" onclick={() => openTicket(lane.member.ulid)}>{lane.member.slug}</button>
          <div class="track">
            {#each periods(lane.events) as p}
              <div
                class="period"
                style:left="{pct(Math.max(p.start, span.min))}%"
                style:width="{Math.max(pct(p.end === Infinity ? span.max : Math.min(p.end, span.max)) - pct(Math.max(p.start, span.min)), 0.5)}%"
                style:background={STATUS_COLOR[p.status] || 'var(--text-dim)'}
                title="{p.status}"
              ></div>
            {/each}
            {#each lane.events.filter((e) => SIGNAL_KINDS.includes(e.kind)) as e (e.id)}
              <span
                class="mark k-{e.kind}"
                style:left="{pct(new Date(e.ts).getTime())}%"
                title="{e.kind}: {e.payload?.v || e.payload?.what || e.payload?.finding || ''}"
              >{kindGlyph(e.kind)}</span>
            {/each}
          </div>
        </div>
      {/each}
      <div class="legend muted">
        <span><i style="background: var(--text-dim)"></i> queued</span>
        <span><i style="background: var(--accent)"></i> active</span>
        <span><i style="background: var(--fail)"></i> blocked</span>
        <span><i style="background: var(--ok)"></i> done</span>
        <span>✎ signal (note/decision/gate — hover/tap for detail)</span>
      </div>
    </div>
  {:else if !loading && !error}
    <p class="muted">no events yet</p>
  {/if}
</section>

<style>
  .wrap { max-width: 1100px; margin: 0 auto; }
  .head { display: flex; align-items: baseline; gap: 14px; margin-bottom: 14px; }
  h2 { font-size: 15px; margin: 0; }
  .back { font-size: 11px; padding: 4px 10px; }
  .timeline { padding: 16px 18px; }
  .axis {
    display: flex; justify-content: space-between; font-size: 10px;
    text-transform: uppercase; letter-spacing: 0.06em;
    border-bottom: 1px solid var(--line); padding-bottom: 4px; margin-bottom: 8px;
  }
  .lane { display: flex; align-items: center; gap: 10px; padding: 9px 0; }
  .who {
    min-width: 110px; max-width: 110px; text-align: left; font-size: 11px;
    border: none; background: none; color: var(--accent); padding: 0;
    cursor: pointer; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    min-height: 32px;
  }
  .track {
    position: relative; flex: 1; height: 26px;
    background: var(--bg-inset); border-radius: 3px; overflow: visible;
  }
  .period {
    position: absolute; top: 7px; height: 12px; border-radius: 2px; opacity: 0.75;
  }
  .mark {
    position: absolute; top: 50%; transform: translate(-50%, -50%);
    font-size: 11px; line-height: 1; color: var(--text);
    background: var(--bg-raised); border: 1px solid var(--line-strong);
    border-radius: 50%; width: 18px; height: 18px;
    display: flex; align-items: center; justify-content: center;
    cursor: default;
  }
  .mark.k-gate { border-color: var(--accent); color: var(--accent); }
  .mark.k-decision { border-color: var(--warn); color: var(--warn); }
  .legend { display: flex; gap: 14px; flex-wrap: wrap; margin-top: 12px; font-size: 10px; align-items: center; }
  .legend i { display: inline-block; width: 10px; height: 10px; border-radius: 2px; margin-right: 4px; vertical-align: middle; }
  @media (max-width: 700px) {
    .lane { flex-direction: column; align-items: stretch; gap: 4px; }
    .who { max-width: none; min-height: 24px; }
    .mark { width: 22px; height: 22px; font-size: 13px; }
  }
</style>
