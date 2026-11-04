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

import React from 'react';
import {
  Button,
  Dropdown,
  InputNumber,
  Modal,
  Progress,
  Space,
  SplitButtonGroup,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  getChannelIcon,
  renderQuotaWithAmount,
  showSuccess,
  showError,
  showInfo,
} from '../../../helpers';
import {
  CHANNEL_OPTIONS,
  MODEL_FETCHABLE_CHANNEL_TYPES,
} from '../../../constants';
import { parseUpstreamUpdateMeta } from '../../../hooks/channels/upstreamUpdateUtils';
import { getTagAggregationStatus } from './tagAggregationStatus';
import {
  IconTreeTriangleDown,
  IconMore,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { FaRandom } from 'react-icons/fa';

// Render functions
const renderType = (type, record = {}, t) => {
  const channelInfo = record?.channel_info;
  let type2label = new Map();
  for (let i = 0; i < CHANNEL_OPTIONS.length; i++) {
    type2label[CHANNEL_OPTIONS[i].value] = CHANNEL_OPTIONS[i];
  }
  type2label[0] = { value: 0, label: t('未知类型'), color: 'grey' };

  let icon = getChannelIcon(type);

  if (channelInfo?.is_multi_key) {
    icon =
      channelInfo?.multi_key_mode === 'random' ? (
        <div className='flex items-center gap-1'>
          <FaRandom className='text-blue-500' />
          {icon}
        </div>
      ) : (
        <div className='flex items-center gap-1'>
          <IconTreeTriangleDown className='text-blue-500' />
          {icon}
        </div>
      );
  }

  const typeTag = (
    <Tag color={type2label[type]?.color} shape='circle' prefixIcon={icon}>
      {type2label[type]?.label}
    </Tag>
  );

  let ionetMeta = null;
  if (record?.other_info) {
    try {
      const parsed = JSON.parse(record.other_info);
      if (parsed && typeof parsed === 'object' && parsed.source === 'ionet') {
        ionetMeta = parsed;
      }
    } catch (error) {
      // ignore invalid metadata
    }
  }

  if (!ionetMeta) {
    return typeTag;
  }

  const handleNavigate = (event) => {
    event?.stopPropagation?.();
    if (!ionetMeta?.deployment_id) {
      return;
    }
    const targetUrl = `/console/deployment?deployment_id=${ionetMeta.deployment_id}`;
    window.open(targetUrl, '_blank', 'noopener');
  };

  return (
    <Space spacing={6}>
      {typeTag}
      <Tooltip
        content={
          <div className='max-w-xs'>
            <div className='text-xs text-gray-600'>
              {t('来源于 IO.NET 部署')}
            </div>
            {ionetMeta?.deployment_id && (
              <div className='text-xs text-gray-500 mt-1'>
                {t('部署 ID')}: {ionetMeta.deployment_id}
              </div>
            )}
          </div>
        }
      >
        <span>
          <Tag
            color='purple'
            type='light'
            className='cursor-pointer'
            onClick={handleNavigate}
          >
            IO.NET
          </Tag>
        </span>
      </Tooltip>
    </Space>
  );
};

const renderTagType = (t) => {
  return (
    <Tag color='light-blue' shape='circle' type='light'>
      {t('标签聚合')}
    </Tag>
  );
};

const buildEffectiveAvailabilityMessage = (record, t) => {
  if (!record) {
    return '';
  }
  const messages = [];
  if (record.pending_disable_reason) {
    messages.push(
      t('待确认禁用原因') + ': ' + record.pending_disable_reason,
    );
  }
  if (record.pending_disable_until) {
    messages.push(
      t('待确认截止时间') +
        ': ' +
        timestamp2string(record.pending_disable_until),
    );
  }
  if (record.multi_key_pending_disable_count > 0) {
    messages.push(
      t('待确认 Key 数') + ': ' + record.multi_key_pending_disable_count,
    );
  }
  if (record.multi_key_cooldown_key_count > 0) {
    messages.push(
      t('冷却中的 Key 数') + ': ' + record.multi_key_cooldown_key_count,
    );
  }
  if (messages.length === 0) {
    return t('当前渠道暂不可被分发选中');
  }
  return messages.join('；');
};

const renderStatus = (status, record = {}, t) => {
  if (record?.children !== undefined) {
    const summary =
      record?.tagAggregationStatus || getTagAggregationStatus(record.children);
    if (summary.isMixed) {
      return (
        <Tooltip
          content={`${summary.enabledCount}/${summary.totalCount} ${t('个渠道已启用')}`}
        >
          <Tag color='white' shape='circle'>
            <div className='flex items-center gap-1.5' style={{ lineHeight: 1 }}>
              <span className='text-xs leading-none'>
                {summary.countLabel}
              </span>
              <Progress
                percent={summary.progressPercent}
                stroke={summary.progressStroke}
                showInfo={false}
                size='small'
                style={{ width: 40, height: 3 }}
                className='[&_.semi-progress-bar-inner]:!h-[3px] [&_.semi-progress-bar-track]:!h-[3px]'
              />
            </div>
          </Tag>
        </Tooltip>
      );
    }
    if (summary.isAllDisabled) {
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')}
        </Tag>
      );
    }
    if (summary.isAllEnabled) {
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')}
        </Tag>
      );
    }
  }
  const channelInfo = record?.channel_info;
  if (channelInfo) {
    if (channelInfo.is_multi_key) {
      let keySize = channelInfo.multi_key_size;
      let enabledKeySize = keySize;
      if (channelInfo.multi_key_status_list) {
        enabledKeySize =
          keySize - Object.keys(channelInfo.multi_key_status_list).length;
      }
      return renderMultiKeyStatus(status, keySize, enabledKeySize, t);
    }
  }
  if (status === 1 && record?.effective_available === false) {
    return (
      <Tooltip content={buildEffectiveAvailabilityMessage(record, t)}>
        <Tag color='orange' shape='circle'>
          {t('已启用（暂不可选）')}
        </Tag>
      </Tooltip>
    );
  }
  switch (status) {
    case 1:
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')}
        </Tag>
      );
    case 2:
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')}
        </Tag>
      );
    case 3:
      return (
        <Tag color='yellow' shape='circle'>
          {t('自动禁用')}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知状态')}
        </Tag>
      );
  }
};

const renderMultiKeyStatus = (status, keySize, enabledKeySize, t) => {
  switch (status) {
    case 1:
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    case 2:
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    case 3:
      return (
        <Tag color='yellow' shape='circle'>
          {t('自动禁用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知状态')} {enabledKeySize}/{keySize}
        </Tag>
      );
  }
};

const renderResponseTime = (responseTime, t) => {
  let time = responseTime / 1000;
  time = time.toFixed(2) + t(' 秒');
  if (responseTime === 0) {
    return (
      <Tag color='grey' shape='circle'>
        {t('未测试')}
      </Tag>
    );
  } else if (responseTime <= 1000) {
    return (
      <Tag color='green' shape='circle'>
        {time}
      </Tag>
    );
  } else if (responseTime <= 3000) {
    return (
      <Tag color='lime' shape='circle'>
        {time}
      </Tag>
    );
  } else if (responseTime <= 5000) {
    return (
      <Tag color='yellow' shape='circle'>
        {time}
      </Tag>
    );
  } else {
    return (
      <Tag color='red' shape='circle'>
        {time}
      </Tag>
    );
  }
};

const getChannelMaxConcurrencyOverride = (record) => {
  if (!record || record.children !== undefined || !record.setting) return 0;
  try {
    const setting =
      typeof record.setting === 'string'
        ? JSON.parse(record.setting)
        : record.setting;
    const value = Number(setting?.max_concurrency);
    return Number.isInteger(value) && value > 0 ? value : 0;
  } catch (error) {
    return 0;
  }
};

const getChannelConcurrency = (record, snapshot) => {
  if (!record || !snapshot?.loaded || !snapshot.enabled) return null;
  if (record.children !== undefined) {
    return record.children.reduce(
      (total, child) => {
        const current = getChannelConcurrency(child, snapshot);
        if (!current) return total;
        total.inflight += current.inflight;
        total.maxConcurrency += current.maxConcurrency;
        return total;
      },
      { inflight: 0, maxConcurrency: 0 },
    );
  }

  const live = snapshot.byChannelId?.[record.id];
  const override = getChannelMaxConcurrencyOverride(record);
  return {
    vmCapacity: live?.vmCapacity,
    staticMaxConcurrency: live?.staticMaxConcurrency,
    inflight: Math.max(0, Number(live?.inflight) || 0),
    maxConcurrency:
      live?.vmCapacity || Number(live?.maxConcurrency) > 0
        ? Number(live.maxConcurrency)
        : override || snapshot.defaultMaxConcurrency || 100,
  };
};

const getConcurrencyProgressColor = (percent) => {
  if (percent >= 100) return 'var(--semi-color-danger)';
  if (percent >= 80) return 'var(--semi-color-warning)';
  return 'var(--semi-color-primary)';
};

const renderChannelConcurrency = (record, snapshot, t) => {
  if (!snapshot?.loaded) {
    return <Typography.Text type='tertiary'>-</Typography.Text>;
  }
  if (!snapshot.enabled) {
    return (
      <Tooltip content={t('请先在运营设置中启用渠道并发控制')}>
        <Typography.Text type='tertiary' size='small'>
          {t('未启用')}
        </Typography.Text>
      </Tooltip>
    );
  }

  const concurrency = getChannelConcurrency(record, snapshot);
  if (concurrency?.vmCapacity) {
    const vm = concurrency.vmCapacity;
    const known = vm.state === 'known' || vm.state === 'partial';
    return (
      <Tooltip content={
        <div>
          <div>{t('VM 实时容量')} · {vm.model}</div>
          <div>{t('静态上限 {{limit}}，快照年龄 {{age}}ms', {
            limit: concurrency.staticMaxConcurrency, age: vm.age_ms,
          })}</div>
          {known && <div>{t('VM 当前可用 {{count}}', { count: vm.snapshot?.available_now })}</div>}
        </div>
      }>
        <Typography.Text type={known && concurrency.maxConcurrency > 0 ? 'secondary' : 'warning'} size='small'>
          {known ? `${concurrency.inflight}/${concurrency.maxConcurrency}` : t('容量状态未知')}
          {vm.state === 'partial' && ` · ${t('部分状态未知')}`}
        </Typography.Text>
      </Tooltip>
    );
  }
  if (!concurrency || concurrency.maxConcurrency <= 0) {
    return <Typography.Text type='tertiary'>-</Typography.Text>;
  }
  const percent = Math.min(
    100,
    (concurrency.inflight / concurrency.maxConcurrency) * 100,
  );
  const tooltip =
    record.children !== undefined
      ? t('标签内子渠道并发汇总')
      : t('当前在途请求数 / 实际并发上限');

  return (
    <Tooltip content={tooltip}>
      <div style={{ width: 92 }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 3,
          }}
        >
          <Typography.Text
            size='small'
            strong={concurrency.inflight >= concurrency.maxConcurrency}
          >
            {concurrency.inflight}/{concurrency.maxConcurrency}
          </Typography.Text>
          <Typography.Text type='tertiary' size='small'>
            {Math.round(percent)}%
          </Typography.Text>
        </div>
        <Progress
          percent={percent}
          size='small'
          stroke={getConcurrencyProgressColor(percent)}
          showInfo={false}
          style={{ height: 4 }}
        />
      </div>
    </Tooltip>
  );
};

const isRequestPassThroughEnabled = (record) => {
  if (!record || record.children !== undefined) {
    return false;
  }
  const settingValue = record.setting;
  if (!settingValue) {
    return false;
  }
  if (typeof settingValue === 'object') {
    return settingValue.pass_through_body_enabled === true;
  }
  if (typeof settingValue !== 'string') {
    return false;
  }
  try {
    const parsed = JSON.parse(settingValue);
    return parsed?.pass_through_body_enabled === true;
  } catch (error) {
    return false;
  }
};

const getUpstreamUpdateMeta = (record) => {
  const supported =
    !!record &&
    record.children === undefined &&
    MODEL_FETCHABLE_CHANNEL_TYPES.has(record.type);
  if (!record || record.children !== undefined) {
    return {
      supported: false,
      enabled: false,
      pendingAddModels: [],
      pendingRemoveModels: [],
    };
  }
  const parsed =
    record?.upstreamUpdateMeta && typeof record.upstreamUpdateMeta === 'object'
      ? record.upstreamUpdateMeta
      : parseUpstreamUpdateMeta(record?.settings);
  return {
    supported,
    enabled: parsed?.enabled === true,
    pendingAddModels: Array.isArray(parsed?.pendingAddModels)
      ? parsed.pendingAddModels
      : [],
    pendingRemoveModels: Array.isArray(parsed?.pendingRemoveModels)
      ? parsed.pendingRemoveModels
      : [],
  };
};

export const getChannelsColumns = ({
  t,
  COLUMN_KEYS,
  updateChannelBalance,
  manageChannel,
  manageTag,
  submitTagEdit,
  testChannel,
  openTagTestModal,
  setCurrentTestChannel,
  setShowModelTestModal,
  setEditingChannel,
  setShowEdit,
  setShowEditTag,
  setEditingTag,
  copySelectedChannel,
  refresh,
  activePage,
  channels,
  checkOllamaVersion,
  setShowMultiKeyManageModal,
  setCurrentMultiKeyChannel,
  openUpstreamUpdateModal,
  detectChannelUpstreamUpdates,
  channelConcurrencySnapshot,
}) => {
  return [
    {
      key: COLUMN_KEYS.ID,
      title: t('ID'),
      dataIndex: 'id',
    },
    {
      key: COLUMN_KEYS.NAME,
      title: t('名称'),
      dataIndex: 'name',
      render: (text, record, index) => {
        const passThroughEnabled = isRequestPassThroughEnabled(record);
        const upstreamUpdateMeta = getUpstreamUpdateMeta(record);
        const pendingAddCount = upstreamUpdateMeta.pendingAddModels.length;
        const pendingRemoveCount =
          upstreamUpdateMeta.pendingRemoveModels.length;
        const showUpstreamUpdateTag =
          upstreamUpdateMeta.supported &&
          upstreamUpdateMeta.enabled &&
          (pendingAddCount > 0 || pendingRemoveCount > 0);
        const nameNode =
          record.remark && record.remark.trim() !== '' ? (
            <Tooltip
              content={
                <div className='flex flex-col gap-2 max-w-xs'>
                  <div className='text-sm'>{record.remark}</div>
                  <Button
                    size='small'
                    type='primary'
                    theme='outline'
                    onClick={(e) => {
                      e.stopPropagation();
                      navigator.clipboard
                        .writeText(record.remark)
                        .then(() => {
                          showSuccess(t('复制成功'));
                        })
                        .catch(() => {
                          showError(t('复制失败'));
                        });
                    }}
                  >
                    {t('复制')}
                  </Button>
                </div>
              }
              trigger='hover'
              position='topLeft'
            >
              <span>{text}</span>
            </Tooltip>
          ) : (
            <span>{text}</span>
          );

        if (!passThroughEnabled && !showUpstreamUpdateTag) {
          return nameNode;
        }

        return (
          <Space spacing={6} align='center'>
            {nameNode}
            {passThroughEnabled && (
              <Tooltip
                content={t(
                  '该渠道已开启请求透传：参数覆写、模型重定向、渠道适配等 NewAPI 内置功能将失效，非最佳实践；如因此产生问题，请勿提交 issue 反馈。',
                )}
                trigger='hover'
                position='topLeft'
              >
                <span className='inline-flex items-center'>
                  <IconAlertTriangle
                    style={{ color: 'var(--semi-color-warning)' }}
                  />
                </span>
              </Tooltip>
            )}
            {showUpstreamUpdateTag && (
              <Space spacing={4} align='center'>
                {pendingAddCount > 0 ? (
                  <Tooltip content={t('点击处理新增模型')} position='top'>
                    <Tag
                      color='green'
                      type='light'
                      size='small'
                      shape='circle'
                      className='cursor-pointer transition-all duration-150 hover:opacity-85 hover:-translate-y-px active:scale-95'
                      onClick={(e) => {
                        e.stopPropagation();
                        openUpstreamUpdateModal(
                          record,
                          upstreamUpdateMeta.pendingAddModels,
                          upstreamUpdateMeta.pendingRemoveModels,
                          'add',
                        );
                      }}
                    >
                      +{pendingAddCount}
                    </Tag>
                  </Tooltip>
                ) : null}
                {pendingRemoveCount > 0 ? (
                  <Tooltip content={t('点击处理删除模型')} position='top'>
                    <Tag
                      color='red'
                      type='light'
                      size='small'
                      shape='circle'
                      className='cursor-pointer transition-all duration-150 hover:opacity-85 hover:-translate-y-px active:scale-95'
                      onClick={(e) => {
                        e.stopPropagation();
                        openUpstreamUpdateModal(
                          record,
                          upstreamUpdateMeta.pendingAddModels,
                          upstreamUpdateMeta.pendingRemoveModels,
                          'remove',
                        );
                      }}
                    >
                      -{pendingRemoveCount}
                    </Tag>
                  </Tooltip>
                ) : null}
              </Space>
            )}
          </Space>
        );
      },
    },
    {
      key: COLUMN_KEYS.GROUP,
      title: t('分组'),
      dataIndex: 'group',
      render: (text, record, index) => (
        <div>
          <Space spacing={2}>
            {text
              ?.split(',')
              .sort((a, b) => {
                if (a === 'default') return -1;
                if (b === 'default') return 1;
                return a.localeCompare(b);
              })
              .map((item, index) => renderGroup(item))}
          </Space>
        </div>
      ),
    },
    {
      key: COLUMN_KEYS.TYPE,
      title: t('类型'),
      dataIndex: 'type',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return <>{renderType(text, record, t)}</>;
        } else {
          return <>{renderTagType(t)}</>;
        }
      },
    },
    {
      key: COLUMN_KEYS.STATUS,
      title: t('状态'),
      dataIndex: 'status',
      render: (text, record, index) => {
        if (text === 3) {
          if (record.other_info === '') {
            record.other_info = '{}';
          }
          let otherInfo = JSON.parse(record.other_info);
          let reason = otherInfo['status_reason'];
          let time = otherInfo['status_time'];
          return (
            <div>
              <Tooltip
                content={
                  t('原因：') + reason + t('，时间：') + timestamp2string(time)
                }
              >
                {renderStatus(text, record, t)}
              </Tooltip>
            </div>
          );
        } else {
          return renderStatus(text, record, t);
        }
      },
    },
    {
      key: COLUMN_KEYS.CONCURRENCY,
      title: t('并发'),
      dataIndex: 'concurrency',
      render: (text, record) =>
        renderChannelConcurrency(record, channelConcurrencySnapshot, t),
    },
    {
      key: COLUMN_KEYS.RESPONSE_TIME,
      title: t('响应时间'),
      dataIndex: 'response_time',
      render: (text, record, index) => <div>{renderResponseTime(text, t)}</div>,
    },
    {
      key: COLUMN_KEYS.BALANCE,
      title: t('已用/剩余'),
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <Space spacing={1}>
                <Tooltip content={t('已用额度')}>
                  <Tag color='white' type='ghost' shape='circle'>
                    {renderQuota(record.used_quota)}
                  </Tag>
                </Tooltip>
                <Tooltip
                  content={
                    record.type === 57
                      ? t('查看 Codex 帐号信息与用量')
                      : t('剩余额度') +
                        ': ' +
                        renderQuotaWithAmount(record.balance) +
                        t('，点击更新')
                  }
                >
                  <Tag
                    color={record.type === 57 ? 'light-blue' : 'white'}
                    type={record.type === 57 ? 'light' : 'ghost'}
                    shape='circle'
                    className={record.type === 57 ? 'cursor-pointer' : ''}
                    onClick={() => updateChannelBalance(record)}
                  >
                    {record.type === 57
                      ? t('帐号信息')
                      : renderQuotaWithAmount(record.balance)}
                  </Tag>
                </Tooltip>
              </Space>
            </div>
          );
        } else {
          return (
            <Tooltip content={t('已用额度')}>
              <Tag color='white' type='ghost' shape='circle'>
                {renderQuota(record.used_quota)}
              </Tag>
            </Tooltip>
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.PRIORITY,
      title: t('优先级'),
      dataIndex: 'priority',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <InputNumber
                style={{ width: 70 }}
                name='priority'
                onBlur={(e) => {
                  manageChannel(record.id, 'priority', record, e.target.value);
                }}
                keepFocus={true}
                innerButtons
                defaultValue={record.priority}
                min={-999}
                size='small'
              />
            </div>
          );
        } else {
          return (
            <InputNumber
              style={{ width: 70 }}
              name='priority'
              keepFocus={true}
              onBlur={(e) => {
                Modal.warning({
                  title: t('修改子渠道优先级'),
                  content:
                    t('确定要修改所有子渠道优先级为 ') +
                    e.target.value +
                    t(' 吗？'),
                  onOk: () => {
                    if (e.target.value === '') {
                      return;
                    }
                    submitTagEdit('priority', {
                      tag: record.key,
                      priority: e.target.value,
                    });
                  },
                });
              }}
              innerButtons
              defaultValue={record.priority}
              min={-999}
              size='small'
            />
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.WEIGHT,
      title: t('权重'),
      dataIndex: 'weight',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <InputNumber
                style={{ width: 70 }}
                name='weight'
                onBlur={(e) => {
                  manageChannel(record.id, 'weight', record, e.target.value);
                }}
                keepFocus={true}
                innerButtons
                defaultValue={record.weight}
                min={0}
                size='small'
              />
            </div>
          );
        } else {
          return (
            <InputNumber
              style={{ width: 70 }}
              name='weight'
              keepFocus={true}
              onBlur={(e) => {
                Modal.warning({
                  title: t('修改子渠道权重'),
                  content:
                    t('确定要修改所有子渠道权重为 ') +
                    e.target.value +
                    t(' 吗？'),
                  onOk: () => {
                    if (e.target.value === '') {
                      return;
                    }
                    submitTagEdit('weight', {
                      tag: record.key,
                      weight: e.target.value,
                    });
                  },
                });
              }}
              innerButtons
              defaultValue={record.weight}
              min={-999}
              size='small'
            />
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.QUOTA_USAGE,
      title: t('周期用量'),
      dataIndex: 'quota_usage',
      render: (text, record, index) => {
        if (record.children !== undefined) {
          return null;
        }
        const usage = record.quota_usage;
        if (!usage || (!usage.quota_limit && !usage.count_limit)) {
          return (
            <Typography.Text type='tertiary' size='small'>
              -
            </Typography.Text>
          );
        }
        const periodLabel =
          usage.period === 'day'
            ? t('日')
            : usage.period === 'week'
              ? t('周')
              : usage.period === 'month'
                ? t('月')
                : usage.period;
        const scopeLabel =
          usage.scope === 'tag' ? t('标签') : t('渠道');
        const quotaPct =
          Number(usage.quota_limit) > 0
            ? Math.min(
                100,
                Math.floor(
                  (Number(usage.used_quota || 0) /
                    Math.max(1, Number(usage.quota_limit))) *
                    100,
                ),
              )
            : 0;
        const countPct =
          Number(usage.count_limit) > 0
            ? Math.min(
                100,
                Math.floor(
                  (Number(usage.used_count || 0) /
                    Math.max(1, Number(usage.count_limit))) *
                    100,
                ),
              )
            : 0;
        const tooltipContent = (
          <div className='text-xs'>
            <div>
              {scopeLabel} · {periodLabel}
            </div>
            {Number(usage.quota_limit) > 0 && (
              <div>
                {t('额度')}: {Number(usage.used_quota || 0)} /{' '}
                {Number(usage.quota_limit)}
              </div>
            )}
            {Number(usage.count_limit) > 0 && (
              <div>
                {t('次数')}: {Number(usage.used_count || 0)} /{' '}
                {Number(usage.count_limit)}
              </div>
            )}
            {usage.period_end ? (
              <div>
                {t('重置于')}:{' '}
                {new Date(Number(usage.period_end) * 1000).toLocaleString()}
              </div>
            ) : null}
          </div>
        );
        return (
          <Tooltip content={tooltipContent}>
            <div style={{ minWidth: 110 }}>
              <div className='text-xs mb-1 text-tertiary'>
                {scopeLabel} · {periodLabel}
              </div>
              {Number(usage.quota_limit) > 0 && (
                <Progress percent={quotaPct} size='small' showInfo={false} />
              )}
              {Number(usage.count_limit) > 0 && (
                <Progress
                  percent={countPct}
                  size='small'
                  showInfo={false}
                  className='mt-1'
                />
              )}
            </div>
          </Tooltip>
        );
      },
    },
    {
      key: COLUMN_KEYS.OPERATE,
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      render: (text, record, index) => {
        if (record.children === undefined) {
          const upstreamUpdateMeta = getUpstreamUpdateMeta(record);
          const moreMenuItems = [
            {
              node: 'item',
              name: t('删除'),
              type: 'danger',
              onClick: () => {
                Modal.confirm({
                  title: t('确定是否要删除此渠道？'),
                  content: t('此修改将不可逆'),
                  onOk: () => {
                    (async () => {
                      await manageChannel(record.id, 'delete', record);
                      await refresh();
                      setTimeout(() => {
                        if (channels.length === 0 && activePage > 1) {
                          refresh(activePage - 1);
                        }
                      }, 100);
                    })();
                  },
                });
              },
            },
            {
              node: 'item',
              name: t('复制'),
              type: 'tertiary',
              onClick: () => {
                Modal.confirm({
                  title: t('确定是否要复制此渠道？'),
                  content: t('复制渠道的所有信息'),
                  onOk: () => copySelectedChannel(record),
                });
              },
            },
          ];

          if (upstreamUpdateMeta.supported) {
            moreMenuItems.push({
              node: 'item',
              name: t('仅检测上游模型更新'),
              type: 'tertiary',
              onClick: () => {
                detectChannelUpstreamUpdates(record);
              },
            });
            moreMenuItems.push({
              node: 'item',
              name: t('处理上游模型更新'),
              type: 'tertiary',
              onClick: () => {
                if (!upstreamUpdateMeta.enabled) {
                  showInfo(t('该渠道未开启上游模型更新检测'));
                  return;
                }
                if (
                  upstreamUpdateMeta.pendingAddModels.length === 0 &&
                  upstreamUpdateMeta.pendingRemoveModels.length === 0
                ) {
                  showInfo(t('该渠道暂无可处理的上游模型更新'));
                  return;
                }
                openUpstreamUpdateModal(
                  record,
                  upstreamUpdateMeta.pendingAddModels,
                  upstreamUpdateMeta.pendingRemoveModels,
                  upstreamUpdateMeta.pendingAddModels.length > 0
                    ? 'add'
                    : 'remove',
                );
              },
            });
          }

          if (record.type === 4) {
            moreMenuItems.unshift({
              node: 'item',
              name: t('测活'),
              type: 'tertiary',
              onClick: () => checkOllamaVersion(record),
            });
          }

          return (
            <Space wrap>
              <SplitButtonGroup
                className='overflow-hidden'
                aria-label={t('测试单个渠道操作项目组')}
              >
                <Button
                  size='small'
                  type='tertiary'
                  onClick={() =>
                    testChannel(record, '', '', false, {
                      promptEnableOnSuccess: true,
                    })
                  }
                >
                  {t('测试')}
                </Button>
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconTreeTriangleDown />}
                  onClick={() => {
                    setCurrentTestChannel(record);
                    setShowModelTestModal(true);
                  }}
                />
              </SplitButtonGroup>

              {record.status === 1 ? (
                <Button
                  type='danger'
                  size='small'
                  onClick={() => manageChannel(record.id, 'disable', record)}
                >
                  {t('禁用')}
                </Button>
              ) : (
                <Button
                  size='small'
                  onClick={() => manageChannel(record.id, 'enable', record)}
                >
                  {t('启用')}
                </Button>
              )}

              {record.channel_info?.is_multi_key ? (
                <SplitButtonGroup aria-label={t('多密钥渠道操作项目组')}>
                  <Button
                    type='tertiary'
                    size='small'
                    onClick={() => {
                      setEditingChannel(record);
                      setShowEdit(true);
                    }}
                  >
                    {t('编辑')}
                  </Button>
                  <Dropdown
                    trigger='click'
                    position='bottomRight'
                    menu={[
                      {
                        node: 'item',
                        name: t('多密钥管理'),
                        onClick: () => {
                          setCurrentMultiKeyChannel(record);
                          setShowMultiKeyManageModal(true);
                        },
                      },
                    ]}
                  >
                    <Button
                      type='tertiary'
                      size='small'
                      icon={<IconTreeTriangleDown />}
                    />
                  </Dropdown>
                </SplitButtonGroup>
              ) : (
                <Button
                  type='tertiary'
                  size='small'
                  onClick={() => {
                    setEditingChannel(record);
                    setShowEdit(true);
                  }}
                >
                  {t('编辑')}
                </Button>
              )}

              <Dropdown
                trigger='click'
                position='bottomRight'
                menu={moreMenuItems}
              >
                <Button icon={<IconMore />} type='tertiary' size='small' />
              </Dropdown>
            </Space>
          );
        } else {
          // 标签操作按钮
          return (
            <Space wrap>
              <SplitButtonGroup
                className='overflow-hidden'
                aria-label={t('测试标签渠道操作项目组')}
              >
                <Button
                  size='small'
                  type='tertiary'
                  onClick={() => openTagTestModal(record, 'default')}
                >
                  {t('测试')}
                </Button>
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconTreeTriangleDown />}
                  onClick={() => openTagTestModal(record, 'model')}
                />
              </SplitButtonGroup>
              <Button
                type='tertiary'
                size='small'
                onClick={() => manageTag(record.key, 'enable')}
              >
                {t('启用全部')}
              </Button>
              <Button
                type='tertiary'
                size='small'
                onClick={() => manageTag(record.key, 'disable')}
              >
                {t('禁用全部')}
              </Button>
              <Button
                type='tertiary'
                size='small'
                onClick={() => {
                  setShowEditTag(true);
                  setEditingTag(record.key);
                }}
              >
                {t('编辑')}
              </Button>
            </Space>
          );
        }
      },
    },
  ];
};
