import assert from 'node:assert/strict';
import test from 'node:test';
import {
  announcementDraftReducer,
  createAnnouncementID,
  parseAnnouncementDraft,
  publishAnnouncements,
  fetchAnnouncementEmailJobs,
} from '../src/helpers/announcementEmail.js';

test('HTTP installations can create unique UUIDs without randomUUID', () => {
  const source = { getRandomValues: (bytes) => crypto.getRandomValues(bytes) };
  const ids = new Set(
    Array.from({ length: 50 }, () => createAnnouncementID(source)),
  );
  assert.equal(ids.size, 50);
  for (const id of ids)
    assert.match(
      id,
      /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/,
    );
});

test('legacy numeric and new UUID identities survive editing and saving', () => {
  const uuid = crypto.randomUUID();
  const items = parseAnnouncementDraft(
    JSON.stringify([
      { id: 20, content: 'old' },
      { content: 'legacy without id' },
      { id: uuid, content: 'new' },
      { id: 0, content: 'legacy zero id' },
      { id: '', content: 'legacy empty id' },
    ]),
  );
  assert.deepEqual(
    items.map((item) => item.id),
    [20, 2, uuid, 4, 5],
  );
  assert.throws(() => parseAnnouncementDraft('{broken'));
  assert.throws(() => parseAnnouncementDraft('{}'));
});

test('publication sends explicit opt-in and failure leaves the same draft available', async () => {
  let draft = { items: [], dirty: false, revision: 0 };
  draft = announcementDraftReducer(draft, {
    type: 'edit',
    items: [{ id: crypto.randomUUID(), content: 'maintenance' }],
  });
  const calls = [];
  let success = false;
  const api = {
    put: async (path, body) => {
      calls.push({ path, body });
      return {
        data: {
          success,
          message: 'SMTP unavailable',
          data: { email_jobs_queued: 1 },
        },
      };
    },
  };
  await assert.rejects(
    publishAnnouncements(api, draft.items, true, 'failure'),
    /SMTP unavailable/,
  );
  assert.equal(draft.dirty, true);
  assert.equal(calls[0].body.email_sync, true);
  assert.deepEqual(JSON.parse(calls[0].body.value), draft.items);
  success = true;
  assert.equal(
    await publishAnnouncements(api, draft.items, false, 'failure'),
    1,
  );
  assert.equal(calls[1].body.email_sync, false);
  assert.equal(calls[1].path, '/api/option/');
  draft = announcementDraftReducer(draft, {
    type: 'saved',
    revision: draft.revision,
  });
  assert.equal(draft.dirty, false);
});

test('refreshes and earlier save responses cannot erase newer unsaved edits', () => {
  let draft = { items: [{ id: 1 }], dirty: false, revision: 0 };
  draft = announcementDraftReducer(draft, { type: 'edit', items: [{ id: 2 }] });
  const earlierRevision = draft.revision;
  draft = announcementDraftReducer(draft, { type: 'edit', items: [{ id: 3 }] });
  assert.equal(
    announcementDraftReducer(draft, { type: 'receive', items: [] }),
    draft,
  );
  assert.equal(
    announcementDraftReducer(draft, {
      type: 'saved',
      revision: earlierRevision,
    }),
    draft,
  );
  assert.equal(draft.dirty, true);
  assert.deepEqual(draft.items, [{ id: 3 }]);
});

test('history requests the selected page and exposes a failed response for retry', async () => {
  const calls = [];
  const data = { items: [{ id: 99 }], page: 2, page_size: 10, total: 11 };
  const api = {
    get: async (path, options) => {
      calls.push({ path, options });
      return { data: { success: true, data } };
    },
  };
  assert.deepEqual(
    await fetchAnnouncementEmailJobs(api, 2, 10, 'failure'),
    data,
  );
  assert.deepEqual(calls[0], {
    path: '/api/option/announcement-email/jobs',
    options: { params: { p: 2, page_size: 10 } },
  });
  await assert.rejects(
    fetchAnnouncementEmailJobs(
      { get: async () => ({ data: { success: false } }) },
      2,
      10,
      'failure',
    ),
    /failure/,
  );
});
