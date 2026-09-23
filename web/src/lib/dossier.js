// Fold a ticket aggregate (/api/ticket/{ref}) and its history
// (/api/ticket/{ref}/history) into typed dossier records. Pure functions,
// no Svelte, so `node --test` covers them.
//
// The ledger holds three shapes and each instrument on the ticket page
// draws one of them:
//   documents  — contract, plan: latest state plus a version history
//   intervals  — questions (asked .. answered), plan steps (added .. done)
//   points     — decisions, feedback, corrections, gates
//
// Structured sources (contract/review events, subitems, decision/feedback
// events, gates; shapes in skills/servitor/references/events.md) come
// first; when a ticket only has notes, the conventions agents already use
// fill the instrument instead, and the record says so in `source`:
//   "Intake: ..." / "INTAKE (tier 2, ...): ..." / "Contract ...: ..." -> contract
//   "DECISION: ..." / "DECISION (chat, Preston): ..."                -> decisions
//   "CORRECTION: ..."                                                -> corrections
//   "... open question ..." / "Question: ..."                        -> questions
//   "ANSWER: ..."                                                    -> answers the latest open question

// Flowcharts come from the flow lib: foldFlow picks and validates the
// latest flow.set snapshot. Imported here so buildDossier is the one place
// every instrument is assembled.
import { foldFlow as flow } from './flow.js';

const INTAKE = /^\s*intake\b[^:\n]*:/i;
const CONTRACT = /^\s*contract\b[^:\n]*:/i;
const DECISION = /^\s*decision\b[^:\n]*:/i;
const CORRECTION = /^\s*correction\b[^:\n]*:/i;
const ANSWER = /^\s*answer\b[^:\n]*:/i;

const strip = (body, re) => body.replace(re, '').trim();
const t = (iso) => (iso ? new Date(iso).getTime() : null);
const byId = (a, b) => a.id - b.id;

export function notes(history) {
  return history
    .filter((e) => e.kind === 'note' && typeof e.payload?.v === 'string')
    .sort(byId)
    .map((e) => ({ id: e.id, ts: e.ts, actor: e.actor, actor_type: e.actor_type, body: e.payload.v }));
}

// ---- contract (document) ---------------------------------------------------

// Split contract prose into labelled sections. Agents write the contract
// as one paragraph with inline labels ("Intent: ... In: ... Out: ..."),
// or as lines; both split on the label tokens.
const SECTION = /\b(intent|in scope|in|out of scope|out|not doing|verification|verify|risks?|open questions?)\s*:/gi;
const SECTION_KEY = { 'in scope': 'in', in: 'in', 'out of scope': 'out', out: 'out', 'not doing': 'out', verify: 'verification', risk: 'risks', 'open question': 'questions', 'open questions': 'questions' };

export function splitContract(body) {
  const out = { intent: null, in: [], out: [], verification: null, risks: null, questions: [] };
  const parts = body.split(SECTION);
  if (parts.length < 3) return { ...out, intent: body.trim() };
  // parts: [preamble, label, text, label, text, ...]
  if (parts[0].trim()) out.intent = parts[0].trim();
  for (let i = 1; i < parts.length; i += 2) {
    const key = SECTION_KEY[parts[i].toLowerCase()] || parts[i].toLowerCase();
    const text = parts[i + 1].trim().replace(/[.;]\s*$/, '');
    if (key === 'in' || key === 'out' || key === 'questions') {
      const items = text.split(/;\s+|\n+|\s+·\s+|\s+•\s+/).map((s) => s.trim()).filter(Boolean);
      out[key].push(...items);
    } else if (key in out) {
      out[key] = out[key] ? out[key] + ' ' + text : text;
    }
  }
  return out;
}

export function contract(doc, history, ns) {
  const gates = doc.gates || [];
  const approved = gates.find((g) => g.gate === 'contract_approved') || null;
  const sets = history.filter((e) => e.kind === 'subitem.set').sort(byId);
  const criteria = (doc.criteria || []).map((c) => {
    const state = c.state === 'pass' || c.state === 'fail' ? c.state : 'open';
    // evidence: the subitem.set that put it in pass/fail, plus the text it carried
    const ev = sets.filter((e) => e.payload?.ulid && c.ulid?.startsWith(e.payload.ulid) && (e.payload.state === 'pass' || e.payload.state === 'fail')).pop();
    const text = c.evidence || ev?.payload?.evidence || null;
    return { body: c.body, state, evidence: ev || text ? { actor: ev?.actor || null, ts: ev?.ts || null, id: ev?.id || null, text } : null };
  });
  const prose = ns.filter((n) => INTAKE.test(n.body) || CONTRACT.test(n.body));
  const intake = prose.find((n) => INTAKE.test(n.body)) || null;
  const contractNotes = prose.filter((n) => CONTRACT.test(n.body));
  // the newest contract event is the document; contract notes are the
  // fallback for tickets that predate the event
  const events = history.filter((e) => e.kind === 'contract').sort(byId);
  const cev = events[events.length - 1] || null;
  const docNote = contractNotes[contractNotes.length - 1] || null;
  const sections = cev
    ? { intent: cev.payload?.intent || null, in: cev.payload?.in || [], out: cev.payload?.out || [], verification: cev.payload?.verification || null, risks: cev.payload?.risks || null }
    : docNote ? splitContract(strip(docNote.body, CONTRACT)) : intake ? splitContract(strip(intake.body, INTAKE)) : null;
  const sources = [];
  if (criteria.length) sources.push('criteria subitems');
  if (cev) sources.push('contract event');
  else if (docNote) sources.push('contract note');
  else if (intake) sources.push('intake note');
  if (!sources.length && !approved) return null;
  const tier = intake?.body.match(/tier\s*(\d)/i)?.[1] || null;
  const complexity = (intake?.body.match(/complexity[:\s]*(low|medium|high)/i) || intake?.body.match(/(low|medium|high)\s+complexity/i))?.[1]?.toLowerCase() || null;
  const amendments = approved
    ? ns.filter((n) => t(n.ts) > t(approved.ts) && /\bamend/i.test(n.body) && !CORRECTION.test(n.body)).map((n) => ({ id: n.id, ts: n.ts, actor: n.actor, body: n.body }))
    : [];
  const tally = { pass: 0, fail: 0, open: 0 };
  for (const c of criteria) tally[c.state]++;
  return {
    source: sources.join(' + ') || 'gate only',
    approved,
    tier,
    complexity,
    intent: sections?.intent || null,
    in: sections?.in || [],
    out: sections?.out || [],
    verification: sections?.verification || null,
    risks: sections?.risks || null,
    criteria,
    tally,
    amendments,
    versions: cev ? events.length : contractNotes.length,
    fed: [intake?.id, ...(cev ? [] : contractNotes.map((n) => n.id))].filter(Boolean)
  };
}

// ---- plan (track) ----------------------------------------------------------

export function plan(doc, history) {
  const items = doc.plan || [];
  if (!items.length) return null;
  const adds = history.filter((e) => e.kind === 'subitem.add' && e.payload?.kind === 'plan').sort(byId);
  const sets = history.filter((e) => e.kind === 'subitem.set').sort(byId);
  const steps = items.map((p) => {
    const add = adds.find((e) => e.payload.body === p.body) || null;
    const done = p.state ? sets.filter((e) => e.payload?.ulid && p.ulid?.startsWith(e.payload.ulid) && e.payload.state).pop() || null : null;
    return { body: p.body, state: p.state || null, added: add ? { ts: add.ts, actor: add.actor } : null, done: done ? { ts: done.ts, actor: done.actor } : null };
  });
  // durations: from the previous step's finish (or this step's add) to its finish
  let prevEnd = null;
  for (const s of steps) {
    const start = prevEnd ?? t(s.added?.ts);
    s.took = s.done && start ? t(s.done.ts) - start : null;
    if (s.done) prevEnd = t(s.done.ts);
  }
  const current = steps.findIndex((s) => !s.state);
  return { source: 'plan subitems', steps, current, done: steps.filter((s) => s.state).length };
}

// ---- decisions (points, forks) ----------------------------------------------

// "X over Y because Z": only on short statements, so prose never splits
const FORK = /^(.{1,80}?)\s+(?:over|instead of|rather than)\s+(.{1,80}?)(?:\s+because\s+(.+))?$/i;

export function decisions(doc, history, ns) {
  const items = [];
  const seen = new Set();
  const push = (d) => {
    const key = (d.what || '').trim().toLowerCase();
    if (!key || seen.has(key)) return;
    seen.add(key);
    // "X over Y because Z" -> chosen / rejected
    const m = d.what.length <= 200 ? d.what.match(FORK) : null;
    const why = d.why || m?.[3] || null;
    let what = d.what;
    // the CLI once glued --why onto what; trim a trailing duplicate of why
    if (why && what.endsWith(why)) what = what.slice(0, -why.length).trim();
    items.push({ ...d, what, why, chosen: m ? m[1] : what, rejected: m ? m[2].replace(new RegExp('\\s+because\\s+.*$', 'i'), '') : null });
  };
  for (const e of history.filter((e) => e.kind === 'decision').sort(byId)) {
    push({ what: e.payload?.what || '', why: e.payload?.why || null, actor: e.actor, actor_type: e.actor_type, ts: e.ts, id: e.id, via: 'event' });
  }
  for (const d of doc.decisions || []) push({ what: d.what, why: d.why || null, actor: null, actor_type: null, ts: null, id: null, via: 'subitem' });
  for (const n of ns.filter((n) => DECISION.test(n.body))) {
    push({ what: strip(n.body, DECISION), why: null, actor: n.actor, actor_type: n.actor_type, ts: n.ts, id: n.id, via: 'note' });
  }
  if (!items.length) return null;
  const sources = [];
  if (items.some((i) => i.via !== 'note')) sources.push('decision events');
  if (items.some((i) => i.via === 'note')) sources.push('DECISION: notes');
  return { source: sources.join(' + '), items, fed: items.filter((i) => i.via === 'note').map((i) => i.id) };
}

// ---- questions (intervals: asked .. answered) --------------------------------

const Q_SENTENCE = /open question[^.?\n]*[.?]?/i;

export function questions(doc, ns, now = Date.now(), consumed = new Set()) {
  const items = (doc.questions || []).map((q) => ({
    body: q.body, ts: null, actor: null, via: 'subitem',
    answer: q.state ? { body: q.state, ts: null, actor: null } : null
  }));
  for (const n of ns) {
    if (consumed.has(n.id)) continue;
    if (ANSWER.test(n.body)) {
      const open = [...items].reverse().find((q) => !q.answer);
      if (open) open.answer = { body: strip(n.body, ANSWER), ts: n.ts, actor: n.actor, id: n.id };
    } else if (/^\s*question\b[^:\n]*:/i.test(n.body)) {
      items.push({ body: n.body.replace(/^\s*question\b[^:\n]*:/i, '').trim(), ts: n.ts, actor: n.actor, via: 'note', id: n.id, answer: null });
    } else if (Q_SENTENCE.test(n.body)) {
      // a note that raises a question mid-prose: keep only that sentence
      items.push({ body: n.body.match(Q_SENTENCE)[0].trim(), ts: n.ts, actor: n.actor, via: 'note', id: n.id, answer: null });
    }
  }
  if (!items.length) return null;
  for (const q of items) {
    const end = q.answer?.ts ? t(q.answer.ts) : now;
    q.openFor = q.ts ? end - t(q.ts) : null;
  }
  const sources = [];
  if (items.some((i) => i.via === 'subitem')) sources.push('question subitems');
  if (items.some((i) => i.via === 'note')) sources.push('question notes');
  return { source: sources.join(' + '), items, open: items.filter((q) => !q.answer).length, fed: items.flatMap((q) => [q.id, q.answer?.id]).filter(Boolean) };
}

// ---- feedback (points, tallied) ----------------------------------------------

export function feedback(doc, history) {
  const events = history.filter((e) => e.kind === 'feedback').sort(byId);
  const items = events.length
    ? events.map((e) => ({ finding: e.payload?.finding || '', source: e.payload?.source || null, tag: e.payload?.tag || null, actor: e.actor, ts: e.ts, id: e.id }))
    : (doc.feedback || []).map((f) => ({ finding: f.finding, source: f.source || null, tag: f.tag || null, actor: f.actor, ts: f.ts, id: f.id }));
  if (!items.length) return null;
  const byTag = {};
  const bySource = {};
  for (const f of items) {
    byTag[f.tag || 'untagged'] = (byTag[f.tag || 'untagged'] || 0) + 1;
    bySource[f.source || 'unknown'] = (bySource[f.source || 'unknown'] || 0) + 1;
  }
  return { source: 'feedback events', items, byTag, bySource };
}

// ---- corrections (points that amend an earlier note) ---------------------------

const WORD = /[a-z][a-z0-9_-]{5,}/g;
function overlap(a, b) {
  const wa = new Set((a.toLowerCase().match(WORD) || []));
  let n = 0;
  for (const w of b.toLowerCase().match(WORD) || []) if (wa.has(w)) n++;
  return n;
}

export function corrections(ns) {
  const items = [];
  for (const n of ns) {
    if (!CORRECTION.test(n.body)) continue;
    const body = strip(n.body, CORRECTION);
    // the note it amends: the earlier note sharing the most distinctive words
    let best = null, score = 1;
    for (const m of ns) {
      if (m.id >= n.id || CORRECTION.test(m.body)) continue;
      const s = overlap(body, m.body);
      if (s > score) { score = s; best = m; }
    }
    items.push({ body, ts: n.ts, actor: n.actor, id: n.id, amends: best ? { id: best.id, ts: best.ts, body: best.body } : null });
  }
  if (!items.length) return null;
  return { source: 'CORRECTION: notes', items, fed: items.map((i) => i.id) };
}

// ---- ledger --------------------------------------------------------------------

export function ledgerCounts(history) {
  const counts = {};
  for (const e of history) counts[e.kind] = (counts[e.kind] || 0) + 1;
  return Object.entries(counts).sort((a, b) => b[1] - a[1]);
}

// The whole dossier. `fed` is the set of note ids an instrument consumed,
// so the ledger can mark them instead of repeating them.
export function buildDossier(doc, history = [], now = Date.now()) {
  const ns = notes(history);
  const d = {
    contract: contract(doc, history, ns),
    plan: plan(doc, history),
    decisions: decisions(doc, history, ns),
    corrections: corrections(ns),
    feedback: feedback(doc, history),
    flow: flow(history),
    links: (doc.links || []).length ? { source: 'link subitems', items: doc.links.map((l) => ({ url: l.url })) } : null,
    counts: ledgerCounts(history)
  };
  const consumed = new Set([d.contract, d.decisions, d.corrections].flatMap((i) => i?.fed || []));
  d.questions = questions(doc, ns, now, consumed);
  const fed = new Map();
  for (const [name, inst] of Object.entries(d)) for (const id of inst?.fed || []) fed.set(id, name);
  d.fed = fed;
  d.empty = !d.contract && !d.plan && !d.decisions && !d.questions && !d.feedback && !d.corrections && !d.links;
  return d;
}
