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
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Table,
  Button,
  Tag,
  Modal,
  Form,
  Select,
  TextArea,
  Input,
  Typography,
  Space,
  Empty,
  Spin,
} from '@douyinfe/semi-ui';
import { API } from '../../helpers/apiCore';
import { showError, showSuccess } from '../../helpers/toast';
import { IconPlus, IconRefresh } from '@douyinfe/semi-icons';
import TicketDetail from './TicketDetail';

const { Title, Text } = Typography;

const TICKET_CATEGORIES = [
  { value: 'billing', label: '计费问题' },
  { value: 'api', label: '接口问题' },
  { value: 'account', label: '账号问题' },
  { value: 'invoice', label: '发票问题' },
  { value: 'enterprise', label: '企业功能' },
  { value: 'other', label: '其他问题' },
];

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

function Ticket() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [tickets, setTickets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [selectedTicketId, setSelectedTicketId] = useState(null);
  const [pagination, setPagination] = useState({
    currentPage: 1,
    pageSize: 20,
    total: 0,
  });

  // 检查是否从其他页面传入的 requestId
  useEffect(() => {
    const id = Number(searchParams.get('id'));
    if (Number.isSafeInteger(id) && id > 0) setSelectedTicketId(id);
    const shouldCreate = searchParams.get('create');
    if (shouldCreate === '1') {
      setCreateModalVisible(true);
    }
  }, [searchParams]);

  const loadTickets = async (page = 1) => {
    try {
      setLoading(true);
      const res = await API.get('/api/ticket', {
        params: {
          page,
          page_size: pagination.pageSize,
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
    loadTickets();
  }, []);

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
          {record.user_unread && (
            <Tag color='red' size='small'>
              {t('新消息')}
            </Tag>
          )}
        </div>
      ),
    },
    {
      title: t('分类'),
      dataIndex: 'category',
      width: 120,
      render: (category) => {
        const cat = TICKET_CATEGORIES.find((c) => c.value === category);
        return cat ? cat.label : category;
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
          <Title heading={3}>{t('我的工单')}</Title>
          <Space>
            <Button
              icon={<IconRefresh />}
              onClick={() => loadTickets(pagination.currentPage)}
            >
              {t('刷新')}
            </Button>
            <Button
              theme='solid'
              type='primary'
              icon={<IconPlus />}
              onClick={() => setCreateModalVisible(true)}
            >
              {t('创建工单')}
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

      <CreateTicketModal
        visible={createModalVisible}
        onClose={() => setCreateModalVisible(false)}
        onSuccess={(id) => {
          setSelectedTicketId(id);
          setCreateModalVisible(false);
          loadTickets(1);
        }}
        initialRequestId={searchParams.get('request_id')}
      />

      {selectedTicketId && (
        <TicketDetail
          ticketId={selectedTicketId}
          onClose={() => setSelectedTicketId(null)}
          onUpdate={() => loadTickets(pagination.currentPage)}
        />
      )}
    </div>
  );
}

function CreateTicketModal({
  visible,
  onClose,
  onSuccess,
  initialRequestId = null,
}) {
  const { t } = useTranslation();
  const form = useRef(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (visible && initialRequestId) {
      form.current?.setValues({ request_id: initialRequestId });
    }
  }, [visible, initialRequestId]);

  const handleSubmit = async () => {
    try {
      const values = await form.current?.validate();
      setSubmitting(true);
      const res = await API.post('/api/ticket', values);
      if (res.data.success) {
        showSuccess(t('工单创建成功'));
        form.current?.reset();
        onSuccess(res.data.data.id);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      if (error.message) {
        showError(error.message);
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={t('创建工单')}
      visible={visible}
      onCancel={onClose}
      onOk={handleSubmit}
      confirmLoading={submitting}
      width={600}
    >
      <Form
        getFormApi={(api) => {
          form.current = api;
        }}
        labelPosition='left'
        labelWidth={100}
      >
        <Form.Input
          field='subject'
          label={t('标题')}
          rules={[{ required: true, message: t('请输入标题') }]}
          maxLength={100}
          showClear
        />
        <Form.Select
          field='category'
          label={t('分类')}
          rules={[{ required: true, message: t('请选择分类') }]}
          optionList={TICKET_CATEGORIES.map((o) => ({
            ...o,
            label: t(o.label),
          }))}
        />
        <Form.TextArea
          field='body'
          label={t('详细描述')}
          rules={[{ required: true, message: t('请输入详细描述') }]}
          maxLength={5000}
          rows={6}
          showClear
        />
        <Form.Input
          field='request_id'
          label={t('请求ID')}
          placeholder={t('选填，关联具体请求')}
          maxLength={64}
          showClear
        />
      </Form>
    </Modal>
  );
}

export default Ticket;
