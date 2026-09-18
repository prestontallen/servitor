<script>
  import { live } from './live.svelte.js';
  import { kindGlyph, fmtTs, openTicket, eventText } from './state.svelte.js';

  function actorClass(actor) {
    if (!actor) return 'agent';
    return actor.startsWith('human') ? 'human' : 'agent';
  }
</script>

<section class="panel ledger">
  <div class="head">
    <h2>The Ledger</h2>
    <span class="muted">append-only · {live.events.length} events · head {live.head}</span>
  </div>

  {#if live.status === 'down'}
    <p class="muted empty">stream down — showing last known state; retrying…</p>
  {:else if live.events.length === 0}
    <p class="muted empty">awaiting events…</p>
  {/if}

  <div class="stream">
    {#each live.events as e (e.id)}
      <div
        class="evt"
        class:flash={live.flashes[e.id]}
        onclick={() => e.ticket && openTicket(e.ticket)}
      >
        <span class="glyph">{kindGlyph(e.kind)}</span>
        <span class="kind">{e.kind}</span>
        <span class="body">{eventText(e)}</span>
        <span class="actor {actorClass(e.actor)}">{e.actor}</span>
        <span class="ts meta-inline">{e.ts ? fmtTs(e.ts) : ''}</span>
      </div>
    {/each}
  </div>
</section>

<style>
  .ledger {
    max-width: 980px;
    margin: 0 auto;
    padding: 14px 18px;
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    border-bottom: 1px solid var(--line);
    padding-bottom: 8px;
    margin-bottom: 10px;
  }
  h2 { margin: 0; font-size: 15px; }
  .empty { padding: 12px 0; }
  .stream { display: flex; flex-direction: column; }
  .evt {
    display: flex;
    gap: 10px;
    align-items: baseline;
    padding: 5px 4px;
    border-bottom: 1px solid var(--line);
    cursor: pointer;
  }
  .evt:hover { background: var(--bg-inset); }
  .glyph { color: var(--brass); width: 16px; text-align: center; }
  .kind {
    min-width: 110px;
    color: var(--text-dim);
    text-transform: uppercase;
    font-size: 10px;
    letter-spacing: 0.08em;
  }
  .body { flex: 1; }
  .actor {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    padding: 0 6px;
    border: 1px solid var(--line-strong);
  }
  .actor.human { border-color: var(--rust); color: var(--rust); }
  .actor.agent { border-color: var(--brass-dim); color: var(--brass); }
  .ts { min-width: 130px; text-align: right; }
</style>
