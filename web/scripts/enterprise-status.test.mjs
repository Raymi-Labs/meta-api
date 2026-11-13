import assert from 'node:assert/strict';
import test from 'node:test';
import {
  invalidateSiteStatusReads,
  readCurrentSiteStatus,
} from '../src/helpers/siteStatusRequest.js';

test('late layout response cannot undo a completed enterprise toggle', async () => {
  let finish;
  const oldRead = readCurrentSiteStatus(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  invalidateSiteStatusReads();
  finish({ enterprise_enabled: false });
  assert.equal(await oldRead, null);
  assert.deepEqual(
    await readCurrentSiteStatus(async () => ({ enterprise_enabled: true })),
    { enterprise_enabled: true },
  );
});

test('ordinary status reads and failures keep their existing behavior', async () => {
  const expected = { enterprise_enabled: false, version: 'fixture' };
  assert.equal(await readCurrentSiteStatus(async () => expected), expected);
  await assert.rejects(
    readCurrentSiteStatus(async () => {
      throw new Error('offline');
    }),
    /offline/,
  );
});
