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
