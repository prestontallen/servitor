<script>
  // The login view replaces the ?token=/localStorage flow: user + password,
  // HttpOnly session cookie. After login the shell reloads so every view
  // boots with the session in place.
  import { auth, login } from './api.svelte.js';

  let user = $state('');
  let password = $state('');
  let busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    if (!user || !password || busy) return;
    busy = true;
    try {
      await login(user, password);
      location.reload();
    } catch {
      busy = false;
    }
  }
</script>

<div class="login-veil">
  <form class="login" onsubmit={submit}>
    <div class="login-title">servitor</div>
    <label>
      user
      <input type="text" bind:value={user} autocomplete="username" autofocus />
    </label>
    <label>
      password
      <input type="password" bind:value={password} autocomplete="current-password" />
    </label>
    {#if auth.error}
      <div class="login-error" role="alert">{auth.error}</div>
    {/if}
    <button type="submit" disabled={busy || !user || !password}>
      {busy ? '...' : 'log in'}
    </button>
  </form>
</div>

<style>
  .login-veil {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    background: var(--crt-bg, #0a0c08);
    z-index: 50;
  }
  .login {
    display: grid;
    gap: 0.9rem;
    width: min(20rem, 90vw);
    padding: 1.5rem;
    border: 1px solid #3f4a33;
    font-family: inherit;
  }
  .login-title {
    text-align: center;
    letter-spacing: 0.35em;
    text-transform: uppercase;
  }
  label {
    display: grid;
    gap: 0.25rem;
    font-size: 0.8rem;
    text-transform: uppercase;
    letter-spacing: 0.15em;
  }
  input {
    background: transparent;
    border: 1px solid #3f4a33;
    color: inherit;
    padding: 0.4rem;
    font: inherit;
  }
  .login-error {
    color: #e06c5a;
    font-size: 0.85rem;
  }
  button {
    background: #161a12;
    border: 1px solid #3f4a33;
    color: inherit;
    padding: 0.45rem;
    text-transform: uppercase;
    letter-spacing: 0.2em;
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
