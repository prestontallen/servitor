<script>
  import { view, show, ui, setMode, setAccent } from './state.svelte.js';
  import { live } from './live.svelte.js';

  const statusText = $derived(
    { connecting: 'connecting…', live: 'connected', down: 'signal lost' }[live.status]
  );
</script>

<header>
  <h1 class="brand" onclick={() => show('arcs')}>servitor</h1>
  <nav>
    <button class:active={view.name === 'arcs' || view.name === 'timeline'} onclick={() => show('arcs')}>Arcs</button>
    <button class:active={view.name === 'board'} onclick={() => show('board')}>Board</button>
    <button class:active={view.name === 'analytics'} onclick={() => show('analytics')}>Activity</button>
  </nav>
  <div class="spacer"></div>
  <span class="conn" class:down={live.status === 'down'} title="SSE change stream">
    <span class="dot" class:on={live.status === 'live'}></span>
    <span class="conn-text">{statusText}</span>
  </span>
  <div class="pickers">
    <button
      class="mode"
      onclick={() => setMode(ui.mode === 'dark' ? 'light' : 'dark')}
      title="Toggle light/dark"
      aria-label="Toggle light/dark mode"
    >
      {ui.mode === 'dark' ? '☀' : '☾'}
    </button>
    <button
      class="accent"
      onclick={() => setAccent(ui.accent === 'forge' ? 'auspex' : 'forge')}
      title="Toggle accent"
      aria-label="Toggle accent color"
    >
      <span class="swatch"></span>
    </button>
  </div>
</header>

<style>
  header {
    display: flex;
    align-items: center;
    gap: 14px;
    height: 46px;
    padding: 0 16px;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--line);
  }
  .brand {
    font-size: 15px;
    margin: 0;
    cursor: pointer;
    font-weight: 600;
  }
  nav { display: flex; gap: 6px; }
  .spacer { flex: 1; }
  .conn {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--text-dim);
    font-size: 11px;
  }
  .conn.down { color: var(--fail); }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--line-strong);
  }
  .dot.on { background: var(--ok); }
  .pickers { display: flex; gap: 6px; }
  .accent {
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .swatch {
    width: 12px;
    height: 12px;
    border-radius: 3px;
    background: var(--accent);
  }
  @media (max-width: 700px) {
    header { gap: 8px; padding: 0 10px; }
    .conn-text { display: none; }
    .brand { font-size: 14px; }
  }
</style>
