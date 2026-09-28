// API client. Same token flow as the old vanilla GUI: token stored in
// localStorage, appended as ?token= (EventSource can't set headers).

const TOKEN_KEY = 'servitor_token';

export const token = $state({ value: localStorage.getItem(TOKEN_KEY) || '' });

export function setToken(t) {
  token.value = t;
  if (t) localStorage.setItem(TOKEN_KEY, t);
  else localStorage.removeItem(TOKEN_KEY);
}

function withToken(path) {
  if (!token.value) return path;
  return path + (path.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(token.value);
}

export async function get(path) {
  const r = await fetch(withToken(path));
  if (r.status === 401) {
    const t = prompt('API token:');
    if (t) {
      setToken(t);
      return get(path);
    }
  }
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  return r.json();
}

// One GraphQL read: POST {query, variables} to /api/graphql, same token
// flow as get(). A GraphQL error becomes an Error carrying the server's
// stable code (extensions.code) so callers can key on it like REST's.
export async function gql(query, variables = {}) {
  const r = await fetch(withToken('/api/graphql'), {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ query, variables })
  });
  if (r.status === 401) {
    const t = prompt('API token:');
    if (t) {
      setToken(t);
      return gql(query, variables);
    }
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
