import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parse, viewForKey, VIEWS, HELP } from './prompt.js';

test('a bare view verb switches views, case and whitespace ignored', () => {
  assert.deepEqual(parse('board'), { action: 'view', name: 'board' });
  assert.deepEqual(parse('  TIME '), { action: 'view', name: 'time' });
  assert.deepEqual(parse('ledger'), { action: 'view', name: 'journal' });
  assert.deepEqual(parse('a'), { action: 'view', name: 'arcs' });
});

test('ctx <ref> opens a ticket; ctx alone is an error with a reason', () => {
  assert.deepEqual(parse('ctx flow-diagram'), { action: 'ticket', ref: 'flow-diagram' });
  assert.deepEqual(parse('open 01M37ZRQ'), { action: 'ticket', ref: '01M37ZRQ' });
  assert.deepEqual(parse('ctx'), { action: 'unknown', verb: 'ctx', why: 'needs a ticket ref' });
});

test('empty input is a no-op; help is help; anything else is unknown and does not navigate', () => {
  assert.deepEqual(parse(''), { action: 'noop' });
  assert.deepEqual(parse(null), { action: 'noop' });
  assert.deepEqual(parse('help'), { action: 'help', text: HELP });
  assert.deepEqual(parse('rm -rf /'), { action: 'unknown', verb: 'rm' });
});

test('digit keys map onto the views in order and nothing else', () => {
  VIEWS.forEach((v, i) => assert.equal(viewForKey(String(i + 1)), v));
  assert.equal(viewForKey('0'), null);
  assert.equal(viewForKey(String(VIEWS.length + 1)), null);
  assert.equal(viewForKey('a'), null);
});
