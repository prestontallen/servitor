import test from 'node:test';
import assert from 'node:assert/strict';
import { createAuth, boot } from './auth.js';

const ok = (body) => ({
  ok: true,
  status: 200,
  json: async () => body
});

test('login success sets actor, clears needed, does not set error', async () => {
  const calls = [];
  const a = createAuth(async (url, init) => {
    calls.push({ url, init });
    return ok({ actor: 'human:preston' });
  });
  a.state.needed = true;
  const actor = await a.login('preston', 'pw');
  assert.equal(actor, 'human:preston');
  assert.equal(a.state.actor, 'human:preston');
  assert.equal(a.state.needed, false);
  assert.equal(a.state.error, '');
  assert.equal(calls[0].url, '/api/login');
  assert.deepEqual(JSON.parse(calls[0].init.body), { user: 'preston', password: 'pw' });
});

test('login 401 shows one generic message — unknown user indistinguishable from wrong password', async () => {
  const a = createAuth(async () => ({ ok: false, status: 401 }));
  for (const [u, p] of [['nobody', 'x'], ['preston', 'wrong']]) {
    await assert.rejects(a.login(u, p));
    assert.equal(a.state.error, 'Invalid user or password');
    assert.equal(a.state.needed, false);
  }
});

test('boot: first_run wins, then a live session, then login', async () => {
  assert.deepEqual(await boot(async () => ({ first_run: true }), async () => { throw 0; }), { mode: 'setup' });
  assert.deepEqual(
    await boot(async () => ({ first_run: false }), async () => ({ ok: true, json: async () => ({ actor: 'human:preston' }) })),
    { mode: 'ok', actor: 'human:preston' }
  );
  assert.deepEqual(await boot(async () => ({ first_run: false }), async () => ({ ok: false })), { mode: 'login' });
  assert.deepEqual(await boot(async () => { throw new Error('down'); }), { mode: 'ok', actor: '' });
});

test('logout flips needed and clears actor', async () => {
  const a = createAuth(async () => ok({}));
  a.state.actor = 'human:preston';
  await a.logout();
  assert.equal(a.state.actor, '');
  assert.equal(a.state.needed, true);
});

test('onDenied (a 401 on a data read) shows the login view', () => {
  const a = createAuth(async () => ok({}));
  a.onDenied();
  assert.equal(a.state.needed, true);
});
