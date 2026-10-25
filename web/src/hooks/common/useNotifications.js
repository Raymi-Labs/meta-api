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

import { useState, useEffect, useMemo, useCallback } from 'react';
import { API } from '../../helpers/apiCore';
import { isAdmin } from '../../helpers/storage';
import {
  getAnnouncementKey,
  getReadAnnouncementKeys,
  markAnnouncementKeysRead,
  markNoticeContentRead,
  normalizeAnnouncements,
  subscribeNoticeReadState,
} from '../../helpers/noticeReadState';

export const useNotifications = (statusState) => {
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [announcementUnread, setAnnouncementUnread] = useState(0);
  const [pendingWithdrawals, setPendingWithdrawals] = useState([]);
  const [pendingWithdrawalCount, setPendingWithdrawalCount] = useState(0);
  const [pendingInvoices, setPendingInvoices] = useState([]);
  const [pendingInvoiceCount, setPendingInvoiceCount] = useState(0);
  const admin = isAdmin();

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

  const fetchPendingWithdrawals = async () => {
    if (!admin) return;
    try {
      const res = await API.get(
        '/api/user/aff-withdrawals?status=pending&p=1&page_size=5',
      );
      if (res?.data?.success) {
        const data = res.data.data || {};
        setPendingWithdrawals(Array.isArray(data.items) ? data.items : []);
        setPendingWithdrawalCount(Number(data.total) || 0);
      }
    } catch (_) {
      // silent: keep previous value to avoid badge flicker
    }
  };

  const fetchPendingInvoices = async () => {
    if (!admin) return;
    try {
      const res = await API.get(
        '/api/user/invoices?status=pending&p=1&page_size=5',
      );
      if (res?.data?.success) {
        const data = res.data.data || {};
        setPendingInvoices(Array.isArray(data.items) ? data.items : []);
        setPendingInvoiceCount(Number(data.total) || 0);
      }
    } catch (_) {
      // silent: keep previous value to avoid badge flicker
    }
  };

  const getUnreadKeys = useCallback(() => {
    const readSet = new Set(getReadAnnouncementKeys());
    return announcements
      .filter((a) => !readSet.has(getAnnouncementKey(a)))
      .map(getAnnouncementKey);
  }, [announcements]);

  // Effects
  useEffect(() => {
    const update = () => setAnnouncementUnread(getUnreadKeys().length);
    const unsubscribe = subscribeNoticeReadState(update);
    update();
    return unsubscribe;
  }, [getUnreadKeys]);

  useEffect(() => {
    fetchPendingWithdrawals();
    fetchPendingInvoices();
  }, [admin]);

  // 操作函数
  const handleNoticeOpen = () => {
    fetchPendingWithdrawals();
    fetchPendingInvoices();
    setNoticeVisible(true);
  };

  const handleNoticeClose = (viewed) => {
    setNoticeVisible(false);
    markAnnouncementKeysRead(viewed?.announcementKeys);
    markNoticeContentRead(viewed?.content);
  };

  return {
    noticeVisible,
    unreadCount:
      announcementUnread +
      (admin ? pendingWithdrawalCount + pendingInvoiceCount : 0),
    announcementUnread,
    pendingWithdrawalCount,
    pendingWithdrawals,
    pendingInvoiceCount,
    pendingInvoices,
    isAdminUser: admin,
    announcements,
    handleNoticeOpen,
    handleNoticeClose,
    getUnreadKeys,
  };
};
