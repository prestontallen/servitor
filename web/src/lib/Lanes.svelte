<script>
  // Thin Svelte adapter over the dotlattice package's Lanes chart, the
  // same bridge DotLattice.svelte is for the lattice: the package owns
  // the rows, the bars and the DOM; this component only carries props
  // and Svelte reactivity across, so there is one lanes implementation.
  import { Lanes } from 'dotlattice';

  let {
    span,                    // {min, max} in ms
    lanes = [],              // [{id, label, title?, bars: [{start, end, kind, title?}]}]
    kinds = null,            // {name: color | {color, hatch?}}
    axis = [],               // [{t, label}]
    autoAxis = null,         // 'auto' | 'minute' | ... | 'off'
    cell = 8,                // ruling pitch (px), the lattice's grid
    rowH = 9,                // bar height (px)
    gap = 3,                 // px between rows
    labelWidth = 150,        // px for the label column
    highlight = null,        // {from, to}
    onLabel = null,          // (lane, event) => void
    tooltip = null,          // (bar, lane) => string
    label = 'lanes'
  } = $props();

  let el = $state(null);
  let chart = $state(null);

  $effect(() => {
    if (!el) return;
    chart = new Lanes(el, {
      cell, rowH, gap, labelWidth, label, kinds,
      autoAxis: autoAxis ?? 'off',
      onLabel: (lane, ev) => onLabel?.(lane, ev),
      tooltip: tooltip ?? undefined,
    });
    return () => { chart?.destroy(); chart = null; };
  });

  $effect(() => {
    if (!chart) return;
    chart.setOptions({ span, axis, highlight, kinds, labelWidth });
    chart.setData(lanes);
  });
</script>

<div class="lanes" bind:this={el} data-testid="lanes"></div>

<style>
  .lanes { position: relative; width: 100%; }
</style>
