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

  // handoff latency
  let handoffs = $state([]);
  let handoffError = $state(null);
  let handoffEl = $state(null);
  let handoffChart = null;

  function fmtDur(secs) {
    if (secs == null) return '—';
    if (secs < 60) return `${Math.round(secs)}s`;
    if (secs < 3600) return `${Math.round(secs / 60)}m`;
    if (secs < 86400) return `${(secs / 3600).toFixed(1)}h`;
    return `${(secs / 86400).toFixed(1)}d`;
  }

  $effect(() => {
    // reload when the range changes or the ledger grows
    void live.changeCount;
    get(`/api/analytics?days=${days}`)
      .then((b) => {
        buckets = b;
        error = null;
      })
      .catch((e) => (error = e.message));
    get('/api/analytics/handoffs')
      .then((r) => {
        handoffs = r;
        handoffError = null;
      })
      .catch((e) => (handoffError = e.message));
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

  $effect(() => {
    if (!handoffEl) return;
    handoffChart = handoffChart || echarts.init(handoffEl);
    renderHandoffChart();
    return () => {
      handoffChart?.dispose();
      handoffChart = null;
    };
  });

  // weekly average of each wait, keyed on the week the wait ENDED
  // (human waits: week of presented; agent waits: week of approval).
  function weeklyWaits() {
    const wk = (ts) => {
      const d = new Date(ts);
      const day = (d.getUTCDay() + 6) % 7; // monday-start
      d.setUTCDate(d.getUTCDate() - day);
      return d.toISOString().slice(0, 10);
    };
    const acc = {};
    for (const r of handoffs) {
      if (r.human_wait_secs != null && r.presented_at) {
        const k = wk(r.presented_at);
        (acc[k] ??= { h: [], a: [] }).h.push(r.human_wait_secs);
      }
      if (r.agent_wait_secs != null && r.approved_at) {
        const k = wk(r.approved_at);
        (acc[k] ??= { h: [], a: [] }).a.push(r.agent_wait_secs);
      }
    }
    const weeks = Object.keys(acc).sort();
    const avg = (a) => (a.length ? a.reduce((x, y) => x + y, 0) / a.length : null);
    return { weeks, human: weeks.map((w) => avg(acc[w].h)), agent: weeks.map((w) => avg(acc[w].a)) };
  }

  function baseChartOpts() {
    const css = getComputedStyle(document.documentElement);
    return {
      color: ['--accent', '--accent-dim', '--ok', '--warn', '--text-dim', '--fail'].map(
        (v) => css.getPropertyValue(v).trim() || '#b08d57'
      ),
      textStyle: { color: css.getPropertyValue('--text').trim(), fontFamily: 'monospace', fontSize: 11 },
      tooltip: { trigger: 'axis' },
      legend: { textStyle: { color: css.getPropertyValue('--text-dim').trim() } },
      grid: { left: 48, right: 12, top: 30, bottom: 24 }
    };
  }

  function renderChart() {
    if (!chart) return;
    const css = getComputedStyle(document.documentElement);
    const kinds = [...new Set(buckets.flatMap((b) => Object.keys(b.by_kind || {})))];
    chart.setOption({
      ...baseChartOpts(),
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

  function renderHandoffChart() {
    if (!handoffChart) return;
    const { weeks, human, agent } = weeklyWaits();
    if (weeks.length === 0) return;
    handoffChart.setOption({
      ...baseChartOpts(),
      tooltip: { ...baseChartOpts().tooltip, valueFormatter: (v) => fmtDur(v) },
      legend: { data: ['waiting on human', 'waiting on agent'], ...baseChartOpts().legend },
      xAxis: { type: 'category', data: weeks, name: 'week of' },
      yAxis: {
        type: 'value',
        name: 'avg wait',
        axisLabel: { formatter: (v) => fmtDur(v) },
        splitLine: { lineStyle: { color: getComputedStyle(document.documentElement).getPropertyValue('--line').trim() } }
      },
      series: [
        { name: 'waiting on human', type: 'bar', data: human, barMaxWidth: 18 },
        { name: 'waiting on agent', type: 'bar', data: agent, barMaxWidth: 18 }
      ]
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

<section class="panel analytics">
  <div class="head"><h2>Handoff latency</h2></div>
  {#if handoffError}
    <p class="muted">unreachable: {handoffError}</p>
  {:else if handoffs.length === 0}
    <p class="muted">no presented tickets yet</p>
  {/if}
  {#if handoffs.length > 0}
    <div class="chart short" bind:this={handoffEl}></div>
    <div class="rows" role="table" aria-label="handoff latency per ticket">
      {#each handoffs.slice(0, 30) as r (r.ulid)}
        <a class="row" href={`#/ticket/${r.ulid}`}>
          <span class="slug">{r.slug}</span>
          <span class="wait human">on human {fmtDur(r.human_wait_secs)}</span>
          <span class="wait agent">on agent {fmtDur(r.agent_wait_secs)}</span>
          <span class="when">{r.presented_at ? r.presented_at.slice(0, 10) : ''}</span>
        </a>
      {/each}
    </div>
  {/if}
</section>

<style>
  .analytics { max-width: 980px; margin: 0 auto; padding: 14px 18px; }
  .head { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 10px; }
  h2 { margin: 0; font-size: 15px; }
  .range { display: flex; gap: 4px; }
  .chart { width: 100%; height: 380px; }
  .chart.short { height: 240px; margin-bottom: 10px; }
  .rows { display: flex; flex-direction: column; }
  .row {
    display: grid;
    grid-template-columns: minmax(10rem, 1.4fr) 1fr 1fr auto;
    gap: 8px;
    align-items: baseline;
    min-height: 44px;
    padding: 6px 4px;
    border-bottom: 1px solid var(--line);
    text-decoration: none;
    color: var(--text);
    font-size: 13px;
  }
  .slug { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .wait.human { color: var(--warn); }
  .wait.agent { color: var(--accent); }
  .when { color: var(--text-dim); font-size: 11px; }
  @media (max-width: 640px) {
    .row { grid-template-columns: 1fr auto; }
    .wait.agent, .when { display: none; }
  }
</style>
