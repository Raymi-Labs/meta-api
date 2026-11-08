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
import React, { useState, useCallback, useMemo } from 'react';
import {
  Button,
  Collapsible,
  Input,
  Select,
  Tag,
  Typography,
  Popconfirm,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconDelete,
  IconChevronDown,
  IconChevronUp,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

import {
  flattenUsableRules,
  serializeGroupSpecialUsable,
} from '../../../../helpers/userGroupRules';
export { serializeGroupSpecialUsable } from '../../../../helpers/userGroupRules';

const { Text } = Typography;

let _idCounter = 0;
const uid = () => `gsu_${++_idCounter}`;

const OP_ADD = 'add';
const OP_REMOVE = 'remove';
const OP_APPEND = 'append';

function parseJSON(str) {
  if (!str || !str.trim()) return {};
  try {
    return JSON.parse(str);
  } catch {
    return {};
  }
}

const OP_TAG_MAP = {
  [OP_ADD]: { color: 'green', label: '添加 (+:)' },
  [OP_REMOVE]: { color: 'red', label: '移除 (-:)' },
  [OP_APPEND]: { color: 'blue', label: '追加' },
};

function UsableGroupSection({
  groupName,
  items,
  groupOptions,
  opOptions,
  onUpdate,
  onRemove,
  onAdd,
  t,
}) {
  const [open, setOpen] = useState(false);

  return (
    <div
      style={{
        border: '1px solid var(--semi-color-border)',
        borderRadius: 8,
        overflow: 'hidden',
      }}
    >
      <div
        className='flex items-center justify-between cursor-pointer'
        style={{
          padding: '8px 12px',
          background: 'var(--semi-color-fill-0)',
        }}
        onClick={() => setOpen(!open)}
      >
        <div className='flex items-center gap-2'>
          {open ? (
            <IconChevronUp size='small' />
          ) : (
            <IconChevronDown size='small' />
          )}
          <Text strong>{groupName}</Text>
          <Tag size='small' color='blue'>
            {items.length} {t('条规则')}
          </Tag>
        </div>
        <div
          className='flex items-center gap-1'
          onClick={(e) => e.stopPropagation()}
        >
          <Button
            icon={<IconPlus />}
            size='small'
            theme='borderless'
            onClick={() => onAdd(groupName)}
          />
          <Popconfirm
            title={t('确认删除该分组的所有规则？')}
            onConfirm={() => onRemove(items.map((item) => item._id))}
            position='left'
          >
            <Button
              icon={<IconDelete />}
              size='small'
              type='danger'
              theme='borderless'
            />
          </Popconfirm>
        </div>
      </div>
      <Collapsible isOpen={open} keepDOM>
        <div style={{ padding: '8px 12px' }}>
          {items.map((rule) => (
            <div
              key={rule._id}
              className='flex items-center gap-2'
              style={{ marginBottom: 6 }}
            >
              <Select
                size='small'
                value={rule.op}
                optionList={opOptions}
                onChange={(v) => onUpdate(rule._id, 'op', v)}
                style={{ width: 120 }}
                renderSelectedItem={(optionNode) => {
                  const info = OP_TAG_MAP[optionNode.value] || {};
                  return (
                    <Tag size='small' color={info.color}>
                      {optionNode.label}
                    </Tag>
                  );
                }}
              />
              <Select
                multiple
                filter
                allowCreate
                optionList={groupOptions}
                size='small'
                value={rule.targetGroups}
                placeholder={t('分组名称')}
                onChange={(v) => onUpdate(rule._id, 'targetGroups', v)}
                style={{ flex: 1 }}
              />
              {rule.op !== OP_REMOVE ? (
                <Input
                  size='small'
                  value={rule.description}
                  placeholder={t('分组描述')}
                  onChange={(v) => onUpdate(rule._id, 'description', v)}
                  style={{ flex: 1 }}
                />
              ) : (
                <div style={{ flex: 1 }}>
                  <Text type='tertiary' size='small'>
                    -
                  </Text>
                </div>
              )}
              <Popconfirm
                title={t('确认删除该规则？')}
                onConfirm={() => onRemove(rule._id)}
                position='left'
              >
                <Button
                  icon={<IconDelete />}
                  type='danger'
                  theme='borderless'
                  size='small'
                />
              </Popconfirm>
            </div>
          ))}
        </div>
      </Collapsible>
    </div>
  );
}

export default function GroupSpecialUsableRules({
  value,
  groupNames = [],
  // Names offered for the user-group side of a rule; channel groups are
  // offered for the target side.
  userGroupNames,
  onChange,
}) {
  const { t } = useTranslation();
  const [rules, setRules] = useState(() =>
    flattenUsableRules(parseJSON(value), uid),
  );
  const [newGroupName, setNewGroupName] = useState([]);

  const emitChange = useCallback(
    (newRules) => {
      setRules(newRules);
      const serialized = serializeGroupSpecialUsable(newRules);
      onChange?.(serialized, serialized !== null);
    },
    [onChange],
  );

  const updateRule = useCallback(
    (id, field, val) => {
      emitChange(
        rules.map((r) => {
          if (r._id !== id) return r;
          const updated = { ...r, [field]: val };
          if (field === 'op' && val === OP_REMOVE)
            updated.description = 'remove';
          else if (field === 'op' && r.op === OP_REMOVE && val !== OP_REMOVE) {
            if (updated.description === 'remove') updated.description = '';
          }
          return updated;
        }),
      );
    },
    [rules, emitChange],
  );

  const removeRule = useCallback(
    (id) =>
      emitChange(
        rules.filter((r) => !(Array.isArray(id) ? id : [id]).includes(r._id)),
      ),
    [rules, emitChange],
  );

  const addRuleToGroup = useCallback(
    (groupName) => {
      emitChange([
        ...rules,
        {
          _id: uid(),
          userGroup: groupName,
          op: OP_APPEND,
          targetGroups: [],
          description: '',
        },
      ]);
    },
    [rules, emitChange],
  );

  const addNewGroup = useCallback(() => {
    if (!newGroupName.length) return;
    emitChange([
      ...rules,
      ...newGroupName.map((name) => ({
        _id: uid(),
        userGroup: name,
        op: OP_APPEND,
        targetGroups: [],
        description: '',
      })),
    ]);
    setNewGroupName([]);
  }, [rules, emitChange, newGroupName]);

  const groupOptions = useMemo(
    () => groupNames.map((n) => ({ value: n, label: n })),
    [groupNames],
  );
  const userGroupOptions = useMemo(
    () => (userGroupNames || groupNames).map((n) => ({ value: n, label: n })),
    [userGroupNames, groupNames],
  );

  const opOptions = useMemo(
    () => [
      { value: OP_ADD, label: t('添加 (+:)') },
      { value: OP_REMOVE, label: t('移除 (-:)') },
      { value: OP_APPEND, label: t('追加') },
    ],
    [t],
  );

  const grouped = useMemo(() => {
    const map = Object.create(null);
    const order = [];
    rules.forEach((r) => {
      if (!r.userGroup) return;
      if (!map[r.userGroup]) {
        map[r.userGroup] = [];
        order.push(r.userGroup);
      }
      map[r.userGroup].push(r);
    });
    return order.map((name) => ({ name, items: map[name] }));
  }, [rules]);

  if (grouped.length === 0 && rules.length === 0) {
    return (
      <div>
        <Text type='tertiary' className='block text-center py-4'>
          {t('暂无规则，点击下方按钮添加')}
        </Text>
        <div className='mt-2 flex justify-center gap-2'>
          <Select
            size='small'
            filter
            allowCreate
            placeholder={t('选择用户分组')}
            optionList={userGroupOptions}
            multiple
            value={newGroupName}
            onChange={setNewGroupName}
            style={{ width: 200 }}
            position='bottomLeft'
          />
          <Button icon={<IconPlus />} theme='outline' onClick={addNewGroup}>
            {t('添加分组规则')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className='space-y-2'>
      {serializeGroupSpecialUsable(rules) === null && (
        <Text type='danger'>{t('存在重复的分组名称：')}</Text>
      )}
      {grouped.map((group) => (
        <UsableGroupSection
          key={group.name}
          groupName={group.name}
          items={group.items}
          groupOptions={groupOptions}
          opOptions={opOptions}
          onUpdate={updateRule}
          onRemove={removeRule}
          onAdd={addRuleToGroup}
          t={t}
        />
      ))}
      <div className='mt-3 flex justify-center gap-2'>
        <Select
          size='small'
          filter
          allowCreate
          placeholder={t('选择用户分组')}
          optionList={userGroupOptions}
          multiple
          value={newGroupName}
          onChange={setNewGroupName}
          style={{ width: 200 }}
          position='bottomLeft'
        />
        <Button icon={<IconPlus />} theme='outline' onClick={addNewGroup}>
          {t('添加分组规则')}
        </Button>
      </div>
    </div>
  );
}
