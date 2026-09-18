// App-wide UI state: view routing, theme, board data.

import { get } from './api.svelte.js';
import { seedLedger } from './live.svelte.js';

export const view = $state({
  name: 'ledger', // ledger | board | ticket | analytics
  ticketRef: null
});

export function show(name) {
  view.name = name;
  view.ticketRef = null;
  if (name === 'board') loadBoard();
}

export function openTicket(ref) {
  view.name = 'ticket';
  view.ticketRef = ref;
}

export const theme = $state({
  name: localStorage.getItem('servitor_theme') || 'forge'
});

export function setTheme(t) {
  theme.name = t;
  localStorage.setItem('servitor_theme', t);
  document.documentElement.dataset.theme = t;
}

export const board = $state({ cards: [], error: null });

export async function loadBoard() {
  try {
    board.cards = await get('/api/board');
    board.error = null;
    seedLedger(board.cards);
  } catch (e) {
    board.error = e.message;
  }
}

const WORD_CLASS = { shaping: 'word-shaping', building: 'word-building', checking: 'word-checking', shipping: 'word-shipping' };
export function wordClass(w) {
  return WORD_CLASS[w] || '';
}

const KIND_GLYPH = {
  note: '✎', decision: '⚖', status_change: '⇄', field_diff: '≡',
  contract_approved: '⊗', presented: '◎', shipped: '▲',
  ticket_create: '✚', gate: '⊗', hook: '⚡'
};
export function kindGlyph(kind) {
  return KIND_GLYPH[kind] || '•';
}

export function eventText(e) {
  const p = e.payload || {};
  return p.v || p.text || p.body || p.what || p.title || p.status || '';
}

export function fmtTs(ts) {
  return new Date(ts).toLocaleString();
}
