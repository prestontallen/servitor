// The prompt line's grammar, pure so `node --test` covers it. A line is a
// verb and at most one argument; the component only dispatches on the
// result. Views are the hash routes the app already serves.

export const VIEWS = ['board', 'time', 'arcs', 'journal'];
const ALIASES = { b: 'board', t: 'time', a: 'arcs', j: 'journal', ledger: 'journal' };

export const HELP = 'verbs: board · time · arcs · journal · ctx <ref> · help';

// parse('board')          -> {action: 'view', name: 'board'}
// parse('ctx flow-diagram') -> {action: 'ticket', ref: 'flow-diagram'}
// parse('')               -> {action: 'noop'}
// parse('help')           -> {action: 'help', text: HELP}
// parse('rm -rf')         -> {action: 'unknown', verb: 'rm'}
export function parse(line) {
  const words = String(line || '').trim().split(/\s+/).filter(Boolean);
  if (!words.length) return { action: 'noop' };
  const verb = words[0].toLowerCase();
  const arg = words[1];
  const name = ALIASES[verb] || verb;
  if (VIEWS.includes(name)) return { action: 'view', name };
  if ((verb === 'ctx' || verb === 'ticket' || verb === 'open') && arg) return { action: 'ticket', ref: arg };
  if (verb === 'ctx' || verb === 'ticket' || verb === 'open') return { action: 'unknown', verb, why: 'needs a ticket ref' };
  if (verb === 'help' || verb === '?') return { action: 'help', text: HELP };
  return { action: 'unknown', verb };
}

// Digit keys switch views when focus is not in a field: '1' is the first view.
export function viewForKey(key) {
  const i = Number(key) - 1;
  return Number.isInteger(i) && i >= 0 && i < VIEWS.length ? VIEWS[i] : null;
}
