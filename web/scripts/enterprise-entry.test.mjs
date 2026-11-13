import assert from 'node:assert/strict';
import test from 'node:test';
import { getEnterpriseEntry } from '../src/pages/Enterprise/enterpriseEntry.js';
const user = { id: 901, role: 1 };
const membership = (role = 'member', status = 'active') => ({
  enabled: true,
  isMember: true,
  data: { member: { role }, enterprise: { name: 'Fixture', status } },
});
test('visitors and nonmembers get contact instead of a private workspace', () => {
  assert.equal(getEnterpriseEntry(null, membership()).state, 'contact');
  assert.equal(getEnterpriseEntry(user, { enabled: true }).state, 'contact');
  assert.equal(
    getEnterpriseEntry(user, { ...membership(), enabled: false }).member,
    undefined,
  );
});
test('owners and admins enter the workspace; members enter their quota page', () => {
  for (const role of ['owner', 'admin'])
    assert.equal(
      getEnterpriseEntry(user, membership(role)).to,
      '/enterprise/console',
    );
  assert.equal(
    getEnterpriseEntry(user, membership()).to,
    '/console/enterprise',
  );
  assert.equal(getEnterpriseEntry(user, membership('owner')).role, '所有者');
});
test('loading, error and unknown roles never leak a stale enterprise entry', () => {
  for (const field of ['loading', 'error']) {
    const entry = getEnterpriseEntry(user, {
      ...membership('owner'),
      [field]: true,
    });
    assert.equal(entry.state, field);
    assert.equal(entry.name, undefined);
    assert.equal(entry.to, undefined);
  }
  assert.equal(getEnterpriseEntry(user, membership('invalid')).state, 'error');
});
test('frozen enterprises stay manageable and platform role does not create membership', () => {
  assert.equal(
    getEnterpriseEntry(user, membership('owner', 'frozen')).frozen,
    true,
  );
  assert.equal(
    getEnterpriseEntry({ id: 1, role: 100 }, { enabled: false }).platform,
    true,
  );
  assert.equal(
    getEnterpriseEntry({ id: 1, role: 10 }, { enabled: false }).platform,
    false,
  );
  const platform = getEnterpriseEntry({ id: 1, role: 10 }, { enabled: true });
  assert.equal(platform.platform, true);
  assert.equal(platform.member, undefined);
  assert.equal(platform.to, '/console/enterprise-admin');
});
