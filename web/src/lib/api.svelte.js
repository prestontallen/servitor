// API client. Session-cookie auth (user-auth ticket): POST /api/login sets
// an HttpOnly cookie; fetch and EventSource send it same-origin with no
// token handling in JS. A 401 flips auth.needed and the shell shows the
// login view. The legacy ?token= flow (SERVITOR_TOKEN bootstrap) still
// works when a token is set.
//
// The logic lives in auth.js (rune-free, node:test covered); this module
// binds it to the $state the shell reads.
import { createAuth, boot } from './auth.js';

const TOKEN_KEY = 'servitor_token';

export const token = $state({ value: localStorage.getItem(TOKEN_KEY) || '' });

// auth drives the shell: needed shows Login/Setup; actor is who logged in.
// auth.js holds the logic against its own plain object; this $state is a
// COPY, kept in step by sync() after every mutation — a $state proxy does
// not track writes to the raw object behind it.
const _auth = createAuth();
export const auth = $state({ needed: false, actor: '', error: '', mode: 'login' });

function sync() {
  auth.needed = _auth.state.needed;
  auth.actor = _auth.state.actor;
  auth.error = _auth.state.error;
  auth.mode = _auth.state.mode || 'login';
}

export async function login(user, password) {
  const actor = await _auth.login(user, password);
  sync();
  return actor;
}

// First-run bootstrap: create the first user, then log in with it.
export async function setup(user, password) {
  auth.error = '';
  const r = await fetch('/api/user', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ username: user, password, actor: '' })
  });
  if (!r.ok) {
    auth.error = `could not create the user (${r.status})`;
    throw new Error(auth.error);
  }
  return login(user, password);
}

// Boot-time check: with no users at all, the first page is the setup view;
// with a live session, learn who we are; with neither, show login.
export async function initAuth() {
  const outcome = await boot(
    () => fetch('/api/auth-state').then((r) => r.json()),
    () => fetch('/api/me')
  );
  if (outcome.mode === 'setup') {
    _auth.state.needed = true;
    _auth.state.mode = 'setup';
  } else if (outcome.mode === 'login') {
    _auth.state.needed = true;
    _auth.state.mode = 'login';
  } else {
    _auth.state.actor = outcome.actor;
  }
  sync();
}

// Change the session user's password. Other sessions are revoked
// server-side; this one survives.
export async function changePassword(current, next) {
  auth.error = '';
  const r = await fetch('/api/password', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ current, new: next })
  });
  if (!r.ok) {
    auth.error = r.status === 401 ? 'current password is wrong' : `change failed (${r.status})`;
    throw new Error(auth.error);
  }
  return true;
}

export async function logout() {
  await _auth.logout();
  sync();
}

export function setToken(t) {
  token.value = t;
  if (t) localStorage.setItem(TOKEN_KEY, t);
  else localStorage.removeItem(TOKEN_KEY);
}

function withToken(path) {
  if (!token.value) return path;
  return path + (path.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(token.value);
}

function onDenied() {
  token.value = '';
  localStorage.removeItem(TOKEN_KEY);
  _auth.onDenied();
  sync();
}

export async function get(path) {
  const r = await fetch(withToken(path));
  if (r.status === 401) {
    onDenied();
    throw new Error('401 unauthorized');
  }
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  return r.json();
}

// One GraphQL read: POST {query, variables} to /api/graphql, same auth as
// get(). A GraphQL error becomes an Error carrying the server's stable code
// (extensions.code) so callers can key on it like REST's.
export async function gql(query, variables = {}) {
  const r = await fetch(withToken('/api/graphql'), {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ query, variables })
  });
  if (r.status === 401) {
    onDenied();
    throw new Error('401 unauthorized');
  }
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  const body = await r.json();
  if (body.errors?.length) {
    const err = new Error(body.errors[0].message);
    err.code = body.errors[0].extensions?.code;
    throw err;
  }
  return body.data;
}

export function streamURL() {
  return withToken('/api/events/stream');
}
