<script>
  import { board, openTicket, wordClass, ageOfState, relTs } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import { attention, sparkline, laneOf, LANES } from './inbox.js';

  const byLane = $derived.by(() => {
    const out = Object.fromEntries([...LANES, 'blocked'].map((l) => [l, []]));
    for (const c of board.cards) out[laneOf(c)].push(c);
    return out;
  });
  // cards an inbox rule fires on get the amber edge
  const waiting = $derived(new Map(attention(board.cards, live.events).map((r) => [r.ulid, r])));
  const sparks = $derived(Object.fromEntries(board.cards.map((c) => [c.ulid, sparkline(live.events, c.ulid)])));

  const short = (a) => (a || '').replace(/^\w+:/, '');
  function points(counts) {
    const max = Math.max(1, ...counts);
    return counts.map((v, i) => `${i * 8},${15 - (v / max) * 13}`).join(' ');
  }
  function py(counts, i) {
    const max = Math.max(1, ...counts);
    return 15 - (counts[i] / max) * 13;
  }
</script>

<section class="board">
  {#each [...LANES, 'blocked'] as lane (lane)}
    <div class="lane" class:rail={lane === 'blocked'}>
      <h2><span>{lane}</span><span class="n">{byLane[lane].length}</span></h2>
      {#each byLane[lane] as c (c.ulid)}
        {@const a = ageOfState(c.status, c.blocked_since, c.updated_at)}
        {@const w = waiting.get(c.ulid)}
        {@const s = sparks[c.ulid]}
        <div class="card panel" class:stale={a?.stale} class:you={!!w} class:blocked={lane === 'blocked'} onclick={() => openTicket(c.ulid)}>
          {#if s}
            <svg class="spark" viewBox="0 0 48 16" aria-hidden="true">
              <polyline points={points(s.counts)} />
              {#each s.human as h, i}{#if h}<circle cx={i * 8} cy={py(s.counts, i)} r="2" />{/if}{/each}
            </svg>
          {/if}
          <div class="title">{c.title || c.slug}</div>
          <div class="meta">
            <span class="muted slug">{c.slug}</span>
            {#if lane === 'blocked' && c.card_word}<span class="badge {wordClass(c.card_word)}">{c.card_word}</span>{/if}
            {#if lane === 'blocked' && c.blocked_on}<span class="badge blocked_on">on {c.blocked_on}</span>{/if}
            {#if c.active_by}<span class="hold">{short(c.active_by)}</span>{/if}
            {#if a}
              <span class="age" class:stale={a.stale} title={c.status === 'blocked' ? 'blocked since' : 'last activity'}>
                {c.status === 'blocked' ? '⏸' : '·'} {relTs(a.ts)}
              </span>
            {/if}
          </div>
          {#if w}<div class="why">{w.hint}</div>{/if}
        </div>
      {:else}
        <p class="muted empty">—</p>
      {/each}
    </div>
  {/each}
</section>

<style>
  .board {
    display: grid;
    grid-template-columns: repeat(5, minmax(150px, 1fr)) minmax(150px, 0.9fr);
    gap: 14px;
    max-width: 1240px;
    margin: 0 auto;
  }
  h2 {
    display: flex; justify-content: space-between;
    font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em;
    margin: 0 0 10px; border-bottom: 1px solid var(--line-strong); padding-bottom: 5px;
  }
  h2 .n { color: var(--text-dim); font-weight: 400; }
  .rail { border-left: 1px dashed var(--line-strong); padding-left: 14px; }
  .rail h2 { color: var(--fail); border-bottom-color: var(--fail); }
  .card { position: relative; padding: 8px 10px; margin-bottom: 10px; cursor: pointer; }
  .card:hover { border-color: var(--accent); }
  .card.stale { border-style: dashed; }
  .card.you { border-color: var(--warn); box-shadow: inset 3px 0 0 var(--warn); }
  .card.blocked { box-shadow: inset 3px 0 0 var(--fail); }
  .spark { position: absolute; right: 8px; top: 8px; width: 48px; height: 16px; }
  .spark polyline { fill: none; stroke: var(--accent); stroke-width: 1.5; opacity: 0.8; }
  .spark circle { fill: var(--accent); }
  .title { margin: 0 52px 5px 0; line-height: 1.3; }
  .meta { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; font-size: 11px; }
  .slug { font-size: 10px; }
  .hold { font-size: 10px; color: var(--text); border: 1px solid var(--line-strong); border-radius: 3px; padding: 0 5px; }
  .age { color: var(--text-dim); margin-left: auto; white-space: nowrap; }
  .age.stale { color: var(--warn); font-weight: 600; }
  .why { margin-top: 5px; font-size: 11px; color: var(--warn); }
  .empty { margin: 0; }
  @media (max-width: 1100px) {
    .board { grid-template-columns: repeat(3, 1fr); }
    .rail { border-left: none; padding-left: 0; border-top: 1px dashed var(--line-strong); padding-top: 10px; grid-column: 1 / -1; }
  }
  @media (max-width: 700px) {
    .board { grid-template-columns: 1fr; gap: 10px; }
    .rail { grid-column: auto; }
  }
</style>
