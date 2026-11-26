import React, { useEffect, useRef, useState } from 'react';
import { Button, Checkbox, Image, TextArea } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import TurndownService from 'turndown';
import { API } from '../../helpers/apiCore';
import { showError, showSuccess } from '../../helpers/toast';

const types = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];
const markdown = new TurndownService({
  headingStyle: 'atx',
  codeBlockStyle: 'fenced',
});
markdown.remove(['script', 'style']);

export default function TicketReply({
  path,
  admin,
  onSent,
  onCloseTicket,
  closing,
}) {
  const { t } = useTranslation();
  const [body, setBody] = useState('');
  const [file, setFile] = useState(null);
  const [internal, setInternal] = useState(false);
  const [sending, setSending] = useState(false);
  const [preview, setPreview] = useState('');
  const picker = useRef(null);
  useEffect(() => {
    const url = file ? URL.createObjectURL(file) : '';
    setPreview(url);
    return () => {
      if (url) URL.revokeObjectURL(url);
    };
  }, [file]);

  function selectFile(next) {
    if (!next) return;
    if (!types.includes(next.type)) {
      showError(t('仅支持') + ' PNG / JPEG / GIF / WebP');
      return;
    }
    if (next.size > 5 * 1024 * 1024) {
      showError(t('文件大小不能超过5MB'));
      return;
    }
    setFile(next);
  }

  function paste(event) {
    if (sending || closing) return;
    const clipboard = event.clipboardData;
    const image = Array.from(clipboard.files).find((item) =>
      item.type.startsWith('image/'),
    );
    const html = clipboard.getData('text/html');
    if (!image && !html) return;
    event.preventDefault();
    if (image) selectFile(image);
    let text = clipboard.getData('text/plain');
    if (html) {
      const template = document.createElement('template');
      template.innerHTML = html;
      if (image)
        template.content.querySelectorAll('img').forEach((img) => img.remove());
      text = markdown.turndown(template.content) || text;
    }
    if (!text) return;
    const field = event.target;
    const start = field.selectionStart;
    const next = body.slice(0, start) + text + body.slice(field.selectionEnd);
    if (next.length > 5000) {
      showError(t('回复内容') + ' ≤ 5000');
      return;
    }
    setBody(next);
    requestAnimationFrame(() =>
      field.setSelectionRange(start + text.length, start + text.length),
    );
  }

  async function send() {
    if (sending || closing || (!body.trim() && !file)) return;
    setSending(true);
    try {
      let payload = { body };
      if (admin) payload.is_internal = internal;
      if (file) {
        const form = new FormData();
        Object.entries(payload).forEach(([key, value]) =>
          form.append(key, value),
        );
        form.append('file', file);
        payload = form;
      }
      const { data } = await API.post(path, payload, {
        skipErrorHandler: true,
      });
      if (!data.success) throw Error(data.message);
      setBody('');
      setFile(null);
      setInternal(false);
      showSuccess(t('回复成功'));
      await onSent();
    } catch (error) {
      showError(error.response?.data?.message || error.message);
    } finally {
      setSending(false);
    }
  }

  const busy = sending || closing;
  return (
    <div style={{ paddingBottom: 24 }}>
      <label
        htmlFor='ticket-reply-body'
        style={{ display: 'block', marginBottom: 8 }}
      >
        {t('回复内容')}
      </label>
      <div onPaste={paste}>
        <TextArea
          id='ticket-reply-body'
          value={body}
          onChange={setBody}
          showClear
          placeholder={t('支持 Ctrl+V 粘贴图片')}
          rows={4}
          maxLength={5000}
          disabled={busy}
        />
      </div>
      {preview && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            marginTop: 12,
          }}
        >
          <Image
            src={preview}
            alt={file?.name}
            width={72}
            height={72}
            style={{ objectFit: 'contain' }}
          />
          <Button size='small' disabled={busy} onClick={() => setFile(null)}>
            {t('移除')}
          </Button>
        </div>
      )}
      {admin && (
        <Checkbox
          checked={internal}
          disabled={busy}
          onChange={(event) => setInternal(event.target.checked)}
          style={{ marginTop: 12 }}
        >
          {t('内部备注（用户不可见）')}
        </Checkbox>
      )}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          flexWrap: 'wrap',
          marginTop: 16,
        }}
      >
        <input
          ref={picker}
          hidden
          type='file'
          accept={types.join(',')}
          onChange={(event) => {
            selectFile(event.target.files[0]);
            event.target.value = '';
          }}
        />
        <Button
          disabled={busy}
          onClick={() => picker.current.click()}
          style={{ marginInlineEnd: 'auto' }}
        >
          {t('上传图片')}
        </Button>
        {onCloseTicket && (
          <Button
            type='tertiary'
            disabled={sending}
            loading={closing}
            onClick={onCloseTicket}
          >
            {t('关闭工单')}
          </Button>
        )}
        <Button
          theme='solid'
          type='primary'
          disabled={busy || (!body.trim() && !file)}
          loading={sending}
          onClick={send}
        >
          {t('发送回复')}
        </Button>
      </div>
    </div>
  );
}
