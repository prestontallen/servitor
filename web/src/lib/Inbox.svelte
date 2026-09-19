<script>
  // What is waiting on the human, oldest first, with the next CLI action.
  import { board, openTicket, relTs, fmtTs } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import { attention } from './inbox.js';

  const rows = $derived(attention(board.cards, live.events));
</script>

<section class="wrap">
  <h2>Inbox <span class="muted n">{rows.length ? `${rows.length} waiting on you` : ''}</span></h2>
  {#if board.error}
    <p class="muted">board unavailable: {board.error}</p>
  {:else if rows.length === 0}
    <p class="muted empty">nothing waiting on you</p>
  {:else}
    <ol class="rows">
      {#each rows as r (r.ulid)}
        <li class="row sev-{r.sev} panel">
          <div class="wait" title={fmtTs(r.since)}>{relTs(r.since).replace(' ago', '')}</div>
          <div class="body">
            <button class="title" onclick={() => openTicket(r.ulid)}>{r.title}</button>
            <div class="why">{r.why}</div>
            <div class="hint muted">{r.hint}</div>
          </div>
          <code class="action">{r.action}</code>
        </li>
      {/each}
    </ol>
  {/if}
</section>

<style>
  .wrap { max-width: 860px; margin: 0 auto; }
  h2 { font-size: 15px; margin: 0 0 12px; display: flex; gap: 10px; align-items: baseline; }
  .n { font-size: 11px; font-weight: 400; }
  .rows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .row { display: grid; grid-template-columns: 64px 1fr auto; gap: 12px; align-items: center; padding: 10px 12px; border-left: 3px solid var(--warn); }
  .row.sev-high { border-left-color: var(--fail); }
  .wait { font-size: 14px; font-weight: 600; color: var(--warn); font-variant-numeric: tabular-nums; }
  .sev-high .wait { color: var(--fail); }
  .title { border: none; background: none; padding: 0; text-align: left; color: var(--text); font-size: 13px; cursor: pointer; min-height: 0; }
  .title:hover { color: var(--accent); }
  .why { font-size: 12px; color: var(--text-dim); margin-top: 2px; }
  .hint { font-size: 11px; }
  .action { font-size: 11px; color: var(--accent); background: var(--bg-inset); padding: 4px 8px; border-radius: 3px; white-space: nowrap; user-select: all; }
  .empty { padding: 24px 0; }
  @media (max-width: 700px) {
    .row { grid-template-columns: 52px 1fr; }
    .action { grid-column: 1 / -1; white-space: normal; word-break: break-all; }
  }
</style>
