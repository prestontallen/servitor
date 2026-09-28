<script>
  // The dossier: one ticket as framed cards over the folded ledger
  // (dossier.js). Gates as a ✠ strip, the contract as a signed form, the
  // criteria as a scorecard with glyphs, then the ledger newest first.
  import { get } from './api.svelte.js';
  import { view, arcs, openArc, kindGlyph, fmtTs, relTs, eventText } from './state.svelte.js';
  import { buildDossier } from './dossier.js';
  import DossierTimeline from './DossierTimeline.svelte';
  import Flow from './Flow.svelte';

  let doc = $state(null);
  let error = $state(null);
  let history = $state([]);

  // ledger fold: closed by default; a kind pill opens it filtered to that kind
  let ledgerOpen = $state(false);
  let kindFilter = $state(null);
  // long bodies are clamped; keys of the ones the reader opened
  let expanded = $state({});

  $effect(() => {
    const ref = view.ticketRef;
    if (!ref) return;
    doc = null;
    error = null;
    history = [];
    ledgerOpen = false;
    kindFilter = null;
    expanded = {};
    get(`/api/ticket/${ref}`)
      .then((d) => (doc = d))
      .catch((e) => (error = e.message));
    get(`/api/ticket/${ref}/history?limit=500`)
      .then((h) => (history = h))
      .catch(() => {});
  });

  const parentArc = $derived(doc?.parent ? arcs.list.find((a) => a.ulid === doc.parent) : null);
  const fields = $derived(doc?.fields || {});
  // provenance, then the handoff record (skills/servitor: Handoff), in a fixed order
  const prov = $derived(['source', 'source_ref', 'depends', 'area', 'branch', 'worktree', 'head', 'pushed', 'staging', 'checkpoint'].filter((k) => fields[k] !== undefined && fields[k] !== ''));

  const d = $derived(doc ? buildDossier(doc, history) : null);

  // the three gates, lit as they pass
  const GATES = ['contract_approved', 'presented', 'shipped'];
  const gateAt = $derived(Object.fromEntries((doc?.gates || []).map((g) => [g.gate, g])));
  const terminal = $derived(doc && ['done', 'dropped'].includes(doc.status));
  const GLYPH = { queued: '◇', active: '◆', blocked: '☠', done: '■', dropped: '✕' };

  const CLAMP_AT = 220;
  const isLong = (s) => (s || '').length > CLAMP_AT;
  const short = (a) => (a || '').replace(/^\w+:/, '');

  function fmtDur(ms) {
    if (ms == null) return '';
    const m = Math.round(ms / 60000);
    if (m < 1) return '<1m';
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    if (h < 48) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
    const dd = Math.floor(h / 24);
    return h % 24 ? `${dd}d ${h % 24}h` : `${dd}d`;
  }

  // criteria as a block meter, the same glyphs the board uses
  const CELLS = 12;
  function meter(pass, fail, total) {
    if (!total) return null;
    const p = Math.round((CELLS * pass) / total);
    const f = Math.round((CELLS * fail) / total);
    return { pass: '█'.repeat(p), fail: '█'.repeat(f), off: '░'.repeat(Math.max(0, CELLS - p - f)) };
  }

  function togglePill(kind) {
    if (ledgerOpen && kindFilter === kind) {
      ledgerOpen = false;
      kindFilter = null;
    } else {
      ledgerOpen = true;
      kindFilter = kind;
    }
  }

  // ledger rows: signals full, transitions compact, newest first, day separators
  const rows = $derived.by(() => {
    const sorted = history.filter((e) => !kindFilter || e.kind === kindFilter).sort((a, b) => b.id - a.id);
    const out = [];
    let lastDay = '';
    for (const e of sorted) {
      const day = new Date(e.ts).toDateString();
      if (day !== lastDay) {
        out.push({ type: 'day', day, id: 'day-' + e.id });
        lastDay = day;
      }
      out.push({ type: e.class === 'signal' ? 'signal' : 'transition', e, id: e.id });
    }
    return out;
  });

  const TAG_CLASS = { correction: 'fail', rework: 'warn', stall: 'warn', drift: 'dim' };
  const tsShort = (ts) => new Date(ts).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
</script>

{#snippet clamp(key, text)}
  {@const open = !!expanded[key]}
  <span class="body" class:clamped={isLong(text) && !open}>{text}</span>
  {#if isLong(text)}
    <button class="more" onclick={() => (expanded[key] = !open)}>{open ? 'less' : 'more'}</button>
  {/if}
{/snippet}

{#snippet sig(actor, human, ts)}
  <span class="sig">
    {#if actor}<span class="actor" class:human>{short(actor)}</span>{/if}
    {#if ts}<span title={fmtTs(ts)}>{relTs(ts)}</span>{/if}
  </span>
{/snippet}

<button class="back" onclick={() => doc?.parent ? openArc(doc.parent) : (location.hash = '#/board')}>← {doc?.parent ? 'arc' : 'board'}</button>

{#if error}
  <p class="muted">unreachable: {error}</p>
{:else if !doc}
  <p class="muted">querying…</p>
{:else}
  <article class="dossier">
    <!-- ================= header: identity, gates, facts, timeline ================= -->
    <header class="frame head s-{doc.status}">
      <span class="cap">Dossier{#if parentArc} · <button class="caplink" onclick={() => openArc(parentArc.ulid)}>arc {parentArc.slug}</button>{/if}</span>
      <span class="cap right">{doc.ulid}</span>
      <h2 class="goth">{doc.slug}</h2>
      <div class="title">{doc.title || doc.slug}</div>

      <div class="gates">
        {#each GATES as g (g)}
          {@const hit = gateAt[g]}
          <span class="gate" class:hit title={hit ? `${g} by ${hit.actor} · ${fmtTs(hit.ts)}` : `${g}: not yet`}>{g.replace('_', ' ')}</span>
        {/each}
        {#if terminal}<span class="gate end s-{doc.status}">{GLYPH[doc.status]} {doc.status}</span>{/if}
      </div>

      <div class="kv">
        <span class="k">status</span>
        <span>
          <span class="st s-{doc.status}">{GLYPH[doc.status]} {doc.status}{#if doc.blocked_on} · on {doc.blocked_on} {relTs(doc.blocked_since)}{/if}</span>
          {#if doc.card_word}· card word <span class="phos">{doc.card_word}</span>{/if}
          {#if doc.active_by}· held by <b>{doc.active_by}</b> {relTs(doc.active_since)}{/if}
        </span>
        {#if fields.next}<span class="k">next</span><span class="phos">{fields.next}</span>{/if}
        {#if prov.length}
          <span class="k">handoff</span>
          <span class="facts">{#each prov as k (k)}<span class="fact"><i>{k}</i> {fields[k]}</span>{/each}</span>
        {/if}
        <span class="k">pr</span>
        <span>{#if doc.pr}<a href={doc.pr} target="_blank" rel="noreferrer">{doc.pr.replace(/^https?:\/\/github\.com\//, '')}</a>{:else}<span class="muted">none yet</span>{/if}</span>
        <span class="k">updated</span><span class="muted">{relTs(doc.updated_at)}</span>
      </div>

      <div class="strip"><DossierTimeline {doc} {history} /></div>
    </header>

    <!-- ================= contract: a signed form ================= -->
    {#if d.contract}
      {@const c = d.contract}
      {@const m = meter(c.tally.pass, c.tally.fail, c.criteria.length)}
      <section class="frame inst contract" class:draft={!c.approved}>
        <span class="cap">Contract{#if c.tier} · tier {c.tier}{/if}{#if c.complexity} · {c.complexity}{/if}{#if c.versions > 1} · v{c.versions}{/if}</span>
        <span class="cap right" class:verd={!!c.approved}>{#if c.approved}✠ approved · {short(c.approved.actor)} · {new Date(c.approved.ts).toLocaleDateString()}{:else}draft · awaiting sanction{/if}</span>
        <div class="src">from {c.source}</div>

        {#if c.intent}
          <div class="field"><span class="label">Intent</span><div>{@render clamp('intent', c.intent)}</div></div>
        {/if}

        {#if c.in.length || c.out.length}
          <div class="scope">
            <div class="col">
              <span class="label in">In</span>
              <ul class="glyphs">{#each c.in as it}<li><span class="g phos">▸</span><span>{it}</span></li>{/each}</ul>
            </div>
            <div class="col">
              <span class="label">Out</span>
              <ul class="glyphs out">{#each c.out as it}<li><span class="g">✕</span><span>{it}</span></li>{/each}</ul>
            </div>
          </div>
        {/if}

        {#if c.criteria.length}
          <div class="field">
            <div class="crithead">
              <span class="label">Criteria</span>
              {#if m}<span class="meter" title="{c.tally.pass} of {c.criteria.length} pass"><b class="verd">{m.pass}</b><b class="blood">{m.fail}</b><span class="off">{m.off}</span></span>{/if}
              <span class="muted">{c.tally.pass} of {c.criteria.length} pass{#if c.tally.fail} · {c.tally.fail} fail{/if}</span>
            </div>
            <ul class="glyphs criteria">
              {#each c.criteria as cr, i (i)}
                <li class="st-{cr.state}">
                  <span class="g">{cr.state === 'pass' ? '■' : cr.state === 'fail' ? '☠' : '□'}</span>
                  <span class="crbody">{cr.body}
                    {#if cr.evidence}<span class="evidence">— {#if cr.evidence.actor}proven by {short(cr.evidence.actor)} {relTs(cr.evidence.ts)}{:else}proven{/if}{#if cr.evidence.text}: <i>{cr.evidence.text}</i>{/if}</span>{/if}
                  </span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if c.verification}
          <div class="field"><span class="label">Verified by</span><div>{@render clamp('verif', c.verification)}</div></div>
        {/if}
        {#if c.risks}
          <div class="field"><span class="label">Risks</span><div class="muted">{@render clamp('risks', c.risks)}</div></div>
        {/if}
        {#if c.amendments.length}
          <div class="amend">
            {#each c.amendments as a (a.id)}
              <div class="arow"><span class="label warn">amended</span>{@render sig(a.actor, a.actor.startsWith('human:'), a.ts)}<div>{@render clamp('amend-' + a.id, a.body)}</div></div>
            {/each}
          </div>
        {/if}
      </section>
    {:else}
      <section class="frame inst empty">
        <span class="cap">Contract</span>
        <div class="src">none yet · servitor-plan fills this</div>
      </section>
    {/if}

    <!-- ================= plan: a track ================= -->
    {#if d.plan}
      {@const p = d.plan}
      <section class="frame inst plan">
        <span class="cap">Plan · {p.done} of {p.steps.length} done</span>
        <span class="cap right">from {p.source}</span>
        <ol class="track">
          {#each p.steps as s, i (i)}
            <li class:done={!!s.state} class:now={i === p.current}>
              <span class="g">{s.state ? '■' : i === p.current ? '▶' : '□'}</span>
              <span class="sbody">{s.body}</span>
              <span class="smeta">
                {#if s.done}{short(s.done.actor)}{#if s.took != null} · {fmtDur(s.took)}{/if}
                {:else if i === p.current}in progress{#if s.added} · {relTs(s.added.ts)}{/if}{/if}
              </span>
            </li>
          {/each}
        </ol>
      </section>
    {:else}
      <section class="frame inst empty">
        <span class="cap">Plan</span>
        <div class="src">none yet · servitor-plan fills this</div>
      </section>
    {/if}

    <!-- ================= flow: agent-submitted flowchart ================= -->
    {#if d.flow?.ok}
      {@const fl = d.flow}
      <section class="frame inst flow">
        <span class="cap">Flow · {fl.nodes.length} nodes</span>
        <span class="cap right">flow.set · v{fl.versions} · {fl.actor}</span>
        <Flow {history} />
      </section>
    {:else if d.flow}
      <section class="frame inst flow">
        <span class="cap">Flow</span>
        <p class="muted" data-testid="flow-error">flow unparsable: {d.flow.error}</p>
      </section>
    {/if}

    <div class="pair">
      <!-- ================= decisions: forks taken ================= -->
      {#if d.decisions}
        <section class="frame inst decisions">
          <span class="cap">Decisions · {d.decisions.items.length}</span>
          <span class="cap right">from {d.decisions.source}</span>
          <ul class="glyphs forks">
            {#each d.decisions.items as dc, i (i)}
              <li>
                <span class="g phos">◆</span>
                <div class="fbody">
                  <div class="choice"><b>{dc.chosen}</b>{#if dc.rejected}<s class="muted">{dc.rejected}</s>{/if}</div>
                  {#if dc.why}<div class="why">because {dc.why}</div>{/if}
                  <div class="sigline" class:missing={!dc.actor || dc.actor_type !== 'human'}>
                    {#if dc.actor}— {@render sig(dc.actor, dc.actor_type === 'human', dc.ts)}{:else}— unsigned{/if}
                  </div>
                </div>
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      <!-- ================= questions: open loops ================= -->
      {#if d.questions}
        <section class="frame inst questions" class:alarm={d.questions.open > 0}>
          <span class="cap">Questions{#if d.questions.open} · {d.questions.open} open{/if}</span>
          <span class="cap right">{#if !d.questions.open}all answered · {/if}from {d.questions.source}</span>
          <ul class="glyphs threads">
            {#each d.questions.items as q, i (i)}
              <li class:open={!q.answer}>
                <span class="g" class:blood={!q.answer}>{q.answer ? '!' : '?'}</span>
                <div class="qbody">
                  <div>{@render clamp('q-' + i, q.body)}</div>
                  <div class="qmeta" class:blood={!q.answer}>
                    {#if q.answer}
                      answered{#if q.answer.actor} by {short(q.answer.actor)}{/if}{#if q.openFor != null} after {fmtDur(q.openFor)}{/if}
                    {:else}
                      open{#if q.openFor != null} {fmtDur(q.openFor)}{/if}{#if q.actor} · asked by {short(q.actor)}{/if}
                    {/if}
                  </div>
                  {#if q.answer}<div class="answer">└ {q.answer.body}</div>{/if}
                </div>
              </li>
            {/each}
          </ul>
        </section>
      {/if}
    </div>

    <!-- ================= feedback: tally, then callouts ================= -->
    {#if d.feedback}
      {@const f = d.feedback}
      <section class="frame inst feedback alarm">
        <span class="cap">Feedback · {f.items.length}</span>
        <span class="cap right">{#each Object.entries(f.bySource) as [s, n], i}{i ? ' · ' : ''}{n} from {s}{/each}</span>
        {#if Object.keys(f.byTag).length}
          <div class="tally">
            {#each Object.entries(f.byTag) as [tag, n] (tag)}
              <span class="tcount {TAG_CLASS[tag] || 'dim'}"><b>{n}</b> {tag}</span>
            {/each}
          </div>
        {/if}
        <ul class="glyphs callouts">
          {#each f.items as it, i (i)}
            <li class={TAG_CLASS[it.tag] || 'dim'}>
              <span class="g" class:blood={it.source === 'human'}>⚠</span>
              <div>
                <div class="chead">{#if it.tag}<span class="badge">{it.tag}</span>{/if}<span class="src-inline" class:blood={it.source === 'human'}>{it.source || ''}</span>{@render sig(null, false, it.ts)}</div>
                <div>{@render clamp('fb-' + i, it.finding)}</div>
              </div>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <!-- ================= corrections: diffs against the record ================= -->
    {#if d.corrections}
      <section class="frame inst corrections">
        <span class="cap">Corrections · {d.corrections.items.length}</span>
        <span class="cap right">from {d.corrections.source}</span>
        <ul class="diffs">
          {#each d.corrections.items as cr, i (i)}
            <li>
              {#if cr.amends}
                <div class="was"><span class="dmark">−</span><s>{@render clamp('was-' + i, cr.amends.body)}</s><span class="muted dwhen">{relTs(cr.amends.ts)}</span></div>
              {/if}
              <div class="now"><span class="dmark">+</span><span>{@render clamp('now-' + i, cr.body)}</span>{@render sig(cr.actor, false, cr.ts)}</div>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if d.links}
      <section class="frame inst links">
        <span class="cap">Links</span>
        <ul class="glyphs">{#each d.links.items as l}<li><span class="g">↗</span><a href={l.url} target="_blank" rel="noreferrer">{l.url}</a></li>{/each}</ul>
      </section>
    {/if}

    <!-- ================= ledger: the feed ================= -->
    <section class="frame inst ledger" class:open={ledgerOpen}>
      <span class="cap">Ledger · {history.length}</span>
      <div class="fold">
        <button class="toggle" onclick={() => { ledgerOpen = !ledgerOpen; if (!ledgerOpen) kindFilter = null; }}>
          {ledgerOpen ? '▾ fold' : '▸ unfold'}
        </button>
        <span class="pills">
          {#each d.counts as [kind, n] (kind)}
            <button class="pill" class:active={ledgerOpen && kindFilter === kind} onclick={() => togglePill(kind)}>{kind} <b>{n}</b></button>
          {/each}
        </span>
      </div>
      {#if ledgerOpen}
        <ul class="history">
          {#each rows as r (r.id)}
            {#if r.type === 'day'}
              <li class="daysep"><span>{r.day}</span></li>
            {:else if r.type === 'signal'}
              <li class="signal" class:human={r.e.actor_type === 'human'}>
                <span class="ts" title={fmtTs(r.e.ts)}>{tsShort(r.e.ts)}</span>
                <span class="hglyph">{kindGlyph(r.e.kind)}</span>
                <span class="kind">{r.e.kind}</span>
                <span class="line">{eventText(r.e)}{#if d.fed.has(r.e.id)}<span class="fedmark"> → {d.fed.get(r.e.id)}</span>{/if}</span>
                <span class="actor" class:human={r.e.actor_type === 'human'}>{r.e.actor}</span>
              </li>
            {:else}
              <li class="transition">
                <span class="ts" title={fmtTs(r.e.ts)}>{tsShort(r.e.ts)}</span>
                <span class="hglyph"></span>
                <span class="kind">{r.e.kind}</span>
                <span class="line muted">{eventText(r.e)}</span>
                <span class="actor">{short(r.e.actor)}</span>
              </li>
            {/if}
          {/each}
        </ul>
      {/if}
    </section>
  </article>
{/if}

<style>
  .back { margin-bottom: 14px; border: none; padding: 0; font-size: 11px; color: var(--phos-dim); min-height: 0; }
  .back:hover { color: var(--phos); }
  .dossier { max-width: 980px; margin: 0 auto; display: flex; flex-direction: column; gap: 18px; font-size: 12px; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; }
  .pair:empty { display: none; }
  .frame { padding: 16px 14px 12px; }
  .caplink { border: none; padding: 0; font: inherit; color: var(--phos); min-height: 0; min-width: 0; letter-spacing: inherit; }

  /* ---- header ---- */
  .head.s-blocked { border-color: var(--blood-dim); }
  .head.s-blocked > .cap { color: var(--blood); }
  .head .goth { font-size: 28px; }
  .title { color: var(--bone-dim); font-size: 12px; letter-spacing: 0.02em; margin: 2px 0 10px; text-wrap: pretty; }
  .gates { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 10px; }
  .gate { border: 1px solid var(--rust); padding: 3px 8px; font-size: 10px; letter-spacing: 0.16em; text-transform: uppercase; color: var(--bone-dim); }
  .gate.hit { border-color: var(--phos-dim); color: var(--phos); }
  .gate.hit::before { content: '✠ '; }
  .gate.end.s-done { border-color: var(--verdigris); color: var(--verdigris); }
  .gate.end.s-dropped { border-color: var(--bone-dim); color: var(--bone-dim); }
  .kv { display: grid; grid-template-columns: auto 1fr; gap: 3px 14px; align-items: baseline; }
  .kv .k { font-size: 10px; letter-spacing: 0.2em; text-transform: uppercase; color: var(--bone-dim); }
  .kv b { color: var(--bone); font-weight: 500; }
  .st { text-transform: uppercase; letter-spacing: 0.1em; font-size: 11px; }
  .st.s-active { color: var(--phos); }
  .st.s-blocked { color: var(--blood); }
  .st.s-done { color: var(--verdigris); }
  .st.s-queued, .st.s-dropped { color: var(--bone-dim); }
  .facts { display: flex; gap: 4px 12px; flex-wrap: wrap; }
  .fact { color: var(--bone); white-space: nowrap; }
  .fact i { font-style: normal; color: var(--bone-dim); font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase; margin-right: 2px; }
  .strip { border-top: 1px solid var(--rust); margin-top: 10px; padding-top: 6px; }

  /* ---- shared instrument chrome ---- */
  .src { font-size: 10px; color: var(--bone-dim); margin: -4px 0 10px; letter-spacing: 0.06em; }
  .field { margin-bottom: 12px; }
  .field .label { display: block; margin-bottom: 3px; }
  .label.in { color: var(--phos); }
  .label.warn { color: var(--phos-dim); }
  .body { white-space: pre-wrap; line-height: 1.5; }
  .body.clamped { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .more { border: none; background: none; padding: 0; color: var(--phos); font-size: 10px; cursor: pointer; min-height: 0; min-width: 0; margin-left: 4px; }
  .sig { display: inline-flex; gap: 6px; align-items: center; font-size: 10px; color: var(--bone-dim); }
  .actor { font-size: 10px; letter-spacing: 0.06em; color: var(--bone-dim); white-space: nowrap; }
  .actor.human { color: var(--phos); }
  .glyphs { list-style: none; margin: 0; padding: 0; display: grid; gap: 5px; }
  .glyphs li { display: grid; grid-template-columns: 16px 1fr; gap: 8px; align-items: baseline; }
  .glyphs .g { color: var(--phos-dim); text-align: center; }
  .glyphs.out li { color: var(--bone-dim); }
  .inst.empty { border-style: dashed; }
  .inst.empty .src { margin: 0; }

  /* ---- contract ---- */
  .contract.draft { border-style: dashed; }
  .scope { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; margin-bottom: 12px; }
  .scope .label { display: block; margin-bottom: 4px; }
  .crithead { display: flex; gap: 10px; align-items: baseline; margin-bottom: 8px; font-size: 11px; flex-wrap: wrap; }
  .meter { letter-spacing: 0.02em; font-size: 11px; }
  .meter b { font-weight: 400; }
  .meter .off { color: var(--bone-dim); opacity: 0.6; }
  .criteria li { padding: 5px 0; border-bottom: 1px dashed var(--rust); }
  .criteria li:last-child { border-bottom: none; }
  .criteria .st-pass .g { color: var(--verdigris); }
  .criteria .st-fail .g { color: var(--blood); }
  .st-pass .crbody { color: var(--bone-dim); }
  .evidence { font-size: 10px; color: var(--verdigris); }
  .st-fail .evidence { color: var(--blood); }
  .amend { border-top: 1px dashed var(--rust-2); padding-top: 8px; font-size: 11px; }
  .arow { display: flex; gap: 8px; align-items: baseline; flex-wrap: wrap; margin-bottom: 4px; }

  /* ---- plan ---- */
  .track { list-style: none; margin: 0; padding: 0; display: grid; gap: 5px; }
  .track li { display: grid; grid-template-columns: 16px 1fr auto; gap: 8px; align-items: baseline; }
  .track .g { color: var(--phos-dim); text-align: center; }
  .track li.done .g { color: var(--verdigris); }
  .track li.done .sbody { color: var(--bone-dim); }
  .track li.now .g { color: var(--phos); }
  .track li.now .sbody { color: var(--phos); }
  .smeta { font-size: 10px; color: var(--bone-dim); white-space: nowrap; }
  .track li.now .smeta { color: var(--phos); }

  /* ---- decisions ---- */
  .forks li { padding: 6px 0; border-bottom: 1px dashed var(--rust); }
  .forks li:last-child { border-bottom: none; }
  .fbody { min-width: 0; }
  .choice b { font-weight: 500; color: var(--bone); }
  .choice s { margin-left: 8px; }
  .why { color: var(--bone-dim); margin-top: 2px; }
  .sigline { margin-top: 4px; font-size: 10px; color: var(--bone-dim); }
  .sigline.missing { color: var(--phos-dim); }

  /* ---- questions ---- */
  .threads li { padding: 6px 0; border-bottom: 1px dashed var(--rust); }
  .threads li:last-child { border-bottom: none; }
  .threads .g { font-weight: 600; }
  .qbody { min-width: 0; }
  .qmeta { font-size: 10px; color: var(--bone-dim); margin-top: 2px; letter-spacing: 0.06em; }
  .answer { margin-top: 4px; color: var(--bone); }

  /* ---- feedback ---- */
  .tally { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 10px; }
  .tcount { font-size: 11px; padding: 1px 8px; border: 1px solid var(--rust); color: var(--bone-dim); }
  .tcount b { color: var(--bone); margin-right: 3px; }
  .tcount.fail b { color: var(--blood); }
  .tcount.warn b { color: var(--phos-dim); }
  .callouts li { padding: 6px 0; border-bottom: 1px dashed var(--rust); }
  .callouts li:last-child { border-bottom: none; }
  .chead { display: flex; gap: 8px; align-items: center; font-size: 10px; margin-bottom: 3px; }
  .src-inline { letter-spacing: 0.14em; text-transform: uppercase; color: var(--bone-dim); }
  .chead .sig { margin-left: auto; }

  /* ---- corrections ---- */
  .diffs { list-style: none; margin: 0; padding: 0; }
  .diffs li { padding: 6px 0; border-bottom: 1px dashed var(--rust); }
  .diffs li:last-child { border-bottom: none; }
  .was, .now { display: flex; gap: 8px; align-items: baseline; padding: 2px 6px; }
  .was { background: color-mix(in srgb, var(--blood) 10%, transparent); color: var(--bone-dim); }
  .now { background: color-mix(in srgb, var(--verdigris) 10%, transparent); }
  .dmark { flex: none; font-weight: 700; width: 12px; }
  .was .dmark { color: var(--blood); }
  .now .dmark { color: var(--verdigris); }
  .was s { flex: 1; }
  .now > span:nth-child(2) { flex: 1; }
  .dwhen { font-size: 10px; white-space: nowrap; }

  /* ---- ledger ---- */
  .fold { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
  .toggle { border: none; padding: 0; color: var(--phos-dim); min-height: 0; font-size: 11px; }
  .toggle:hover { color: var(--phos); }
  .pills { display: flex; gap: 4px; flex-wrap: wrap; margin-left: auto; }
  .pill { font-size: 10px; padding: 0 8px; min-height: 0; line-height: 1.8; }
  .pill b { font-weight: 400; color: var(--bone); margin-left: 2px; }
  .ledger.open .fold { border-bottom: 1px solid var(--rust); padding-bottom: 8px; margin-bottom: 4px; }
  .history { list-style: none; padding: 0; margin: 0; }
  .history li { display: grid; grid-template-columns: 78px 14px 90px 1fr auto; gap: 8px; align-items: baseline; padding: 5px 0; border-bottom: 1px dashed var(--rust); font-size: 11.5px; }
  .history li.daysep { display: block; border-bottom: none; padding: 10px 0 2px; color: var(--bone-dim); font-size: 10px; text-transform: uppercase; letter-spacing: 0.2em; }
  .history li.transition { padding: 3px 0; }
  .history .ts { color: var(--phos-dim); font-size: 10px; letter-spacing: 0.04em; white-space: nowrap; }
  .history li.human .ts { color: var(--phos); }
  .hglyph { color: var(--phos); text-align: center; }
  .kind { font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--bone-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .line { min-width: 0; overflow-wrap: anywhere; }
  .transition .line { font-size: 11px; }
  .fedmark { font-size: 10px; color: var(--phos); white-space: nowrap; }

  @media (max-width: 700px) {
    .pair, .scope { grid-template-columns: 1fr; }
    .frame { padding: 14px 10px 10px; }
    .head .goth { font-size: 24px; }
    .kv { grid-template-columns: 1fr; gap: 2px; }
    .kv .k { margin-top: 6px; }
    .fact { white-space: normal; }
    .smeta { white-space: normal; }
    .pills { margin-left: 0; }
    .history li { grid-template-columns: 78px 14px 1fr; }
    .history .kind { display: none; }
    .history .actor { grid-column: 3; }
  }
  @media (pointer: coarse) {
    .pill, .more, .toggle, .back { min-height: 36px; }
  }
</style>
