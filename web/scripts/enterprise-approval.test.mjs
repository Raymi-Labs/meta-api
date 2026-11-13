import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';

const source = fs.readFileSync(
  new URL('../src/pages/EnterpriseConsole/Approvals.jsx', import.meta.url),
  'utf8',
);
const start = source.indexOf('const actionable =');
const end = source.indexOf('\n\n  const columns', start);
assert.ok(start >= 0 && end > start);
const predicate = source.slice(start, end);
const allowed = (role, row) =>
  vm.runInNewContext(`${predicate}; actionable(row)`, {
    role,
    memberId: 7,
    row,
  });

test('owner sees actions for own pending requests and other members', () => {
  assert.equal(
    allowed('owner', { status: 'pending', member_id: 7, member_role: 'owner' }),
    true,
  );
  assert.equal(
    allowed('owner', { status: 'pending', member_id: 8, member_role: 'admin' }),
    true,
  );
});
test('admin only sees actions for another ordinary member', () => {
  assert.equal(
    allowed('admin', { status: 'pending', member_id: 7, member_role: 'admin' }),
    false,
  );
  assert.equal(
    allowed('admin', { status: 'pending', member_id: 8, member_role: 'owner' }),
    false,
  );
  assert.equal(
    allowed('admin', { status: 'pending', member_id: 8, member_role: 'admin' }),
    false,
  );
  assert.equal(
    allowed('admin', {
      status: 'pending',
      member_id: 8,
      member_role: 'member',
    }),
    true,
  );
});
test('finished requests and ordinary members never expose approval actions', () => {
  for (const status of ['approved', 'rejected', 'cancelled', 'expired']) {
    assert.equal(
      allowed('owner', { status, member_id: 7, member_role: 'owner' }),
      false,
    );
  }
  assert.equal(
    allowed('member', {
      status: 'pending',
      member_id: 8,
      member_role: 'member',
    }),
    false,
  );
});
