// Auth-state logic, rune-free so node:test can drive it (the repo's web
// test runner). api.svelte.js wraps this in $state.

export function createAuth(fetchImpl = fetch) {
  const state = { needed: false, actor: '', error: '' };

  async function login(user, password) {
    state.error = '';
    const r = await fetchImpl('/api/login', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ user, password })
    });
    if (!r.ok) {
      state.error = r.status === 401 ? 'Invalid user or password' : `login failed (${r.status})`;
      throw new Error(state.error);
    }
    state.actor = (await r.json()).actor;
    state.needed = false;
    return state.actor;
  }

  async function logout() {
    await fetchImpl('/api/logout', { method: 'POST' });
    state.actor = '';
    state.needed = true;
    state.mode = 'login';
  }

  // A 401 on any data read: the session is gone — clear the legacy token
  // and show the login view.
  function onDenied() {
    state.needed = true;
  }

  return { state, login, logout, onDenied };
}

// Boot decision, pure and node:test covered. authStateFetch -> {first_run};
// meFetch -> /api/me (200 with {actor} on a live session, 401 otherwise).
// setup: no users yet; login: users exist but this browser has no session;
// ok: session live, actor attached.
export async function boot(authStateFetch, meFetch) {
  try {
    const s = await authStateFetch();
    if (s && s.first_run) return { mode: 'setup' };
  } catch {
    // daemon unreachable: fall through and let the data loads surface it
    return { mode: 'ok', actor: '' };
  }
  try {
    const me = await meFetch();
    if (me.ok) return { mode: 'ok', actor: (await me.json()).actor };
  } catch {
    // fall through to login
  }
  return { mode: 'login' };
}
