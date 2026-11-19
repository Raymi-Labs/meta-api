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
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Typography,
  Button,
  Space,
  Empty,
  Spin,
  Banner,
  List,
  Tag,
  Modal,
  Form,
} from '@douyinfe/semi-ui';
import TicketAttachments from './TicketAttachments';
import { API } from '../../helpers/apiCore';
import { showError, showSuccess } from '../../helpers/toast';
import TicketDetail from './TicketDetail';

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

function Appeal() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [session, setSession] = useState(null);
  const [tickets, setTickets] = useState([]);
  const [selectedTicketId, setSelectedTicketId] = useState(null);
  const [createModalVisible, setCreateModalVisible] = useState(false);

  const checkSession = async () => {
    try {
      const res = await API.get('/api/appeal/session', {
        skipErrorHandler: true,
      });
      if (res.data.success) {
        setSession(res.data.data);
        return true;
      } else {
        // 会话无效，跳转登录
        showError(t('申诉会话无效或已过期，请重新登录'));
        setTimeout(() => navigate('/login'), 2000);
        return false;
      }
    } catch (error) {
      showError(t('无法验证申诉会话，请重新登录'));
      setTimeout(() => navigate('/login'), 2000);
      return false;
    }
  };

  const loadTickets = async () => {
    try {
      const res = await API.get('/api/appeal/tickets', {
        skipErrorHandler: true,
      });
      if (res.data.success) {
        setTickets(res.data.data.tickets || []);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      // 会话过期
      if (error.response?.status === 401) {
        showError(t('申诉会话已过期，请重新登录'));
        setTimeout(() => navigate('/login'), 2000);
      } else {
        showError(error.message);
      }
    }
  };

  useEffect(() => {
    const init = async () => {
      setLoading(true);
      const valid = await checkSession();
      if (valid) {
        await loadTickets();
      }
      setLoading(false);
    };
    init();
  }, []);

  if (loading) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
        }}
      >
        <Spin size='large' />
      </div>
    );
  }

  if (!session) {
    return null;
  }

  const hasOpenTicket = tickets.some((t) => t.status !== 'closed');
  const expiresIn = Math.max(
    0,
    Math.floor((session.expires_at - Date.now() / 1000) / 60),
  );

  return (
    <div
      style={{
        minHeight: '100vh',
        background: 'var(--semi-color-bg-1)',
        padding: '40px 20px',
      }}
    >
      <div style={{ maxWidth: 800, margin: '0 auto' }}>
        <Card>
          <Title heading={2} style={{ textAlign: 'center', marginBottom: 24 }}>
            {t('账号申诉')}
          </Title>

          <Banner
            type='warning'
            description={
              <div>
                <Paragraph>
                  {t(
                    '您的账号当前处于封禁状态。您可以通过此页面提交申诉工单。',
                  )}
                </Paragraph>
                <Paragraph>
                  {t('申诉会话有效期')}: {expiresIn} {t('分钟')}
                </Paragraph>
                <Paragraph type='tertiary'>
                  {t('账号')}: {session.username || `#${session.user_id}`}
                </Paragraph>
              </div>
            }
            style={{ marginBottom: 24 }}
          />

          <div style={{ marginBottom: 24 }}>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 16,
              }}
            >
              <Title heading={4}>{t('我的申诉工单')}</Title>
              <Space>
                <Button onClick={() => navigate('/login')}>
                  {t('返回登录')}
                </Button>
                <Button
                  theme='solid'
                  type='primary'
                  onClick={() => setCreateModalVisible(true)}
                  disabled={hasOpenTicket}
                >
                  {t('创建申诉工单')}
                </Button>
              </Space>
            </div>

            {hasOpenTicket && (
              <Banner
                type='info'
                description={t('您已有未关闭的申诉工单，暂时无法创建新的申诉')}
                style={{ marginBottom: 16 }}
              />
            )}

            {tickets.length === 0 ? (
              <Empty description={t('暂无申诉工单')} />
            ) : (
              <List
                dataSource={tickets}
                renderItem={(ticket) => (
                  <List.Item
                    main={
                      <div>
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                            marginBottom: 8,
                          }}
                        >
                          <Text
                            link
                            onClick={() => setSelectedTicketId(ticket.id)}
                            strong
                          >
                            {ticket.subject}
                          </Text>
                          {ticket.user_unread && (
                            <Tag color='red' size='small'>
                              {t('新消息')}
                            </Tag>
                          )}
                        </div>
                        <Space>
                          <Tag color={STATUS_COLORS[ticket.status]}>
                            {t(STATUS_LABELS[ticket.status] || ticket.status)}
                          </Tag>
                          <Text type='tertiary' size='small'>
                            {new Date(ticket.created_at * 1000).toLocaleString(
                              'zh-CN',
                            )}
                          </Text>
                        </Space>
                      </div>
                    }
                  />
                )}
              />
            )}
          </div>
        </Card>

        <CreateAppealModal
          visible={createModalVisible}
          onClose={() => setCreateModalVisible(false)}
          onSuccess={(id) => {
            setSelectedTicketId(id);
            setCreateModalVisible(false);
            loadTickets();
          }}
        />

        {selectedTicketId && (
          <AppealTicketDetail
            ticketId={selectedTicketId}
            onClose={() => setSelectedTicketId(null)}
            onUpdate={() => loadTickets()}
          />
        )}
      </div>
    </div>
  );
}

function CreateAppealModal({ visible, onClose, onSuccess }) {
  const { t } = useTranslation();
  const form = useRef(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async () => {
    try {
      const values = await form.current?.validate();
      setSubmitting(true);
      const res = await API.post('/api/appeal/tickets', values, {
        skipErrorHandler: true,
      });
      if (res.data.success) {
        showSuccess(t('申诉工单创建成功'));
        form.current?.reset();
        onSuccess(res.data.data.id);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={t('创建申诉工单')}
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
        <Form.TextArea
          field='body'
          label={t('申诉内容')}
          rules={[{ required: true, message: t('请输入申诉内容') }]}
          maxLength={5000}
          rows={8}
          showClear
          placeholder={t('请详细说明您认为账号被封禁的原因以及您的申诉理由')}
        />
      </Form>
    </Modal>
  );
}

function AppealTicketDetail({ ticketId, onClose, onUpdate }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [ticket, setTicket] = useState(null);
  const [messages, setMessages] = useState([]);
  const replyForm = useRef(null);
  const [submitting, setSubmitting] = useState(false);

  const loadTicketDetail = async () => {
    try {
      setLoading(true);
      const res = await API.get(`/api/appeal/tickets/${ticketId}`, {
        skipErrorHandler: true,
      });
      if (res.data.success) {
        setTicket(res.data.data.ticket);
        setMessages(res.data.data.messages || []);
      } else {
        if (res.status === 401) {
          showError(t('申诉会话已过期'));
          setTimeout(() => navigate('/login'), 2000);
        } else {
          showError(res.data.message);
          onClose();
        }
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
      const res = await API.post(
        `/api/appeal/tickets/${ticketId}/reply`,
        values,
        {
          skipErrorHandler: true,
        },
      );
      if (res.data.success) {
        showSuccess(t('回复成功'));
        replyForm.current?.reset();
        await loadTicketDetail();
        onUpdate && onUpdate();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setSubmitting(false);
    }
  };

  // 复用 TicketDetail 组件的样式
  if (loading) {
    return (
      <Modal
        title={t('申诉详情')}
        visible={true}
        onCancel={onClose}
        footer={null}
        width={800}
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

  return (
    <Modal
      title={t('申诉详情')}
      visible={true}
      onCancel={onClose}
      footer={null}
      width={800}
      bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
    >
      <div>
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
            <Tag color={STATUS_COLORS[ticket.status]}>
              {t(STATUS_LABELS[ticket.status] || ticket.status)}
            </Tag>
          </div>
          <Text type='tertiary'>
            {t('创建时间')}:{' '}
            {new Date(ticket.created_at * 1000).toLocaleString('zh-CN')}
          </Text>
        </div>

        <TicketAttachments
          ticketId={ticketId}
          mode='appeal'
          closed={ticket.status === 'closed'}
        />
        {/* 消息列表 - 简化版 */}
        <div style={{ marginBottom: 24 }}>
          {messages.map((msg, idx) => (
            <div
              key={msg.id}
              style={{
                marginBottom: 16,
                padding: 16,
                background:
                  msg.sender_role === 'staff'
                    ? 'var(--semi-color-fill-0)'
                    : 'var(--semi-color-bg-2)',
                borderRadius: 8,
              }}
            >
              <div style={{ marginBottom: 8 }}>
                <Tag size='small'>
                  {msg.sender_role === 'staff' ? t('管理员') : t('我')}
                </Tag>
                <Text type='tertiary' size='small' style={{ marginLeft: 8 }}>
                  {new Date(msg.created_at * 1000).toLocaleString('zh-CN')}
                </Text>
              </div>
              <Paragraph style={{ whiteSpace: 'pre-wrap', margin: 0 }}>
                {msg.body}
              </Paragraph>
            </div>
          ))}
        </div>

        {/* 回复区域 */}
        {canReply && (
          <>
            <Form
              getFormApi={(api) => {
                replyForm.current = api;
              }}
            >
              <Form.TextArea
                field='body'
                label={t('回复内容')}
                rules={[{ required: true, message: t('请输入回复内容') }]}
                maxLength={5000}
                rows={4}
                showClear
              />
            </Form>
            <div
              style={{
                display: 'flex',
                justifyContent: 'flex-end',
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
            <Text type='tertiary'>{t('申诉工单已关闭')}</Text>
          </div>
        )}
      </div>
    </Modal>
  );
}

export default Appeal;
