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

import React, { useCallback, useMemo, useRef, useState } from 'react';
import {
  Button,
  Input,
  Popconfirm,
  Select,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import CardTable from '../../../../components/common/ui/CardTable';
import {
  DEFAULT_USER_GROUP,
  findGroupNameConflicts,
} from '../../../../helpers/userGroups';

const { Text } = Typography;

let _idCounter = 0;
const uid = () => `ug_${++_idCounter}`;

function buildRows(userGroupsJSON) {
  let map = {};
  try {
    const parsed = JSON.parse(userGroupsJSON || '{}');
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      map = parsed;
    }
  } catch {
    map = {};
  }
  if (!(DEFAULT_USER_GROUP in map)) {
    map = { [DEFAULT_USER_GROUP]: '', ...map };
  }
  return Object.entries(map).map(([name, description]) => ({
    _id: uid(),
    name,
    description: description ?? '',
  }));
}

function serializeRows(rows) {
  const result = {};
  rows.forEach((row) => {
    const name = (row.name || '').trim();
    if (!name) return;
    result[name] = row.description || '';
  });
  return JSON.stringify(result, null, 2);
}

export default function UserGroupTable({
  value,
  overview = [],
  channelGroupNames = [],
  onChange,
  onReassign,
}) {
  const { t } = useTranslation();
  const [rows, setRows] = useState(() => buildRows(value));
  const [reassignTargets, setReassignTargets] = useState({});
  const [reassigning, setReassigning] = useState('');

  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

  const emitAndSet = useCallback((updater) => {
    setRows((prev) => {
      const next = typeof updater === 'function' ? updater(prev) : updater;
      onChangeRef.current?.(serializeRows(next));
      return next;
    });
  }, []);

  const updateRow = useCallback(
    (id, field, fieldValue) => {
      emitAndSet((prev) =>
        prev.map((row) =>
          row._id === id ? { ...row, [field]: fieldValue } : row,
        ),
      );
    },
    [emitAndSet],
  );

  const removeRow = useCallback(
    (id) => emitAndSet((prev) => prev.filter((row) => row._id !== id)),
    [emitAndSet],
  );

  const addRow = useCallback(
    (name = '') => {
      emitAndSet((prev) => {
        if (name && prev.some((row) => row.name === name)) return prev;
        let nextName = name;
        if (!nextName) {
          const existing = new Set(prev.map((row) => row.name));
          let counter = 1;
          nextName = `user_group_${counter}`;
          while (existing.has(nextName)) {
            counter++;
            nextName = `user_group_${counter}`;
          }
        }
        return [...prev, { _id: uid(), name: nextName, description: '' }];
      });
    },
    [emitAndSet],
  );

  const usage = useMemo(() => {
    const map = {};
    (overview || []).forEach((item) => {
      map[item.name] = item;
    });
    return map;
  }, [overview]);

  const names = useMemo(() => rows.map((row) => row.name), [rows]);
  const duplicateNames = useMemo(() => {
    const counts = {};
    names.forEach((name) => {
      counts[name] = (counts[name] || 0) + 1;
    });
    return Object.keys(counts).filter((name) => counts[name] > 1);
  }, [names]);
  const conflicts = useMemo(
    () => findGroupNameConflicts(names, channelGroupNames),
    [names, channelGroupNames],
  );
  const problemNamesRef = useRef(new Set());
  problemNamesRef.current = new Set([...duplicateNames, ...conflicts]);

  // Targets for moving users must already be saved on the server.
  const savedUserGroupOptions = useMemo(
    () =>
      (overview || [])
        .filter((item) => item.registered)
        .map((item) => ({ value: item.name, label: item.name })),
    [overview],
  );
  const unregistered = useMemo(
    () =>
      (overview || []).filter(
        (item) => !item.registered && !names.includes(item.name),
      ),
    [overview, names],
  );

  const columns = useMemo(
    () => [
      {
        title: t('用户分组名称'),
        dataIndex: 'name',
        key: 'name',
        width: 200,
        render: (_, record) => (
          <Input
            size='small'
            value={record.name}
            disabled={record.name === DEFAULT_USER_GROUP}
            status={
              problemNamesRef.current.has(record.name) ? 'warning' : undefined
            }
            onChange={(v) => updateRow(record._id, 'name', v)}
          />
        ),
      },
      {
        title: t('描述'),
        dataIndex: 'description',
        key: 'description',
        render: (_, record) => (
          <Input
            size='small'
            value={record.description}
            placeholder={t('用户分组描述')}
            onChange={(v) => updateRow(record._id, 'description', v)}
          />
        ),
      },
      {
        title: t('用户数'),
        key: 'users',
        width: 90,
        align: 'right',
        render: (_, record) => usage[record.name]?.user_count ?? 0,
      },
      {
        title: t('订阅套餐'),
        key: 'plans',
        width: 90,
        align: 'right',
        render: (_, record) => usage[record.name]?.plan_count ?? 0,
      },
      {
        title: '',
        key: 'actions',
        width: 50,
        render: (_, record) => {
          if (record.name === DEFAULT_USER_GROUP) return null;
          const userCount = usage[record.name]?.user_count ?? 0;
          return (
            <Popconfirm
              title={
                userCount > 0
                  ? t(
                      '该用户分组还有 {{count}} 个用户，删除后这些用户会显示为未登记。确认删除？',
                      { count: userCount },
                    )
                  : t('确认删除该用户分组？')
              }
              onConfirm={() => removeRow(record._id)}
              position='left'
            >
              <Button
                icon={<IconDelete />}
                type='danger'
                theme='borderless'
                size='small'
              />
            </Popconfirm>
          );
        },
      },
    ],
    // usage only changes when the overview reloads, so rebuilding columns
    // then does not disturb typing.
    [t, updateRow, removeRow, usage],
  );

  const handleReassign = async (item) => {
    const target = reassignTargets[item.name] || DEFAULT_USER_GROUP;
    setReassigning(item.name);
    try {
      await onReassign?.(item.name, target);
    } finally {
      setReassigning('');
    }
  };

  return (
    <div>
      <CardTable
        columns={columns}
        dataSource={rows}
        rowKey='_id'
        hidePagination
        size='small'
      />
      <div className='mt-3 flex justify-center'>
        <Button icon={<IconPlus />} theme='outline' onClick={() => addRow()}>
          {t('添加用户分组')}
        </Button>
      </div>
      {duplicateNames.length > 0 && (
        <Text type='warning' size='small' className='mt-2 block'>
          {t('存在重复的分组名称：')}
          {duplicateNames.join(', ')}
        </Text>
      )}
      {conflicts.length > 0 && (
        <Text type='warning' size='small' className='mt-2 block'>
          {t('与渠道分组重名，请换一个名称：')}
          {conflicts.join(', ')}
        </Text>
      )}

      {unregistered.length > 0 && (
        <div
          className='mt-4'
          style={{
            border: '1px solid var(--semi-color-warning-light-active)',
            background: 'var(--semi-color-warning-light-default)',
            borderRadius: 8,
            padding: '10px 12px',
          }}
        >
          <Text strong>{t('仍在使用但未登记的用户分组')}</Text>
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', margin: '4px 0 8px' }}
          >
            {t(
              '这些名称仍被用户、订阅套餐或特殊规则使用，但不在用户分组名单里。可以登记为用户分组，或把这些用户改到已保存的用户分组。',
            )}
          </Text>
          <div className='space-y-2'>
            {unregistered.map((item) => (
              <div
                key={item.name}
                className='flex flex-wrap items-center gap-2'
              >
                <Text strong>{item.name}</Text>
                {item.user_count > 0 && (
                  <Tag size='small' color='blue'>
                    {t('{{count}} 个用户', { count: item.user_count })}
                  </Tag>
                )}
                {item.plan_count > 0 && (
                  <Tag size='small' color='violet'>
                    {t('{{count}} 个订阅套餐', { count: item.plan_count })}
                  </Tag>
                )}
                {item.rule_count > 0 && (
                  <Tag size='small' color='grey'>
                    {t('{{count}} 类特殊规则', { count: item.rule_count })}
                  </Tag>
                )}
                {item.channel_group && (
                  <Tag size='small' color='orange'>
                    {t('与渠道分组同名')}
                  </Tag>
                )}
                <div className='flex items-center gap-2 ml-auto'>
                  <Button
                    size='small'
                    disabled={item.channel_group}
                    onClick={() => addRow(item.name)}
                  >
                    {t('登记')}
                  </Button>
                  {item.user_count > 0 && (
                    <>
                      <Select
                        size='small'
                        style={{ width: 150 }}
                        optionList={savedUserGroupOptions}
                        value={reassignTargets[item.name] || DEFAULT_USER_GROUP}
                        onChange={(v) =>
                          setReassignTargets((prev) => ({
                            ...prev,
                            [item.name]: v,
                          }))
                        }
                      />
                      <Popconfirm
                        title={t(
                          '将 {{count}} 个用户从「{{from}}」改为「{{to}}」？此操作立即生效。',
                          {
                            count: item.user_count,
                            from: item.name,
                            to:
                              reassignTargets[item.name] || DEFAULT_USER_GROUP,
                          },
                        )}
                        onConfirm={() => handleReassign(item)}
                        position='left'
                      >
                        <Button
                          size='small'
                          type='warning'
                          loading={reassigning === item.name}
                        >
                          {t('改为该用户分组')}
                        </Button>
                      </Popconfirm>
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
