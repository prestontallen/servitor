<script>
  import { view, show, theme, setTheme } from './state.svelte.js';
  import { live } from './live.svelte.js';
  import { get } from './api.svelte.js';

  let buckets = $state([]);

  $effect(() => {
    // ambient sparkline: refresh when the ledger grows or view returns
    void live.changeCount;
    void view.name;
    if (view.name === 'analytics') return; // Analytics owns the big chart
    get('/api/analytics?days=30').then((b) => (buckets = b)).catch(() => {});
  });

  const statusText = $derived(
    { connecting: 'connecting…', live: 'connected', down: 'signal lost' }[live.status]
  );
</script>

<header>
  <h1 class="brand" onclick={() => show('ledger')}>servitor</h1>
  <nav>
    <button class:active={view.name === 'ledger'} onclick={() => show('ledger')}>Ledger</button>
    <button class:active={view.name === 'board'} onclick={() => show('board')}>Board</button>
    <button class:active={view.name === 'analytics'} onclick={() => show('analytics')}>Activity</button>
  </nav>
  <svg class="spark" viewBox="0 0 240 24" preserveAspectRatio="none" aria-hidden="true">
    {#each buckets as b, i (b.day)}
      <rect
        x={(i / Math.max(buckets.length, 1)) * 240}
        y={24 - (b.events / Math.max(...buckets.map((x) => x.events), 1)) * 22}
        width={240 / Math.max(buckets.length, 1) - 1}
        height={Math.max(1, (b.events / Math.max(...buckets.map((x) => x.events), 1)) * 22)}
        fill="var(--brass)"
        opacity="0.7"
      />
    {/each}
  </svg>
  <span class="conn" class:down={live.status === 'down'} title="SSE change stream">
    <svg viewBox="0 0 24 24" class="cog" class:spin={live.status === 'live'} aria-hidden="true">
      <circle cx="12" cy="12" r="7" fill="none" stroke="currentColor" stroke-width="2" />
      <circle cx="12" cy="12" r="2.5" fill="currentColor" />
      {#each [0, 60, 120, 180, 240, 300] as a}
        <line
          x1={12 + 9 * Math.cos((a * Math.PI) / 180)} y1={12 + 9 * Math.sin((a * Math.PI) / 180)}
          x2={12 + 12 * Math.cos((a * Math.PI) / 180)} y2={12 + 12 * Math.sin((a * Math.PI) / 180)}
          stroke="currentColor" stroke-width="2"
        />
      {/each}
    </svg>
    {statusText}
  </span>
  <button
    class="theme"
    onclick={() => setTheme(theme.name === 'forge' ? 'auspex' : 'forge')}
    title="Toggle theme"
  >
    {theme.name === 'forge' ? 'auspex' : 'forge'}
  </button>
</header>

<style>
  header {
    display: flex;
    align-items: center;
    gap: 18px;
    height: 46px;
    padding: 0 16px;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--line-strong);
  }
  .brand {
    font-size: 17px;
    margin: 0;
    cursor: pointer;
    letter-spacing: 0.3em;
  }
  nav { display: flex; gap: 6px; }
  .spark {
    flex: 1;
    height: 24px;
    border-bottom: 1px solid var(--line);
  }
  .conn {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--auspex);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .conn.down { color: var(--red); }
  .cog { width: 16px; height: 16px; color: var(--brass); }
  .cog.spin { animation: cogspin 6s linear infinite; }
  @keyframes cogspin { to { transform: rotate(360deg); } }
</style>
