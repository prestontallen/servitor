<script>
  import { board, openTicket, wordClass } from './state.svelte.js';

  const lanes = ['blocked', 'active', 'queued'];
  const byLane = $derived(
    Object.fromEntries(lanes.map((s) => [s, board.cards.filter((c) => c.status === s)]))
  );
</script>

<section class="lanes">
  {#each lanes as lane}
    <div class="lane">
      <h2>{lane} · {byLane[lane].length}</h2>
      {#each byLane[lane] as c (c.ulid)}
        <div class="card panel" onclick={() => openTicket(c.ulid)}>
          <div class="title">{c.title || c.slug}</div>
          <div class="meta">
            <span class="muted">{c.slug}</span>
            {#if c.card_word}<span class="badge {wordClass(c.card_word)}">{c.card_word}</span>{/if}
            {#if c.status === 'blocked'}<span class="badge blocked_on">on {c.blocked_on}</span>{/if}
          </div>
        </div>
      {:else}
        <p class="muted empty">—</p>
      {/each}
    </div>
  {/each}
</section>

<style>
  .lanes {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 18px;
    max-width: 980px;
    margin: 0 auto;
  }
  h2 {
    font-size: 12px;
    margin: 0 0 10px;
    border-bottom: 1px solid var(--line-strong);
    padding-bottom: 5px;
  }
  .card {
    padding: 9px 11px;
    margin-bottom: 10px;
    cursor: pointer;
  }
  .card:hover { border-color: var(--rust); }
  .title { margin-bottom: 4px; }
  .meta { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; font-size: 11px; }
  .empty { margin: 0; }
</style>
