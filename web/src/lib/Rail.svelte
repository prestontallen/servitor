<script>
  import { board, openTicket, wordClass } from './state.svelte.js';

  let filter = $state('all');
  const statuses = ['all', 'blocked', 'active', 'queued'];

  const shown = $derived(
    filter === 'all' ? board.cards : board.cards.filter((c) => c.status === filter)
  );
</script>

<aside class="panel rail">
  <h3>Machine bank</h3>
  <div class="filters">
    {#each statuses as s}
      <button class:active={filter === s} onclick={() => (filter = s)}>{s}</button>
    {/each}
  </div>
  {#if board.error}
    <p class="muted">board unavailable: {board.error}</p>
  {/if}
  {#each shown as c (c.ulid)}
    <div class="card" onclick={() => openTicket(c.ulid)}>
      <div class="title">{c.title || c.slug}</div>
      <div class="meta">
        <span class="muted">{c.slug}</span>
        {#if c.card_word}<span class="badge {wordClass(c.card_word)}">{c.card_word}</span>{/if}
        {#if c.status === 'blocked'}<span class="badge blocked_on">on {c.blocked_on}</span>{/if}
      </div>
    </div>
  {:else}
    <p class="muted">no cards</p>
  {/each}
</aside>

<style>
  .rail {
    width: 270px;
    min-width: 270px;
    margin: 16px 0 16px 16px;
    padding: 12px;
    overflow: auto;
  }
  h3 { margin: 0 0 10px; font-size: 12px; }
  .filters { display: flex; gap: 4px; margin-bottom: 12px; }
  .card {
    border: 1px solid var(--line-strong);
    border-left: 3px solid var(--brass-dim);
    padding: 7px 9px;
    margin-bottom: 8px;
    cursor: pointer;
    background: var(--bg-inset);
  }
  .card:hover { border-left-color: var(--rust); }
  .title { margin-bottom: 3px; }
  .meta { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; font-size: 11px; }
</style>
