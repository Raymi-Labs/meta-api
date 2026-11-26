import React, { useEffect, useState } from 'react';
import { Image, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { API } from '../../helpers/apiCore';
import './ticket-message.css';

export function conversationMessages(messages) {
  return (messages || []).filter((msg) => msg.sender_role !== 'system');
}

function ReplyImage({ file, base }) {
  const { t } = useTranslation();
  const [src, setSrc] = useState('');
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    let url;
    setFailed(false);
    setSrc('');
    API.get(base + '/attachment/' + file.id + '/download', {
      responseType: 'blob',
      skipGlobalLoading: true,
      skipErrorHandler: true,
    })
      .then(({ data }) => {
        if (active) {
          url = URL.createObjectURL(data);
          setSrc(url);
        }
      })
      .catch(() => {
        if (active) setFailed(true);
      });
    return () => {
      active = false;
      if (url) URL.revokeObjectURL(url);
    };
  }, [base, file.id]);
  if (failed)
    return (
      <Typography.Text type='tertiary'>
        {file.filename} · {t('加载失败')}
      </Typography.Text>
    );
  return (
    src && (
      <Image
        src={src}
        alt={file.filename}
        style={{ maxWidth: '100%', maxHeight: 240, objectFit: 'contain' }}
      />
    )
  );
}

const components = {
  a: ({ href, children }) => (
    <a href={href} target='_blank' rel='noopener noreferrer'>
      {children}
    </a>
  ),
  // Only authenticated ticket attachments load automatically.
  img: ({ src, alt }) => (
    <a href={src} target='_blank' rel='noopener noreferrer'>
      {alt || src}
    </a>
  ),
};

export default function TicketMessageBody({ message, base }) {
  return (
    <div
      className='ticket-message'
      style={{ maxWidth: '100%', overflowX: 'auto' }}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkBreaks]}
        components={components}
      >
        {message.body}
      </ReactMarkdown>
      {message.attachments
        ?.filter((file) =>
          ['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(
            file.mime_type,
          ),
        )
        .map((file) => (
          <ReplyImage key={file.id} file={file} base={base} />
        ))}
    </div>
  );
}
