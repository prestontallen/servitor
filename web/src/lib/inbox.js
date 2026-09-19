// What is waiting on the human, and the per-card activity sparkline.
// Pure functions over the board payload and the seeded ledger stream
// (live.events), so `node --test` covers them and the board's amber
// edges and the arcs "needs you" panel cannot disagree: both call
// attention().

const DAY = 86400000;
const STALE_DAYS = 3;
const HUMAN_ON = /^(human|preston)\b/i;
const CONTRACT = /^\s*contract\b[^:\n]*:/i;

const t = (iso) => (iso ? new Date(iso).getTime() : null);

// latest pr from field.set events, and the timestamps of the gates and
// the last CONTRACT: note, per ticket
function facts(events, ulid) {
  const mine = events.filter((e) => (e.ticket || e.ticket_ulid) === ulid).sort((a, b) => a.id - b.id);
  const out = { pr: null, gates: {}, contractNote: null };
  for (const e of mine) {
    if (e.kind === 'field.set' && e.payload?.field === 'pr') out.pr = 'v' in (e.payload || {}) ? e.payload.v : null;
    if (e.kind === 'gate' && e.payload?.gate) out.gates[e.payload.gate] = e.ts;
    if (e.kind === 'note' && CONTRACT.test(e.payload?.v || '')) out.contractNote = e.ts;
  }
  return out;
}

// Rows the human has to act on, oldest wait first. `events` is the
// seeded ledger (recent history per board ticket); it only sharpens the
// rows (pr, gate times, contract note) and every rule degrades to the
// board payload alone.
export function attention(cards, events = [], now = Date.now()) {
  const rows = [];
  for (const c of cards) {
    const title = c.title || c.slug;
    const f = facts(events, c.ulid);
    if (c.status === 'blocked') {
      if (!HUMAN_ON.test(c.blocked_on || '')) continue;
      rows.push({ ulid: c.ulid, title, rule: 'blocked', sev: 'high',
        why: `blocked on ${c.blocked_on}`, since: c.blocked_since || c.updated_at });
      continue;
    }
    if (c.status !== 'active') continue;
    if (c.card_word === 'checking') {
      rows.push({ ulid: c.ulid, title, rule: 'presented', sev: 'mid',
        why: f.pr ? `presented · ${f.pr.replace(/^https?:\/\/github\.com\//, '')} open` : 'presented · awaiting acceptance',
        since: f.gates.presented || c.updated_at });
    } else if (c.card_word === 'shipping') {
      rows.push({ ulid: c.ulid, title, rule: 'shipped', sev: 'mid',
        why: 'shipped, still active', since: f.gates.shipped || c.updated_at });
    } else if ((!c.card_word || c.card_word === 'shaping') && f.contractNote) {
      rows.push({ ulid: c.ulid, title, rule: 'contract', sev: 'mid',
        why: 'contract drafted · awaiting your approval', since: f.contractNote });
    } else if (now - t(c.updated_at) >= STALE_DAYS * DAY) {
      rows.push({ ulid: c.ulid, title, rule: 'stale', sev: 'mid',
        why: `active but silent ${Math.floor((now - t(c.updated_at)) / DAY)}d`, since: c.updated_at });
    }
  }
  return rows.sort((a, b) => t(a.since) - t(b.since));
}

// Daily event counts for the last `days` days (oldest first) and whether
// a human touched the ticket that day.
export function sparkline(events, ulid, now = Date.now(), days = 7) {
  const counts = new Array(days).fill(0);
  const human = new Array(days).fill(false);
  const start = now - days * DAY;
  for (const e of events) {
    if ((e.ticket || e.ticket_ulid) !== ulid) continue;
    const ts = t(e.ts);
    if (ts < start || ts > now) continue;
    const i = Math.min(days - 1, Math.floor((ts - start) / DAY));
    counts[i]++;
    if (e.actor_type === 'human') human[i] = true;
  }
  return { counts, human };
}

// Which lane a board card belongs in. Blocked cards go to the rail;
// active cards follow their card word; queued cards wait on the left.
export const LANES = ['queued', 'shaping', 'building', 'checking', 'shipping'];
export function laneOf(card) {
  if (card.status === 'blocked') return 'blocked';
  if (card.status === 'queued') return 'queued';
  return LANES.includes(card.card_word) ? card.card_word : 'shaping';
}
