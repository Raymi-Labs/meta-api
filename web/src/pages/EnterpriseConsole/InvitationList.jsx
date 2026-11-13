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

import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Modal, Table, Tag } from '@douyinfe/semi-ui';
import {
  ENTERPRISE_ROLE_LABEL,
  enterpriseRequest,
  formatEnterpriseDate,
} from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { showError, showSuccess } from '../../helpers/toast';

const STATUS = {
  pending: '待接受',
  accepted: '已接受',
  revoked: '已撤销',
  expired: '已过期',
};
const COLOR = {
  pending: 'orange',
  accepted: 'green',
  revoked: 'grey',
  expired: 'grey',
};

// 待接受但已过有效期的邀请，按已过期展示。
const effective = (r) =>
  r.status === 'pending' && r.expires_at && r.expires_at * 1000 < Date.now()
    ? 'expired'
    : r.status;

export default function InvitationList({ enterpriseId, version }) {
  const { t } = useTranslation();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const base = `/api/enterprises/${enterpriseId}/invitations`;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await enterpriseRequest('get', base, undefined, {
        p: page,
        page_size: 10,
      });
      setItems(data?.items || []);
      setTotal(data?.total || 0);
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  }, [base, page]);

  useEffect(() => {
    load();
  }, [load, version]);

  const revoke = (r) => {
    Modal.confirm({
      className: 'ec-modal',
      title: t('撤销邀请 {{email}}？', { email: r.email }),
      content: t('撤销后邀请链接立即失效。'),
      okText: t('撤销'),
      okButtonProps: { type: 'danger' },
      onOk: async () => {
        try {
          await enterpriseRequest('post', `${base}/${r.id}/revoke`, {});
          showSuccess(t('已撤销'));
          load();
        } catch (e) {
          showError(e.message);
        }
      },
    });
  };

  if (total === 0) return null;

  const columns = [
    { title: t('邮箱'), dataIndex: 'email' },
    {
      title: t('角色'),
      dataIndex: 'role',
      render: (v) => t(ENTERPRISE_ROLE_LABEL[v] || v),
    },
    {
      title: t('初始额度'),
      dataIndex: 'initial_quota',
      align: 'right',
      render: (v) => <span className='em-num'>{renderQuota(v)}</span>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (_, r) => {
        const s = effective(r);
        return <Tag color={COLOR[s] || 'grey'}>{t(STATUS[s] || s)}</Tag>;
      },
    },
    {
      title: t('有效期至'),
      dataIndex: 'expires_at',
      render: (v) => formatEnterpriseDate(v),
    },
    {
      title: '',
      dataIndex: 'id',
      align: 'right',
      render: (_, r) =>
        effective(r) === 'pending' ? (
          <Button
            size='small'
            theme='borderless'
            type='danger'
            onClick={() => revoke(r)}
          >
            {t('撤销')}
          </Button>
        ) : null,
    },
  ];

  return (
    <section className='em-section'>
      <h2 className='em-sub'>{t('邀请记录')}</h2>
      <div className='em-card'>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={items}
          loading={loading}
          pagination={
            total > 10
              ? {
                  currentPage: page,
                  pageSize: 10,
                  total,
                  onPageChange: setPage,
                }
              : false
          }
        />
      </div>
    </section>
  );
}
