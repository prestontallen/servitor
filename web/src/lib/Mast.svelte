<script>
  // The mast: the wordmark, the rites line (what the machine knows about
  // itself and the board), the mode toggle, and the F-key row.
  import { view, show, ui, setMode, board } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import { VIEWS } from './prompt.js';

  const statusText = $derived({ connecting: 'dialing', live: 'nominal', down: 'signal lost' }[live.status]);
  const counts = $derived.by(() => {
    const c = board.cards;
    return {
      open: c.length,
      active: c.filter((x) => x.status === 'active').length,
      blocked: c.filter((x) => x.status === 'blocked').length
    };
  });
  const LABEL = { board: 'board', time: 'time', arcs: 'arcs', journal: 'journal' };
  const current = $derived(view.name === 'timeline' ? 'arcs' : view.name);
</script>

<header class="mast">
  <div class="row">
    <h1 class="word"><button class="wordbtn" onclick={() => show('board')} aria-label="servitor, to the board">Servitor</button></h1>
    <div class="rites">
      <div><span class="k">daemon</span> servitord <span class="st" class:ok={live.status === 'live'} class:warn={live.status === 'down'}>■ {statusText}</span></div>
      <div><span class="k">cards</span> {counts.open} open · {counts.active} active · <span class:warn={counts.blocked > 0} class:ok={counts.blocked === 0}>{counts.blocked} awaiting the human</span></div>
    </div>
    <button class="mode" onclick={() => setMode(ui.mode === 'dark' ? 'light' : 'dark')} title="Toggle light/dark" aria-label="Toggle light/dark mode">
      {ui.mode === 'dark' ? '☀' : '☾'}
    </button>
  </div>
  <nav class="fkeys" aria-label="views">
    {#each VIEWS as v, i (v)}
      <button class="fkey" aria-current={current === v ? 'true' : 'false'} onclick={() => show(v)}>
        <b>F{i + 1}</b>{LABEL[v]}
      </button>
    {/each}
    {#if view.name === 'ticket'}
      <span class="fkey here"><b>◆</b>dossier</span>
    {/if}
  </nav>
</header>

<style>
  .mast {
    padding: 12px 18px 0;
    border-bottom: 1px solid var(--rust-2);
    background: var(--iron);
  }
  .row { display: flex; align-items: flex-end; gap: 22px; }
  .word { margin: 0; line-height: 0.9; }
  .wordbtn {
    font-family: var(--goth); font-weight: 700; font-size: 38px; line-height: 0.9;
    color: var(--phos); letter-spacing: 0.02em; text-transform: none;
    border: none; padding: 0; min-height: 0; min-width: 0; background: none;
    text-shadow: 0 0 14px var(--glow);
  }
  .wordbtn:hover { color: var(--phos); }
  .rites { font-size: 11px; color: var(--bone-dim); line-height: 1.6; letter-spacing: 0.06em; }
  .rites .k { color: var(--bone); font-weight: 500; margin-right: 4px; }
  .ok { color: var(--verdigris); }
  .warn { color: var(--blood); }
  .st { margin-left: 6px; }
  .mode { margin-left: auto; align-self: flex-start; font-size: 14px; padding: 2px 8px; text-transform: none; letter-spacing: 0; }
  .fkeys { display: flex; flex-wrap: wrap; gap: 6px 14px; margin-top: 10px; }
  .fkey {
    border: none; border-bottom: 1px solid transparent; padding: 3px 4px;
    color: var(--bone-dim); font-size: 12px; letter-spacing: 0.1em; text-transform: uppercase;
    background: none; min-height: 0; min-width: 0;
  }
  .fkey b { color: var(--phos-dim); font-weight: 500; margin-right: 6px; }
  .fkey:hover { color: var(--bone); }
  .fkey[aria-current='true'], .fkey.here { color: var(--phos); border-bottom-color: var(--phos); }
  .fkey[aria-current='true'] b, .fkey.here b { color: var(--phos); }
  @media (max-width: 700px) {
    .mast { padding: 10px 16px 0; }
    .row { gap: 12px; flex-wrap: wrap; }
    .wordbtn { font-size: 30px; }
    .rites { flex-basis: 100%; order: 3; line-height: 1.5; }
    .rites div { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  }
</style>
