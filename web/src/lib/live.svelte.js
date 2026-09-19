// Live store: SSE connection with watermark resync + the global ledger
// stream. The SSE payload is {event_id, ticket, kind}; the stream merges
// each changed ticket's recent history, deduping by the global event_id.
// No new endpoints required (contract scope).

import { get, streamURL } from './api.svelte.js';

export const live = $state({
  status: 'connecting', // connecting | live | down
  head: 0, // watermark: max global ledger event id seen
  events: [], // ledger events, newest first, deduped by id
  flashes: {}, // event id -> true while the live flash plays
  changeCount: 0
});

let es = null;
let backoff = 1000;

export function connectStream() {
  if (es) es.close();
  es = new EventSource(streamURL());
  // open fires when the HTTP stream is established, regardless of whether
  // any change events flow — this, not the change handler, is the
  // "connected" signal.
  es.onopen = () => {
    live.status = 'live';
    backoff = 1000;
  };
  es.addEventListener('change', (e) => {
    live.status = 'live';
    const c = JSON.parse(e.data);
    if ((c.event_id || 0) > live.head) live.head = c.event_id;
    ingest(c.ticket);
  });
  es.onerror = () => {
    // Don't trust native EventSource reconnection after a servitord
    // restart (it has stalled on "signal lost" here). Close and dial
    // again ourselves, capped exponential backoff, reset on onopen.
    live.status = 'down';
    es.close();
    setTimeout(connectStream, backoff);
    backoff = Math.min(backoff * 2, 30000);
  };
}

// Pull a ticket's recent history and merge unseen events into the stream.
async function ingest(ticket) {
  try {
    const evs = await get(`/api/ticket/${ticket}/history?limit=5`);
    merge(evs);
  } catch {
    /* transient; the watermark replays on next change */
  }
}

export function merge(incoming) {
  const seen = new Set(live.events.map((e) => e.id));
  const fresh = incoming
    .filter((e) => !seen.has(e.id))
    .map((e) => ({ ...e, ticket: e.ticket || e.ticket_ulid }));
  if (fresh.length === 0) return;
  for (const e of fresh) {
    live.flashes[e.id] = true;
    setTimeout(() => delete live.flashes[e.id], 1300);
  }
  live.events = [...live.events, ...fresh].sort((a, b) => b.id - a.id);
  for (const e of live.events) {
    if (e.id > live.head) live.head = e.id;
  }
  live.changeCount += fresh.length;
}

// Seed the stream from the board: each ticket's recent history.
export async function seedLedger(cards) {
  const results = await Promise.allSettled(
    cards.map((c) => get(`/api/ticket/${c.ulid}/history?limit=100`))
  );
  merge(results.flatMap((r) => (r.status === 'fulfilled' ? r.value : [])));
}
