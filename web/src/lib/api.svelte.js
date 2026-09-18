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

export function streamURL() {
  return withToken('/api/events/stream');
}
