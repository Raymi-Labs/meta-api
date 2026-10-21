import assert from 'node:assert/strict';
import { BILLING_VAR_REGEX } from '../src/constants/billing.constants.js';

const coefficients = (expr) =>
  Object.fromEntries(
    Array.from(
      expr.matchAll(new RegExp(BILLING_VAR_REGEX.source, 'g')),
      (match) => [match[1], Number(match[2])],
    ),
  );
assert.deepEqual(coefficients('p*10+c*20+cr*1+cc*12.5'), {
  p: 10,
  c: 20,
  cr: 1,
  cc: 12.5,
});
assert.deepEqual(coefficients('p * 1e-3 + c * 2.5E+2 + cr * .05 + cc1h * 0'), {
  p: 0.001,
  c: 250,
  cr: 0.05,
  cc1h: 0,
});
assert.deepEqual(coefficients('p*-1+c*+2-img*0.5'), { p: -1, c: 2, img: 0.5 });
console.log(
  'Billing coefficient parsing passed: adjacent operators, exponent, decimals, zero and signed coefficients.',
);
