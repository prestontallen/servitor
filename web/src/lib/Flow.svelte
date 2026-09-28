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
  .fl :global(.fl-node.k-edge rect) { stroke: var(--accent); fill: color-mix(in srgb, var(--accent) 12%, var(--bg-inset)); }
  .fl :global(.fl-node.k-service rect) { stroke: var(--k-decision); fill: color-mix(in srgb, var(--k-decision) 10%, var(--bg-inset)); }
  .fl :global(.fl-node.k-store rect) { stroke: var(--k-feedback); fill: color-mix(in srgb, var(--k-feedback) 10%, var(--bg-inset)); }
  .fl :global(.fl-node.k-queue rect) { stroke: var(--warn); stroke-dasharray: 3 3; }
  .fl :global(.fl-node.k-client rect) { stroke: var(--ok); }
  .fl :global(.fl-node.k-external rect) { stroke: var(--text-dim); }

  .fl :global(.fl-edge) {
    fill: none;
    stroke: var(--line-strong);
    stroke-width: 1.5;
  }
  .fl :global(.fl-head) { stroke: var(--line-strong); stroke-width: 1.5; }
  .fl :global(.fl-elabel) {
    fill: var(--text-dim);
    font-size: 10px;
    font-family: inherit;
    paint-order: stroke;
    stroke: var(--bg-raised);
    stroke-width: 4px;
    stroke-linejoin: round;
  }
</style>
