<script>
  // The prompt line at the foot of the screen: a verb switches views or
  // opens a ticket, and the line under it is the machine's one-line reply.
  import { show, openTicket, say, toast } from './state.svelte.js';
  import { parse, HELP } from './prompt.js';

  let line = $state('');

  function run(e) {
    if (e.key !== 'Enter') return;
    const r = parse(line);
    line = '';
    switch (r.action) {
      case 'noop': return;
      case 'view': show(r.name); say(`servitor ${r.name}`); return;
      case 'ticket': openTicket(r.ref); say(`servitor ctx ${r.ref}`); return;
      case 'help': say(r.text); return;
      default: say(r.why ? `${r.verb}: ${r.why}` : `unknown verb "${r.verb}" — the machine spirit does not answer to that. ${HELP}`);
    }
  }
</script>

<footer class="prompt">
  <div class="line">
    <span class="ps">servitor&gt;</span>
    <span class="field">
      <input id="prompt" bind:value={line} onkeydown={run} aria-label="command" placeholder="board · time · arcs · journal · ctx <ref>" autocomplete="off" spellcheck="false">
      <span class="cursor" aria-hidden="true"></span>
    </span>
    <span class="hint">enter to run · 1-4 switch views</span>
  </div>
  <div class="toast" aria-live="polite">{toast.text}</div>
</footer>

<style>
  .prompt { border-top: 1px solid var(--rust-2); padding: 8px 18px 10px; background: var(--iron); font-size: 12px; }
  .line { display: grid; grid-template-columns: auto 1fr auto; gap: 10px; align-items: baseline; }
  .ps { color: var(--phos); }
  .field { display: flex; align-items: center; min-width: 0; }
  input {
    flex: 1; min-width: 0; font: inherit; color: var(--bone); background: none; border: none;
    padding: 0; caret-color: var(--phos); outline: none;
  }
  input::placeholder { color: var(--bone-dim); opacity: 0.7; }
  .cursor { display: inline-block; width: 8px; height: 13px; background: var(--phos); margin-left: 2px; flex: none; }
  .field:focus-within .cursor { display: none; }
  .hint { font-size: 10px; letter-spacing: 0.14em; color: var(--bone-dim); text-transform: uppercase; }
  .toast { font-size: 11px; color: var(--phos-dim); min-height: 1.4em; margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  @media (max-width: 700px) {
    .prompt { padding: 8px 16px 10px; }
    .line { grid-template-columns: auto 1fr; }
    .hint { display: none; }
  }
</style>
