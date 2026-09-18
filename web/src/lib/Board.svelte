<script>
  import { board, openTicket, wordClass, ageOfState, relTs } from './state.svelte.js';

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
        {@const a = ageOfState(c.status, c.blocked_since, c.updated_at)}
        <div class="card panel" class:stale={a?.stale} onclick={() => openTicket(c.ulid)}>
          <div class="title">{c.title || c.slug}</div>
          <div class="meta">
            <span class="muted">{c.slug}</span>
            {#if c.card_word}<span class="badge {wordClass(c.card_word)}">{c.card_word}</span>{/if}
            {#if c.status === 'blocked' && c.blocked_on}<span class="badge blocked_on">on {c.blocked_on}</span>{/if}
            {#if a}
              <span class="age" class:stale={a.stale}
                title={c.status === 'blocked' ? 'blocked since' : 'last activity'}>
                {c.status === 'blocked' ? '⏸' : '·'} {relTs(a.ts)}
              </span>
            {/if}
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
  .card { padding: 10px 12px; margin-bottom: 10px; cursor: pointer; }
  .card:hover { border-color: var(--accent); }
  .card.stale { border-color: var(--warn); }
  .title { margin-bottom: 5px; }
  .meta { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; font-size: 11px; }
  .age { color: var(--text-dim); }
  .age.stale { color: var(--warn); font-weight: 600; }
  .empty { margin: 0; }
  @media (max-width: 900px) {
    .lanes { grid-template-columns: 1fr; }
  }
</style>
