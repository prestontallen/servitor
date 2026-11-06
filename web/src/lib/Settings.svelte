<script>
  // Settings: who you are, and changing your own password. Other sessions
  // are revoked server-side on a successful change; this one survives.
  import { auth, logout, changePassword } from './api.svelte.js';

  let current = $state('');
  let next = $state('');
  let confirm = $state('');
  let busy = $state(false);
  let ok = $state(false);

  let mismatch = $derived(next !== confirm && confirm !== '');

  async function submit(e) {
    e.preventDefault();
    if (!current || !next || next !== confirm || busy) return;
    busy = true;
    ok = false;
    try {
      await changePassword(current, next);
      ok = true;
      current = next = confirm = '';
    } catch {
      // auth.error carries the message
    }
    busy = false;
  }
</script>

<main class="settings">
  <h2>settings</h2>
  <section>
    <div class="row"><span class="k">user</span> {auth.actor}</div>
    <button class="out" onclick={() => logout().then(() => location.reload())}>log out</button>
  </section>
  <section>
    <h3>change password</h3>
    <form onsubmit={submit}>
      <label>
        current password
        <input type="password" bind:value={current} autocomplete="current-password" />
      </label>
      <label>
        new password
        <input type="password" bind:value={next} autocomplete="new-password" />
      </label>
      <label>
        confirm new password
        <input type="password" bind:value={confirm} autocomplete="new-password" />
      </label>
      {#if auth.error}
        <div class="err" role="alert">{auth.error}</div>
      {:else if mismatch}
        <div class="err" role="alert">passwords do not match</div>
      {:else if ok}
        <div class="okd">password changed — your other sessions were signed out</div>
      {/if}
      <button type="submit" disabled={busy || !current || !next || next !== confirm}>
        {busy ? '...' : 'change'}
      </button>
    </form>
  </section>
</main>

<style>
  .settings {
    padding: 1.5rem;
    max-width: 34rem;
    display: grid;
    gap: 1.25rem;
    align-content: start;
  }
  h2 { text-transform: uppercase; letter-spacing: 0.25em; font-size: 1rem; }
  section {
    border: 1px solid #3f4a33;
    padding: 1rem;
    display: grid;
    gap: 0.75rem;
  }
  h3 { font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.2em; }
  .row { font-size: 0.95rem; }
  .k { opacity: 0.6; text-transform: uppercase; font-size: 0.75rem; letter-spacing: 0.15em; }
  form { display: grid; gap: 0.75rem; }
  label { display: grid; gap: 0.25rem; font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.15em; }
  input {
    background: transparent;
    border: 1px solid #3f4a33;
    color: inherit;
    padding: 0.4rem;
    font: inherit;
  }
  .err { color: #e06c5a; font-size: 0.85rem; }
  .okd { color: #98c379; font-size: 0.85rem; }
  button {
    background: #161a12;
    border: 1px solid #3f4a33;
    color: inherit;
    padding: 0.45rem;
    text-transform: uppercase;
    letter-spacing: 0.2em;
    cursor: pointer;
    justify-self: start;
  }
  button:disabled { opacity: 0.5; cursor: default; }
</style>
