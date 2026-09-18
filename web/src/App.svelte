<script>
  import TopBar from './lib/TopBar.svelte';
  import Rail from './lib/Rail.svelte';
  import Ledger from './lib/Ledger.svelte';
  import Board from './lib/Board.svelte';
  import Ticket from './lib/Ticket.svelte';
  import Analytics from './lib/Analytics.svelte';
  import { view, show, theme, setTheme, loadBoard } from './lib/state.svelte.js';
  import { connectStream, live } from './lib/live.svelte.js';
  import { onMount } from 'svelte';

  onMount(() => {
    document.documentElement.dataset.theme = theme.name;
    loadBoard();
    connectStream();
  });
</script>

<TopBar />
<div class="shell">
  <Rail />
  <main>
    {#if view.name === 'ledger'}<Ledger />{/if}
    {#if view.name === 'board'}<Board />{/if}
    {#if view.name === 'ticket'}<Ticket />{/if}
    {#if view.name === 'analytics'}<Analytics />{/if}
  </main>
</div>

<style>
  .shell {
    display: flex;
    height: calc(100vh - 46px);
  }
  main {
    flex: 1;
    overflow: auto;
    padding: 16px 20px;
  }
</style>
