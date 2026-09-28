<script>
  // Arc timeline: one lane per member from the arc's first event to now,
  // status periods as bars in the card-word ramp, signals as glyphs.
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

  const SIGNAL_KINDS = ['note', 'decision', 'gate', 'feedback'];
  const STATUSES = ['queued', 'active', 'blocked', 'done', 'dropped'];
</script>

<section class="wrap">
  <button class="back" onclick={() => show('arcs')}>← arcs</button>
  <div class="frame">
    <span class="cap">Arc timeline</span>
    <span class="cap right">{#if span}{new Date(span.min).toLocaleDateString()} → now · {Math.max(1, Math.round((span.max - span.min) / DAY))} days{/if}</span>
    <h2 class="goth">{arc ? arc.slug : 'arc'}</h2>
    <div class="title">{arc?.title || ''}</div>
    {#if error}<p class="muted">{error}</p>{/if}
    {#if loading}<p class="muted">querying…</p>{/if}

    {#if span && lanes.length}
      <div class="lanes">
        {#each lanes as lane, i (lane.member.ulid)}
          <div class="lane" class:root={i === 0}>
            <button class="who" onclick={() => openTicket(lane.member.ulid)}>{i === 0 ? '' : '├─ '}{lane.member.slug}</button>
            <div class="track">
              {#each periods(lane.events) as p}
                <div
                  class="period s-{p.status}"
                  style:left="{pct(Math.max(p.start, span.min))}%"
                  style:width="{Math.max(pct(p.end === Infinity ? span.max : Math.min(p.end, span.max)) - pct(Math.max(p.start, span.min)), 0.5)}%"
                  title="{p.status}"
                ></div>
              {/each}
              {#each lane.events.filter((e) => SIGNAL_KINDS.includes(e.kind)) as e (e.id)}
                <span
                  class="mark k-{e.kind}"
                  style:left="{pct(new Date(e.ts).getTime())}%"
                  title="{e.kind}: {e.payload?.v || e.payload?.what || e.payload?.finding || e.payload?.gate || ''}"
                >{e.kind === 'gate' ? '✠' : kindGlyph(e.kind)}</span>
              {/each}
            </div>
          </div>
        {/each}
      </div>
      <div class="legend">
        {#each STATUSES as s (s)}<span><i class="s-{s}"></i>{s}</span>{/each}
        <span>✠ gate · ⚖ decision · ✎ note · ✦ feedback — hover for detail</span>
      </div>
    {:else if !loading && !error}
      <p class="muted">no events yet</p>
    {/if}
  </div>
</section>

<style>
  .wrap { max-width: 1100px; margin: 0 auto; }
  .back { margin-bottom: 14px; border: none; padding: 0; font-size: 11px; color: var(--phos-dim); min-height: 0; }
  .back:hover { color: var(--phos); }
  .frame { padding: 16px 14px 12px; }
  .title { color: var(--bone-dim); font-size: 12px; margin: 2px 0 14px; }
  .lanes { display: grid; gap: 4px; }
  .lane { display: grid; grid-template-columns: 150px 1fr; gap: 10px; align-items: center; }
  .lane.root .who { color: var(--phos); font-weight: 500; }
  .who {
    text-align: left; font-size: 11px; border: none; background: none; color: var(--phos-dim); padding: 0;
    cursor: pointer; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-height: 24px;
    text-transform: none; letter-spacing: 0;
  }
  .who:hover { color: var(--phos); }
  .track {
    position: relative; height: 22px; min-width: 0;
    background: repeating-linear-gradient(90deg, var(--rust) 0 1px, transparent 1px 8px);
  }
  .period { position: absolute; top: 7px; height: 8px; }
  .s-queued { background: var(--w-queued); }
  .s-active { background: var(--phos); }
  .s-blocked { background: var(--hatch); }
  .s-done { background: var(--verdigris); }
  .s-dropped { background: var(--bone-dim); }
  .mark {
    position: absolute; top: 50%; transform: translate(-50%, -50%);
    font-size: 10px; line-height: 1; color: var(--bone); background: var(--iron);
    border: 1px solid var(--rust-2); width: 16px; height: 16px;
    display: flex; align-items: center; justify-content: center; cursor: default;
  }
  .mark.k-gate { border-color: var(--phos); color: var(--phos); }
  .mark.k-decision { border-color: var(--phos-dim); color: var(--phos-dim); }
  .mark.k-feedback { border-color: var(--blood); color: var(--blood); }
  .legend { display: flex; gap: 6px 16px; flex-wrap: wrap; margin-top: 14px; font-size: 10px; letter-spacing: 0.14em; text-transform: uppercase; color: var(--bone-dim); }
  .legend i { display: inline-block; width: 8px; height: 8px; margin-right: 5px; vertical-align: middle; }
  @media (max-width: 700px) {
    .frame { padding: 14px 10px 10px; }
    .lane { grid-template-columns: 90px 1fr; }
    .who { font-size: 10px; }
    .mark { width: 20px; height: 20px; font-size: 12px; }
  }
</style>
