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

import React, { useEffect, useState, useContext, useMemo, useRef } from 'react';
import {
  Avatar,
  Button,
  Modal,
  Empty,
  Tabs,
  TabPane,
  Timeline,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { API } from '../../helpers/apiCore';
import { getRelativeTime } from '../../helpers/date';
import { renderQuota, stringToColor } from '../../helpers/dashboardFormat';
import { showError } from '../../helpers/toast';
import {
  IllustrationNoContent,
  IllustrationNoContentDark,
} from '@douyinfe/semi-illustrations';
import { StatusContext } from '../../context/Status';
import { Bell, FileText, Megaphone, Wallet } from 'lucide-react';
import RichContent from '../common/RichContent';
import {
  getAnnouncementKey,
  muteNoticeToday,
  normalizeAnnouncements,
} from '../../helpers/noticeReadState';

const { Text } = Typography;

const NoticeModal = ({
  visible,
  onClose,
  isMobile,
  defaultTab = 'inApp',
  unreadKeys = [],
  pendingWithdrawalCount = 0,
  pendingWithdrawals = [],
  pendingInvoiceCount = 0,
  pendingInvoices = [],
  showWithdrawalTab = false,
  showInvoiceTab = false,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [noticeContent, setNoticeContent] = useState('');
  const [loading, setLoading] = useState(false);
  const [noticeLoaded, setNoticeLoaded] = useState(false);
  const [activeTab, setActiveTab] = useState(defaultTab);
  const viewed = useRef({ announcementKeys: new Set(), content: '' });
  const closed = useRef(false);

  const [statusState] = useContext(StatusContext);

  const announcements = useMemo(
    () =>
      statusState?.status?.announcements_enabled === false
        ? []
        : normalizeAnnouncements(statusState?.status?.announcements),
    [
      statusState?.status?.announcements,
      statusState?.status?.announcements_enabled,
    ],
  );

  const unreadSet = useMemo(() => new Set(unreadKeys), [unreadKeys]);

  const formatBadgeCount = (count) => (count > 99 ? '99+' : count);

  const processedAnnouncements = useMemo(() => {
    return (announcements || []).slice(0, 20).map((item) => {
      const pubDate = item?.publishDate ? new Date(item.publishDate) : null;
      const absoluteTime =
        pubDate && !isNaN(pubDate.getTime())
          ? `${pubDate.getFullYear()}-${String(pubDate.getMonth() + 1).padStart(2, '0')}-${String(pubDate.getDate()).padStart(2, '0')} ${String(pubDate.getHours()).padStart(2, '0')}:${String(pubDate.getMinutes()).padStart(2, '0')}`
          : item?.publishDate || '';
      return {
        key: getAnnouncementKey(item),
        type: item.type || 'default',
        time: absoluteTime,
        content: item.content,
        extra: item.extra,
        relative: getRelativeTime(item.publishDate),
        isUnread: unreadSet.has(getAnnouncementKey(item)),
      };
    });
  }, [announcements, unreadSet]);

  const closeNotice = () => {
    closed.current = true;
    onClose({
      announcementKeys: [...viewed.current.announcementKeys],
      content: viewed.current.content,
    });
  };
  const handleCloseTodayNotice = () => {
    muteNoticeToday();
    closeNotice();
  };

  useEffect(() => {
    if (!visible) return;
    let cancelled = false;
    closed.current = false;
    viewed.current = { announcementKeys: new Set(), content: '' };
    setNoticeContent('');
    setNoticeLoaded(false);
    setLoading(true);
    const displayNotice = async () => {
      try {
        const res = await API.get('/api/notice');
        if (cancelled || closed.current) return;
        const { success, message, data } = res.data;
        if (success && typeof data === 'string') {
          setNoticeContent(data);
          setNoticeLoaded(true);
        } else {
          showError(message || t('加载失败'));
        }
      } catch (error) {
        if (!cancelled && !closed.current) showError(error.message);
      } finally {
        if (!cancelled && !closed.current) setLoading(false);
      }
    };
    displayNotice();
    return () => {
      cancelled = true;
    };
  }, [visible]);

  useEffect(() => {
    if (visible) {
      setActiveTab(defaultTab);
    }
  }, [defaultTab, visible]);

  useEffect(() => {
    if (!visible || closed.current) return;
    if (activeTab === 'system') {
      for (const item of processedAnnouncements)
        viewed.current.announcementKeys.add(item.key);
    } else if (activeTab === 'inApp' && noticeLoaded && !loading) {
      viewed.current.content = noticeContent;
    }
  }, [
    visible,
    activeTab,
    processedAnnouncements,
    noticeLoaded,
    loading,
    noticeContent,
  ]);

  const renderMarkdownNotice = () => {
    if (loading) {
      return (
        <div className='py-12'>
          <Empty description={t('加载中...')} />
        </div>
      );
    }

    if (!noticeContent) {
      return (
        <div className='py-12'>
          <Empty
            image={
              <IllustrationNoContent style={{ width: 150, height: 150 }} />
            }
            darkModeImage={
              <IllustrationNoContentDark style={{ width: 150, height: 150 }} />
            }
            description={t('暂无公告')}
          />
        </div>
      );
    }

    return (
      <RichContent
        content={noticeContent}
        breaks
        className='notice-content-scroll max-h-[55vh] overflow-y-auto pr-2'
      />
    );
  };

  const renderAnnouncementTimeline = () => {
    if (processedAnnouncements.length === 0) {
      return (
        <div className='py-12'>
          <Empty
            image={
              <IllustrationNoContent style={{ width: 150, height: 150 }} />
            }
            darkModeImage={
              <IllustrationNoContentDark style={{ width: 150, height: 150 }} />
            }
            description={t('暂无系统公告')}
          />
        </div>
      );
    }

    return (
      <div className='max-h-[55vh] overflow-y-auto pr-2 card-content-scroll'>
        <Timeline mode='left'>
          {processedAnnouncements.map((item, idx) => (
            <Timeline.Item
              key={idx}
              type={item.type}
              time={`${item.relative ? item.relative + ' ' : ''}${item.time}`}
              extra={
                item.extra ? (
                  <RichContent
                    className='text-xs text-gray-500'
                    content={item.extra}
                    breaks
                  />
                ) : null
              }
              className={item.isUnread ? '' : ''}
            >
              <div>
                <RichContent
                  className={item.isUnread ? 'shine-text' : ''}
                  content={item.content || ''}
                  breaks
                />
              </div>
            </Timeline.Item>
          ))}
        </Timeline>
      </div>
    );
  };

  const renderBody = () => {
    if (activeTab === 'inApp') {
      return renderMarkdownNotice();
    }
    if (activeTab === 'withdrawals') {
      return renderWithdrawalTab();
    }
    if (activeTab === 'invoices') {
      return renderInvoiceTab();
    }
    return renderAnnouncementTimeline();
  };

  const renderWithdrawalTab = () => {
    if (pendingWithdrawalCount === 0) {
      return (
        <div className='py-12'>
          <Empty
            image={
              <IllustrationNoContent style={{ width: 150, height: 150 }} />
            }
            darkModeImage={
              <IllustrationNoContentDark style={{ width: 150, height: 150 }} />
            }
            description={t('暂无待审核提现申请')}
          />
        </div>
      );
    }
    const goToReview = () => {
      closeNotice();
      navigate('/console/topup?tab=withdrawals');
    };
    return (
      <div className='max-h-[55vh] overflow-y-auto card-content-scroll pr-2'>
        <div className='flex items-center justify-between mb-3 px-1'>
          <Text type='tertiary' size='small'>
            {t('共 {{count}} 条待审核', { count: pendingWithdrawalCount })}
          </Text>
          <Button
            type='primary'
            theme='solid'
            size='small'
            onClick={goToReview}
          >
            {t('前往审核')}
          </Button>
        </div>
        <div className='flex flex-col gap-2'>
          {pendingWithdrawals.map((item) => {
            const name =
              item.username || item.display_name || `#${item.user_id}`;
            const ts = item.created_at ? item.created_at * 1000 : null;
            return (
              <div
                key={item.id}
                className='flex items-center justify-between gap-3 p-3 rounded-md bg-gray-50 dark:bg-zinc-800/50'
              >
                <div className='flex items-center gap-2 min-w-0 flex-1'>
                  <Avatar size='small' color={stringToColor(name)}>
                    {name.slice(0, 1).toUpperCase()}
                  </Avatar>
                  <div className='flex flex-col leading-tight min-w-0 flex-1'>
                    <Text size='small' ellipsis={{ showTooltip: true }}>
                      {name}
                    </Text>
                    <Text type='tertiary' size='small'>
                      {ts ? getRelativeTime(ts) : ''}
                    </Text>
                  </div>
                </div>
                <Text strong size='small' className='flex-shrink-0'>
                  {renderQuota(item.quota || 0)}
                </Text>
              </div>
            );
          })}
        </div>
        {pendingWithdrawalCount > pendingWithdrawals.length && (
          <div className='text-center mt-3'>
            <Text
              type='tertiary'
              size='small'
              link
              style={{ cursor: 'pointer' }}
              onClick={goToReview}
            >
              {t('查看全部 {{count}} 条', { count: pendingWithdrawalCount })}
            </Text>
          </div>
        )}
      </div>
    );
  };

  const renderInvoiceTab = () => {
    if (pendingInvoiceCount === 0) {
      return (
        <div className='py-12'>
          <Empty
            image={
              <IllustrationNoContent style={{ width: 150, height: 150 }} />
            }
            darkModeImage={
              <IllustrationNoContentDark style={{ width: 150, height: 150 }} />
            }
            description={t('暂无待处理发票申请')}
          />
        </div>
      );
    }
    const goToReview = () => {
      closeNotice();
      navigate('/console/topup?tab=invoices');
    };
    return (
      <div className='max-h-[55vh] overflow-y-auto card-content-scroll pr-2'>
        <div className='flex items-center justify-between mb-3 px-1'>
          <Text type='tertiary' size='small'>
            {t('共 {{count}} 条待处理', { count: pendingInvoiceCount })}
          </Text>
          <Button
            type='primary'
            theme='solid'
            size='small'
            onClick={goToReview}
          >
            {t('前往处理')}
          </Button>
        </div>
        <div className='flex flex-col gap-2'>
          {pendingInvoices.map((item) => {
            const name =
              item.username || item.display_name || `#${item.user_id}`;
            const ts = item.created_at ? item.created_at * 1000 : null;
            return (
              <div
                key={item.id}
                className='flex items-center justify-between gap-3 p-3 rounded-md bg-gray-50 dark:bg-zinc-800/50'
              >
                <div className='flex items-center gap-2 min-w-0 flex-1'>
                  <Avatar size='small' color={stringToColor(name)}>
                    {name.slice(0, 1).toUpperCase()}
                  </Avatar>
                  <div className='flex flex-col leading-tight min-w-0 flex-1'>
                    <Text size='small' ellipsis={{ showTooltip: true }}>
                      {name}
                    </Text>
                    <Text
                      type='tertiary'
                      size='small'
                      ellipsis={{ showTooltip: true }}
                    >
                      {item.title || t('未填写发票抬头')}
                    </Text>
                    <Text type='tertiary' size='small'>
                      {ts ? getRelativeTime(ts) : ''}
                    </Text>
                  </div>
                </div>
                <Text strong size='small' className='flex-shrink-0'>
                  ¥{Number(item.total_money || 0).toFixed(2)}
                </Text>
              </div>
            );
          })}
        </div>
        {pendingInvoiceCount > pendingInvoices.length && (
          <div className='text-center mt-3'>
            <Text
              type='tertiary'
              size='small'
              link
              style={{ cursor: 'pointer' }}
              onClick={goToReview}
            >
              {t('查看全部 {{count}} 条', { count: pendingInvoiceCount })}
            </Text>
          </div>
        )}
      </div>
    );
  };

  return (
    <Modal
      title={
        <div className='flex items-center justify-between w-full'>
          <span>{t('系统公告')}</span>
          <Tabs activeKey={activeTab} onChange={setActiveTab} type='button'>
            <TabPane
              tab={
                <span className='flex items-center gap-1'>
                  <Bell size={14} /> {t('通知')}
                </span>
              }
              itemKey='inApp'
            />
            <TabPane
              tab={
                <span className='flex items-center gap-1'>
                  <Megaphone size={14} /> {t('系统公告')}
                </span>
              }
              itemKey='system'
            />
            {showWithdrawalTab && (
              <TabPane
                tab={
                  <span className='flex items-center gap-1'>
                    <Wallet size={14} /> {t('提现审核')}
                    {pendingWithdrawalCount > 0 && (
                      <span className='ml-1 px-1.5 rounded-full bg-red-500 text-white text-xs'>
                        {formatBadgeCount(pendingWithdrawalCount)}
                      </span>
                    )}
                  </span>
                }
                itemKey='withdrawals'
              />
            )}
            {showInvoiceTab && (
              <TabPane
                tab={
                  <span className='flex items-center gap-1'>
                    <FileText size={14} /> {t('发票审核')}
                    {pendingInvoiceCount > 0 && (
                      <span className='ml-1 px-1.5 rounded-full bg-red-500 text-white text-xs'>
                        {formatBadgeCount(pendingInvoiceCount)}
                      </span>
                    )}
                  </span>
                }
                itemKey='invoices'
              />
            )}
          </Tabs>
        </div>
      }
      visible={visible}
      onCancel={closeNotice}
      footer={
        <div className='flex justify-end'>
          <Button type='secondary' onClick={handleCloseTodayNotice}>
            {t('今日关闭')}
          </Button>
          <Button type='primary' onClick={closeNotice}>
            {t('关闭公告')}
          </Button>
        </div>
      }
      size={isMobile ? 'full-width' : 'large'}
    >
      {renderBody()}
    </Modal>
  );
};

export default NoticeModal;
