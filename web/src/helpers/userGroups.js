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

// Validate the rows before object serialization can discard empty or duplicate names.
export function validateUserGroupRows(rows) {
  const seen = new Set();
  for (const row of rows) {
    const name = (row.name || '').trim();
    if (!name || [...name].length > 64 || seen.has(name)) return false;
    seen.add(name);
  }
  return seen.has(DEFAULT_USER_GROUP);
}

export function serializeUserGroupRows(rows) {
  if (!validateUserGroupRows(rows)) return null;
  return JSON.stringify(
    Object.fromEntries(
      rows.map((row) => [row.name.trim(), row.description || '']),
    ),
    null,
    2,
  );
}

// A final pair is validated and committed together, even when names cross both ways.
export function buildGroupOptionUpdates(keys, next) {
  if (keys.includes('UserGroups') && keys.includes('GroupRatio')) {
    return [
      {
        key: 'UserChannelGroups',
        value: JSON.stringify({
          UserGroups: next.UserGroups,
          GroupRatio: next.GroupRatio,
        }),
      },
      ...keys
        .filter((key) => key !== 'UserGroups' && key !== 'GroupRatio')
        .map((key) => ({ key, value: next[key] })),
    ];
  }
  return keys.map((key) => ({ key, value: next[key] }));
}
