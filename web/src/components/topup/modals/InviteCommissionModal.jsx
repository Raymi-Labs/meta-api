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
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Button, Card, Modal, Space, Tag, Typography } from '@douyinfe/semi-ui';
import {
  API,
  getPaymentCurrencySymbol,
  renderQuota,
  showError,
} from '../../../helpers';
import CardTable from '../../common/ui/CardTable';
import { useVisibleRefresh } from '../../../hooks/common/useVisibleRefresh';

const { Text } = Typography;

const renderRechargeMoney = (value) => {
  const amount = Number(value);
  return `${getPaymentCurrencySymbol()}${Number.isFinite(amount) ? amount.toFixed(2) : '0.00'}`;
};

const initialData = {
  invitees: {
    items: [],
    total: 0,
    page: 1,
    page_size: 10,
  },
  summary: {
    recharge_total_quota: 0,
  },
};

const InviteCommissionModal = ({ visible, onCancel, t }) => {
  const [loading, setLoading] = useState(false);
  const [refreshError, setRefreshError] = useState(false);
  const [data, setData] = useState(initialData);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const latestRequestRef = useRef(0);

  const loadData = async (
    targetPage = page,
    targetPageSize = pageSize,
    silent = false,
  ) => {
    if (silent && loading) return;
    const requestId = latestRequestRef.current + 1;
    latestRequestRef.current = requestId;
    if (!silent) setLoading(true);
    try {
      const res = await API.get('/api/user/invite-recharge-commissions/self', {
        params: {
          p: targetPage,
          page_size: targetPageSize,
        },
      });
      const { success, message, data: payload } = res.data || {};
      if (latestRequestRef.current !== requestId) {
        return;
      }
      if (!success) {
        setRefreshError(true);
        if (!silent) showError(message || t('加载失败'));
        return;
      }
      setRefreshError(false);
      setData({
        invitees: payload?.invitees || initialData.invitees,
        summary: payload?.summary || initialData.summary,
      });
    } catch (error) {
      if (latestRequestRef.current === requestId) {
        setRefreshError(true);
        if (!silent) showError(t('请求失败'));
      }
    } finally {
      if (latestRequestRef.current === requestId) {
        setLoading(false);
      }
    }
  };

  useVisibleRefresh(() => loadData(page, pageSize, true), visible);

  useEffect(() => {
    if (!visible) {
      latestRequestRef.current += 1;
      setData(initialData);
      setRefreshError(false);
      setLoading(false);
      return;
    }
    setPage(1);
    setPageSize(10);
    loadData(1, 10);
  }, [visible]);

  const columns = useMemo(
    () => [
      {
        title: t('用户'),
        dataIndex: 'alias',
        render: (alias) => alias || '-',
      },
      {
        title: t('注册时间'),
        dataIndex: 'registered_date',
        render: (registeredDate) => registeredDate || '-',
      },
      {
        title: t('返佣层级'),
        dataIndex: 'commission_level',
        render: (level) => (
          <Tag color={Number(level) === 2 ? 'violet' : 'blue'} shape='circle'>
            {Number(level) === 2 ? t('二级') : t('一级')}
          </Tag>
        ),
      },
      {
        title: t('充值金额'),
        dataIndex: 'recharge_total_money',
        render: (money) => renderRechargeMoney(money),
      },
      {
        title: t('充值返佣'),
        dataIndex: 'recharge_commission_quota',
        render: (quota) => (
          <Tag color='white' shape='circle'>
            {renderQuota(quota || 0)}
          </Tag>
        ),
      },
    ],
    [t],
  );

  const invitees = (data?.invitees?.items || []).map((item, index) => ({
    ...item,
    key: item?.alias || index,
  }));
  const inviteesTotal = Number(data?.invitees?.total || 0);

  return (
    <Modal
      title={t('邀请返佣明细')}
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width='min(860px, calc(100vw - 24px))'
      maskClosable={false}
    >
      <div className='space-y-3'>
        {refreshError && (
          <Space>
            <Text type='danger'>{t('加载失败')}</Text>
            <Button onClick={() => loadData(page, pageSize)}>
              {t('重试')}
            </Button>
          </Space>
        )}
        <Card className='!rounded-2xl shadow-sm border-0'>
          <Space wrap>
            <Tag color='white' shape='circle'>
              {t('返佣来源项数')}：{inviteesTotal}
            </Tag>
            <Tag color='white' shape='circle'>
              {t('充值返佣汇总')}：
              {renderQuota(data?.summary?.recharge_total_quota || 0)}
            </Tag>
          </Space>
          <div className='mt-2'>
            <Text type='tertiary' size='small'>
              {t(
                '一级按匿名用户、二级按直接下级分支汇总已结算返佣；不展示下级的下级账号和逐人注册信息，支付成功后自动结算，页面定期刷新。',
              )}
            </Text>
          </div>
        </Card>

        <CardTable
          columns={columns}
          dataSource={invitees}
          loading={loading}
          rowKey='key'
          pagination={{
            currentPage: page,
            pageSize: pageSize,
            total: inviteesTotal,
            pageSizeOpts: [10, 20, 50, 100],
            showSizeChanger: true,
            onPageChange: (nextPage) => {
              setPage(nextPage);
              loadData(nextPage, pageSize);
            },
            onPageSizeChange: (nextPageSize) => {
              setPage(1);
              setPageSize(nextPageSize);
              loadData(1, nextPageSize);
            },
          }}
          hidePagination={false}
        />
      </div>
    </Modal>
  );
};

export default InviteCommissionModal;
