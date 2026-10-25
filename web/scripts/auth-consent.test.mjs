/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import assert from 'node:assert/strict';
import test from 'node:test';
import { hasRequiredAuthConsent } from '../src/helpers/authConsent.js';

const policies = (agreement, privacy) => ({
  user_agreement_enabled: agreement,
  privacy_policy_enabled: privacy,
});

test('disabled policies do not require an unrelated checkbox', () => {
  for (const consent of [undefined, {}, { agreement: false, privacy: false }]) {
    assert.equal(hasRequiredAuthConsent(policies(false, false), consent), true);
  }
});

test('each policy requires its own acceptance across all flag combinations', () => {
  for (const agreementEnabled of [false, true]) {
    for (const privacyEnabled of [false, true]) {
      for (const agreementAccepted of [false, true]) {
        for (const privacyAccepted of [false, true]) {
          const status = policies(agreementEnabled, privacyEnabled);
          const consent = {
            agreement: agreementAccepted,
            privacy: privacyAccepted,
          };
          assert.equal(
            hasRequiredAuthConsent(status, consent),
            (!agreementEnabled || agreementAccepted) &&
              (!privacyEnabled || privacyAccepted),
            JSON.stringify({ status, consent }),
          );
        }
      }
    }
  }
});

test('withdrawing either required acceptance blocks the next action', () => {
  const status = policies(true, true);
  let consent = { agreement: true, privacy: true };
  assert.equal(hasRequiredAuthConsent(status, consent), true);
  consent = { ...consent, agreement: false };
  assert.equal(hasRequiredAuthConsent(status, consent), false);
  consent = { ...consent, agreement: true, privacy: false };
  assert.equal(hasRequiredAuthConsent(status, consent), false);
  consent = { ...consent, privacy: true };
  assert.equal(hasRequiredAuthConsent(status, consent), true);
});

test('enabling another policy does not reuse the first checkbox acceptance', () => {
  const consent = { agreement: true, privacy: false };
  assert.equal(hasRequiredAuthConsent(policies(true, false), consent), true);
  assert.equal(hasRequiredAuthConsent(policies(true, true), consent), false);
  assert.equal(hasRequiredAuthConsent(policies(false, true), consent), false);
  assert.equal(hasRequiredAuthConsent(policies(false, false), consent), true);
});

test('missing status and missing consent are safe for initial rendering', () => {
  assert.equal(hasRequiredAuthConsent(), true);
  assert.equal(hasRequiredAuthConsent(undefined, {}), true);
  assert.equal(hasRequiredAuthConsent({}, undefined), true);
  assert.equal(hasRequiredAuthConsent(policies(true, true)), false);
  assert.equal(hasRequiredAuthConsent(policies(true, false), {}), false);
  assert.equal(hasRequiredAuthConsent(policies(false, true), {}), false);
});

test('required acceptance must be an explicit boolean', () => {
  for (const value of ['true', 'false', 1, {}, null, undefined]) {
    assert.equal(
      hasRequiredAuthConsent(policies(true, false), { agreement: value }),
      false,
    );
    assert.equal(
      hasRequiredAuthConsent(policies(false, true), { privacy: value }),
      false,
    );
  }
});
