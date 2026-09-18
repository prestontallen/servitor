<script>
  import * as echarts from 'echarts/core';
  import { BarChart } from 'echarts/charts';
  import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components';
  import { CanvasRenderer } from 'echarts/renderers';
  import { get } from './api.svelte.js';
  import { live } from './live.svelte.js';

  echarts.use([BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer]);

  let days = $state(30);
  let buckets = $state([]);
  let error = $state(null);
  let el = $state(null); // bind:this
  let chart = null;

  $effect(() => {
    // reload when the range changes or the ledger grows
    void live.changeCount;
    get(`/api/analytics?days=${days}`)
      .then((b) => {
        buckets = b;
        error = null;
      })
      .catch((e) => (error = e.message));
  });

  $effect(() => {
    if (!el) return;
    chart = chart || echarts.init(el);
    renderChart();
    return () => {
      chart?.dispose();
      chart = null;
    };
  });

  function renderChart() {
    if (!chart) return;
    const kinds = [...new Set(buckets.flatMap((b) => Object.keys(b.by_kind || {})))];
    const css = getComputedStyle(document.documentElement);
    const palette = ['--accent', '--accent-dim', '--ok', '--warn', '--text-dim', '--fail'].map(
      (v) => css.getPropertyValue(v).trim() || '#b08d57'
    );
    chart.setOption({
      color: palette,
      textStyle: { color: css.getPropertyValue('--text').trim(), fontFamily: 'monospace', fontSize: 11 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      legend: { data: kinds, textStyle: { color: css.getPropertyValue('--text-dim').trim() } },
      grid: { left: 40, right: 12, top: 30, bottom: 24 },
      xAxis: {
        type: 'category',
        data: buckets.map((b) => b.day),
        axisLine: { lineStyle: { color: css.getPropertyValue('--line-strong').trim() } }
      },
      yAxis: { type: 'value', splitLine: { lineStyle: { color: css.getPropertyValue('--line').trim() } } },
      series: kinds.map((k) => ({
        name: k,
        type: 'bar',
        stack: 'events',
        data: buckets.map((b) => (b.by_kind && b.by_kind[k]) || 0),
        barMaxWidth: 18
      }))
    });
  }
</script>

<section class="panel analytics">
  <div class="head">
    <h2>Ledger activity</h2>
    <div class="range">
      {#each [30, 90, 180] as d}
        <button class:active={days === d} onclick={() => (days = d)}>{d}d</button>
      {/each}
    </div>
  </div>
  {#if error}
    <p class="muted">unreachable: {error}</p>
  {:else if buckets.length === 0}
    <p class="muted">no activity in range</p>
  {/if}
  <div class="chart" bind:this={el}></div>
</section>

<style>
  .analytics { max-width: 980px; margin: 0 auto; padding: 14px 18px; }
  .head { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 10px; }
  h2 { margin: 0; font-size: 15px; }
  .range { display: flex; gap: 4px; }
  .chart { width: 100%; height: 380px; }
</style>
