// App-wide UI state: view routing (hash), theme, board/arcs data.

import { get } from './api.svelte.js';
import { seedLedger } from './live.svelte.js';

// ---- routing ------------------------------------------------------------
// Hash routes: #/inbox | #/arcs | #/board | #/journal | #/analytics | #/ticket/<ref> | #/timeline/<arcRef>
// Refresh and iPad tab-switching restore the exact view.

export const view = $state({
  name: 'arcs', // inbox | arcs | board | journal | timeline | ticket | analytics
  ticketRef: null,
  arcRef: null
});

function parseHash() {
  const h = location.hash.replace(/^#\/?/, '');
  const [name, ref] = h.split('/');
  if (name === 'ticket' && ref) return { name: 'ticket', ticketRef: decodeURIComponent(ref), arcRef: null };
  if (name === 'timeline' && ref) return { name: 'timeline', ticketRef: null, arcRef: decodeURIComponent(ref) };
  if (['inbox', 'arcs', 'board', 'journal', 'analytics'].includes(name)) return { name, ticketRef: null, arcRef: null };
  return null;
}

function apply(p) {
  view.name = p.name;
  view.ticketRef = p.ticketRef;
  view.arcRef = p.arcRef;
  if (p.name === 'board' || p.name === 'inbox') loadBoard();
  if (p.name === 'arcs') loadArcs();
}

// programmatic navigation: set hash, let the hashchange handler apply it
export function show(name) {
  location.hash = '#/' + name;
}

export function openTicket(ref) {
  location.hash = `#/ticket/${encodeURIComponent(ref)}`;
}

export function openArc(ref) {
  location.hash = `#/timeline/${encodeURIComponent(ref)}`;
}

// No hash: a phone lands on the inbox (what is waiting on you), a wider
// screen on arcs. Explicit hashes are honoured as they are.
export function defaultView() {
  return window.innerWidth < 700 ? 'inbox' : 'arcs';
}

export function routeFromLocation() {
  apply(parseHash() || { name: defaultView(), ticketRef: null, arcRef: null });
}

export function onRouteChange() {
  window.addEventListener('hashchange', () => {
    const p = parseHash();
    if (p) apply(p);
  });
}

// ---- theme ---------------------------------------------------------------
export const ui = $state({
  mode: localStorage.getItem('servitor_mode') || 'dark', // light | dark
  accent: localStorage.getItem('servitor_accent') || 'forge' // forge | auspex
});

export function setMode(m) {
  ui.mode = m;
  localStorage.setItem('servitor_mode', m);
  document.documentElement.dataset.mode = m;
}

export function setAccent(a) {
  ui.accent = a;
  localStorage.setItem('servitor_accent', a);
  document.documentElement.dataset.accent = a;
}

// ---- data ----------------------------------------------------------------
export const board = $state({ cards: [], error: null });
export const arcs = $state({ list: [], error: null });

export async function loadArcs() {
  try {
    arcs.list = await get('/api/arcs');
    arcs.error = null;
  } catch (e) {
    arcs.error = e.message;
  }
}

export async function loadBoard() {
  try {
    board.cards = await get('/api/board');
    board.error = null;
    seedLedger(board.cards);
  } catch (e) {
    board.error = e.message;
  }
}

// ---- display helpers -------------------------------------------------------
const WORD_CLASS = { shaping: 'word-shaping', building: 'word-building', checking: 'word-checking', shipping: 'word-shipping' };
export function wordClass(w) {
  return WORD_CLASS[w] || '';
}

const KIND_GLYPH = {
  note: '✎', decision: '⚖', gate: '⚖', feedback: '✦',
  'ticket.create': '✚', 'status.set': '⇄', 'field.set': '≡',
  'subitem.add': '＋', 'subitem.set': '≡', 'subitem.rank': '↕', hook: '⚡'
};
export function kindGlyph(kind) {
  return KIND_GLYPH[kind] || '•';
}

// Human line for any event kind, using the payload keys that actually
// exist ({v} note, {what,why} decision, {gate} gate, {status,on} status.set,
// {field,v} field.set, {finding,source} feedback).
export function eventText(e) {
  const p = e.payload || {};
  switch (e.kind) {
    case 'gate': return `gate passed: ${p.gate || '?'}`;
    case 'status.set': return p.status === 'blocked' ? `blocked on ${p.on || '?'}` : p.status || '';
    case 'field.set': {
      if (!('v' in p)) return `cleared ${p.field}`;
      const v = typeof p.v === 'string' ? p.v : JSON.stringify(p.v);
      return `${p.field} = ${v === '' ? "''" : v}`;
    }
    case 'ticket.create': return p.title ? `created — ${p.title}` : 'created';
    case 'decision': return p.what ? `${p.what}${p.why ? ' — ' + p.why : ''}` : 'decision';
    case 'feedback': return p.finding || 'feedback';
    case 'subitem.add': return p.body || `added ${p.kind || 'subitem'}`;
    case 'subitem.set': return p.body || 'updated subitem';
    case 'subitem.rank': return 'reordered subitem';
    default: return p.v || p.body || p.text || p.what || p.finding || '';
  }
}

export function relTs(ts) {
  if (!ts) return '';
  const d = (Date.now() - new Date(ts)) / 1000;
  if (d < 90) return 'just now';
  if (d < 3600) return Math.round(d / 60) + 'm ago';
  if (d < 86400) return Math.round(d / 3600) + 'h ago';
  if (d < 86400 * 14) return Math.round(d / 86400) + 'd ago';
  return new Date(ts).toLocaleDateString();
}

export function ageOf(ts, staleDays = 3) {
  if (!ts) return null;
  const d = (Date.now() - new Date(ts)) / 86400000;
  return { days: d, stale: d >= staleDays };
}

// Age source rule, shared by every view that shows age: blocked tickets
// age from blocked_since, everything else from last activity.
export function ageOfState(status, blockedSince, updatedAt) {
  const ts = status === 'blocked' && blockedSince ? blockedSince : updatedAt;
  return ts ? { ts, ...ageOf(ts) } : null;
}

export function fmtTs(ts) {
  return new Date(ts).toLocaleString();
}
