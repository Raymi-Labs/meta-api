import assert from 'node:assert/strict';
import test from 'node:test';
import {
  createNoticeReadStateStore,
  getAnnouncementKey,
  normalizeAnnouncements,
} from '../src/helpers/noticeReadState.js';

const memoryStorage = (initial = {}) => {
  const values = new Map(Object.entries(initial));
  return {
    values,
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
};
const fixture = (initial = {}) => {
  const storage = memoryStorage(initial);
  const eventTarget = new EventTarget();
  const store = createNoticeReadStateStore({
    storage: () => storage,
    eventTarget,
    now: () => new Date(2026, 9, 25, 12),
  });
  return { storage, eventTarget, store };
};
const storageEvent = (target, fields) => {
  const event = new Event('storage');
  for (const [key, value] of Object.entries(fields))
    Object.defineProperty(event, key, { value });
  target.dispatchEvent(event);
};

test('announcement identity remains compatible with date and first 30 content characters', () => {
  const item = {
    id: 17,
    publishDate: '2026-10-25T12:00:00+08:00',
    content: '😀'.repeat(20) + ' updated',
  };
  assert.equal(
    getAnnouncementKey(item),
    `${item.publishDate}-${item.content.slice(0, 30)}`,
  );
  assert.equal(getAnnouncementKey(null), '-');
  assert.equal(getAnnouncementKey({ content: 4, publishDate: {} }), '-');
});

test('malformed announcements are filtered without mutating valid source content or ordering', () => {
  for (const value of [undefined, null, 4, {}, '[]'])
    assert.deepEqual(normalizeAnnouncements(value), []);
  const first = {
    content: '  first announcement  ',
    publishDate: 42,
    type: 'unexpected',
    extra: {},
  };
  const second = { content: 'second', type: 'warning', extra: 'details' };
  const result = normalizeAnnouncements([
    null,
    false,
    { content: {} },
    { content: ' \n ' },
    first,
    second,
  ]);
  assert.equal(result.length, 2);
  assert.equal(result[0].content, first.content);
  assert.equal(result[0].publishDate, '');
  assert.equal(result[0].type, 'default');
  assert.equal(result[0].extra, '');
  assert.equal(result[1].content, 'second');
  assert.equal(result[1].type, 'warning');
  assert.equal(first.publishDate, 42);
});

test('legacy read keys tolerate malformed JSON, non-arrays and invalid members', () => {
  for (const raw of ['{bad', 'null', '4', '{}', '"string"']) {
    const { store } = fixture({ notice_read_keys: raw });
    assert.deepEqual(store.getReadAnnouncementKeys(), []);
    assert.equal(store.markAnnouncementKeysRead(['viewed']), true);
    assert.deepEqual(store.getReadAnnouncementKeys(), ['viewed']);
  }
  const { store } = fixture({
    notice_read_keys: '["a","a",null,7,{},"","b"]',
  });
  const keys = store.getReadAnnouncementKeys();
  assert.deepEqual(keys, ['a', 'b']);
  keys.push('not persisted');
  assert.deepEqual(store.getReadAnnouncementKeys(), ['a', 'b']);
});

test('marking only rendered announcement keys leaves hidden or unvisited items unread', () => {
  const { store, storage } = fixture({ notice_read_keys: '["previous"]' });
  const list = Array.from({ length: 25 }, (_, index) => ({
    publishDate: '2026-10-25T00:00:00Z',
    content: `announcement ${index}`,
  }));
  const keys = list.map(getAnnouncementKey);
  assert.equal(store.markAnnouncementKeysRead([]), true);
  assert.deepEqual(store.getReadAnnouncementKeys(), ['previous']);
  assert.equal(store.markAnnouncementKeysRead(keys.slice(0, 20)), true);
  const read = new Set(store.getReadAnnouncementKeys());
  assert.equal(keys.filter((key) => !read.has(key)).length, 5);
  storage.values.set('notice_read_keys', JSON.stringify([...read, keys[21]]));
  store.markAnnouncementKeysRead([keys[20], keys[20], null]);
  assert.ok(store.getReadAnnouncementKeys().includes(keys[21]));
  assert.equal(store.getReadAnnouncementKeys().length, 23);
});

test('full notice content is independent from announcement keys and daily mute', () => {
  const { store } = fixture();
  const original = 'A'.repeat(200) + ' original';
  const changed = 'A'.repeat(200) + ' changed';
  assert.equal(store.isNoticeContentUnread(original), true);
  assert.equal(store.muteNoticeToday(), true);
  assert.equal(store.isNoticeMutedToday(), true);
  assert.equal(store.isNoticeContentUnread(original), true);
  assert.equal(store.markNoticeContentRead(`\n${original}  `), true);
  assert.equal(store.isNoticeContentUnread(original), false);
  assert.equal(store.isNoticeContentUnread(changed), true);
  assert.deepEqual(store.getReadAnnouncementKeys(), []);
  assert.equal(store.markNoticeContentRead(' '), true);
  assert.equal(store.isNoticeContentUnread(original), false);
  for (const value of ['', ' \n ', null, undefined, 4, {}])
    assert.equal(store.isNoticeContentUnread(value), false);
});

test('reading a notice alone never mutes the daily automatic announcement', () => {
  const { store } = fixture();
  store.markNoticeContentRead('notice');
  store.markAnnouncementKeysRead(['announcement']);
  assert.equal(store.isNoticeMutedToday(), false);
});

test('legacy daily mute expires according to the local calendar day', () => {
  const storage = memoryStorage({
    notice_close_date: new Date(2026, 9, 25, 12).toDateString(),
  });
  let date = new Date(2026, 9, 25, 23, 59);
  const store = createNoticeReadStateStore({
    storage: () => storage,
    now: () => date,
  });
  assert.equal(store.isNoticeMutedToday(), true);
  date = new Date(2026, 9, 26, 0, 1);
  assert.equal(store.isNoticeMutedToday(), false);
  store.muteNoticeToday();
  assert.equal(store.isNoticeMutedToday(), true);
});

test('blocked storage retains a session fallback while reporting every failed save', () => {
  const store = createNoticeReadStateStore({
    storage: () => {
      throw new Error('blocked');
    },
  });
  let changes = 0;
  const unsubscribe = store.subscribeNoticeReadState(() => changes++);
  assert.equal(store.markAnnouncementKeysRead(['a']), false);
  assert.equal(store.markAnnouncementKeysRead(['b']), false);
  assert.equal(store.markNoticeContentRead('notice'), false);
  assert.equal(store.muteNoticeToday(), false);
  assert.deepEqual(store.getReadAnnouncementKeys(), ['a', 'b']);
  assert.equal(store.isNoticeContentUnread('notice'), false);
  assert.equal(store.isNoticeMutedToday(), true);
  assert.equal(changes, 4);
  unsubscribe();
});

test('silent write failures do not masquerade as persistence, and a later retry persists session state', () => {
  const storage = memoryStorage({ notice_read_keys: '["old"]' });
  const original = storage.setItem;
  storage.setItem = () => {};
  const store = createNoticeReadStateStore({ storage: () => storage });
  assert.equal(store.markAnnouncementKeysRead(['new']), false);
  assert.deepEqual(store.getReadAnnouncementKeys(), ['old', 'new']);
  storage.setItem = original;
  assert.equal(store.markAnnouncementKeysRead(['later']), true);
  assert.deepEqual(JSON.parse(storage.getItem('notice_read_keys')), [
    'old',
    'new',
    'later',
  ]);
});

test('same-tab subscribers receive one update and independent stores synchronize without event loops', () => {
  const { store, storage, eventTarget } = fixture();
  const second = createNoticeReadStateStore({
    storage: () => storage,
    eventTarget,
  });
  let firstChanges = 0;
  let secondChanges = 0;
  const offFirst = store.subscribeNoticeReadState(() => firstChanges++);
  const offSecond = second.subscribeNoticeReadState(() => secondChanges++);
  store.markAnnouncementKeysRead(['a']);
  assert.deepEqual(second.getReadAnnouncementKeys(), ['a']);
  assert.equal(firstChanges, 1);
  assert.equal(secondChanges, 1);
  second.markNoticeContentRead('notice');
  assert.equal(store.isNoticeContentUnread('notice'), false);
  assert.equal(firstChanges, 2);
  assert.equal(secondChanges, 2);
  offFirst();
  offSecond();
  store.markAnnouncementKeysRead(['b']);
  assert.equal(firstChanges, 2);
  assert.equal(secondChanges, 2);
});

test('same-tab fallback changes also synchronize when localStorage writes are blocked', () => {
  const eventTarget = new EventTarget();
  const storage = memoryStorage({ notice_read_keys: '["old"]' });
  storage.setItem = () => {
    throw new Error('quota');
  };
  const create = () =>
    createNoticeReadStateStore({ storage: () => storage, eventTarget });
  const first = create();
  const second = create();
  const off = second.subscribeNoticeReadState(() => {});
  assert.equal(first.markAnnouncementKeysRead(['new']), false);
  assert.deepEqual(second.getReadAnnouncementKeys(), ['old', 'new']);
  off();
});

test('cross-tab updates read the latest storage value and ignore unrelated storage areas or keys', () => {
  const { store, storage, eventTarget } = fixture();
  let changes = 0;
  const off = store.subscribeNoticeReadState(() => changes++);
  storage.values.set('notice_read_keys', '["latest"]');
  storageEvent(eventTarget, {
    key: 'notice_read_keys',
    newValue: '["stale"]',
    storageArea: storage,
  });
  assert.deepEqual(store.getReadAnnouncementKeys(), ['latest']);
  assert.equal(changes, 1);
  storageEvent(eventTarget, {
    key: 'unrelated',
    storageArea: storage,
  });
  storageEvent(eventTarget, {
    key: 'notice_read_keys',
    storageArea: memoryStorage(),
  });
  assert.equal(changes, 1);
  off();
});

test('cross-tab data clearing resets notice content, mute and an unsaved local read fallback', () => {
  const { store, storage, eventTarget } = fixture();
  const off = store.subscribeNoticeReadState(() => {});
  store.markNoticeContentRead('notice');
  store.muteNoticeToday();
  storage.setItem = () => {
    throw new Error('quota');
  };
  store.markAnnouncementKeysRead(['local']);
  assert.deepEqual(store.getReadAnnouncementKeys(), ['local']);
  storage.values.clear();
  storageEvent(eventTarget, { key: null, storageArea: storage });
  assert.deepEqual(store.getReadAnnouncementKeys(), []);
  assert.equal(store.isNoticeContentUnread('notice'), true);
  assert.equal(store.isNoticeMutedToday(), false);
  off();
});

test('an environment without storage or a window can still use the in-memory state', () => {
  const store = createNoticeReadStateStore({
    storage: () => undefined,
    eventTarget: undefined,
  });
  assert.equal(store.markNoticeContentRead('notice'), false);
  assert.equal(store.isNoticeContentUnread('notice'), false);
  assert.equal(store.markAnnouncementKeysRead(null), true);
  assert.deepEqual(store.getReadAnnouncementKeys(), []);
});
