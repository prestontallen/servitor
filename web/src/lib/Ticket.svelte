<script>
  import { get } from './api.svelte.js';
  import { view, arcs, openArc, kindGlyph, fmtTs, relTs, wordClass, eventText } from './state.svelte.js';

  let doc = $state(null);
  let error = $state(null);
  let history = $state([]);

  $effect(() => {
    const ref = view.ticketRef;
    if (!ref) return;
    doc = null;
    error = null;
    history = [];
    get(`/api/ticket/${ref}`)
      .then((d) => (doc = d))
      .catch((e) => (error = e.message));
    get(`/api/ticket/${ref}/history?limit=500`)
      .then((h) => (history = h))
      .catch(() => {});
  });

  const GATES = ['contract_approved', 'presented', 'shipped'];
  const passedGates = $derived(new Set((doc?.gates || []).map((g) => g.gate)));

  const parentArc = $derived(doc?.parent ? arcs.list.find((a) => a.ulid === doc.parent) : null);

  // provenance / relations free fields
  const fields = $derived(doc?.fields || {});
  const prov = $derived(
    ['source', 'source_ref', 'depends', 'area'].filter((k) => fields[k] !== undefined)
  );

  // signals get full rows; consecutive transitions collapse into one
  // compact line each, newest first, grouped under day separators.
  const rows = $derived.by(() => {
    const sorted = [...history].sort((a, b) => b.id - a.id);
    const out = [];
    let lastDay = '';
    for (const e of sorted) {
      const day = new Date(e.ts).toDateString();
      if (day !== lastDay) {
        out.push({ type: 'day', day, id: 'day-' + e.id });
        lastDay = day;
      }
      const isSignal = e.class === 'signal';
      out.push({ type: isSignal ? 'signal' : 'transition', e, id: e.id });
    }
    return out;
  });
</script>

<button class="back" onclick={() => doc?.parent ? openArc(doc.parent) : (location.hash = '#/arcs')}>← back</button>

{#if error}
  <p class="muted">unreachable: {error}</p>
{:else if !doc}
  <p class="muted">querying…</p>
{:else}
  <article class="panel dossier">
    <header>
      <div class="kv">
        <span class="badge">{doc.slug}</span>
        <span class="badge">{doc.status}</span>
        {#if doc.card_word}<span class="badge {wordClass(doc.card_word)}">{doc.card_word}</span>{/if}
        {#if doc.blocked_on}<span class="badge blocked_on">on {doc.blocked_on} {relTs(doc.blocked_since)}</span>{/if}
      </div>
      <h2>{doc.title || doc.slug}</h2>
      <div class="facts">
        {#if parentArc}
          <button class="fact arc-link" onclick={() => openArc(parentArc.ulid)}>arc: {parentArc.slug}</button>
        {/if}
        {#each prov as k (k)}
          <span class="fact">{k}: {fields[k]}</span>
        {/each}
        {#if doc.pr}<span class="fact">pr: {doc.pr}</span>{/if}
        <span class="fact muted">updated {relTs(doc.updated_at)}</span>
      </div>
      <div class="ratchet">
        {#each GATES as g}
          <span class="gate" class:passed={passedGates.has(g)}>
            {passedGates.has(g) ? '⊗' : '○'} {g.replace('_approved', '')}
          </span>
        {/each}
      </div>
    </header>

    {#each [
      ['criteria', doc.criteria, (c) => ({ body: c.body, cls: c.state === 'pass' ? 'state-pass' : c.state === 'fail' ? 'state-fail' : '', mark: c.state ? '●' : '○' })],
      ['plan', doc.plan, (p) => ({ body: p.body })],
      ['decisions', doc.decisions, (d) => ({ body: `${d.what}${d.why ? ' — ' + d.why : ''}` })],
      ['questions', doc.questions, (q) => ({ body: q.body })],
      ['links', doc.links, (l) => ({ body: l.url, href: l.url })]
    ] as [name, items, render] (name)}
      {#if items && items.length}
        <div class="section">
          <h3>{name}</h3>
          <ul>
            {#each items as item}
              {@const r = render(item)}
              <li class={r.cls || ''}>
                {#if r.mark}<span class="muted">{r.mark}</span>{/if}
                {#if r.href}<a href={r.href} target="_blank" rel="noreferrer">{r.body}</a>{:else}{r.body}{/if}
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    {/each}

    <div class="section">
      <h3>history</h3>
      <ul class="history">
        {#each rows as r (r.id)}
          {#if r.type === 'day'}
            <li class="daysep"><span>{r.day}</span></li>
          {:else if r.type === 'signal'}
            <li class="signal">
              <span class="glyph">{kindGlyph(r.e.kind)}</span>
              <span class="muted kind">{r.e.kind}</span>
              <span class="line">{eventText(r.e)}</span>
              <span class="actor" class:human={r.e.actor_type === 'human'}>{r.e.actor}</span>
              <span class="meta-inline" title={fmtTs(r.e.ts)}>{relTs(r.e.ts)}</span>
            </li>
          {:else}
            <li class="transition">
              <span class="muted kind">{r.e.kind}</span>
              <span class="line muted">{eventText(r.e)}</span>
              <span class="meta-inline" title={fmtTs(r.e.ts)}>{relTs(r.e.ts)}</span>
            </li>
          {/if}
        {/each}
      </ul>
    </div>
  </article>
{/if}

<style>
  .back { margin-bottom: 12px; }
  .dossier { max-width: 860px; margin: 0 auto; padding: 16px 20px; }
  header { border-bottom: 1px solid var(--line); padding-bottom: 10px; margin-bottom: 12px; }
  .kv { display: flex; gap: 8px; align-items: center; margin-bottom: 6px; flex-wrap: wrap; }
  h2 { margin: 0 0 8px; font-size: 16px; }
  .facts { display: flex; gap: 12px; flex-wrap: wrap; font-size: 11px; margin-bottom: 8px; }
  .fact { color: var(--text-dim); }
  .arc-link { border: none; background: none; padding: 0; color: var(--accent); font-size: 11px; cursor: pointer; min-height: 0; }
  .ratchet { display: flex; gap: 16px; }
  .gate { color: var(--text-dim); font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; }
  .gate.passed { color: var(--ok); }
  .section { margin-bottom: 14px; }
  h3 {
    font-size: 11px;
    margin: 0 0 6px;
    color: var(--text-dim);
    border-bottom: 1px solid var(--line);
    padding-bottom: 3px;
  }
  ul { margin: 0; padding-left: 20px; }
  .history { list-style: none; padding-left: 0; }
  .history li {
    display: flex;
    gap: 10px;
    align-items: baseline;
    padding: 6px 0;
    border-bottom: 1px solid var(--line);
    font-size: 12px;
  }
  .history li.daysep {
    border-bottom: none; padding: 10px 0 2px; color: var(--text-dim);
    font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em;
  }
  .history li.transition { padding: 3px 0; }
  .glyph { color: var(--accent); }
  .kind { min-width: 84px; font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; }
  .line { flex: 1; }
  .transition .line { font-size: 11px; }
  .actor { font-size: 10px; text-transform: uppercase; border: 1px solid var(--line-strong); padding: 0 5px; border-radius: 3px; }
  .actor.human { border-color: var(--accent); color: var(--accent); }
</style>
