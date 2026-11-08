// The editor groups equal rules for multi-selection; the stored format remains
// user-group -> channel-group -> value so existing servers and rules stay valid.
export function flattenRatioRules(nested, uid) {
  const rows = [];
  for (const [userGroup, inner] of Object.entries(nested || {})) {
    if (!inner || typeof inner !== 'object' || Array.isArray(inner)) continue;
    const grouped = new Map();
    for (const [usingGroup, ratio] of Object.entries(inner)) {
      const value = typeof ratio === 'number' ? ratio : 1;
      if (!grouped.has(value)) {
        const row = { _id: uid(), userGroup, usingGroups: [], ratio: value };
        grouped.set(value, row);
        rows.push(row);
      }
      grouped.get(value).usingGroups.push(usingGroup);
    }
  }
  return rows;
}

export function serializeGroupGroupRatio(rows) {
  const result = Object.create(null);
  for (const { userGroup, usingGroups = [], ratio } of rows) {
    if (!userGroup) continue;
    for (const group of new Set(usingGroups)) {
      if (!group) continue;
      result[userGroup] ??= Object.create(null);
      if (Object.hasOwn(result[userGroup], group)) return null;
      result[userGroup][group] = ratio;
    }
  }
  return JSON.stringify(result, null, 2);
}

const parseKey = (key) =>
  key.startsWith('+:')
    ? ['add', key.slice(2)]
    : key.startsWith('-:')
      ? ['remove', key.slice(2)]
      : ['append', key];
const rawKey = (op, group) =>
  op === 'add' ? `+:${group}` : op === 'remove' ? `-:${group}` : group;

export function flattenUsableRules(nested, uid) {
  const rows = [];
  for (const [userGroup, inner] of Object.entries(nested || {})) {
    if (!inner || typeof inner !== 'object' || Array.isArray(inner)) continue;
    const grouped = new Map();
    for (const [key, desc] of Object.entries(inner)) {
      const [op, group] = parseKey(key);
      const description = typeof desc === 'string' ? desc : '';
      const signature = JSON.stringify([op, description]);
      if (!grouped.has(signature)) {
        const row = {
          _id: uid(),
          userGroup,
          op,
          targetGroups: [],
          description,
        };
        grouped.set(signature, row);
        rows.push(row);
      }
      grouped.get(signature).targetGroups.push(group);
    }
  }
  return rows;
}

export function serializeGroupSpecialUsable(rows) {
  const result = Object.create(null);
  for (const { userGroup, op, targetGroups = [], description } of rows) {
    if (!userGroup) continue;
    for (const group of new Set(targetGroups)) {
      if (!group) continue;
      result[userGroup] ??= Object.create(null);
      const key = rawKey(op, group);
      if (Object.hasOwn(result[userGroup], key)) return null;
      result[userGroup][key] = description;
    }
  }
  return Object.keys(result).length ? JSON.stringify(result, null, 2) : '';
}
