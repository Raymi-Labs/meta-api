import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Modal,
  Space,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../../../helpers';

const initialBody =
  '{\n  "model": "example-model",\n  "messages": [{"role": "user", "content": "Hello"}]\n}';
const initialContext = JSON.stringify(
  {
    original_model: 'example-model',
    upstream_model: 'example-model',
    request_path: '/v1/chat/completions',
    is_channel_test: false,
    retry_index: 0,
    request_headers: {},
    header_override: {},
  },
  null,
  2,
);
const codeStyle = {
  whiteSpace: 'pre-wrap',
  overflowWrap: 'anywhere',
  maxHeight: 320,
  overflowY: 'auto',
};

export default function ParamOverridePreview({
  visible,
  draft,
  draftError,
  onClose,
}) {
  const { t } = useTranslation();
  const [body, setBody] = useState(initialBody);
  const [context, setContext] = useState(initialContext);
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const generation = useRef(0);

  useEffect(() => {
    generation.current += 1;
    setResult(null);
    setError('');
    setLoading(false);
    return () => {
      generation.current += 1;
    };
  }, [visible, draft, draftError, body, context]);

  const run = async () => {
    const current = ++generation.current;
    setLoading(true);
    setError('');
    setResult(null);
    try {
      const sample = JSON.parse(context);
      if (!sample || typeof sample !== 'object' || Array.isArray(sample))
        throw new Error(t('JSON格式错误'));
      const response = await API.post('/api/channel/param_override/preview', {
        body,
        override: draft || '{}',
        context: sample,
      });
      if (current !== generation.current) return;
      if (!response.data.success) throw new Error(response.data.message);
      setResult(response.data.data);
    } catch (err) {
      if (current === generation.current)
        setError(err.response?.data?.message || err.message || t('操作失败'));
    } finally {
      if (current === generation.current) setLoading(false);
    }
  };

  return (
    <Modal
      title={t('参数覆盖试运行')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={840}
      style={{ maxWidth: 'calc(100vw - 24px)' }}
      bodyStyle={{ maxHeight: '75vh', overflowY: 'auto' }}
    >
      <Space vertical align='start' style={{ width: '100%' }} spacing={12}>
        <Banner
          type='info'
          closeIcon={null}
          description={t(
            '仅运行当前草稿与样例，不保存、不调用模型、不计费。结果仅代表参数覆盖阶段。',
          )}
        />
        <Typography.Text strong>{t('请求体 JSON')}</Typography.Text>
        <TextArea
          aria-label={t('请求体 JSON')}
          value={body}
          onChange={setBody}
          autosize={{ minRows: 5, maxRows: 12 }}
          style={{ width: '100%' }}
        />
        <Typography.Text strong>{t('模拟上下文 JSON')}</Typography.Text>
        <TextArea
          aria-label={t('模拟上下文 JSON')}
          value={context}
          onChange={setContext}
          autosize={{ minRows: 5, maxRows: 12 }}
          style={{ width: '100%' }}
        />
        <Typography.Text type='tertiary'>
          {t(
            '上下文和请求头均为自填样例；请勿填入真实密钥。最多 100 条操作、256 KiB 输入。',
          )}
        </Typography.Text>
        <Button
          theme='solid'
          onClick={run}
          loading={loading}
          disabled={!!draftError}
        >
          {t('运行试例')}
        </Button>
        {(draftError || error) && (
          <Banner
            type='danger'
            closeIcon={null}
            description={draftError || error}
          />
        )}
        {result && (
          <div style={{ width: '100%' }} aria-live='polite'>
            {result.error && (
              <Banner
                type='warning'
                closeIcon={null}
                description={result.error}
              />
            )}
            {result.rejection && (
              <pre style={codeStyle}>
                {JSON.stringify(result.rejection, null, 2)}
              </pre>
            )}
            <Typography.Text strong>{t('输出')}</Typography.Text>
            <pre style={codeStyle}>{result.body || '—'}</pre>
            <Typography.Text strong>{t('请求头覆盖')}</Typography.Text>
            <pre style={codeStyle}>
              {JSON.stringify(result.header_override, null, 2)}
            </pre>
            <Typography.Text strong>
              {t('规则条件命中（不代表字段一定改变）')}
            </Typography.Text>
            <Space wrap style={{ marginTop: 8 }}>
              {result.steps.map((step) => (
                <Tag
                  key={step.index}
                  color={step.conditions_matched ? 'green' : 'grey'}
                >
                  {step.index}. {step.mode}:{' '}
                  {step.conditions_matched ? t('是') : t('否')}
                </Tag>
              ))}
            </Space>
          </div>
        )}
      </Space>
    </Modal>
  );
}
