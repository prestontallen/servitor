<script>
  import { arcs, board, openArc, openTicket, loadArcs, wordClass, relTs, ageOfState, fmtTs } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import { attention } from './inbox.js';

  let expanded = $state({});

  // refresh on live changes while this view is shown
  $effect(() => {
    void live.changeCount;
    loadArcs();
  });

  const ROLLUP_LABEL = { queued: 'queued', active: 'in motion', blocked: 'blocked', done: 'done' };

  // attention: what needs the human — the same rule the inbox uses
  const attentionRows = $derived(attention(board.cards, live.events));

  function rel(ts) {
    if (!ts) return '—';
    const d = (Date.now() - new Date(ts)) / 1000;
    if (d < 90) return 'just now';
    if (d < 3600) return Math.round(d / 60) + 'm ago';
    if (d < 86400) return Math.round(d / 3600) + 'h ago';
    return Math.round(d / 86400) + 'd ago';
  }
</script>

<section class="wrap">
  <h2>Arcs</h2>
  {#if attentionRows.length}
    <div class="panel attention">
      <h3>needs you</h3>
      <ul>
        {#each attentionRows as it (it.ulid)}
          <li class="sev-{it.sev}" onclick={() => openTicket(it.ulid)}>
            <span class="why">{it.why}</span>
            <span class="a-title">{it.title}</span>
            <span class="muted since">{relTs(it.since)}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
  {#if arcs.error}
    <p class="muted">arcs unavailable: {arcs.error}</p>
  {/if}
  {#each arcs.list as a (a.ulid)}
    <article class="panel arc roll-{a.rollup}">
      <header>
        <button class="title" onclick={() => openArc(a.ulid)}>{a.title || a.slug}</button>
        <span class="rollup r-{a.rollup}">{ROLLUP_LABEL[a.rollup]}</span>
      </header>
      <div class="meta">
        <span class="muted">{a.slug}</span>
        {#if a.card_word}<span class="badge {wordClass(a.card_word)}">{a.card_word}</span>{/if}
        <span class="muted">{a.members.length} member{a.members.length === 1 ? '' : 's'}</span>
        <span class="muted" title={a.last_activity ? fmtTs(a.last_activity) : ''}>active {rel(a.last_activity)}</span>
        <button class="expand" onclick={() => (expanded[a.ulid] = !expanded[a.ulid])}>
          {expanded[a.ulid] ? 'hide' : 'members'}
        </button>
      </div>
      {#if expanded[a.ulid]}
        <ul class="members">
          {#each a.members as m (m.ulid)}
            {@const ma = ageOfState(m.status, m.blocked_since, m.updated_at)}
            <li onclick={() => openTicket(m.ulid)}>
              <span class="m-status s-{m.status}">{m.status}</span>
              <span class="m-title">{m.title || m.slug}</span>
              <span class="muted m-slug">{m.slug}</span>
              {#if ma}
                <span class="age" class:stale={ma.stale}>
                  {m.status === 'blocked' ? '⏸' : '·'} {relTs(ma.ts)}
                </span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </article>
  {:else}
    <p class="muted empty">
      no arcs yet — point a ticket at another with<br />
      <code>servitor set &lt;ref&gt; parent=&lt;arc-ulid&gt;</code>
    </p>
  {/each}
</section>

<style>
  .wrap { max-width: 760px; margin: 0 auto; }
  h2 { font-size: 15px; margin: 0 0 12px; }
  .attention { padding: 10px 14px; margin-bottom: 14px; border-left: 3px solid var(--warn); }
  .attention h3 { margin: 0 0 6px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--warn); }
  .attention ul { list-style: none; margin: 0; padding: 0; }
  .attention li {
    display: flex; gap: 10px; align-items: center; padding: 8px 4px;
    border-bottom: 1px solid var(--line); cursor: pointer; min-height: 44px;
    font-size: 12px;
  }
  .attention li:last-child { border-bottom: none; }
  .attention li:hover { background: var(--bg-inset); }
  .why { color: var(--warn); }
  .sev-high .why { color: var(--fail); }
  .a-title { flex: 1; }
  .since { font-size: 10px; white-space: nowrap; }
  @media (max-width: 700px) {
    .attention li { flex-wrap: wrap; }
    .why { flex-basis: 100%; }
    .a-title { flex-basis: calc(100% - 60px); }
  }
  .arc { padding: 12px 14px; margin-bottom: 12px; border-left: 3px solid var(--line-strong); }
  .arc.roll-active { border-left-color: var(--accent); }
  .arc.roll-blocked { border-left-color: var(--fail); }
  .arc.roll-done { border-left-color: var(--ok); }
  header { display: flex; align-items: baseline; gap: 10px; }
  .title {
    font-size: 14px; font-weight: 600; color: var(--text);
    border: none; background: none; padding: 0; cursor: pointer;
    text-align: left; flex: 1;
  }
  .title:hover { color: var(--accent); }
  .rollup {
    font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em;
    padding: 1px 7px; border-radius: 3px; white-space: nowrap;
  }
  .r-active { background: var(--accent); color: var(--bg-raised); }
  .r-blocked { background: var(--fail); color: var(--bg-raised); }
  .r-done { background: var(--ok); color: var(--bg-raised); }
  .r-queued { border: 1px solid var(--line-strong); color: var(--text-dim); }
  .meta { display: flex; gap: 10px; align-items: center; margin-top: 5px; flex-wrap: wrap; font-size: 11px; }
  .expand { margin-left: auto; font-size: 11px; padding: 2px 8px; }
  .members { list-style: none; margin: 10px 0 0; padding: 0; border-top: 1px solid var(--line); }
  .members li {
    display: flex; gap: 8px; align-items: baseline; padding: 7px 4px;
    border-bottom: 1px solid var(--line); cursor: pointer; min-height: 44px;
    align-items: center;
  }
  .members li:hover { background: var(--bg-inset); }
  .m-status { font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; min-width: 58px; color: var(--text-dim); }
  .s-active { color: var(--accent); }
  .s-blocked { color: var(--fail); }
  .s-done { color: var(--ok); }
  .m-title { flex: 1; }
  .age { color: var(--text-dim); font-size: 11px; white-space: nowrap; }
  .age.stale { color: var(--warn); font-weight: 600; }
  .m-slug { font-size: 10px; }
  .empty { line-height: 1.8; }
  code { color: var(--accent); }
</style>
