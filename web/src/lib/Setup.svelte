<script>
  // First-run setup: shown when the daemon has no users at all. Creates
  // the first user (actor human:<username> server-side) and logs in.
  import { auth, setup } from './api.svelte.js';

  let user = $state('');
  let password = $state('');
  let confirm = $state('');
  let busy = $state(false);

  let mismatch = $derived(password !== confirm && confirm !== '');

  async function submit(e) {
    e.preventDefault();
    if (!user || !password || password !== confirm || busy) return;
    busy = true;
    try {
      await setup(user, password);
      location.reload();
    } catch {
      busy = false;
    }
  }
</script>

<div class="login-veil">
  <form class="login" onsubmit={submit}>
    <div class="login-title">servitor · first run</div>
    <div class="login-sub">No users exist yet. Create the first one.</div>
    <label>
      username
      <input type="text" bind:value={user} autocomplete="username" autofocus />
    </label>
    <label>
      password
      <input type="password" bind:value={password} autocomplete="new-password" />
    </label>
    <label>
      confirm password
      <input type="password" bind:value={confirm} autocomplete="new-password" />
    </label>
    {#if mismatch}
      <div class="login-error" role="alert">passwords do not match</div>
    {:else if auth.error}
      <div class="login-error" role="alert">{auth.error}</div>
    {/if}
    <button type="submit" disabled={busy || !user || !password || password !== confirm}>
      {busy ? '...' : 'create user'}
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
  .login-sub {
    text-align: center;
    font-size: 0.8rem;
    opacity: 0.7;
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
