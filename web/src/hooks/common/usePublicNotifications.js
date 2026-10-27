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

import { useCallback, useEffect, useRef, useState } from 'react';
import { API } from '../../helpers/apiCore';
import {
  getAnnouncementKey,
  getReadAnnouncementKeys,
  isNoticeContentUnread,
  markAnnouncementKeysRead,
  markNoticeContentRead,
  normalizeAnnouncements,
  subscribeNoticeReadState,
} from '../../helpers/noticeReadState';

export const usePublicNotifications = ({ status }) => {
  const [visible, setVisible] = useState(false);
  const [content, setContent] = useState('');
  const [announcements, setAnnouncements] = useState(() =>
    status?.announcements_enabled === false
      ? []
      : normalizeAnnouncements(status?.announcements),
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [, setReadRevision] = useState(0);
  const alive = useRef(false);
  const requestId = useRef(0);
  const manualOpened = useRef(false);

  useEffect(() => {
    alive.current = true;
    const unsubscribe = subscribeNoticeReadState(() =>
      setReadRevision((value) => value + 1),
    );
    return () => {
      alive.current = false;
      requestId.current += 1;
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    setAnnouncements(
      status?.announcements_enabled === false
        ? []
        : normalizeAnnouncements(status?.announcements),
    );
  }, [status?.announcements, status?.announcements_enabled]);

  const load = useCallback(async (refreshAnnouncements = false) => {
    const id = ++requestId.current;
    setLoading(true);
    setError(false);
    const options = { skipGlobalLoading: true, skipErrorHandler: true };
    const requests = [API.get('/api/notice', options)];
    if (refreshAnnouncements) requests.push(API.get('/api/status', options));
    const [notice, updates] = await Promise.allSettled(requests);
    if (!alive.current || id !== requestId.current) return null;

    const noticeData =
      notice.status === 'fulfilled' ? notice.value?.data : null;
    const noticeOK = noticeData?.success && typeof noticeData.data === 'string';
    const nextContent = noticeOK ? noticeData.data.trim() : null;
    if (noticeOK) setContent(nextContent);
    let updatesOK = true;
    if (refreshAnnouncements) {
      const data = updates.status === 'fulfilled' ? updates.value?.data : null;
      updatesOK = Boolean(
        data?.success && data.data && typeof data.data === 'object',
      );
      if (updatesOK)
        setAnnouncements(
          data.data.announcements_enabled === false
            ? []
            : normalizeAnnouncements(data.data.announcements),
        );
    }
    setError(!noticeOK || !updatesOK);
    setLoading(false);
    return nextContent;
  }, []);

  useEffect(() => {
    let cancelled = false;
    const timer = window.setTimeout(async () => {
      if (manualOpened.current) return;
      if (!cancelled) await load();
    }, 4500);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [load]);

  const open = () => {
    manualOpened.current = true;
    setVisible(true);
    void load(true);
  };
  const close = ({
    announcementKeys = [],
    content: viewedContent = '',
  } = {}) => {
    manualOpened.current = true;
    requestId.current += 1;
    setVisible(false);
    setLoading(false);
    markAnnouncementKeysRead(announcementKeys);
    if (viewedContent) markNoticeContentRead(viewedContent);
  };
  const readKeys = new Set(getReadAnnouncementKeys());
  const hasUnread =
    announcements.some((item) => !readKeys.has(getAnnouncementKey(item))) ||
    isNoticeContentUnread(content);

  return {
    visible,
    content,
    announcements,
    loading,
    error,
    hasUnread,
    open,
    close,
    reload: () => load(true),
  };
};
