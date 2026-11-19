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

import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { API } from '../../helpers/apiCore';
import {
  getAnnouncementKey,
  getReadAnnouncementKeys,
  markAnnouncementKeysRead,
  markNoticeContentRead,
  normalizeAnnouncements,
  subscribeNoticeReadState,
} from '../../helpers/noticeReadState';

export const useNotifications = (statusState, userState) => {
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [announcementUnread, setAnnouncementUnread] = useState(0);
  const [pendingWithdrawals, setPendingWithdrawals] = useState([]);
  const [pendingWithdrawalCount, setPendingWithdrawalCount] = useState(0);
  const [pendingInvoices, setPendingInvoices] = useState([]);
  const [pendingInvoiceCount, setPendingInvoiceCount] = useState(0);
  const [ticketUnread, setTicketUnread] = useState(0);
  const userID = userState?.user?.id;
  const admin = Number(userState?.user?.role) >= 10;
  const ticketRequest = useRef(0);
  const ticketAbort = useRef(null);
  const identity = `${userID || ''}:${admin}`;
  const currentIdentity = useRef(identity);
  currentIdentity.current = identity;
  const [staffTicketUnread, setStaffTicketUnread] = useState(0);

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

  const fetchTicketUnread = async () => {
    if (!userID) return;
    ticketAbort.current?.abort();
    const controller = new AbortController();
    ticketAbort.current = controller;
    const request = ++ticketRequest.current;
    const isCurrent = () =>
      request === ticketRequest.current && currentIdentity.current === identity;
    const config = {
      skipErrorHandler: true,
      skipGlobalLoading: true,
      disableDuplicate: true,
      signal: controller.signal,
      headers: { 'New-API-User': userID },
    };
    try {
      const res = await API.get('/api/ticket/unread', config);
      if (!isCurrent()) return;
      if (res?.data?.success) {
        setTicketUnread(Number(res.data.data?.count) || 0);
      }
      if (admin) {
        const staff = await API.get('/api/admin/ticket/unread', config);
        if (!isCurrent()) return;
        if (staff.data?.success)
          setStaffTicketUnread(Number(staff.data.data?.count) || 0);
      }
    } catch (_) {
      // Keep the current account's last value on a transient failure.
    }
  };

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

  useEffect(() => {
    setTicketUnread(0);
    setStaffTicketUnread(0);
    fetchTicketUnread();
    const timer = setInterval(fetchTicketUnread, 60000);
    return () => {
      ticketRequest.current += 1;
      ticketAbort.current?.abort();
      clearInterval(timer);
    };
  }, [userID, admin]);

  // 操作函数
  const handleNoticeOpen = () => {
    fetchTicketUnread();
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
      ticketUnread +
      staffTicketUnread +
      (admin ? pendingWithdrawalCount + pendingInvoiceCount : 0),
    announcementUnread,
    ticketUnread,
    staffTicketUnread,
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
