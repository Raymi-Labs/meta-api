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

import { readTokenRouting } from './tokenRouting';
import React from 'react';
import {
  Button,
  Dropdown,
  Space,
  SplitButtonGroup,
  Tag,
  AvatarGroup,
  Avatar,
  Tooltip,
  Progress,
  Popover,
  Typography,
  Input,
  Modal,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  getModelCategories,
  showError,
} from '../../../helpers';
import {
  formatConcurrencyLabel,
  formatWindowLimitShort,
} from '../../../helpers/render';
import {
  IconTreeTriangleDown,
  IconCopy,
  IconEyeOpened,
  IconEyeClosed,
} from '@douyinfe/semi-icons';

// 进度颜色辅助函数
const getProgressColor = (pct) => {
  if (pct === 100) return 'var(--semi-color-success)';
  if (pct <= 10) return 'var(--semi-color-danger)';
  if (pct <= 30) return 'var(--semi-color-warning)';
  return undefined;
};

// 渲染函数
function renderTimestamp(timestamp) {
  return <>{timestamp2string(timestamp)}</>;
}

// 仅渲染状态列（不含用量）
const renderStatus = (text, record, t) => {
  const enabled = text === 1;

  let tagColor = 'black';
  let tagText = t('未知状态');
  if (enabled) {
    tagColor = 'green';
    tagText = t('已启用');
  } else if (text === 2) {
    tagColor = 'red';
    tagText = t('已禁用');
  } else if (text === 3) {
    tagColor = 'yellow';
    tagText = t('已过期');
  } else if (text === 4) {
    tagColor = 'grey';
    tagText = t('已耗尽');
  }

  return (
    <Tag color={tagColor} shape='circle' size='small'>
      {tagText}
    </Tag>
  );
};

// 渲染分组列
const renderGroupColumn = (text, record, t) => {
  if (record?.routing_config)
    return (
      <Space wrap spacing={4}>
        {readTokenRouting(record.routing_config).groups.map((group) => (
          <Tag key={group}>{group}</Tag>
        ))}
      </Space>
    );
  if (text === 'auto') {
    return (
      <Tooltip
        content={t(
          '当前分组为 auto，会自动选择最优分组，当一个组不可用时自动降级到下一个组（熔断机制）',
        )}
        position='top'
      >
        <Tag color='white' shape='circle'>
          {t('智能熔断')}
          {record && record.cross_group_retry ? `(${t('跨分组')})` : ''}
        </Tag>
      </Tooltip>
    );
  }
  if (record?.auto_group_fallback && record?.source_type !== 'sellable_token') {
    return (
      <Space wrap spacing={4}>
        {renderGroup(text)}
        <Tooltip content={t('首选分组不可用时自动切组，按实际使用分组计费。')}>
          <Tag color='blue' size='small'>
            Auto
          </Tag>
        </Tooltip>
      </Space>
    );
  }
  return renderGroup(text);
};

// The API resolves inherited groups and user-specific pricing for this page.
// Missing/invalid metadata (including an older backend) must not become 1x.
const renderGroupRatio = (record, t) => {
  const ratio = record?.group_ratio;
  const isFixed =
    !record?.routing_config &&
    record?.group_ratio_status === 'fixed' &&
    typeof ratio === 'number' &&
    Number.isFinite(ratio) &&
    ratio >= 0;
  const isAuto =
    record?.group_ratio_status === 'auto' || !!record?.routing_config;
  const label = isFixed ? `${ratio}x` : isAuto ? t('自动') : '—';
  const hint =
    isFixed &&
    record?.auto_group_fallback &&
    record?.source_type !== 'sellable_token'
      ? t(
          '此处为首选分组倍率；自动切组后按实际使用分组计费，请以使用日志为准。',
        )
      : isFixed
        ? t(
            '当前分组倍率，已包含适用的专属倍率，不含模型价格和时间倍率；实际扣费以使用日志为准。',
          )
        : isAuto
          ? t('自动分组的倍率由实际使用的分组决定，请以使用日志为准。')
          : t('当前无法确定分组倍率，请刷新或联系管理员；不会按 1x 估算。');

  return (
    <Tooltip content={hint} position='top'>
      <Tag color='white' shape='circle'>
        {label}
      </Tag>
    </Tooltip>
  );
};

// 渲染 Token key 列（支持显示/隐藏与复制）
const renderTokenKey = (
  text,
  record,
  showKeys,
  tokenFullKeys,
  toggleTokenKeyVisibility,
  copyTokenKey,
) => {
  const formatKey = (key) => (key?.startsWith('sk-') ? key : `sk-${key || ''}`);
  const fullKey = formatKey(tokenFullKeys[record.id] || record.key);
  const maskedKey = formatKey(record.key);
  const revealed = !!showKeys[record.id];

  return (
    <div className='w-[200px]'>
      <Input
        readOnly
        value={revealed ? fullKey : maskedKey}
        size='small'
        suffix={
          <div className='flex items-center'>
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={revealed ? <IconEyeClosed /> : <IconEyeOpened />}
              aria-label='toggle token visibility'
              onClick={async (e) => {
                e.stopPropagation();
                await toggleTokenKeyVisibility(record);
              }}
            />
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={<IconCopy />}
              aria-label='copy token key'
              onClick={async (e) => {
                e.stopPropagation();
                await copyTokenKey(record);
              }}
            />
          </div>
        }
      />
    </div>
  );
};

// 渲染模型限制列
const renderModelLimits = (text, record, t) => {
  if (record.model_limits_enabled && text) {
    const models = text.split(',').filter(Boolean);
    const categories = getModelCategories(t);

    const vendorAvatars = [];
    const matchedModels = new Set();
    Object.entries(categories).forEach(([key, category]) => {
      if (key === 'all') return;
      if (!category.icon || !category.filter) return;
      const vendorModels = models.filter((m) =>
        category.filter({ model_name: m }),
      );
      if (vendorModels.length > 0) {
        vendorAvatars.push(
          <Tooltip
            key={key}
            content={vendorModels.join(', ')}
            position='top'
            showArrow
          >
            <Avatar
              size='extra-extra-small'
              alt={category.label}
              color='transparent'
            >
              {category.icon}
            </Avatar>
          </Tooltip>,
        );
        vendorModels.forEach((m) => matchedModels.add(m));
      }
    });

    const unmatchedModels = models.filter((m) => !matchedModels.has(m));
    if (unmatchedModels.length > 0) {
      vendorAvatars.push(
        <Tooltip
          key='unknown'
          content={unmatchedModels.join(', ')}
          position='top'
          showArrow
        >
          <Avatar size='extra-extra-small' alt='unknown'>
            {t('其他')}
          </Avatar>
        </Tooltip>,
      );
    }

    return <AvatarGroup size='extra-extra-small'>{vendorAvatars}</AvatarGroup>;
  } else {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }
};

// 渲染 IP 限制列
const renderAllowIps = (text, t) => {
  if (!text || text.trim() === '') {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }

  const ips = text
    .split('\n')
    .map((ip) => ip.trim())
    .filter(Boolean);

  const displayIps = ips.slice(0, 1);
  const extraCount = ips.length - displayIps.length;

  const ipTags = displayIps.map((ip, idx) => (
    <Tag key={idx} shape='circle'>
      {ip}
    </Tag>
  ));

  if (extraCount > 0) {
    ipTags.push(
      <Tooltip
        key='extra'
        content={ips.slice(1).join(', ')}
        position='top'
        showArrow
      >
        <Tag shape='circle'>{'+' + extraCount}</Tag>
      </Tooltip>,
    );
  }

  return <Space wrap>{ipTags}</Space>;
};

const getRuntimeColor = (pct) => {
  if (pct > 90) return 'var(--semi-color-danger)';
  if (pct >= 70) return 'var(--semi-color-warning)';
  return 'var(--semi-color-primary)';
};

const renderRuntimeLimits = (record, t) => {
  const maxConcurrency = Number(record?.max_concurrency || 0);
  const windowRequestLimit = Number(record?.window_request_limit || 0);
  const windowSeconds = Number(record?.window_seconds || 0);
  const hasConcurrency = maxConcurrency > 0;
  const hasWindow = windowRequestLimit > 0 && windowSeconds > 0;

  if (!hasConcurrency && !hasWindow) {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }

  const rs = record?.runtime_status;
  const currentConc = Number(rs?.current_concurrency || 0);
  const windowUsed = Number(rs?.window_used || 0);

  const concPct = hasConcurrency
    ? Math.min((currentConc / maxConcurrency) * 100, 100)
    : 0;
  const winPct = hasWindow
    ? Math.min((windowUsed / windowRequestLimit) * 100, 100)
    : 0;

  const popoverContent = (
    <div style={{ padding: '4px 0', minWidth: 160 }}>
      {hasConcurrency && (
        <div style={{ marginBottom: hasWindow ? 10 : 0 }}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              fontSize: '0.82rem',
              marginBottom: 4,
            }}
          >
            <span>{t('当前并发')}</span>
            <strong>
              {rs
                ? `${currentConc}/${maxConcurrency}`
                : formatConcurrencyLabel(maxConcurrency, t)}
            </strong>
          </div>
          {rs && (
            <Progress
              percent={concPct}
              size='small'
              stroke={getRuntimeColor(concPct)}
              showInfo={false}
              style={{ height: 4 }}
            />
          )}
        </div>
      )}
      {hasWindow && (
        <div>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              fontSize: '0.82rem',
              marginBottom: 4,
            }}
          >
            <span>{t('窗口请求')}</span>
            <strong>
              {rs
                ? `${windowUsed}/${windowRequestLimit}`
                : formatWindowLimitShort(windowSeconds, windowRequestLimit, t)}
            </strong>
          </div>
          {rs && (
            <Progress
              percent={winPct}
              size='small'
              stroke={getRuntimeColor(winPct)}
              showInfo={false}
              style={{ height: 4 }}
            />
          )}
        </div>
      )}
    </div>
  );

  return (
    <Popover content={popoverContent} position='top' showArrow>
      <Space wrap style={{ cursor: 'pointer' }}>
        {hasConcurrency && (
          <Tag color='white' shape='circle'>
            {rs
              ? `⇄ ${currentConc}/${maxConcurrency}`
              : `⇄ ${formatConcurrencyLabel(maxConcurrency, t)}`}
          </Tag>
        )}
        {hasWindow && (
          <Tag color='white' shape='circle'>
            {rs
              ? `↻ ${windowUsed}/${windowRequestLimit}`
              : `↻ ${formatWindowLimitShort(windowSeconds, windowRequestLimit, t)}`}
          </Tag>
        )}
      </Space>
    </Popover>
  );
};

// 渲染独立额度用量列
const renderQuotaUsage = (text, record, t) => {
  const { Paragraph } = Typography;
  const used = parseInt(record.used_quota) || 0;
  const remain = parseInt(record.remain_quota) || 0;
  const total = used + remain;
  const isPackageToken = !!record?.package_enabled;
  const packageLimit = Number(record?.package_limit_quota || 0);
  const packageUsed = Number(record?.package_used_quota || 0);
  const packageRemain =
    packageLimit > 0 ? Math.max(0, packageLimit - packageUsed) : null;
  const packageNextResetText =
    Number(record?.package_next_reset_time || 0) > 0
      ? timestamp2string(Number(record.package_next_reset_time))
      : t('待初始化');
  if (record.unlimited_quota) {
    const popoverContent = (
      <div className='text-xs p-2'>
        <Paragraph copyable={{ content: renderQuota(used) }}>
          {t('已用额度')}: {renderQuota(used)}
        </Paragraph>
        {isPackageToken && (
          <>
            <Paragraph
              copyable={{
                content:
                  packageRemain === null ? '-' : renderQuota(packageRemain),
              }}
            >
              {t('本周期剩余')}:{' '}
              {packageRemain === null ? '-' : renderQuota(packageRemain)}
            </Paragraph>
            <Paragraph>
              {t('下次重置')}: {packageNextResetText}
            </Paragraph>
          </>
        )}
      </div>
    );
    return (
      <Popover content={popoverContent} position='top'>
        <Tag color='white' shape='circle'>
          {t('无限额度')}
        </Tag>
      </Popover>
    );
  }
  const percent = total > 0 ? (remain / total) * 100 : 0;
  const popoverContent = (
    <div className='text-xs p-2'>
      <Paragraph copyable={{ content: renderQuota(used) }}>
        {t('已用额度')}: {renderQuota(used)}
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(remain) }}>
        {t('剩余额度')}: {renderQuota(remain)} ({percent.toFixed(0)}%)
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(total) }}>
        {t('总额度')}: {renderQuota(total)}
      </Paragraph>
      {isPackageToken && (
        <>
          <Paragraph
            copyable={{
              content:
                packageRemain === null ? '-' : renderQuota(packageRemain),
            }}
          >
            {t('本周期剩余')}:{' '}
            {packageRemain === null ? '-' : renderQuota(packageRemain)}
          </Paragraph>
          <Paragraph>
            {t('下次重置')}: {packageNextResetText}
          </Paragraph>
        </>
      )}
    </div>
  );
  return (
    <Popover content={popoverContent} position='top'>
      <Tag color='white' shape='circle'>
        <div className='flex flex-col items-end'>
          <span className='text-xs leading-none'>{`${renderQuota(remain)} / ${renderQuota(total)}`}</span>
          <Progress
            percent={percent}
            stroke={getProgressColor(percent)}
            aria-label='quota usage'
            format={() => `${percent.toFixed(0)}%`}
            style={{ width: '100%', marginTop: '1px', marginBottom: 0 }}
          />
        </div>
      </Tag>
    </Popover>
  );
};

// 将已用额度保存在独立字段中，便于跨 Token 更快比较。
const renderUsedQuota = (text, record) => {
  const used = parseInt(record.used_quota) || 0;
  return (
    <Tag color='white' shape='circle'>
      {renderQuota(used)}
    </Tag>
  );
};

const formatResetTimeCompact = (timestamp) => {
  const ts = Number(timestamp || 0);
  if (ts <= 0) return '';
  const d = new Date(ts * 1000);
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  const hour = String(d.getHours()).padStart(2, '0');
  const minute = String(d.getMinutes()).padStart(2, '0');
  return `${month}-${day} ${hour}:${minute}`;
};

const renderPackageCycleBalance = (text, record, t) => {
  const isPackageToken = !!record?.package_enabled;
  const packageLimit = Number(record?.package_limit_quota || 0);
  const packageUsed = Number(record?.package_used_quota || 0);
  const packageRemain =
    packageLimit > 0 ? Math.max(0, packageLimit - packageUsed) : 0;
  const packageNextResetTs = Number(record?.package_next_reset_time || 0);
  const packageNextResetText =
    packageNextResetTs > 0
      ? timestamp2string(packageNextResetTs)
      : t('待初始化');
  const packageNextResetCompact =
    packageNextResetTs > 0
      ? formatResetTimeCompact(packageNextResetTs)
      : t('待初始化');

  if (!isPackageToken || packageLimit <= 0) {
    return (
      <Tag color='white' shape='circle'>
        -
      </Tag>
    );
  }

  const remainPct = (packageRemain / packageLimit) * 100;
  const popoverContent = (
    <div className='text-xs p-2'>
      <Typography.Paragraph copyable={{ content: renderQuota(packageUsed) }}>
        {t('本周期已用')}: {renderQuota(packageUsed)}
      </Typography.Paragraph>
      <Typography.Paragraph copyable={{ content: renderQuota(packageRemain) }}>
        {t('本周期剩余')}: {renderQuota(packageRemain)}
      </Typography.Paragraph>
      <Typography.Paragraph copyable={{ content: renderQuota(packageLimit) }}>
        {t('周期额度')}: {renderQuota(packageLimit)}
      </Typography.Paragraph>
      <Typography.Paragraph>
        {t('下次重置')}: {packageNextResetText}
      </Typography.Paragraph>
    </div>
  );

  return (
    <div className='flex flex-col gap-1'>
      <Popover content={popoverContent} position='top'>
        <Tag color='white' shape='circle'>
          <div className='flex flex-col items-end'>
            <span className='text-xs leading-none'>{`${renderQuota(packageRemain)} / ${renderQuota(packageLimit)}`}</span>
            <Progress
              percent={remainPct}
              stroke={getProgressColor(remainPct)}
              aria-label='package cycle balance'
              format={() => `${remainPct.toFixed(0)}%`}
              style={{ width: '100%', marginTop: '1px', marginBottom: 0 }}
            />
          </div>
        </Tag>
      </Popover>
      <Tooltip
        content={`${t('下次重置')}: ${packageNextResetText}`}
        position='top'
      >
        <Tag color='grey' shape='circle' size='small'>
          {t('重置')}: {packageNextResetCompact}
        </Tag>
      </Tooltip>
    </div>
  );
};

const getPackagePeriodLabel = (record, t) => {
  switch (record?.package_period) {
    case 'hourly':
      return t('每小时');
    case 'daily':
      return t('每日');
    case 'weekly':
      return t('每周');
    case 'monthly':
      return t('每月');
    case 'custom':
      return t('自定义');
    default:
      return t('周期');
  }
};

const renderTokenName = (text, record, t) => {
  const isSellableToken = record?.source_type === 'sellable_token';
  const isPackageEnabled = !!record?.package_enabled;

  return (
    <div className='flex items-center gap-1 flex-wrap'>
      <span className='font-medium'>{text}</span>
      {isSellableToken && (
        <Tag color='cyan' shape='circle' size='small'>
          {t('可售令牌')}
        </Tag>
      )}
      {!isSellableToken && isPackageEnabled && (
        <Tag color='blue' shape='circle' size='small'>
          {t('套餐令牌')}
        </Tag>
      )}
    </div>
  );
};

// 渲染操作列
const renderOperations = (
  text,
  record,
  onOpenLink,
  openTestModal,
  setEditingToken,
  setShowEdit,
  manageToken,
  refresh,
  t,
) => {
  let chatsArray = [];
  try {
    const raw = localStorage.getItem('chats');
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      for (let i = 0; i < parsed.length; i++) {
        const item = parsed[i];
        if (!item || typeof item !== 'object') continue;
        const name = Object.keys(item)[0];
        if (!name) continue;
        chatsArray.push({
          node: 'item',
          key: i,
          name,
          value: item[name],
          onClick: () => onOpenLink(name, item[name], record),
        });
      }
    }
  } catch (_) {
    showError(t('聊天链接配置错误，请联系管理员'));
  }
  if (!chatsArray.some((item) => item.value?.startsWith('ccswitch'))) {
    chatsArray.push({
      node: 'item',
      key: 'ccswitch',
      name: 'CC Switch',
      value: 'ccswitch',
      onClick: () => onOpenLink('CC Switch', 'ccswitch', record),
    });
  }

  return (
    <Space wrap>
      <SplitButtonGroup
        className='overflow-hidden'
        aria-label={t('项目操作按钮组')}
      >
        <Button
          size='small'
          type='tertiary'
          data-onboarding='token-chat-action'
          onClick={() => {
            if (chatsArray.length === 0) {
              showError(t('请联系管理员配置聊天链接'));
            } else {
              const first = chatsArray[0];
              onOpenLink(first.name, first.value, record);
            }
          }}
        >
          {t('聊天')}
        </Button>
        <Dropdown trigger='click' position='bottomRight' menu={chatsArray}>
          <Button
            type='tertiary'
            icon={<IconTreeTriangleDown />}
            size='small'
            data-onboarding='token-chat-dropdown'
          />
        </Dropdown>
      </SplitButtonGroup>

      <Button
        size='small'
        type='tertiary'
        data-onboarding='token-test-action'
        onClick={() => openTestModal(record)}
      >
        {t('测试')}
      </Button>

      {record.status === 1 ? (
        <Button
          type='danger'
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'disable', record);
            await refresh();
          }}
        >
          {t('禁用')}
        </Button>
      ) : (
        <Button
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'enable', record);
            await refresh();
          }}
        >
          {t('启用')}
        </Button>
      )}

      <Button
        type='tertiary'
        size='small'
        onClick={() => {
          setEditingToken(record);
          setShowEdit(true);
        }}
      >
        {t('编辑')}
      </Button>

      <Button
        type='danger'
        size='small'
        onClick={() => {
          if (record?.source_type === 'sellable_token') {
            Modal.confirm({
              title: t('确认删除？'),
              content: t('删除后不可恢复'),
              onOk: async () => {
                Modal.confirm({
                  title: t('二次确认'),
                  content: t(
                    '此操作不可逆，删除后令牌将永久失效且无法恢复。确定要继续吗？',
                  ),
                  okType: 'danger',
                  okText: t('确认删除'),
                  onOk: async () => {
                    await manageToken(record.id, 'delete', record);
                    await refresh();
                  },
                });
              },
            });
          } else {
            Modal.confirm({
              title: t('确定是否要删除此令牌？'),
              content: t('此修改将不可逆'),
              onOk: () => {
                (async () => {
                  await manageToken(record.id, 'delete', record);
                  await refresh();
                })();
              },
            });
          }
        }}
      >
        {t('删除')}
      </Button>
    </Space>
  );
};

export const getTokensColumns = ({
  t,
  showKeys,
  tokenFullKeys,
  toggleTokenKeyVisibility,
  copyTokenKey,
  manageToken,
  onOpenLink,
  openTestModal,
  setEditingToken,
  setShowEdit,
  refresh,
}) => {
  return [
    {
      title: t('名称'),
      dataIndex: 'name',
      render: (text, record) => renderTokenName(text, record, t),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      render: (text, record) => renderStatus(text, record, t),
    },
    {
      title: t('剩余额度/总额度'),
      key: 'quota_usage',
      render: (text, record) => renderQuotaUsage(text, record, t),
    },
    {
      title: t('已使用余额'),
      dataIndex: 'used_quota',
      key: 'used_quota',
      render: (text, record) => renderUsedQuota(text, record),
    },
    {
      title: t('周期余额'),
      key: 'package_cycle_balance',
      render: (text, record) => renderPackageCycleBalance(text, record, t),
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      key: 'group',
      render: (text, record) => renderGroupColumn(text, record, t),
    },
    {
      title: t('倍率'),
      dataIndex: 'group_ratio',
      key: 'group_ratio',
      render: (_text, record) => renderGroupRatio(record, t),
    },
    {
      title: t('密钥'),
      key: 'token_key',
      render: (text, record) =>
        renderTokenKey(
          text,
          record,
          showKeys,
          tokenFullKeys,
          toggleTokenKeyVisibility,
          copyTokenKey,
        ),
    },
    {
      title: t('可用模型'),
      dataIndex: 'model_limits',
      render: (text, record) => renderModelLimits(text, record, t),
    },
    {
      title: t('IP限制'),
      dataIndex: 'allow_ips',
      render: (text) => renderAllowIps(text, t),
    },
    {
      title: t('运行限制'),
      key: 'runtime_limits',
      render: (text, record) => renderRuntimeLimits(record, t),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_time',
      render: (text, record, index) => {
        return <div>{renderTimestamp(text)}</div>;
      },
    },
    {
      title: t('最后使用时间'),
      dataIndex: 'accessed_time',
      render: (text, record, index) => {
        return <div>{text ? renderTimestamp(text) : '-'}</div>;
      },
    },
    {
      title: t('过期时间'),
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        return (
          <div>
            {record.expired_time === -1 ? t('永不过期') : renderTimestamp(text)}
          </div>
        );
      },
    },
    {
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      render: (text, record, index) =>
        renderOperations(
          text,
          record,
          onOpenLink,
          openTestModal,
          setEditingToken,
          setShowEdit,
          manageToken,
          refresh,
          t,
        ),
    },
  ];
};
