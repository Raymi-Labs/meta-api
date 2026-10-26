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
import { Button, Table, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, formatDateTimeString } from '../../../helpers';
import { fetchAnnouncementEmailJobs } from '../../../helpers/announcementEmail';

const { Text } = Typography;

export default function AnnouncementEmailHistory({ refreshKey }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [reload, setReload] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState({ items: [], total: 0 });

  useEffect(() => {
    if (!refreshKey) return;
    setPage(1);
    setReload((value) => value + 1);
  }, [refreshKey]);

  useEffect(() => {
    let cancelled = false;
    let timer;
    setLoading(true);
    setError('');
    fetchAnnouncementEmailJobs(API, page, 10, t('加载失败'))
      .then((data) => {
        if (cancelled) return;
        setResult(data);
        if (data.items.some((job) => job.pending > 0 || job.sending > 0)) {
          timer = setTimeout(() => setReload((value) => value + 1), 5000);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err.message || t('加载失败'));
          setResult({ items: [], total: 0 });
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [page, reload, t]);

  const countColumn = (field, label) => ({
    title: label,
    dataIndex: field,
    key: field,
    width: 100,
  });
  const columns = [
    {
      title: t('公告内容'),
      dataIndex: 'content',
      key: 'content',
      width: 260,
      render: (content) => (
        <div style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
          {content.length > 90 ? `${content.slice(0, 90)}…` : content}
        </div>
      ),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (value) => formatDateTimeString(new Date(value * 1000)),
    },
    countColumn('total', t('总计')),
    countColumn('pending', t('待发送')),
    countColumn('sending', t('发送中')),
    countColumn('submitted', t('已提交邮件服务器')),
    countColumn('failed', t('失败')),
    countColumn('unknown', t('发送结果未知')),
    countColumn('skipped', t('已跳过')),
  ];

  return (
    <section
      className='mt-6'
      aria-label={t('公告邮件记录')}
      data-testid='announcement-email-history'
    >
      <div className='flex items-center justify-between gap-3 mb-2'>
        <Text strong>{t('公告邮件记录')}</Text>
        <Button
          loading={loading}
          onClick={() => setReload((value) => value + 1)}
        >
          {t('刷新')}
        </Button>
      </div>
      <p className='text-sm text-[var(--semi-color-text-2)] mb-3'>
        {t(
          '已提交表示邮件服务器已接受，不代表收件人已收到。结果未知的邮件不会自动重发，以免重复通知。',
        )}
      </p>
      {error && (
        <p role='alert' className='text-red-500 mb-3'>
          {error}
        </p>
      )}
      <Table
        rowKey='id'
        columns={columns}
        dataSource={result.items}
        loading={loading}
        empty={t('暂无记录')}
        scroll={{ x: 'max-content' }}
        size='small'
        pagination={{
          currentPage: page,
          pageSize: 10,
          total: result.total,
          onChange: setPage,
        }}
      />
    </section>
  );
}
