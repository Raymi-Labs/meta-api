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

// User groups (who a user is: default, vip, ...) are kept apart from channel
// groups (GroupRatio keys: which channels a token uses and its price).
export const DEFAULT_USER_GROUP = 'default';

function parseObject(str) {
  if (!str || !String(str).trim()) return {};
  try {
    const parsed = JSON.parse(str);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed
      : {};
  } catch {
    return {};
  }
}

export function userGroupNamesFromJSON(userGroupsJSON) {
  return Object.keys(parseObject(userGroupsJSON));
}

// Options for selects that assign a user group. Only registered groups are
// offered; the record's current value stays selectable so editing a legacy
// user or plan does not silently change it.
export function buildUserGroupOptions(items, current, unregisteredLabel) {
  const options = (items || [])
    .filter((item) => item.registered)
    .map((item) => ({ value: item.name, label: item.name }));
  if (current && !options.some((option) => option.value === current)) {
    options.push({
      value: current,
      label: unregisteredLabel ? `${current}（${unregisteredLabel}）` : current,
    });
  }
  return options;
}

// Names that are both a user group and a channel group. default is allowed
// in both lists: new users get it and tokens without a group use it.
export function findGroupNameConflicts(userGroupNames, channelGroupNames) {
  const channelGroups = new Set(channelGroupNames || []);
  return [...new Set(userGroupNames || [])]
    .filter((name) => name !== DEFAULT_USER_GROUP && channelGroups.has(name))
    .sort();
}

// The server rejects a save that would make a name appear in both lists, so
// when a name moves from one list to the other, the list losing it is saved
// first. Other keys keep their order.
export function orderGroupOptionSaves(keys, previous, next) {
  const hasGroupRatio = keys.includes('GroupRatio');
  const hasUserGroups = keys.includes('UserGroups');
  if (!hasGroupRatio || !hasUserGroups) return keys;
  const previousChannelGroups = new Set(
    Object.keys(parseObject(previous.GroupRatio)),
  );
  const movesIntoUserGroups = userGroupNamesFromJSON(next.UserGroups).some(
    (name) => name !== DEFAULT_USER_GROUP && previousChannelGroups.has(name),
  );
  const first = movesIntoUserGroups ? 'GroupRatio' : 'UserGroups';
  const second = first === 'GroupRatio' ? 'UserGroups' : 'GroupRatio';
  return [
    first,
    second,
    ...keys.filter((key) => key !== 'GroupRatio' && key !== 'UserGroups'),
  ];
}
