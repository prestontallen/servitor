<script>
  import { get } from './api.svelte.js';
  import { view, arcs, openArc, kindGlyph, fmtTs, relTs, eventText } from './state.svelte.js';
  import { buildDossier } from './dossier.js';
  import DossierTimeline from './DossierTimeline.svelte';

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
  const prov = $derived(['source', 'source_ref', 'depends', 'area'].filter((k) => fields[k] !== undefined));

  const d = $derived(doc ? buildDossier(doc, history) : null);

  // phase stepper: the four card words, lit up to the latest gate
  const PHASES = ['shaping', 'building', 'checking', 'shipping'];
  const GATE_BEFORE = { building: 'contract_approved', checking: 'presented', shipping: 'shipped' };
  const gateAt = $derived(Object.fromEntries((doc?.gates || []).map((g) => [g.gate, g])));
  const phaseIdx = $derived(doc?.card_word ? PHASES.indexOf(doc.card_word) : 0);
  const terminal = $derived(doc && ['done', 'dropped'].includes(doc.status));

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

<button class="back" onclick={() => doc?.parent ? openArc(doc.parent) : (location.hash = '#/arcs')}>← back</button>

{#if error}
  <p class="muted">unreachable: {error}</p>
{:else if !doc}
  <p class="muted">querying…</p>
{:else}
  <article class="dossier">
    <!-- ================= header: state, phase, stamps, timeline ================= -->
    <header class="panel head s-{doc.status}">
      <div class="titlerow">
        <span class="status s-{doc.status}">{doc.status}{#if doc.blocked_on} · on {doc.blocked_on}{/if}</span>
        <span class="slug muted">{doc.slug}</span>
      </div>
      <h2>{doc.title || doc.slug}</h2>

      <ol class="stepper" class:terminal>
        {#each PHASES as ph, i (ph)}
          {@const g = gateAt[GATE_BEFORE[ph]]}
          {@const state = terminal ? 'past' : i < phaseIdx ? 'past' : i === phaseIdx ? 'now' : 'next'}
          <li class="step {state}">
            <span class="dot"></span>
            <span class="name">{ph}</span>
            {#if g}
              <span class="gate" class:human={g.actor?.startsWith('human:')} title={fmtTs(g.ts)}>✓ {GATE_BEFORE[ph].replace('_approved', '')} · {short(g.actor)} · {relTs(g.ts)}</span>
            {:else if state === 'now' && GATE_BEFORE[PHASES[i + 1]]}
              <span class="gate muted">next gate: {GATE_BEFORE[PHASES[i + 1]].replace('_approved', '')}</span>
            {/if}
          </li>
        {/each}
        {#if terminal}
          <li class="step end"><span class="dot"></span><span class="name">{doc.status}</span></li>
        {/if}
      </ol>

      <div class="facts">
        {#if parentArc}<button class="fact link" onclick={() => openArc(parentArc.ulid)}>arc · {parentArc.slug}</button>{/if}
        {#if doc.active_by}<span class="fact">held by <b>{doc.active_by}</b> {relTs(doc.active_since)}</span>{/if}
        {#if doc.blocked_on}<span class="fact fail">blocked {relTs(doc.blocked_since)}</span>{/if}
        {#each prov as k (k)}<span class="fact">{k} · {fields[k]}</span>{/each}
        {#if doc.pr}<a class="fact link" href={doc.pr} target="_blank" rel="noreferrer">pr · {doc.pr.replace(/^https?:\/\/github\.com\//, '')}</a>{/if}
        <span class="fact">updated {relTs(doc.updated_at)}</span>
      </div>

      <div class="strip"><DossierTimeline {doc} {history} /></div>
    </header>

    <!-- ================= contract: a signed form ================= -->
    {#if d.contract}
      {@const c = d.contract}
      <section class="panel inst contract" class:draft={!c.approved}>
        <div class="stamp" class:approved={c.approved}>
          {#if c.approved}
            <b>approved</b><span>{short(c.approved.actor)} · {new Date(c.approved.ts).toLocaleDateString()}</span>
          {:else}
            <b>draft</b><span>not yet approved</span>
          {/if}
        </div>
        <h3>Contract
          {#if c.tier}<span class="chip">tier {c.tier}</span>{/if}
          {#if c.complexity}<span class="chip">{c.complexity} complexity</span>{/if}
          {#if c.versions > 1}<span class="chip">v{c.versions}</span>{/if}
        </h3>
        <div class="src">from {c.source}</div>

        {#if c.intent}
          <div class="field"><span class="label">Intent</span><div>{@render clamp('intent', c.intent)}</div></div>
        {/if}

        {#if c.in.length || c.out.length}
          <div class="scope">
            <div class="col in">
              <span class="label">In</span>
              <ul>{#each c.in as it}<li>{it}</li>{/each}</ul>
            </div>
            <div class="col out">
              <span class="label">Out</span>
              <ul>{#each c.out as it}<li>{it}</li>{/each}</ul>
            </div>
          </div>
        {/if}

        {#if c.criteria.length}
          <div class="field">
            <div class="crithead">
              <span class="label">Criteria</span>
              <span class="meter" title="{c.tally.pass} of {c.criteria.length} pass">
                <i class="pass" style="width:{(c.tally.pass / c.criteria.length) * 100}%"></i>
                <i class="fail" style="width:{(c.tally.fail / c.criteria.length) * 100}%"></i>
              </span>
              <span class="muted">{c.tally.pass} of {c.criteria.length} pass{#if c.tally.fail} · {c.tally.fail} fail{/if}</span>
            </div>
            <ul class="criteria">
              {#each c.criteria as cr, i (i)}
                <li class="st-{cr.state}">
                  <span class="box">{cr.state === 'pass' ? '✓' : cr.state === 'fail' ? '✕' : ''}</span>
                  <span class="crbody">{cr.body}</span>
                  {#if cr.evidence}<span class="evidence">← proven by {short(cr.evidence.actor)} {relTs(cr.evidence.ts)}</span>{/if}
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if c.verification}
          <div class="field"><span class="label">Verified by</span><div>{@render clamp('verif', c.verification)}</div></div>
        {/if}
        {#if c.risks}
          <div class="field"><span class="label">Risks</span><div>{@render clamp('risks', c.risks)}</div></div>
        {/if}
        {#if c.amendments.length}
          <div class="amend">
            {#each c.amendments as a (a.id)}
              <div class="arow"><span class="label">amended</span>{@render sig(a.actor, a.actor.startsWith('human:'), a.ts)}<div>{@render clamp('amend-' + a.id, a.body)}</div></div>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    <!-- ================= plan: a track ================= -->
    {#if d.plan}
      {@const p = d.plan}
      <section class="panel inst plan">
        <h3>Plan <span class="chip">{p.done} of {p.steps.length} done</span></h3>
        <div class="src">from {p.source}</div>
        <ol class="track">
          {#each p.steps as s, i (i)}
            <li class:done={!!s.state} class:now={i === p.current}>
              <span class="node">{s.state ? '✓' : i + 1}</span>
              <span class="sbody">{s.body}</span>
              <span class="smeta">
                {#if s.done}{short(s.done.actor)}{#if s.took != null} · {fmtDur(s.took)}{/if}
                {:else if i === p.current}in progress{#if s.added} · {relTs(s.added.ts)}{/if}{/if}
              </span>
            </li>
          {/each}
        </ol>
      </section>
    {/if}

    <div class="pair">
      <!-- ================= decisions: forks taken ================= -->
      {#if d.decisions}
        <section class="panel inst decisions">
          <h3>Decisions <span class="chip">{d.decisions.items.length}</span></h3>
          <div class="src">from {d.decisions.source}</div>
          <ul class="forks">
            {#each d.decisions.items as dc, i (i)}
              <li>
                <span class="fork">⑂</span>
                <div class="fbody">
                  <div class="choice">
                    <b>{dc.chosen}</b>
                    {#if dc.rejected}<s class="muted">{dc.rejected}</s>{/if}
                  </div>
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
        <section class="panel inst questions">
          <h3>Questions {#if d.questions.open}<span class="chip warn">{d.questions.open} open</span>{:else}<span class="chip">all answered</span>{/if}</h3>
          <div class="src">from {d.questions.source}</div>
          <ul class="threads">
            {#each d.questions.items as q, i (i)}
              <li class:open={!q.answer}>
                <span class="qmark">{q.answer ? '!' : '?'}</span>
                <div class="qbody">
                  <div>{@render clamp('q-' + i, q.body)}</div>
                  <div class="qmeta">
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
      <section class="panel inst feedback">
        <h3>Feedback</h3>
        <div class="src">from {f.source}</div>
        <div class="tally">
          {#each Object.entries(f.byTag) as [tag, n] (tag)}
            <span class="tcount {TAG_CLASS[tag] || 'dim'}"><b>{n}</b> {tag}</span>
          {/each}
          <span class="tsplit muted">{#each Object.entries(f.bySource) as [s, n], i}{i ? ' · ' : ''}{n} from {s}{/each}</span>
        </div>
        <ul class="callouts">
          {#each f.items as it, i (i)}
            <li class={TAG_CLASS[it.tag] || 'dim'}>
              <div class="chead">{#if it.tag}<span class="chip {TAG_CLASS[it.tag] || ''}">{it.tag}</span>{/if}{#if it.source}<span class="muted">from {it.source}</span>{/if}{@render sig(null, false, it.ts)}</div>
              <div>{@render clamp('fb-' + i, it.finding)}</div>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <!-- ================= corrections: diffs against the record ================= -->
    {#if d.corrections}
      <section class="panel inst corrections">
        <h3>Corrections <span class="chip">{d.corrections.items.length}</span></h3>
        <div class="src">from {d.corrections.source}</div>
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
      <section class="panel inst links">
        <h3>Links</h3>
        <ul class="linklist">{#each d.links.items as l}<li><a href={l.url} target="_blank" rel="noreferrer">{l.url}</a></li>{/each}</ul>
      </section>
    {/if}

    <!-- ================= ledger: the feed ================= -->
    <section class="panel inst ledger" class:open={ledgerOpen}>
      <h3 class="fold">
        <button class="toggle" onclick={() => { ledgerOpen = !ledgerOpen; if (!ledgerOpen) kindFilter = null; }}>
          <span class="tri">{ledgerOpen ? '▾' : '▸'}</span> Ledger <span class="chip">{history.length}</span>
        </button>
        <span class="pills">
          {#each d.counts as [kind, n] (kind)}
            <button class="pill" class:active={ledgerOpen && kindFilter === kind} onclick={() => togglePill(kind)}>{kind} <b>{n}</b></button>
          {/each}
        </span>
      </h3>
      {#if ledgerOpen}
        <ul class="history">
          {#each rows as r (r.id)}
            {#if r.type === 'day'}
              <li class="daysep"><span>{r.day}</span></li>
            {:else if r.type === 'signal'}
              <li class="signal">
                <span class="hglyph">{kindGlyph(r.e.kind)}</span>
                <span class="muted kind">{r.e.kind}</span>
                <span class="line">{eventText(r.e)}</span>
                {#if d.fed.has(r.e.id)}<span class="fedmark">→ {d.fed.get(r.e.id)}</span>{/if}
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
      {/if}
    </section>
  </article>
{/if}

<style>
  .back { margin-bottom: 12px; }
  .dossier { max-width: 920px; margin: 0 auto; display: flex; flex-direction: column; gap: 18px; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; }
  .pair:empty { display: none; }

  /* ---- header ---- */
  .head { padding: 14px 18px 12px; border-left: 4px solid var(--line-strong); }
  .head.s-active { border-left-color: var(--accent); }
  .head.s-blocked { border-left-color: var(--fail); }
  .head.s-done { border-left-color: var(--ok); }
  .titlerow { display: flex; gap: 10px; align-items: center; margin-bottom: 4px; }
  .status { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em; padding: 2px 9px; border-radius: 3px; background: var(--line-strong); color: var(--bg-raised); }
  .status.s-active { background: var(--accent); }
  .status.s-blocked { background: var(--fail); }
  .status.s-done { background: var(--ok); }
  .slug { font-size: 11px; }
  h2 { margin: 0 0 14px; font-size: 18px; line-height: 1.3; }
  .stepper { list-style: none; margin: 0 0 12px; padding: 0; display: grid; grid-template-columns: repeat(4, 1fr); }
  .stepper.terminal { grid-template-columns: repeat(5, 1fr); }
  .step { position: relative; padding-right: 8px; min-width: 0; }
  .step::before { content: ''; position: absolute; left: 14px; right: 0; top: 6px; height: 2px; background: var(--line); }
  .step:last-child::before { display: none; }
  .step.past::before { background: var(--accent); }
  .dot { position: relative; z-index: 1; display: block; width: 14px; height: 14px; border-radius: 50%; background: var(--bg-raised); border: 2px solid var(--line-strong); box-sizing: border-box; }
  .step.past .dot { background: var(--accent); border-color: var(--accent); }
  .step.now .dot { border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 30%, transparent); }
  .step.end .dot { background: var(--ok); border-color: var(--ok); }
  .step .name { display: block; margin-top: 6px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-dim); }
  .step.past .name, .step.end .name { color: var(--text); }
  .step.now .name { color: var(--accent); font-weight: 700; }
  .step .gate { display: block; font-size: 10px; color: var(--text-dim); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .step .gate.human { color: var(--accent); }
  .facts { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 8px; }
  .fact { font-size: 11px; color: var(--text-dim); background: var(--bg-inset); border-radius: 3px; padding: 2px 8px; text-decoration: none; border: none; cursor: default; min-height: 0; line-height: 1.6; }
  .fact b { color: var(--text); font-weight: 500; }
  .fact.link { color: var(--accent); cursor: pointer; }
  .fact.fail { color: var(--fail); }
  .strip { border-top: 1px solid var(--line); padding-top: 6px; }

  /* ---- shared instrument chrome ---- */
  .inst { position: relative; padding: 12px 16px 12px; }
  .inst h3 { display: flex; gap: 8px; align-items: center; margin: 0; font-size: 14px; font-weight: 600; }
  .src { font-size: 10px; color: var(--text-dim); margin: 1px 0 10px; }
  .chip { display: inline-block; font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; padding: 0 6px; border-radius: 3px; border: 1px solid var(--line-strong); color: var(--text-dim); line-height: 1.7; font-weight: 500; white-space: nowrap; }
  .chip.warn { border-color: var(--warn); color: var(--warn); }
  .chip.fail { border-color: var(--fail); color: var(--fail); }
  .label { display: block; font-size: 10px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-dim); margin-bottom: 3px; }
  .field { margin-bottom: 12px; font-size: 12px; }
  .body { white-space: pre-wrap; line-height: 1.5; }
  .body.clamped { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .more { border: none; background: none; padding: 0; color: var(--accent); font-size: 11px; cursor: pointer; min-height: 0; min-width: 0; margin-left: 4px; }
  .sig { display: inline-flex; gap: 6px; align-items: center; font-size: 10px; color: var(--text-dim); }
  .actor { font-size: 10px; text-transform: uppercase; border: 1px solid var(--line-strong); padding: 0 5px; border-radius: 3px; color: var(--text-dim); }
  .actor.human { border-color: var(--accent); color: var(--accent); }

  /* ---- contract: form with a stamp ---- */
  .contract { border-left: 4px solid var(--accent); }
  .contract.draft { border-left-style: dashed; }
  .stamp { position: absolute; top: 12px; right: 14px; display: inline-flex; gap: 8px; align-items: baseline; padding: 3px 10px; border: 1px solid var(--line-strong); border-radius: 3px; color: var(--text-dim); font-size: 10px; line-height: 1.5; background: var(--bg-inset); }
  .stamp b { font-size: 10px; text-transform: uppercase; letter-spacing: 0.1em; }
  .stamp.approved { border-color: var(--ok); color: var(--ok); }
  .stamp.approved b::before { content: '✓ '; }
  .contract h3 { padding-right: 150px; }
  .scope { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 12px; }
  .scope .col { border: 1px solid var(--line); border-radius: 3px; padding: 8px 10px; font-size: 12px; }
  .scope .in { border-color: var(--ok); }
  .scope .in .label { color: var(--ok); }
  .scope .out { border-color: var(--line-strong); }
  .scope ul { margin: 0; padding-left: 16px; }
  .scope li { margin: 2px 0; }
  .scope .out li { color: var(--text-dim); }
  .crithead { display: flex; gap: 10px; align-items: center; margin-bottom: 6px; font-size: 11px; }
  .crithead .label { margin: 0; }
  .meter { display: inline-flex; width: 120px; height: 8px; background: var(--bg-inset); border-radius: 4px; overflow: hidden; }
  .meter i { display: block; height: 100%; }
  .meter .pass { background: var(--ok); }
  .meter .fail { background: var(--fail); }
  .criteria { list-style: none; margin: 0; padding: 0; }
  .criteria li { display: flex; gap: 10px; align-items: baseline; padding: 5px 0; border-bottom: 1px solid var(--line); }
  .criteria li:last-child { border-bottom: none; }
  .box { flex: none; width: 16px; height: 16px; border: 1.5px solid var(--line-strong); border-radius: 3px; font-size: 11px; line-height: 13px; text-align: center; color: var(--bg-raised); transform: translateY(2px); }
  .st-pass .box { background: var(--ok); border-color: var(--ok); }
  .st-fail .box { background: var(--fail); border-color: var(--fail); }
  .crbody { flex: 1; }
  .st-pass .crbody { color: var(--text-dim); }
  .evidence { font-size: 10px; color: var(--ok); white-space: nowrap; }
  .st-fail .evidence { color: var(--fail); }
  .amend { border-top: 1px dashed var(--line-strong); padding-top: 8px; font-size: 11px; }
  .arow { display: flex; gap: 8px; align-items: baseline; flex-wrap: wrap; margin-bottom: 4px; }
  .arow .label { margin: 0; color: var(--warn); }

  /* ---- plan: track ---- */
  .plan { border-left: 4px solid var(--accent-dim); }
  .track { list-style: none; margin: 0; padding: 0 0 0 4px; position: relative; }
  .track::before { content: ''; position: absolute; left: 14px; top: 8px; bottom: 8px; width: 2px; background: var(--line); }
  .track li { position: relative; display: flex; gap: 12px; align-items: baseline; padding: 5px 0; font-size: 12px; }
  .node { position: relative; z-index: 1; flex: none; width: 22px; height: 22px; border-radius: 50%; border: 2px solid var(--line-strong); background: var(--bg-raised); font-size: 10px; line-height: 18px; text-align: center; color: var(--text-dim); box-sizing: border-box; transform: translateY(4px); }
  .track li.done .node { background: var(--accent-dim); border-color: var(--accent-dim); color: var(--bg-raised); }
  .track li.now .node { border-color: var(--accent); color: var(--accent); font-weight: 700; box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .track li.done .sbody { color: var(--text-dim); }
  .track li.now .sbody { font-weight: 600; }
  .sbody { flex: 1; }
  .smeta { font-size: 10px; color: var(--text-dim); white-space: nowrap; }
  .track li.now .smeta { color: var(--accent); }

  /* ---- decisions: forks ---- */
  .decisions { border-left: 4px solid var(--warn); }
  .forks { list-style: none; margin: 0; padding: 0; }
  .forks li { display: flex; gap: 10px; padding: 8px 0; border-bottom: 1px solid var(--line); font-size: 12px; }
  .forks li:last-child { border-bottom: none; }
  .fork { flex: none; font-size: 18px; line-height: 1; color: var(--warn); transform: rotate(180deg); }
  .fbody { flex: 1; min-width: 0; }
  .choice b { font-weight: 600; }
  .choice s { margin-left: 8px; }
  .why { color: var(--text-dim); font-style: italic; margin-top: 2px; }
  .sigline { margin-top: 4px; font-size: 10px; color: var(--text-dim); }
  .sigline.missing { color: var(--warn); }

  /* ---- questions: threads ---- */
  .questions { border-left: 4px solid var(--line-strong); }
  .threads { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .threads li { display: flex; gap: 10px; padding: 8px 10px; border: 1px solid var(--line); border-radius: 3px; font-size: 12px; }
  .threads li.open { border: 1px dashed var(--warn); }
  .qmark { flex: none; width: 20px; height: 20px; border-radius: 50%; text-align: center; line-height: 20px; font-weight: 700; font-size: 12px; background: var(--line-strong); color: var(--bg-raised); }
  .threads li.open .qmark { background: var(--warn); }
  .qbody { flex: 1; min-width: 0; }
  .qmeta { font-size: 10px; color: var(--text-dim); margin-top: 2px; }
  .threads li.open .qmeta { color: var(--warn); }
  .answer { margin-top: 4px; padding-left: 4px; color: var(--text); }

  /* ---- feedback: tally then callouts ---- */
  .feedback { border-left: 4px solid var(--warn); }
  .tally { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 10px; }
  .tcount { font-size: 11px; padding: 2px 9px; border-radius: 999px; background: var(--bg-inset); color: var(--text-dim); }
  .tcount b { font-size: 13px; color: var(--text); margin-right: 3px; }
  .tcount.fail b { color: var(--fail); }
  .tcount.warn b { color: var(--warn); }
  .tsplit { font-size: 11px; margin-left: auto; }
  .callouts { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
  .callouts li { border: 1px solid var(--line); border-left-width: 3px; border-radius: 3px; padding: 8px 10px; font-size: 12px; background: var(--bg-inset); }
  .callouts li.fail { border-left-color: var(--fail); }
  .callouts li.warn { border-left-color: var(--warn); }
  .chead { display: flex; gap: 8px; align-items: center; font-size: 10px; margin-bottom: 4px; }
  .chead .sig { margin-left: auto; }

  /* ---- corrections: diffs ---- */
  .corrections { border-left: 4px solid var(--fail); }
  .diffs { list-style: none; margin: 0; padding: 0; font-size: 12px; }
  .diffs li { padding: 6px 0; border-bottom: 1px solid var(--line); }
  .diffs li:last-child { border-bottom: none; }
  .was, .now { display: flex; gap: 8px; align-items: baseline; padding: 2px 6px; border-radius: 2px; }
  .was { background: color-mix(in srgb, var(--fail) 10%, transparent); color: var(--text-dim); }
  .now { background: color-mix(in srgb, var(--ok) 10%, transparent); }
  .dmark { flex: none; font-family: monospace; font-weight: 700; width: 12px; }
  .was .dmark { color: var(--fail); }
  .now .dmark { color: var(--ok); }
  .was s { flex: 1; }
  .now > span:nth-child(2) { flex: 1; }
  .dwhen { font-size: 10px; white-space: nowrap; }

  .links { border-left: 4px solid var(--line-strong); }
  .linklist { margin: 0; padding-left: 18px; font-size: 12px; }

  /* ---- ledger ---- */
  .ledger { border-left: 4px solid var(--line-strong); padding: 8px 12px; }
  .ledger h3.fold { flex-wrap: wrap; row-gap: 6px; }
  .ledger.open h3.fold { border-bottom: 1px solid var(--line); padding-bottom: 8px; margin-bottom: 4px; }
  .toggle { display: inline-flex; gap: 8px; align-items: center; border: none; background: none; padding: 0; color: var(--text); font: inherit; font-weight: 600; cursor: pointer; min-height: 0; }
  .tri { color: var(--text-dim); }
  .pills { display: flex; gap: 4px; flex-wrap: wrap; margin-left: auto; }
  .pill { font-size: 10px; padding: 0 8px; border-radius: 999px; min-height: 0; line-height: 1.8; }
  .history { list-style: none; padding: 0; margin: 0; }
  .history li { display: flex; gap: 10px; align-items: baseline; padding: 6px 0; border-bottom: 1px solid var(--line); font-size: 12px; }
  .history li.daysep { border-bottom: none; padding: 10px 0 2px; color: var(--text-dim); font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; }
  .history li.transition { padding: 3px 0; }
  .hglyph { color: var(--accent); }
  .kind { min-width: 84px; font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em; }
  .line { flex: 1; min-width: 0; }
  .transition .line { font-size: 11px; }
  .fedmark { font-size: 10px; color: var(--accent); white-space: nowrap; }

  @media (max-width: 700px) {
    .pair, .scope, .callouts { grid-template-columns: 1fr; }
    .head { padding: 12px 12px 10px; }
    h2 { font-size: 16px; }
    .stepper, .stepper.terminal { grid-template-columns: repeat(2, 1fr); row-gap: 12px; }
    .step:nth-child(2n)::before { display: none; }
    .step .gate { white-space: normal; }
    .inst { padding: 12px 12px; }
    .stamp { position: static; align-self: flex-start; margin-bottom: 8px; }
    .contract h3 { padding-right: 0; }
    .evidence { white-space: normal; flex-basis: 100%; padding-left: 26px; }
    .criteria li { flex-wrap: wrap; }
    .smeta { white-space: normal; }
    .pills { margin-left: 0; }
  }
  @media (pointer: coarse) {
    .pill, .more { min-height: 36px; }
  }
</style>
