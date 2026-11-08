/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { describe, expect, test } from 'bun:test';
import {
  buildUserGroupOptions,
  findGroupNameConflicts,
  orderGroupOptionSaves,
  userGroupNamesFromJSON,
} from './userGroups';

const overview = [
  { name: 'default', registered: true },
  { name: 'vip', registered: true },
  { name: 'OpenAI · 蒸馏', registered: false },
];

describe('user group options', () => {
  test('offers registered groups and keeps the current legacy value', () => {
    expect(buildUserGroupOptions(overview, 'vip', '未登记')).toEqual([
      { value: 'default', label: 'default' },
      { value: 'vip', label: 'vip' },
    ]);
    expect(buildUserGroupOptions(overview, 'OpenAI · 蒸馏', '未登记')).toEqual([
      { value: 'default', label: 'default' },
      { value: 'vip', label: 'vip' },
      { value: 'OpenAI · 蒸馏', label: 'OpenAI · 蒸馏（未登记）' },
    ]);
    expect(buildUserGroupOptions(null, '')).toEqual([]);
  });

  test('reads names from the stored JSON', () => {
    expect(userGroupNamesFromJSON('{"default":"","vip":"VIP"}')).toEqual([
      'default',
      'vip',
    ]);
    expect(userGroupNamesFromJSON('[1]')).toEqual([]);
    expect(userGroupNamesFromJSON('oops')).toEqual([]);
  });
});

describe('group name conflicts', () => {
  test('flags shared names except default', () => {
    expect(
      findGroupNameConflicts(
        ['default', 'vip', '福利', 'svip'],
        ['default', '福利', 'vip', 'OpenAI · 优质'],
      ),
    ).toEqual(['vip', '福利']);
    expect(findGroupNameConflicts(['default'], ['default'])).toEqual([]);
  });
});

describe('group option save order', () => {
  const keys = ['AutoGroups', 'UserGroups', 'GroupRatio', 'UserUsableGroups'];

  test('saves channel groups first when a name moves into user groups', () => {
    expect(
      orderGroupOptionSaves(
        keys,
        { GroupRatio: '{"vip":1,"福利":0.6}' },
        { UserGroups: '{"default":"","vip":""}' },
      ),
    ).toEqual(['GroupRatio', 'UserGroups', 'AutoGroups', 'UserUsableGroups']);
  });

  test('saves user groups first otherwise', () => {
    expect(
      orderGroupOptionSaves(
        keys,
        { GroupRatio: '{"福利":0.6}' },
        { UserGroups: '{"default":""}' },
      ),
    ).toEqual(['UserGroups', 'GroupRatio', 'AutoGroups', 'UserUsableGroups']);
  });

  test('leaves unrelated saves alone', () => {
    expect(orderGroupOptionSaves(['AutoGroups', 'GroupRatio'], {}, {})).toEqual(
      ['AutoGroups', 'GroupRatio'],
    );
  });
});
