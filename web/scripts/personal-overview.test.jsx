import { test, mock } from 'bun:test';
mock.module('../src/helpers/apiCore', () => ({
  API: { get: async () => ({ data: { success: false } }) },
}));
import assert from 'node:assert/strict';
import { maskedEmail } from '../src/helpers/personalOverview';
const { DEFAULT_ADMIN_CONFIG, mergeAdminConfig } = await import(
  '../src/hooks/common/useSidebar'
);

test('overview is a default personal module and respects explicit hidden settings', () => {
  assert.equal(DEFAULT_ADMIN_CONFIG.personal.overview, true);
  assert.equal(
    mergeAdminConfig({ personal: { enabled: true, topup: true } }).personal
      .overview,
    true,
  );
  assert.equal(
    mergeAdminConfig({ personal: { enabled: false } }).personal.enabled,
    false,
  );
  assert.equal(
    mergeAdminConfig({ personal: { overview: false } }).personal.overview,
    false,
  );
});
test('account overview never displays a complete email', () => {
  assert.equal(maskedEmail('demo-person@example.test'), 'd***@example.test');
  assert.equal(maskedEmail(''), '');
  assert.equal(maskedEmail(undefined), '');
  assert.equal(maskedEmail('not-an-email'), '');
});

test('retired Pulse menus cannot be restored by saved preferences', () => {
  for (const enabled of [true, false]) {
    const saved = {
      personal: { pulse: enabled, topup: false },
      admin: { pulseOps: enabled, channel: false },
    };
    const merged = mergeAdminConfig(saved);
    assert.equal(Object.hasOwn(merged.personal, 'pulse'), false);
    assert.equal(Object.hasOwn(merged.admin, 'pulseOps'), false);
    assert.equal(merged.personal.topup, false);
    assert.equal(merged.admin.channel, false);
    assert.equal(saved.personal.pulse, enabled);
    assert.equal(saved.admin.pulseOps, enabled);
  }
});
