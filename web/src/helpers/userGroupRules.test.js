import { test, expect } from 'bun:test';
import {
  flattenRatioRules,
  flattenUsableRules,
  serializeGroupGroupRatio,
  serializeGroupSpecialUsable,
} from './userGroupRules';
let id = 0;
const uid = () => String(++id);
test('ratio multi-select expands each target, preserves zero, and reloads compactly', () => {
  const rows = [
    { userGroup: 'vip', usingGroups: ['a', 'b'], ratio: 0 },
    { userGroup: 'default', usingGroups: ['a'], ratio: 0.5 },
  ];
  const stored = serializeGroupGroupRatio(rows);
  expect(JSON.parse(stored)).toEqual({
    vip: { a: 0, b: 0 },
    default: { a: 0.5 },
  });
  const loaded = flattenRatioRules(JSON.parse(stored), uid);
  expect(loaded[0].usingGroups).toEqual(['a', 'b']);
  expect(serializeGroupGroupRatio(loaded)).toBe(stored);
  loaded[0].usingGroups = ['b'];
  expect(JSON.parse(serializeGroupGroupRatio(loaded)).vip).toEqual({ b: 0 });
});
test('usable multi-select round trips all operations, descriptions and legacy names', () => {
  const old = {
    vip: {
      '+:a': 'desc',
      '+:b': 'desc',
      '-:old': 'old removal value',
      legacy: 'plain',
    },
    default: { a: 'free' },
  };
  const loaded = flattenUsableRules(old, uid);
  expect(loaded[0].targetGroups).toEqual(['a', 'b']);
  expect(JSON.parse(serializeGroupSpecialUsable(loaded))).toEqual(old);
  loaded[0].op = 'remove';
  loaded[0].description = 'remove';
  const changed = JSON.parse(serializeGroupSpecialUsable(loaded));
  expect(changed.vip['-:a']).toBe('remove');
  expect(changed.vip['-:b']).toBe('remove');
  expect(changed.vip['+:a']).toBeUndefined();
});
test('conflicting rows cannot silently overwrite a ratio or scope', () => {
  expect(
    serializeGroupGroupRatio([
      { userGroup: 'vip', usingGroups: ['a', 'b'], ratio: 1 },
      { userGroup: 'vip', usingGroups: ['b'], ratio: 0 },
    ]),
  ).toBeNull();
  expect(
    serializeGroupSpecialUsable([
      {
        userGroup: 'vip',
        targetGroups: ['a', 'b'],
        op: 'add',
        description: 'one',
      },
      { userGroup: 'vip', targetGroups: ['b'], op: 'add', description: 'two' },
    ]),
  ).toBeNull();
});
test('empty selections clear rules and distinct user identities stay independent', () => {
  expect(
    serializeGroupGroupRatio([{ userGroup: 'vip', usingGroups: [], ratio: 1 }]),
  ).toBe('{}');
  expect(serializeGroupSpecialUsable([])).toBe('');
  expect(
    JSON.parse(
      serializeGroupGroupRatio(
        ['vip', 'svip'].map((userGroup) => ({
          userGroup,
          usingGroups: ['a', 'b'],
          ratio: 0.8,
        })),
      ),
    ),
  ).toEqual({ vip: { a: 0.8, b: 0.8 }, svip: { a: 0.8, b: 0.8 } });
});
