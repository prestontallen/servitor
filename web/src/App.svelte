<script>
  import TopBar from './lib/TopBar.svelte';
  import Arcs from './lib/Arcs.svelte';
  import Timeline from './lib/Timeline.svelte';
  import Board from './lib/Board.svelte';
  import Ticket from './lib/Ticket.svelte';
  import Journal from './lib/Journal.svelte';
  import Analytics from './lib/Analytics.svelte';
  import { view, ui, loadBoard, loadArcs, routeFromLocation, onRouteChange } from './lib/state.svelte.js';
  import { connectStream } from './lib/live.svelte.js';
  import { onMount } from 'svelte';

  onMount(() => {
    document.documentElement.dataset.mode = ui.mode;
    document.documentElement.dataset.accent = ui.accent;
    routeFromLocation();
    onRouteChange();
    loadBoard();
    loadArcs();
    connectStream();
  });
</script>

<TopBar />
<div class="shell">
  <main>
    {#if view.name === 'arcs'}<Arcs />{/if}
    {#if view.name === 'timeline'}<Timeline />{/if}
    {#if view.name === 'board'}<Board />{/if}
    {#if view.name === 'journal'}<Journal />{/if}
    {#if view.name === 'ticket'}<Ticket />{/if}
    {#if view.name === 'analytics'}<Analytics />{/if}
  </main>
</div>

<style>
  .shell {
    height: calc(100vh - 46px);
  }
  main {
    height: 100%;
    overflow: auto;
    padding: 16px 20px;
  }
  @media (max-width: 700px) {
    main { padding: 10px 10px 16px; }
  }
</style>
