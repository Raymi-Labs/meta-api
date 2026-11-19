import React, { useEffect, useRef, useState } from 'react';
import { Button, Space, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers/apiCore';
import { showError } from '../../helpers/toast';

// Downloads use the authenticated API client, including New-Api-User.
export default function TicketAttachments({
  ticketId,
  mode = 'user',
  closed = false,
}) {
  const { t } = useTranslation();
  const [files, setFiles] = useState([]);
  const [busy, setBusy] = useState(false);
  const input = useRef(null);
  const base =
    mode === 'admin'
      ? '/api/admin/ticket'
      : mode === 'appeal'
        ? '/api/appeal'
        : '/api/ticket';
  const listPath =
    mode === 'appeal'
      ? `${base}/tickets/${ticketId}/attachments`
      : `${base}/${ticketId}/attachments`;
  const load = async () => {
    try {
      const { data } = await API.get(listPath, { skipErrorHandler: true });
      if (!data.success) throw Error(data.message);
      setFiles(data.data || []);
    } catch (e) {
      showError(e.message);
    }
  };
  useEffect(() => {
    load();
  }, [listPath]);
  const upload = async (event) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    if (file.size > 5 * 1024 * 1024) {
      showError(t('文件大小不能超过5MB'));
      return;
    }
    setBusy(true);
    try {
      const body = new FormData();
      body.append('ticket_id', String(ticketId));
      body.append('file', file);
      const { data } = await API.post(`${base}/attachment/upload`, body, {
        skipErrorHandler: true,
      });
      if (!data.success) throw Error(data.message);
      await load();
    } catch (e) {
      showError(e.response?.data?.message || e.message);
    } finally {
      setBusy(false);
    }
  };
  const download = async (file) => {
    try {
      const { data } = await API.get(`${base}/attachment/${file.id}/download`, {
        responseType: 'blob',
        skipErrorHandler: true,
      });
      const url = URL.createObjectURL(data);
      const link = document.createElement('a');
      link.href = url;
      link.download = file.filename;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      showError(t('下载失败'));
    }
  };
  return (
    <div style={{ margin: '16px 0' }}>
      <Typography.Text strong>{t('附件')}</Typography.Text>
      <Space wrap style={{ display: 'flex', marginTop: 8 }}>
        {files.map((file) => (
          <Button key={file.id} onClick={() => download(file)}>
            {file.filename}
          </Button>
        ))}
        {mode !== 'admin' && !closed && files.length < 10 && (
          <>
            <input
              ref={input}
              type='file'
              hidden
              accept='image/jpeg,image/png,image/gif,image/webp,application/pdf,text/plain'
              onChange={upload}
            />
            <Button loading={busy} onClick={() => input.current.click()}>
              {t('上传附件')}
            </Button>
            <Typography.Text type='tertiary'>
              {t('单个附件不超过5MB，每个工单最多10个附件')}
            </Typography.Text>
          </>
        )}
      </Space>
    </div>
  );
}
