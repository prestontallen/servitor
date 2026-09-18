<script>
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

  async function fetchPage(beforeId = 0) {
    const q = new URLSearchParams();
    if (kind) q.set('kind', kind);
    if (actorType) q.set('actor_type', actorType);
    if (beforeId) q.set('before_id', String(beforeId));
    q.set('limit', String(PAGE));
    return get(`/api/events?${q}`);
  }

  async function load(reset = true) {
    loading = true;
    error = null;
    try {
      const page = await fetchPage(0);
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
    void kind, void actorType, void live.changeCount;
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
</script>

<section class="wrap">
  <h2>Journal</h2>
  <div class="filters">
    <label>
      <span class="muted">kind</span>
      <select bind:value={kind} onchange={() => { exhausted = false; }}>
        {#each KINDS as k (k)}<option value={k}>{k || 'all'}</option>{/each}
      </select>
    </label>
    <label>
      <span class="muted">actor</span>
      <select bind:value={actorType} onchange={() => { exhausted = false; }}>
        {#each ACTORS as a (a)}<option value={a}>{a || 'all'}</option>{/each}
      </select>
    </label>
    <span class="muted count">{events.length} events</span>
  </div>

  {#if error}<p class="muted">unreachable: {error}</p>{/if}

  {#each groups as g (g.id)}
    <div class="day">
      <h3>{g.day}</h3>
      {#each g.events as e (e.id)}
        <div
          class="row"
          class:signal={e.class === 'signal'}
          onclick={() => openTicket(e.ticket_ulid)}
        >
          {#if e.class === 'signal'}<span class="glyph">{kindGlyph(e.kind)}</span>{/if}
          <span class="muted kind">{e.kind}</span>
          <span class="line" class:dim={e.class !== 'signal'}>{eventText(e)}</span>
          <span class="actor" class:human={e.actor_type === 'human'}>{e.actor}</span>
          <span class="muted ts" title={fmtTs(e.ts)}>{relTs(e.ts)}</span>
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
      {loading ? 'loading…' : 'load older'}
    </button>
  {/if}
</section>

<style>
  .wrap { max-width: 860px; margin: 0 auto; }
  h2 { font-size: 15px; margin: 0 0 10px; }
  .filters {
    display: flex; gap: 14px; align-items: baseline; margin-bottom: 14px;
    flex-wrap: wrap; font-size: 12px;
  }
  .filters label { display: flex; gap: 6px; align-items: center; }
  select {
    font: inherit; font-size: 12px; color: var(--text);
    background: var(--bg-raised); border: 1px solid var(--line-strong);
    border-radius: 3px; padding: 4px 6px; min-height: 36px;
  }
  .count { margin-left: auto; font-size: 11px; }
  .day h3 {
    font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em;
    color: var(--text-dim); margin: 16px 0 4px;
    border-bottom: 1px solid var(--line); padding-bottom: 3px;
  }
  .row {
    display: flex; gap: 10px; align-items: baseline;
    padding: 6px 4px; border-bottom: 1px solid var(--line);
    cursor: pointer; font-size: 12px;
  }
  .row:hover { background: var(--bg-inset); }
  .row.signal { min-height: 44px; align-items: center; }
  .glyph { color: var(--accent); }
  .kind { min-width: 80px; font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; }
  .line { flex: 1; }
  .line.dim { font-size: 11px; }
  .actor {
    font-size: 10px; text-transform: uppercase; border: 1px solid var(--line-strong);
    padding: 0 5px; border-radius: 3px; color: var(--text-dim);
  }
  .actor.human { border-color: var(--accent); color: var(--accent); }
  .ts { font-size: 10px; white-space: nowrap; }
  .older {
    display: block; margin: 14px auto; padding: 8px 22px;
  }
  .empty { padding: 20px 0; }
</style>
