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

import React, { useEffect, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Pagination,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';

const statusLabels = {
  pending: '待发送',
  sending: '发送处理中',
  submitted: '已提交通知服务',
  unknown: '发送结果未知',
  failed: '发送失败',
  retry_wait: '失败，等待重试',
  skipped: '已跳过',
  suppressed: '已达到滚动24小时上限',
};
const reasonLabels = {
  no_destination: '未设置通知地址',
  notifications_disabled: '通知已关闭或用户已停用',
  inactive_or_recovered: '近期未持续失败或调用已恢复',
  sender_interrupted: '发送过程被中断，避免重复发送',
  submission_outcome_unknown: '未取得最终确认，避免重复发送',
  notification_service_failed: '通知服务返回失败',
  user_lookup_failed: '暂时无法读取通知设置',
  invalid_summary: '通知内容读取失败',
  unsupported_medium: '通知方式不受支持',
};

export default function QuotaNoticeHistory({ userID }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [refresh, setRefresh] = useState(0);
  const [rows, setRows] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const endpoint = userID
    ? `/api/user/${userID}/quota-notices`
    : '/api/user/quota-notices';

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    API.get(endpoint, { params: { p: page } })
      .then(({ data }) => {
        if (cancelled) return;
        if (!data.success) throw new Error('quota-notice-history');
        setRows(data.data.items || []);
        setTotal(data.data.total || 0);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [endpoint, page, refresh]);

  return (
    <Card className='my-4' data-testid='quota-notice-history'>
      <div className='flex items-center justify-between gap-3 mb-2'>
        <Typography.Text strong>{t('调用受阻通知记录')}</Typography.Text>
        <Button
          size='small'
          htmlType='button'
          onClick={() => setRefresh((v) => v + 1)}
          loading={loading}
        >
          {t('刷新')}
        </Button>
      </div>
      <Typography.Paragraph type='secondary'>
        {t(
          '仅展示额度不足持续调用的提醒，保留90天。已提交通知服务不代表收件人已收到。',
        )}
      </Typography.Paragraph>
      {error && (
        <Typography.Paragraph type='danger' role='alert'>
          {t('通知记录加载失败，请重试')}
        </Typography.Paragraph>
      )}
      <Spin spinning={loading}>
        {!error && !loading && rows.length === 0 && (
          <Empty description={t('暂无通知记录')} />
        )}
        {rows.map((row) => {
          let resources = [];
          try {
            const parsed = JSON.parse(row.summary || '[]');
            resources = Array.isArray(parsed) ? parsed : [];
          } catch {
            /* The status remains useful without details. */
          }
          const failures = resources.reduce(
            (sum, item) => sum + (item.count || 0),
            0,
          );
          return (
            <div
              key={row.id}
              className='py-3 border-b border-[var(--semi-color-border)] break-words min-w-0'
            >
              <div className='flex flex-wrap items-center gap-2'>
                <Typography.Text>
                  #{row.id} · {new Date(row.created_at).toLocaleString()}
                </Typography.Text>
                <Tag
                  color={
                    row.status === 'submitted'
                      ? 'green'
                      : row.status === 'unknown'
                        ? 'orange'
                        : 'grey'
                  }
                >
                  {t(statusLabels[row.status] || '发送结果未知')}
                </Tag>
              </div>
              <div className='text-sm mt-2 text-[var(--semi-color-text-1)]'>
                {row.medium && `${row.medium} · ${row.recipient || '—'} · `}
                {t('发送尝试 {{count}} 次', { count: row.attempts })}
                {failures > 0 &&
                  ` · ${t('累计失败 {{count}} 次', { count: failures })}`}
              </div>
              {reasonLabels[row.reason] && (
                <Typography.Text type='secondary'>
                  {t(reasonLabels[row.reason])}
                </Typography.Text>
              )}
              {row.next_attempt_at > Date.now() && (
                <div className='text-sm'>
                  {t('下次检查：{{time}}', {
                    time: new Date(row.next_attempt_at).toLocaleString(),
                  })}
                </div>
              )}
              {resources.slice(0, 5).map((item) => (
                <div key={item.scope} className='text-xs mt-1 break-all'>
                  {item.token_name || '—'} · {item.model_name || '—'} · Request
                  ID: {item.request_id}
                </div>
              ))}
            </div>
          );
        })}
      </Spin>
      {total > 10 && (
        <div className='mt-4 overflow-x-auto'>
          <Pagination
            currentPage={page}
            pageSize={10}
            total={total}
            onPageChange={setPage}
            size='small'
          />
        </div>
      )}
    </Card>
  );
}
