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
import { useTranslation } from 'react-i18next';
import {
  Modal,
  Typography,
  Tag,
  Divider,
  Button,
  Space,
  Form,
  TextArea,
  Spin,
  Timeline,
  Empty,
  Select,
  Checkbox,
  Collapse,
  Table,
} from '@douyinfe/semi-ui';
import { TICKET_QUEUE_CHANGED } from '../../hooks/common/useNotifications';
import TicketAttachments from './TicketAttachments';
import { API } from '../../helpers/apiCore';
import { showError, showSuccess } from '../../helpers/toast';

const { Title, Text, Paragraph } = Typography;

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

const STATUS_OPTIONS = [
  { value: 'open', label: '待处理' },
  { value: 'in_progress', label: '处理中' },
  { value: 'waiting_user', label: '等待回复' },
  { value: 'resolved', label: '已解决' },
  { value: 'closed', label: '已关闭' },
];

const CATEGORY_LABELS = {
  billing: '计费问题',
  api: '接口问题',
  account: '账号问题',
  invoice: '发票问题',
  enterprise: '企业功能',
  appeal: '申诉',
  other: '其他问题',
};

const SENDER_ROLE_LABELS = {
  user: '用户',
  staff: '管理员',
  system: '系统',
};

function TicketAdminDetail({ ticketId, onClose, onUpdate }) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [ticket, setTicket] = useState(null);
  const [messages, setMessages] = useState([]);
  const [logs, setLogs] = useState([]);
  const replyForm = useRef(null);
  const [submitting, setSubmitting] = useState(false);
  const [statusUpdating, setStatusUpdating] = useState(false);

  const loadTicketDetail = async () => {
    try {
      setLoading(true);
      const res = await API.get(`/api/admin/ticket/${ticketId}`);
      if (res.data.success) {
        setTicket(res.data.data.ticket);
        setMessages(res.data.data.messages || []);
        setLogs(res.data.data.logs || []);
      } else {
        showError(res.data.message);
        onClose();
      }
    } catch (error) {
      showError(error.message);
      onClose();
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (ticketId) {
      loadTicketDetail();
    }
  }, [ticketId]);

  const handleReply = async () => {
    try {
      const values = await replyForm.current?.validate();
      setSubmitting(true);
      const res = await API.post(`/api/admin/ticket/${ticketId}/reply`, values);
      if (res.data.success) {
        window.dispatchEvent(new Event(TICKET_QUEUE_CHANGED));
        showSuccess(t('回复成功'));
        replyForm.current?.reset();
        await loadTicketDetail();
        onUpdate && onUpdate();
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

  const handleStatusChange = async (newStatus) => {
    try {
      setStatusUpdating(true);
      const res = await API.put(`/api/admin/ticket/${ticketId}/status`, {
        status: newStatus,
      });
      if (res.data.success) {
        window.dispatchEvent(new Event(TICKET_QUEUE_CHANGED));
        showSuccess(t('状态更新成功'));
        await loadTicketDetail();
        onUpdate && onUpdate();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setStatusUpdating(false);
    }
  };

  const handleAssignToMe = async () => {
    try {
      // 获取当前用户ID
      const userStr = localStorage.getItem('user');
      if (!userStr) {
        showError(t('无法获取当前用户信息'));
        return;
      }
      const user = JSON.parse(userStr);
      const res = await API.put(`/api/admin/ticket/${ticketId}/assign`, {
        assignee_id: user.id,
      });
      if (res.data.success) {
        showSuccess(t('指派成功'));
        await loadTicketDetail();
        onUpdate && onUpdate();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error.message);
    }
  };

  if (loading) {
    return (
      <Modal
        title={t('工单详情')}
        visible={true}
        onCancel={onClose}
        footer={null}
        width={900}
      >
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Spin size='large' />
        </div>
      </Modal>
    );
  }

  if (!ticket) {
    return null;
  }

  const canReply = ticket.status !== 'closed';

  const logColumns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      width: 180,
      render: (timestamp) => new Date(timestamp * 1000).toLocaleString('zh-CN'),
    },
    {
      title: t('请求ID'),
      dataIndex: 'request_id',
      width: 200,
    },
    {
      title: t('模型'),
      dataIndex: 'model_name',
      width: 150,
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 100,
    },
    {
      title: t('状态码'),
      dataIndex: 'status_code',
      width: 100,
    },
  ];

  return (
    <Modal
      title={t('工单详情（管理）')}
      visible={true}
      onCancel={onClose}
      footer={null}
      width={900}
      bodyStyle={{ maxHeight: '75vh', overflow: 'auto' }}
    >
      <div>
        {/* 工单头部信息 */}
        <div style={{ marginBottom: 20 }}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: 12,
            }}
          >
            <Title heading={4} style={{ margin: 0 }}>
              {ticket.subject}
            </Title>
            <Space>
              <Tag color={STATUS_COLORS[ticket.status]}>
                {t(STATUS_LABELS[ticket.status] || ticket.status)}
              </Tag>
              {ticket.category === 'appeal' && (
                <Tag color='red'>{t('申诉工单')}</Tag>
              )}
            </Space>
          </div>
          <Space wrap>
            <Text type='tertiary'>
              {t('工单号')}: #{ticket.id}
            </Text>
            <Divider layout='vertical' />
            <Text type='tertiary'>
              {t('用户ID')}: {ticket.user_id}
            </Text>
            <Divider layout='vertical' />
            <Text type='tertiary'>
              {t('分类')}:{' '}
              {t(CATEGORY_LABELS[ticket.category] || ticket.category)}
            </Text>
            <Divider layout='vertical' />
            <Text type='tertiary'>
              {t('创建时间')}:{' '}
              {new Date(ticket.created_at * 1000).toLocaleString('zh-CN')}
            </Text>
          </Space>
          {ticket.request_id && (
            <div style={{ marginTop: 8 }}>
              <Text type='tertiary'>
                {t('关联请求ID')}: {ticket.request_id}
              </Text>
            </div>
          )}
          {ticket.assignee && (
            <div style={{ marginTop: 8 }}>
              <Text type='tertiary'>
                {t('指派给')}: #{ticket.assignee}
              </Text>
            </div>
          )}
        </div>

        {/* 管理操作 */}
        <div style={{ marginBottom: 20 }}>
          <Space wrap>
            <Select
              value={ticket.status}
              onChange={handleStatusChange}
              disabled={statusUpdating}
              style={{ width: 150 }}
              optionList={STATUS_OPTIONS.map((o) => ({
                ...o,
                label: t(o.label),
              }))}
            />
            <Button type='tertiary' onClick={handleAssignToMe}>
              {t('指派给我')}
            </Button>
          </Space>
        </div>

        <Divider />

        {/* 关联日志 */}
        {logs && logs.length > 0 && (
          <>
            <Collapse>
              <Collapse.Panel
                header={t('关联请求日志') + ` (${logs.length})`}
                itemKey='logs'
              >
                <Table
                  columns={logColumns}
                  dataSource={logs}
                  pagination={false}
                  size='small'
                />
              </Collapse.Panel>
            </Collapse>
            <Divider />
          </>
        )}

        <TicketAttachments
          ticketId={ticketId}
          mode='admin'
          closed={ticket.status === 'closed'}
        />
        {/* 消息列表 */}
        <div style={{ marginBottom: 24 }}>
          {messages.length === 0 ? (
            <Empty description={t('暂无消息')} />
          ) : (
            <Timeline>
              {messages.map((msg) => (
                <Timeline.Item
                  key={msg.id}
                  time={new Date(msg.created_at * 1000).toLocaleString('zh-CN')}
                  type={
                    msg.sender_role === 'staff'
                      ? 'ongoing'
                      : msg.sender_role === 'system'
                        ? 'success'
                        : 'default'
                  }
                >
                  <div style={{ marginBottom: 8 }}>
                    <Tag size='small'>
                      {t(
                        SENDER_ROLE_LABELS[msg.sender_role] || msg.sender_role,
                      )}
                    </Tag>
                    {msg.sender_id && (
                      <Text
                        type='tertiary'
                        size='small'
                        style={{ marginLeft: 8 }}
                      >
                        #{msg.sender_id}
                      </Text>
                    )}
                    {msg.is_internal && (
                      <Tag
                        color='purple'
                        size='small'
                        style={{ marginLeft: 8 }}
                      >
                        {t('内部备注')}
                      </Tag>
                    )}
                    {msg.event && (
                      <Text
                        type='tertiary'
                        size='small'
                        style={{ marginLeft: 8 }}
                      >
                        [{msg.event}]
                      </Text>
                    )}
                  </div>
                  <Paragraph style={{ whiteSpace: 'pre-wrap' }}>
                    {msg.body}
                  </Paragraph>
                </Timeline.Item>
              ))}
            </Timeline>
          )}
        </div>

        {/* 回复区域 */}
        {canReply && (
          <>
            <Divider />
            <Form
              getFormApi={(api) => {
                replyForm.current = api;
              }}
              labelPosition='top'
            >
              <Form.TextArea
                field='body'
                label={t('回复内容')}
                rules={[{ required: true, message: t('请输入回复内容') }]}
                maxLength={5000}
                rows={4}
                showClear
              />
              <Form.Checkbox field='is_internal' noLabel>
                {t('内部备注（用户不可见）')}
              </Form.Checkbox>
            </Form>
            <div
              style={{
                display: 'flex',
                justifyContent: 'flex-end',
                gap: 8,
                marginTop: 16,
              }}
            >
              <Button
                theme='solid'
                type='primary'
                onClick={handleReply}
                loading={submitting}
              >
                {t('发送回复')}
              </Button>
            </div>
          </>
        )}

        {ticket.status === 'closed' && (
          <div style={{ textAlign: 'center', padding: 20 }}>
            <Text type='tertiary'>{t('工单已关闭')}</Text>
          </div>
        )}
      </div>
    </Modal>
  );
}

export default TicketAdminDetail;
