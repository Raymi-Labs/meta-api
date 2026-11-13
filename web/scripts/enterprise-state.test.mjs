import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';

// Exercise the production refresh function against controllable responses, without React or a network.
function harness() {
  const source = fs.readFileSync(
    new URL('../src/hooks/enterprise/useEnterpriseSelf.js', import.meta.url),
    'utf8',
  );
  const start = source.indexOf('export async function refreshEnterpriseSelf()');
  const end = source.indexOf('export function useEnterpriseSelf()', start);
  const code = source
    .slice(start, end)
    .replace('export async function', 'async function');
  let user = 1;
  const requests = [];
  const context = {
    snapshot: {
      loading: false,
      loaded: true,
      userId: 1,
      data: { enterprise: { name: 'Synthetic A' }, member: { available: 100 } },
      error: null,
    },
    requestSeq: 0,
    currentUserId: () => user,
    API: {
      get: () =>
        new Promise((resolve, reject) => requests.push({ resolve, reject })),
    },
  };
  context.emit = (next) => {
    context.snapshot = next;
  };
  vm.createContext(context);
  vm.runInContext(code, context);
  return {
    context,
    requests,
    setUser: (id) => {
      user = id;
    },
    refresh: () => vm.runInContext('refreshEnterpriseSelf()', context),
  };
}
test('new account loading never relabels the previous enterprise snapshot', async () => {
  const h = harness();
  h.setUser(2);
  const p = h.refresh();
  assert.equal(h.context.snapshot.userId, 2);
  assert.equal(h.context.snapshot.data, null);
  h.requests[0].resolve({
    data: { success: true, data: { enterprise: { name: 'B' } } },
  });
  await p;
  assert.equal(h.context.snapshot.data.enterprise.name, 'B');
});
test('same-account background refresh keeps the loaded page and invitation result mounted', async () => {
  const h = harness();
  const p = h.refresh();
  assert.equal(h.context.snapshot.loaded, true);
  assert.equal(h.context.snapshot.data.enterprise.name, 'Synthetic A');
  h.requests[0].resolve({
    data: { success: true, data: { enterprise: { name: 'updated' } } },
  });
  await p;
  assert.equal(h.context.snapshot.data.enterprise.name, 'updated');
});
test('a response arriving after logout or account change cannot repopulate old identity', async () => {
  const h = harness();
  const p = h.refresh();
  h.setUser(2);
  h.requests[0].resolve({
    data: { success: true, data: { enterprise: { name: 'old' } } },
  });
  await p;
  assert.notEqual(h.context.snapshot.data?.enterprise?.name, 'old');
});
test('failed lookup remains an error, not a successful non-member response', async () => {
  const h = harness();
  const p = h.refresh();
  h.requests[0].reject(new Error('synthetic failure'));
  await p;
  assert.ok(h.context.snapshot.error);
  assert.equal(h.context.snapshot.data, null);
});
