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
import { useTranslation } from 'react-i18next';
import {
  Modal,
  Typography,
  Tag,
  Divider,
  Button,
  Space,
  Spin,
  Timeline,
  Empty,
} from '@douyinfe/semi-ui';
import TicketReply from './TicketReply';
import TicketMessageBody, { conversationMessages } from './TicketMessageBody';
import TicketAttachments from './TicketAttachments';
import { API } from '../../helpers/apiCore';
import { showError, showSuccess } from '../../helpers/toast';

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

const SENDER_ROLE_LABELS = {
  user: '用户',
  staff: '管理员',
};

function TicketDetail({ ticketId, onClose, onUpdate }) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [ticket, setTicket] = useState(null);
  const [messages, setMessages] = useState([]);
  const [closing, setClosing] = useState(false);

  const loadTicketDetail = async () => {
    try {
      setLoading(true);
      const res = await API.get(`/api/ticket/${ticketId}`);
      if (res.data.success) {
        setTicket(res.data.data.ticket);
        setMessages(conversationMessages(res.data.data.messages));
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

  const handleClose = async () => {
    try {
      setClosing(true);
      const res = await API.post(`/api/ticket/${ticketId}/close`);
      if (res.data.success) {
        showSuccess(t('工单已关闭'));
        await loadTicketDetail();
        onUpdate && onUpdate();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setClosing(false);
    }
  };

  if (loading) {
    return (
      <Modal
        title={t('工单详情')}
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
      title={t('工单详情')}
      visible={true}
      onCancel={onClose}
      footer={null}
      width={800}
      bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
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
            <Tag color={STATUS_COLORS[ticket.status]}>
              {t(STATUS_LABELS[ticket.status] || ticket.status)}
            </Tag>
          </div>
          <Space>
            <Text type='tertiary'>
              {t('工单号')}: #{ticket.id}
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
        </div>

        <Divider />

        <TicketAttachments
          ticketId={ticketId}
          mode='user'
          closed={ticket.status === 'closed'}
          onUpload={async () => {
            await loadTicketDetail();
            onUpdate?.();
          }}
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
                  type={msg.sender_role === 'staff' ? 'ongoing' : 'default'}
                >
                  <div style={{ marginBottom: 8 }}>
                    <Tag size='small'>
                      {t(
                        SENDER_ROLE_LABELS[msg.sender_role] || msg.sender_role,
                      )}
                    </Tag>
                  </div>
                  <TicketMessageBody message={msg} base='/api/ticket' />
                </Timeline.Item>
              ))}
            </Timeline>
          )}
        </div>

        {/* 回复区域 */}
        {canReply && (
          <>
            <Divider />
            <TicketReply
              key={ticketId}
              path={`/api/ticket/${ticketId}/reply`}
              onCloseTicket={handleClose}
              closing={closing}
              onSent={async () => {
                await loadTicketDetail();
                onUpdate?.();
              }}
            />
          </>
        )}

        {ticket.status === 'closed' && (
          <div style={{ textAlign: 'center', padding: 20 }}>
            <Text type='tertiary'>{t('工单已关闭，无法继续回复')}</Text>
          </div>
        )}
      </div>
    </Modal>
  );
}

export default TicketDetail;
