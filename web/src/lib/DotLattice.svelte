<script>
  // Thin Svelte adapter over the dotlattice package (npm `file:` dep).
  // The package's DotLattice class owns the SVG, the layout and the DOM;
  // this component only bridges props and Svelte reactivity to it, so the
  // GUI keeps its declarative call sites while there is exactly one
  // lattice implementation.
  import { DotLattice } from 'dotlattice';

  let {
    span,                    // {min, max} in ms
    events = [],             // [{t, group, side}], see the package README
    groups = null,           // [{name, color}] — stacking order + hues
    sides = null,            // {sideKey: 'up' | 'down'}
    sideOpacity = null,      // {sideKey: 0..1}
    cell = 8,                // grid pitch (px)
    r = 2.5,                 // dot radius (px)
    maxRows = 12,            // rows a side before a dot becomes a quantum
    ticks = [],              // [{t, label, sig?, color?, title?}]
    runs = [],               // [{start, end, title, color?}]
    highlight = null,        // {from, to}
    axis = [],               // [{t, label}]
    autoAxis = null,         // 'auto' | 'minute' | 'hour' | 'day' | ... | 'off'
    tooltip = null,          // (bucket, bucketMs) => string
    quantum = $bindable(1),  // read back by the caller for its chain
    label = 'dot lattice'
  } = $props();

  let el = $state(null);
  let chart = $state(null);

  $effect(() => {
    if (!el) return;
    chart = new DotLattice(el, {
      cell, dot: r, maxRows, label, groups, sides, sideOpacity,
      autoAxis: autoAxis ?? 'off',
      tooltip: tooltip ?? undefined,
      onQuantum: (q) => (quantum = q),
    });
    return () => { chart?.destroy(); chart = null; };
  });

  $effect(() => {
    if (!chart) return;
    chart.setOptions({ span, ticks, runs, highlight, axis });
    chart.setData(events);
  });
</script>

<div class="lattice" bind:this={el} data-testid="dot-lattice"></div>

<style>
  .lattice { position: relative; width: 100%; }
</style>
