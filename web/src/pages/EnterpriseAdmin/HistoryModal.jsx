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

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Modal, Table, Tabs } from '@douyinfe/semi-ui';
import {
  enterpriseRequest,
  formatEnterpriseTime,
  ENTERPRISE_AUDIT_ACTION,
  ENTERPRISE_LEDGER_KIND,
  ENTERPRISE_ROLE_LABEL,
} from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';

export default function HistoryModal({ enterprise, onClose }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState('usage');
  const [page, setPage] = useState(1);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const sequence = useRef(0);
  const load = useCallback(async () => {
    const id = ++sequence.current;
    setLoading(true);
    setError('');
    setItems([]);
    setTotal(0);
    try {
      const data = await enterpriseRequest(
        'get',
        `/api/enterprise/admin/enterprises/${enterprise.id}/${tab}`,
        undefined,
        {
          p: page,
          page_size: 20,
          ...(tab === 'usage' || tab === 'members' ? { status: 'all' } : {}),
        },
      );
      if (id === sequence.current) {
        setItems(data?.items || []);
        setTotal(data?.total || 0);
      }
    } catch (e) {
      if (id === sequence.current) setError(e.message);
    } finally {
      if (id === sequence.current) setLoading(false);
    }
  }, [enterprise.id, tab, page]);
  useEffect(() => {
    load();
    return () => {
      ++sequence.current;
    };
  }, [load]);
  const time = {
    title: t('时间'),
    dataIndex: 'created_at',
    render: (v) => formatEnterpriseTime(v, enterprise.timezone),
  };
  const amount = (v) => renderQuota(v, 8);
  const columns = {
    usage: [
      { title: t('请求 ID'), dataIndex: 'request_id' },
      { title: t('成员'), dataIndex: 'member_name' },
      { title: t('模型'), dataIndex: 'model_name' },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (v) =>
          t(
            {
              settled: '已结算',
              refunded: '已退回',
              review: '待对账',
              reserved: '进行中',
            }[v] || v,
          ),
      },
      { title: t('金额'), dataIndex: 'quota', render: amount },
      time,
    ],
    ledger: [
      {
        title: t('类型'),
        dataIndex: 'kind',
        render: (v) => t(ENTERPRISE_LEDGER_KIND[v] || v),
      },
      { title: t('成员'), dataIndex: 'member_name' },
      {
        title: `${t('未分配')} · ${t('变动')}`,
        dataIndex: 'unallocated_delta',
        render: amount,
      },
      {
        title: `${t('成员')} · ${t('变动')}`,
        dataIndex: 'member_delta',
        render: amount,
      },
      { title: t('原因'), dataIndex: 'reason' },
      time,
    ],
    audit: [
      {
        title: t('操作'),
        dataIndex: 'action',
        render: (v) => t(ENTERPRISE_AUDIT_ACTION[v] || v),
      },
      { title: t('操作人'), dataIndex: 'actor_name' },
      { title: t('原因'), dataIndex: 'reason' },
      time,
    ],
    members: [
      {
        title: t('成员'),
        dataIndex: 'display_name',
        render: (v, r) => v || r.username || `#${r.user_id}`,
      },
      {
        title: t('角色'),
        dataIndex: 'role',
        render: (v) => t(ENTERPRISE_ROLE_LABEL[v] || v),
      },
      { title: t('已消费'), dataIndex: 'consumed', render: amount },
      {
        title: t('加入时间'),
        dataIndex: 'joined_at',
        render: (v) => formatEnterpriseTime(v, enterprise.timezone),
      },
    ],
  };
  return (
    <Modal
      className='ec-modal ea-history-modal'
      visible
      title={`${t('历史记录')} · ${enterprise.name}`}
      width={1000}
      style={{ maxWidth: 'calc(100vw - 24px)' }}
      footer={null}
      onCancel={onClose}
      bodyStyle={{ maxHeight: 'calc(100dvh - 180px)', overflowY: 'auto' }}
    >
      {enterprise.status === 'archived' && (
        <div className='ea-archive-record'>
          <strong>
            {t('已归档')} ·{' '}
            {formatEnterpriseTime(enterprise.archived_at, enterprise.timezone)}
          </strong>
          <span>{enterprise.archive_reason}</span>
        </div>
      )}
      <Tabs
        activeKey={tab}
        onChange={(v) => {
          setTab(v);
          setPage(1);
        }}
      >
        <Tabs.TabPane itemKey='usage' tab={t('用量')} />
        <Tabs.TabPane itemKey='ledger' tab={t('资金流水')} />
        <Tabs.TabPane itemKey='audit' tab={t('操作记录')} />
        <Tabs.TabPane itemKey='members' tab={t('成员')} />
      </Tabs>
      {error ? (
        <div className='ea-load-error' role='alert'>
          {error}
          <Button onClick={load}>{t('重试')}</Button>
        </div>
      ) : (
        <Table
          rowKey='id'
          columns={columns[tab]}
          dataSource={items}
          loading={loading}
          empty={t('暂无数据')}
          scroll={{ x: 760 }}
          pagination={{
            currentPage: page,
            total,
            pageSize: 20,
            onPageChange: setPage,
          }}
        />
      )}
    </Modal>
  );
}
