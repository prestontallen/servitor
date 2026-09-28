<script>
  // Arcs as a tree: each arc is a root with its members hanging off
  // box-drawing branches; what waits on the human sits in a red frame
  // above them, oldest wait first (the same rule the board uses).
  import { arcs, board, fold, setFold, openArc, openTicket, loadArcs, relTs, ageOfState, fmtTs } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import { attention } from './inbox.js';

  // browse state: whose done members are shown. Gestures, not preferences.
  let doneOpen = $state({});

  // refresh on live changes while this view is shown
  $effect(() => {
    void live.changeCount;
    loadArcs();
  });

  const ROLLUP_LABEL = { queued: 'queued', active: 'in motion', blocked: 'blocked', done: 'done' };
  const GLYPH = { queued: '◇', active: '◆', blocked: '☠', done: '■', dropped: '✕' };

  // attention: what needs the human — the same rule the inbox uses
  const attentionRows = $derived(attention(board.cards, live.events));

  // finished arcs leave the main list; they are still one tap away below it
  const liveArcs = $derived(arcs.list.filter((a) => a.rollup !== 'done'));
  const doneArcs = $derived(arcs.list.filter((a) => a.rollup === 'done'));

  // page-level folds: needs-you open by default, finished closed by default
  const needsOpen = $derived(fold.needs !== false);
  const finishedOpen = $derived(fold.finished === true);

  function members(a) {
    const open = [], done = [];
    for (const m of a.members) (m.status === 'done' || m.status === 'dropped' ? done : open).push(m);
    return { open, done };
  }
</script>

{#snippet memberRow(m, last)}
  {@const ma = ageOfState(m.status, m.blocked_since, m.updated_at)}
  <li class="m s-{m.status}" onclick={() => openTicket(m.ulid)} onkeydown={(k) => k.key === 'Enter' && openTicket(m.ulid)} role="link" tabindex="0">
    <span class="br">{last ? '└─' : '├─'}</span>
    <span class="g">{GLYPH[m.status] || '·'}</span>
    <span class="slug">{m.slug}</span>
    <span class="ttl">{m.title || ''}</span>
    {#if ma}<span class="age" class:stale={ma.stale} title={fmtTs(ma.ts)}>{relTs(ma.ts)}</span>{/if}
  </li>
{/snippet}

{#snippet arcTree(a)}
  {@const ms = members(a)}
  {@const showDone = !!doneOpen[a.ulid]}
  {@const rows = showDone ? [...ms.open, ...ms.done] : ms.open}
  <article class="tree roll-{a.rollup}">
    <div class="root">
      <span class="g">{a.rollup === 'active' ? '◆' : GLYPH[a.rollup] || '◇'}</span>
      <button class="arc" onclick={() => openArc(a.ulid)}>{a.slug}</button>
      <span class="st r-{a.rollup}">{ROLLUP_LABEL[a.rollup] || a.rollup}</span>
      <span class="meta">· {ms.done.length}/{a.members.length} done · last {relTs(a.last_activity)}</span>
      <button class="timeline" onclick={() => openArc(a.ulid)}>timeline ↗</button>
    </div>
    <div class="ttl-row"><span class="br">│</span><span class="ttl">{a.title || ''}</span></div>
    <ul class="members">
      {#each rows as m, i (m.ulid)}
        {@render memberRow(m, i === rows.length - 1 && (showDone || !ms.done.length))}
      {/each}
      {#if ms.done.length && !showDone}
        <li class="m fold" onclick={() => (doneOpen[a.ulid] = true)} onkeydown={(k) => k.key === 'Enter' && (doneOpen[a.ulid] = true)} role="button" tabindex="0">
          <span class="br">└─</span><span class="g">▸</span><span class="slug muted">{ms.done.length} finished</span>
        </li>
      {:else if ms.done.length && showDone}
        <li class="m fold" onclick={() => (doneOpen[a.ulid] = false)} onkeydown={(k) => k.key === 'Enter' && (doneOpen[a.ulid] = false)} role="button" tabindex="0">
          <span class="br">&nbsp;&nbsp;</span><span class="g">▾</span><span class="slug muted">hide finished</span>
        </li>
      {/if}
      {#if !a.members.length}
        <li class="m"><span class="br">└─</span><span class="g"></span><span class="slug muted">no members</span></li>
      {/if}
    </ul>
  </article>
{/snippet}

<section class="wrap">
  {#if attentionRows.length}
    <div class="frame alarm attention">
      <span class="cap">☠ Awaiting the human · {attentionRows.length}</span>
      <button class="cap right toggle" onclick={() => setFold('needs', !needsOpen)}>{needsOpen ? '▾ fold' : '▸ unfold'}</button>
      {#if needsOpen}
        <ul class="needs">
          {#each attentionRows as it (it.ulid)}
            <li class="sev-{it.sev}" onclick={() => openTicket(it.ulid)} onkeydown={(k) => k.key === 'Enter' && openTicket(it.ulid)} role="link" tabindex="0">
              <span class="why">{it.why}</span>
              <span class="a-title">{it.title}</span>
              <span class="since">{relTs(it.since)}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  <div class="frame">
    <span class="cap">Arcs · campaigns of tickets</span>
    <span class="cap right">{liveArcs.length} in play</span>
    {#if arcs.error}
      <p class="muted">arcs unavailable: {arcs.error}</p>
    {/if}
    {#each liveArcs as a (a.ulid)}
      {@render arcTree(a)}
    {/each}
    {#if doneArcs.length}
      <button class="foldbar" onclick={() => setFold('finished', !finishedOpen)}>{finishedOpen ? '▾' : '▸'} finished · {doneArcs.length}</button>
      {#if finishedOpen}
        {#each doneArcs as a (a.ulid)}
          {@render arcTree(a)}
        {/each}
      {/if}
    {/if}
    {#if !arcs.list.length && !arcs.error}
      <p class="muted empty">
        no arcs yet — point a ticket at another with<br />
        <code>servitor set &lt;ref&gt; parent=&lt;arc-ulid&gt;</code>
      </p>
    {/if}
  </div>
</section>

<style>
  .wrap { max-width: 980px; margin: 0 auto; display: flex; flex-direction: column; gap: 18px; }
  .frame { padding: 16px 14px 12px; }
  .toggle { border: none; padding: 0 6px; min-height: 0; font-size: 10px; letter-spacing: 0.12em; }

  /* ---- the alarm: what waits on the human ---- */
  .needs { list-style: none; margin: 0; padding: 0; }
  .needs li {
    display: flex; gap: 12px; align-items: baseline; padding: 6px 2px;
    border-bottom: 1px dashed var(--rust); cursor: pointer; font-size: 12px;
  }
  .needs li:last-child { border-bottom: none; }
  .needs li:hover { background: var(--iron-2); }
  .why { color: var(--phos-dim); white-space: nowrap; }
  .sev-high .why { color: var(--blood); }
  .a-title { flex: 1; min-width: 0; color: var(--bone); }
  .since { font-size: 10px; white-space: nowrap; color: var(--bone-dim); }

  /* ---- the trees ---- */
  .tree { font-size: 12px; line-height: 1.6; }
  .tree + .tree { margin-top: 16px; }
  .root { display: flex; gap: 8px; align-items: baseline; flex-wrap: wrap; }
  .g { color: var(--phos-dim); width: 14px; text-align: center; flex: none; }
  .roll-blocked .root .g { color: var(--blood); }
  .roll-done .root .g { color: var(--verdigris); }
  .arc { border: none; padding: 0; min-height: 0; min-width: 0; font: inherit; color: var(--phos); font-weight: 500; text-transform: none; letter-spacing: 0; }
  .arc:hover { text-decoration: underline; }
  .st { font-size: 10px; letter-spacing: 0.16em; text-transform: uppercase; color: var(--bone-dim); }
  .st.r-active { color: var(--phos); }
  .st.r-blocked { color: var(--blood); }
  .st.r-done { color: var(--verdigris); }
  .meta { color: var(--bone-dim); font-size: 11px; }
  .timeline { margin-left: auto; border: none; padding: 0; min-height: 0; font-size: 10px; color: var(--bone-dim); }
  .timeline:hover { color: var(--phos); }
  .ttl-row { display: flex; gap: 8px; }
  .br { color: var(--rust-2); flex: none; white-space: pre; }
  .ttl { color: var(--bone-dim); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .members { list-style: none; margin: 0; padding: 0; }
  .m { display: flex; gap: 8px; align-items: baseline; cursor: pointer; min-width: 0; padding: 1px 0; }
  .m:hover .slug { text-decoration: underline; }
  .m .slug { color: var(--bone); flex: none; }
  .m.s-active .slug, .m.s-active .g { color: var(--phos); }
  .m.s-active .ttl { color: var(--bone); }
  .m.s-blocked .slug, .m.s-blocked .g { color: var(--blood); }
  .m.s-done .slug, .m.s-done .g { color: var(--bone-dim); }
  .m.s-done .g { color: var(--verdigris); }
  .m.s-dropped .slug { color: var(--bone-dim); text-decoration: line-through; }
  .m .ttl { flex: 1; }
  .age { font-size: 10px; color: var(--bone-dim); white-space: nowrap; flex: none; }
  .age.stale { color: var(--phos-dim); }
  .m.fold .g { color: var(--bone-dim); }
  .foldbar { display: block; margin-top: 14px; border: none; padding: 0; min-height: 0; font-size: 10px; letter-spacing: 0.16em; color: var(--bone-dim); }
  .foldbar:hover { color: var(--phos); }
  .foldbar + .tree { margin-top: 12px; }
  .empty { line-height: 1.8; }
  code { color: var(--phos); }
  @media (max-width: 700px) {
    .frame { padding: 14px 10px 10px; }
    .needs li { flex-wrap: wrap; }
    .why { flex-basis: 100%; }
    .m .ttl { display: none; }
    .m .slug { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .timeline { margin-left: 0; }
  }
  @media (pointer: coarse) {
    .m { min-height: 32px; align-items: center; }
  }
</style>
