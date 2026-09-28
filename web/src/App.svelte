<script>
  import Mast from './lib/Mast.svelte';
  import Prompt from './lib/Prompt.svelte';
  import Arcs from './lib/Arcs.svelte';
  import Timeline from './lib/Timeline.svelte';
  import Board from './lib/Board.svelte';
  import Time from './lib/Time.svelte';
  import Ticket from './lib/Ticket.svelte';
  import Journal from './lib/Journal.svelte';
  import { view, ui, show, loadBoard, loadArcs, routeFromLocation, onRouteChange } from './lib/state.svelte.js';
  import { connectStream } from './lib/live.svelte.js';
  import { viewForKey } from './lib/prompt.js';
  import { onMount } from 'svelte';

  onMount(() => {
    document.documentElement.dataset.mode = ui.mode;
    routeFromLocation();
    onRouteChange();
    loadBoard();
    loadArcs();
    connectStream();
  });

  // digit keys switch views, the way F-keys did on the machines this look
  // remembers; a field keeps its digits
  function onKey(e) {
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    const tag = e.target?.tagName;
    if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA' || e.target?.isContentEditable) return;
    const v = viewForKey(e.key);
    if (v) show(v);
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="crt-scan" aria-hidden="true"></div>
<div class="crt-vignette" aria-hidden="true"></div>

<div class="shell crt-flicker">
  <Mast />
  <main>
    {#if view.name === 'arcs'}<Arcs />{/if}
    {#if view.name === 'timeline'}<Timeline />{/if}
    {#if view.name === 'board'}<Board />{/if}
    {#if view.name === 'time'}<Time />{/if}
    {#if view.name === 'journal'}<Journal />{/if}
    {#if view.name === 'ticket'}<Ticket />{/if}
  </main>
  <Prompt />
</div>

<style>
  .shell {
    height: 100%;
    display: grid;
    grid-template-rows: auto 1fr auto;
    max-width: 1440px;
    margin: 0 auto;
    background: var(--iron);
    border-left: 1px solid var(--rust-2);
    border-right: 1px solid var(--rust-2);
    box-shadow: var(--shadow);
  }
  main {
    min-height: 0;
    overflow: auto;
    padding: 18px 18px 12px;
  }
  @media (max-width: 700px) {
    .shell { border-left: none; border-right: none; box-shadow: none; }
    main { padding: 14px 16px 10px; }
  }
</style>
