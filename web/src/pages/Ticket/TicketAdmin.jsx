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

import React, { useEffect, useState, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Table,
  Button,
  Tag,
  Typography,
  Space,
  Select,
  Empty,
} from '@douyinfe/semi-ui';
import { API } from '../../helpers/apiCore';
import { showError } from '../../helpers/toast';
import { IconRefresh } from '@douyinfe/semi-icons';
import TicketAdminDetail from './TicketAdminDetail';

const { Title, Text } = Typography;

const STATUS_COLORS = {
  open: 'blue',
  in_progress: 'cyan',
  waiting_user: 'orange',
  resolved: 'green',
  closed: 'grey',
};

const STATUS_LABELS = {
  open: '待处理',
  in_progress: '处理中',
  waiting_user: '等待回复',
  resolved: '已解决',
  closed: '已关闭',
};

const CATEGORY_LABELS = {
  billing: '计费问题',
  api: '接口问题',
  account: '账号问题',
  invoice: '发票问题',
  enterprise: '企业功能',
  appeal: '申诉',
  other: '其他问题',
};

function TicketAdmin() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const [status, setStatus] = useState('');
  const [tickets, setTickets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [selectedTicketId, setSelectedTicketId] = useState(
    Number(params.get('id')) > 0 ? Number(params.get('id')) : null,
  );
  const [pagination, setPagination] = useState({
    currentPage: 1,
    pageSize: 20,
    total: 0,
  });

  const loadTickets = async (page = 1) => {
    try {
      setLoading(true);
      const res = await API.get('/api/admin/ticket', {
        params: {
          page,
          page_size: pagination.pageSize,
          status,
        },
      });
      if (res.data.success) {
        setTickets(res.data.data.tickets || []);
        setPagination({
          ...pagination,
          currentPage: page,
          total: res.data.data.total || 0,
        });
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTickets(1);
  }, [status]);

  const handlePageChange = (page) => {
    loadTickets(page);
  };

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: t('标题'),
      dataIndex: 'subject',
      render: (text, record) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text
            link
            onClick={() => setSelectedTicketId(record.id)}
            style={{ flex: 1 }}
          >
            {text}
          </Text>
          {record.staff_unread && (
            <Tag color='red' size='small'>
              {t('新消息')}
            </Tag>
          )}
        </div>
      ),
    },
    {
      title: t('用户ID'),
      dataIndex: 'user_id',
      width: 100,
    },
    {
      title: t('分类'),
      dataIndex: 'category',
      width: 120,
      render: (category) => {
        const isAppeal = category === 'appeal';
        return (
          <Tag color={isAppeal ? 'red' : 'blue'}>
            {t(CATEGORY_LABELS[category] || category)}
          </Tag>
        );
      },
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 120,
      render: (status) => (
        <Tag color={STATUS_COLORS[status] || 'grey'}>
          {t(STATUS_LABELS[status] || status)}
        </Tag>
      ),
    },
    {
      title: t('指派人'),
      dataIndex: 'assignee',
      width: 100,
      render: (assignee) => (assignee ? `#${assignee}` : '-'),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 180,
      render: (timestamp) => new Date(timestamp * 1000).toLocaleString('zh-CN'),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <Card>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 20,
          }}
        >
          <Title heading={3}>{t('工单管理')}</Title>
          <Space>
            <Select
              value={status}
              onChange={setStatus}
              style={{ width: 160 }}
              optionList={[
                { value: '', label: t('全部') },
                ...Object.entries(STATUS_LABELS).map(([value, label]) => ({
                  value,
                  label: t(label),
                })),
              ]}
            />
            <Button
              icon={<IconRefresh />}
              onClick={() => loadTickets(pagination.currentPage)}
            >
              {t('刷新')}
            </Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={tickets}
          pagination={{
            currentPage: pagination.currentPage,
            pageSize: pagination.pageSize,
            total: pagination.total,
            onPageChange: handlePageChange,
          }}
          loading={loading}
          empty={<Empty description={t('暂无工单')} />}
        />
      </Card>

      {selectedTicketId && (
        <TicketAdminDetail
          ticketId={selectedTicketId}
          onClose={() => setSelectedTicketId(null)}
          onUpdate={() => loadTickets(pagination.currentPage)}
        />
      )}
    </div>
  );
}

export default TicketAdmin;
