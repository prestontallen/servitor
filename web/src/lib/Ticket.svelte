<script>
  import { get } from './api.svelte.js';
  import { view, show, kindGlyph, fmtTs, wordClass, eventText } from './state.svelte.js';

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
    get(`/api/ticket/${ref}/history?limit=200`)
      .then((h) => (history = h))
      .catch(() => {});
  });

  const GATES = ['contract_approved', 'presented', 'shipped'];
  const passedGates = $derived(new Set((doc?.gates || []).map((g) => g.gate)));

  function actorClass(actor) {
    if (!actor) return 'agent';
    return actor.startsWith('human') ? 'human' : 'agent';
  }

  function section(name, items, render) {
    if (!items || items.length === 0) return { name, items, render };
    return { name, items, render };
  }
</script>

<button class="back" onclick={() => show('board')}>← board</button>

{#if error}
  <p class="muted">unreachable: {error}</p>
{:else if !doc}
  <p class="muted">querying machine spirit…</p>
{:else}
  <article class="panel dossier">
    <header>
      <div class="kv">
        <span class="badge">{doc.slug}</span>
        <span class="badge">{doc.status}</span>
        {#if doc.card_word}<span class="badge {wordClass(doc.card_word)}">{doc.card_word}</span>{/if}
        {#if doc.pr !== null && doc.pr !== undefined}<span class="muted">pr: {doc.pr || '(empty)'}</span>{/if}
      </div>
      <h2>{doc.title || doc.slug}</h2>
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
        {#each history as e (e.id)}
          <li>
            <span class="glyph">{kindGlyph(e.kind)}</span>
            <span class="muted">{e.kind}</span>
            <span>{eventText(e)}</span>
            <span class="actor {actorClass(e.actor)}">{e.actor}</span>
            <span class="meta-inline">{e.ts ? fmtTs(e.ts) : ''}</span>
          </li>
        {/each}
      </ul>
    </div>
  </article>
{/if}

<style>
  .back { margin-bottom: 12px; }
  .dossier { max-width: 980px; margin: 0 auto; padding: 16px 20px; }
  header { border-bottom: 1px solid var(--line); padding-bottom: 10px; margin-bottom: 12px; }
  .kv { display: flex; gap: 8px; align-items: center; margin-bottom: 6px; }
  h2 { margin: 0 0 8px; font-size: 16px; }
  .ratchet { display: flex; gap: 16px; }
  .gate { color: var(--text-dim); font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; }
  .gate.passed { color: var(--auspex); }
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
    padding: 4px 0;
    border-bottom: 1px solid var(--line);
  }
  .glyph { color: var(--brass); }
  .actor { font-size: 10px; text-transform: uppercase; border: 1px solid var(--line-strong); padding: 0 5px; }
  .actor.human { border-color: var(--rust); color: var(--rust); }
  .actor.agent { border-color: var(--brass-dim); color: var(--brass); }
</style>
