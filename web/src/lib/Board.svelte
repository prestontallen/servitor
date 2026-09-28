<script>
  // The board: five lanes by card word, queued at the left, and the cards
  // that wait on a human in a red rail, oldest wait first.
  import { board, openTicket, ageOfState, relTs } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import { attention, laneOf, LANES } from './inbox.js';

  const byLane = $derived.by(() => {
    const out = Object.fromEntries([...LANES, 'blocked'].map((l) => [l, []]));
    for (const c of board.cards) out[laneOf(c)].push(c);
    out.blocked.sort((a, b) => new Date(a.blocked_since || a.updated_at) - new Date(b.blocked_since || b.updated_at));
    return out;
  });
  // cards an attention rule fires on get the lit edge
  const waiting = $derived(new Map(attention(board.cards, live.events).map((r) => [r.ulid, r])));
  const inMotion = $derived(board.cards.length - byLane.blocked.length);

  const short = (a) => (a || '').replace(/^\w+:/, '');
  const GLYPH = { queued: '◇', active: '◆', blocked: '☠', done: '■', dropped: '✕' };

  // criteria as a block meter: eight cells, filled to pass/total
  const CELLS = 8;
  function meter(c) {
    if (!c.criteria_total) return null;
    const f = Math.round((CELLS * c.criteria_pass) / c.criteria_total);
    return { on: '█'.repeat(f), off: '░'.repeat(CELLS - f), all: c.criteria_pass === c.criteria_total };
  }
</script>

{#snippet card(c, lane)}
  {@const a = ageOfState(c.status, c.blocked_since, c.updated_at)}
  {@const w = waiting.get(c.ulid)}
  {@const m = meter(c)}
  <div class="card" class:you={!!w && lane !== 'blocked'} class:blocked={lane === 'blocked'} class:stale={a?.stale} onclick={() => openTicket(c.ulid)} onkeydown={(k) => k.key === 'Enter' && openTicket(c.ulid)} role="link" tabindex="0" title={c.title || c.slug}>
    <div class="slug"><span>{c.slug}</span><i>{c.ulid.slice(0, 8)}</i></div>
    <div class="t">{c.title || c.slug}</div>
    <div class="meter" title="criteria pass / total">
      {#if m}<b>{m.on}</b><span class="off">{m.off}</span> <span class:verd={m.all}>{c.criteria_pass}/{c.criteria_total}</span>{:else}<span class="muted">no criteria</span>{/if}
    </div>
    <div class="stamp">
      {#if lane === 'blocked'}
        <span class="blood">☠ on {c.blocked_on}</span> · {relTs(c.blocked_since || c.updated_at)}
      {:else if c.active_by}
        ◆ {short(c.active_by)} · {relTs(c.active_since || c.updated_at)}
      {:else}
        {GLYPH[c.status] || '·'} {relTs(c.updated_at)}{#if a?.stale}{' · idle'}{/if}
      {/if}
      {#if w && lane !== 'blocked'}<span class="why">· {w.why}</span>{/if}
    </div>
  </div>
{/snippet}

<section class="board">
  <div class="frame lanesframe">
    <span class="cap">Lanes · by card word</span>
    <span class="cap right">{inMotion} card{inMotion === 1 ? '' : 's'} in motion</span>
    {#if board.error}<p class="muted">board unreachable: {board.error}</p>{/if}
    <div class="lanes">
      {#each LANES as lane (lane)}
        <div class="lane {lane}">
          <h3><span>{lane}</span><span class="n">{byLane[lane].length}</span></h3>
          {#each byLane[lane] as c (c.ulid)}
            {@render card(c, lane)}
          {:else}
            <p class="muted empty">— empty —</p>
          {/each}
        </div>
      {/each}
    </div>
  </div>
  <div class="frame alarm rail">
    <span class="cap">☠ Awaiting the human</span>
    <span class="cap right">{byLane.blocked.length}</span>
    {#each byLane.blocked as c (c.ulid)}
      {@render card(c, 'blocked')}
    {:else}
      <p class="muted empty">nothing waits on you</p>
    {/each}
    <p class="why-rail">Blocked cards sit here until a human acts. Nothing an agent does moves them.</p>
  </div>
</section>

<style>
  .board { display: grid; grid-template-columns: minmax(0, 1fr) 270px; gap: 14px; }
  .lanes {
    display: grid; grid-auto-flow: column; grid-auto-columns: minmax(200px, 1fr);
    gap: 10px; overflow-x: auto; padding-bottom: 4px;
  }
  .lane { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
  .lane h3 {
    margin: 0; font-size: 10px; line-height: 1; letter-spacing: 0.24em; text-transform: uppercase;
    color: var(--phos-dim); border-bottom: 1px solid var(--rust-2); padding-bottom: 6px;
    display: flex; justify-content: space-between; font-weight: 500;
  }
  .lane h3 .n { color: var(--bone-dim); letter-spacing: 0; }
  .lane.queued h3 { color: var(--bone-dim); }
  .lane.shaping h3 { color: var(--w-shaping); }
  .lane.building h3 { color: var(--w-building); }
  .lane.checking h3 { color: var(--w-checking); }
  .lane.shipping h3 { color: var(--w-shipping); }
  .empty { margin: 0; font-size: 11px; }

  .card { border: 1px solid var(--rust); padding: 7px 8px 6px; background: var(--iron-2); cursor: pointer; min-width: 0; }
  .card:hover { border-color: var(--phos-dim); }
  .card.you { border-color: var(--phos-dim); box-shadow: 0 0 12px var(--glow-soft); }
  .card.stale { border-style: dashed; }
  .slug { color: var(--phos); font-weight: 500; display: flex; justify-content: space-between; gap: 6px; min-width: 0; }
  .slug span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .slug i { font-style: normal; color: var(--bone-dim); font-weight: 400; font-size: 10px; letter-spacing: 0.06em; flex: none; }
  .t {
    font-size: 11px; color: var(--bone-dim); margin: 3px 0 6px; line-height: 1.4;
    display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
  }
  .meter { font-size: 11px; letter-spacing: 0.02em; color: var(--phos-dim); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .meter b { color: var(--phos); font-weight: 400; }
  .meter .off { color: var(--bone-dim); opacity: 0.6; }
  .stamp { font-size: 11px; color: var(--phos-dim); margin-top: 3px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .why { color: var(--bone-dim); }
  .card.blocked { border-color: var(--blood-dim); background: linear-gradient(90deg, color-mix(in srgb, var(--blood) 8%, transparent), transparent 60%); }
  .card.blocked .slug { color: var(--bone); }
  .card.blocked .stamp { color: var(--blood); }

  .rail { display: flex; flex-direction: column; gap: 8px; align-self: start; }
  .why-rail { font-size: 11px; color: var(--bone-dim); line-height: 1.5; margin: 4px 0 0; }

  @media (max-width: 900px) {
    .board { grid-template-columns: 1fr; }
  }
</style>
