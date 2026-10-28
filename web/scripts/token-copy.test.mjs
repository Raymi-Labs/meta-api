import assert from 'node:assert/strict';
import test from 'node:test';
import { collectTokenKeys } from '../src/components/table/tokens/tokenCopy.js';
const records = [
  { id: 1, name: 'A', key: 'masked****1' },
  { id: 2, name: 'B', key: 'masked****2' },
];
test('batch copy resolves full keys and does not duplicate the sk prefix', async () => {
  const ids = [];
  const result = await collectTokenKeys(
    records,
    async (record) => {
      ids.push(record.id);
      return record.id === 1 ? 'fixture-one' : 'sk-fixture-two';
    },
    true,
  );
  assert.deepEqual(ids, [1, 2]);
  assert.equal(result, 'A    sk-fixture-one\nB    sk-fixture-two\n');
});
test('a failed or masked response never yields a partially copied credential set', async () => {
  for (const bad of ['', null, 'masked****2'])
    assert.equal(
      await collectTokenKeys(
        records,
        async (record) => (record.id === 1 ? 'fixture-one' : bad),
        false,
      ),
      null,
    );
});
test('closing while a key is loading prevents subsequent key reads and clipboard output', async () => {
  let current = true,
    calls = 0;
  assert.equal(
    await collectTokenKeys(
      records,
      async () => {
        calls++;
        current = false;
        return 'fixture';
      },
      false,
      () => current,
    ),
    null,
  );
  assert.equal(calls, 1);
  assert.equal(
    await collectTokenKeys(
      [],
      async () => {
        throw Error('unexpected');
      },
      false,
    ),
    null,
  );
});
