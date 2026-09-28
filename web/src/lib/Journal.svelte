<script>
  // The journal: the global ledger newest first, filtered by kind and
  // actor, paged backwards by id. A row opens its ticket.
  import { get } from './api.svelte.js';
  import { openTicket, kindGlyph, eventText, relTs, fmtTs } from './state.svelte.js';
  import { live } from './live.svelte.js';

  const PAGE = 50;

  let events = $state([]);
  let kind = $state('');
  let actorType = $state('');
  let loading = $state(false);
  let exhausted = $state(false); // no older events
  let error = $state(null);

  const KINDS = ['', 'note', 'decision', 'gate', 'status.set', 'field.set', 'feedback', 'hook'];
  const ACTORS = ['', 'human', 'agent', 'system'];

  // #/journal?before=<id> opens the page that starts just under that id:
  // the Time view's column click lands here. Read on load and on hashchange,
  // since a hash change within the journal does not remount it.
  const anchorFromHash = () => Number(new URLSearchParams((location.hash.split('?')[1] || '')).get('before')) || 0;
  let anchor = $state(anchorFromHash());
  $effect(() => {
    const apply = () => { const a = anchorFromHash(); if (a !== anchor) anchor = a; };
    window.addEventListener('hashchange', apply);
    return () => window.removeEventListener('hashchange', apply);
  });
  function newest() { location.hash = '#/journal'; }

  async function fetchPage(beforeId = 0) {
    const q = new URLSearchParams();
    if (kind) q.set('kind', kind);
    if (actorType) q.set('actor_type', actorType);
    if (beforeId) q.set('before_id', String(beforeId));
    q.set('limit', String(PAGE));
    return get(`/api/events?${q}`);
  }

  async function load() {
    loading = true;
    error = null;
    try {
      const page = await fetchPage(anchor);
      events = page;
      exhausted = page.length < PAGE;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function loadOlder() {
    if (loading || exhausted || events.length === 0) return;
    loading = true;
    try {
      const min = Math.min(...events.map((e) => e.id));
      const page = await fetchPage(min);
      const seen = new Set(events.map((e) => e.id));
      events = [...events, ...page.filter((e) => !seen.has(e.id))];
      exhausted = page.length < PAGE;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  // reload when filters change; refresh top of the stream on live changes
  $effect(() => {
    void kind, void actorType, void anchor;
    if (!anchor) void live.changeCount;   // an anchored page is a fixed page; the live stream only moves the newest one
    load();
  });

  // group by day, newest first
  const groups = $derived.by(() => {
    const out = [];
    let cur = null;
    for (const e of events) {
      const day = new Date(e.ts).toDateString();
      if (!cur || cur.day !== day) {
        cur = { day, id: 'day-' + day + '-' + e.id, events: [] };
        out.push(cur);
      }
      cur.events.push(e);
    }
    return out;
  });

  const tsShort = (ts) => new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
</script>

<section class="wrap">
  <div class="frame">
    <span class="cap">Journal · the ledger, {anchor ? `from event ${anchor - 1} back` : 'newest first'}</span>
    <span class="cap right">{events.length} events{#if !exhausted}+{/if}</span>
    <div class="filters">
      <label><span class="label">kind</span>
        <select id="journal-kind" bind:value={kind} onchange={() => { exhausted = false; }}>
          {#each KINDS as k (k)}<option value={k}>{k || 'all'}</option>{/each}
        </select>
      </label>
      <label><span class="label">actor</span>
        <select id="journal-actor" bind:value={actorType} onchange={() => { exhausted = false; }}>
          {#each ACTORS as a (a)}<option value={a}>{a || 'all'}</option>{/each}
        </select>
      </label>
      {#if anchor}<button class="newest" onclick={newest}>▴ newest</button>{/if}
    </div>

    {#if error}<p class="muted">unreachable: {error}</p>{/if}

    {#each groups as g (g.id)}
      <div class="day">
        <h3>{g.day}</h3>
        {#each g.events as e (e.id)}
          <div class="row" class:signal={e.class === 'signal'} class:human={e.actor_type === 'human'} onclick={() => openTicket(e.ticket_ulid)} role="link" tabindex="0" onkeydown={(k) => k.key === 'Enter' && openTicket(e.ticket_ulid)}>
            <span class="ts" title={fmtTs(e.ts)}>{tsShort(e.ts)}</span>
            <span class="glyph">{e.class === 'signal' ? (e.kind === 'gate' ? '✠' : kindGlyph(e.kind)) : ''}</span>
            <span class="kind">{e.kind}</span>
            <span class="line" class:dim={e.class !== 'signal'}>{eventText(e)}</span>
            <span class="actor" class:human={e.actor_type === 'human'}>{e.actor}</span>
          </div>
        {/each}
      </div>
    {:else}
      {#if !loading && !error}
        <p class="muted empty">nothing matches</p>
      {/if}
    {/each}
    {#if !exhausted && events.length}
      <button class="older" onclick={loadOlder} disabled={loading}>
        {loading ? 'querying…' : '▾ older'}
      </button>
    {/if}
  </div>
</section>

<style>
  .wrap { max-width: 980px; margin: 0 auto; }
  .frame { padding: 16px 14px 12px; }
  .filters { display: flex; gap: 14px; align-items: center; margin-bottom: 6px; flex-wrap: wrap; font-size: 12px; }
  .filters label { display: flex; gap: 8px; align-items: center; }
  .day h3 {
    font-size: 10px; text-transform: uppercase; letter-spacing: 0.2em; font-weight: 500;
    color: var(--bone-dim); margin: 16px 0 4px; border-bottom: 1px solid var(--rust); padding-bottom: 3px;
  }
  .row {
    display: grid; grid-template-columns: 44px 14px 90px 1fr auto; gap: 8px; align-items: baseline;
    padding: 5px 2px; border-bottom: 1px dashed var(--rust); cursor: pointer; font-size: 11.5px;
  }
  .row:hover { background: var(--iron-2); }
  .row.signal { padding: 6px 2px; }
  .ts { color: var(--phos-dim); font-size: 10px; white-space: nowrap; }
  .row.human .ts { color: var(--phos); }
  .glyph { color: var(--phos); text-align: center; }
  .kind { font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--bone-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .line { min-width: 0; overflow-wrap: anywhere; }
  .line.dim { color: var(--bone-dim); font-size: 11px; }
  .actor { font-size: 10px; letter-spacing: 0.06em; color: var(--bone-dim); white-space: nowrap; }
  .actor.human { color: var(--phos); }
  .older { display: block; margin: 14px auto 0; padding: 6px 22px; }
  .newest { font-size: 10px; padding: 2px 8px; min-height: 0; margin-left: auto; }
  .empty { padding: 20px 0; }
  @media (max-width: 700px) {
    .frame { padding: 14px 10px 10px; }
    .row { grid-template-columns: 44px 14px 1fr; }
    .kind { display: none; }
    .actor { grid-column: 3; }
  }
  @media (pointer: coarse) {
    .row { min-height: 40px; align-items: center; }
  }
</style>
