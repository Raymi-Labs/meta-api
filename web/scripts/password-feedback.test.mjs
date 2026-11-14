import assert from 'node:assert/strict';
import test from 'node:test';
import { passwordFeedback } from '../src/helpers/passwordFeedback.js';

test('password hints follow character bounds and remain advisory', () => {
  assert.equal(passwordFeedback('1234567').validLength, false);
  assert.equal(passwordFeedback('12345678').validLength, true);
  assert.equal(passwordFeedback('x'.repeat(20)).validLength, true);
  assert.equal(passwordFeedback('x'.repeat(21)).validLength, false);
  assert.equal(passwordFeedback('🔐'.repeat(8)).length, 8);
  assert.equal(passwordFeedback('🔐'.repeat(8)).validLength, true);
  assert.equal(
    passwordFeedback('x'.repeat(21), undefined, null).validLength,
    true,
  );
  assert.equal(passwordFeedback('a'.repeat(16)).strength, 0);
  assert.equal(passwordFeedback('Long-pass-2026').strength, 2);
});

test('confirmation recomputes after either field changes and stays quiet when empty', () => {
  assert.equal(passwordFeedback('', '').matches, null);
  assert.equal(passwordFeedback('abcd1234', '').matches, null);
  assert.equal(passwordFeedback('abcd1234', 'abcd1234').matches, true);
  assert.equal(passwordFeedback('newvalue', 'abcd1234').matches, false);
});
