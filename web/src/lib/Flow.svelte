<script>
  // Thin adapter over the flow lib (web/src/lib/flow.js): the lib owns the
  // fold, the layered layout and the SVG string; this component only injects
  // it and holds no logic, mirroring the DotLattice adapter pattern.
  import { foldFlow, layoutFlow, svgFlow } from './flow.js';

  let { history = [] } = $props();

  const flow = $derived(foldFlow(history));
  const svg = $derived(flow?.ok ? svgFlow(layoutFlow(flow)) : '');
</script>

{#if flow?.ok}
  <div class="fl" data-testid="flow-card">{@html svg}</div>
{:else if flow}
  <p class="muted" data-testid="flow-error">flow unparsable: {flow.error}</p>
{/if}

<style>
  .fl { overflow-x: auto; }
  .fl :global(.fl-svg) { display: block; max-width: 100%; height: auto; }

  .fl :global(.fl-node rect) {
    fill: var(--bg-inset);
    stroke: var(--line-strong);
    stroke-width: 1;
  }
  .fl :global(.fl-node text) {
    fill: var(--text);
    font-size: 11px;
    font-family: inherit;
  }
  .fl :global(.fl-node.st-active rect) { stroke: var(--accent); fill: color-mix(in srgb, var(--accent) 12%, var(--bg-inset)); }
  .fl :global(.fl-node.st-blocked rect) { stroke: var(--fail); fill: color-mix(in srgb, var(--fail) 10%, var(--bg-inset)); }
  .fl :global(.fl-node.st-done rect) { stroke: var(--ok); }
  .fl :global(.fl-node.st-done text) { fill: var(--text-dim); }
  .fl :global(.fl-node.st-queued rect) { stroke-dasharray: 3 3; }

  .fl :global(.fl-edge) {
    fill: none;
    stroke: var(--line-strong);
    stroke-width: 1.5;
  }
  .fl :global(.fl-edge.to-blocked) { stroke: var(--fail); }
</style>
