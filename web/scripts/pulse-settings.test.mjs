import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildPulseSettingsUpdate,
  pulseQuotaToUnits,
} from '../src/helpers/pulseSettings.js';

test('an unchanged form does not persist environment defaults or blank secrets', () => {
  const saved = {
    PulseForumSSOCallbackURL:
      'https://forum.example/api/user-center/login/callback',
    PulseEnv: 'production',
    PulseBenefitEnabled: 'false',
  };
  assert.deepEqual(
    buildPulseSettingsUpdate(
      { ...saved },
      saved,
      { PulseServiceHMACSecret: '', PulseRollbackHMACSecret: '  ' },
      [],
    ),
    { config: {}, clear_secrets: [] },
  );
});

test('saving a limit only changes that limit and deliberately entered secrets', () => {
  const saved = {
    PulseBenefitMaxGrantQuota: '250000',
    PulseBenefitUserDailyQuota: '1000000',
  };
  assert.deepEqual(
    buildPulseSettingsUpdate(
      { ...saved, PulseBenefitMaxGrantQuota: ' 50000 ' },
      saved,
      {
        PulseServiceHMACSecret: '  new-grant-secret  ',
        PulseRollbackHMACSecret: '',
      },
      [],
    ),
    {
      config: {
        PulseBenefitMaxGrantQuota: '50000',
        PulseServiceHMACSecret: 'new-grant-secret',
      },
      clear_secrets: [],
    },
  );
});

test('explicit clearing excludes any unsubmitted replacement for the same key', () => {
  assert.deepEqual(
    buildPulseSettingsUpdate(
      {},
      {},
      { PulseServiceHMACSecretPrevious: 'previous-secret' },
      ['PulseServiceHMACSecretPrevious'],
    ),
    {
      config: {},
      clear_secrets: ['PulseServiceHMACSecretPrevious'],
    },
  );
});

test('an explicit disabled switch is preserved as false in the patch', () => {
  assert.deepEqual(
    buildPulseSettingsUpdate(
      { PulseBenefitEnabled: 'false' },
      { PulseBenefitEnabled: 'true' },
      {},
      [],
    ),
    { config: { PulseBenefitEnabled: 'false' }, clear_secrets: [] },
  );
});

test('quota conversion follows the actual server quota-per-unit', () => {
  assert.equal(pulseQuotaToUnits('250000', 500000), 0.5);
  assert.equal(pulseQuotaToUnits('1000000', 500000), 2);
  assert.equal(pulseQuotaToUnits('5000000', 1000000), 5);
  for (const value of [
    '',
    '0',
    '-1',
    '1.5',
    '1e6',
    'Infinity',
    '9007199254740992',
  ]) {
    assert.equal(pulseQuotaToUnits(value, 500000), null);
  }
  for (const rate of [0, -1, undefined, NaN, Infinity]) {
    assert.equal(pulseQuotaToUnits('250000', rate), null);
  }
});
