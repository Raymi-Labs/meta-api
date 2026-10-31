import React, { useEffect, useState } from 'react';
import { Button, Select, Space, Tag, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const stateLabels = {
  unconfigured: '未配置',
  invalid_token: '令牌不可用',
  ready: '自动翻译',
  cooldown: '稍后重试',
  environment_disabled: '环境变量已关闭翻译',
};

export default function SettingModelTranslation() {
  const { t } = useTranslation();
  const [data, setData] = useState(null);
  const [tokenId, setTokenId] = useState(0);
  const [busy, setBusy] = useState(false);
  const load = async () => {
    try {
      const res = await API.get('/api/option/model-description-translation');
      if (!res.data.success) throw new Error(res.data.message);
      setData(res.data.data);
      setTokenId(res.data.data.token_id);
    } catch (error) {
      showError(error.message);
    }
  };
  useEffect(() => {
    load();
  }, []);
  const save = async () => {
    setBusy(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'ModelDescriptionTranslationTokenId',
        value: String(tokenId),
      });
      if (!res.data.success) throw new Error(res.data.message);
      showSuccess(t('保存成功'));
      await load();
    } catch (error) {
      showError(error.message);
    } finally {
      setBusy(false);
    }
  };
  const options = [
    { value: 0, label: t('暂停自动翻译') },
    ...(data?.tokens || []).map((token) => ({
      value: token.id,
      label: `${token.name} · ${token.group || t('默认')} (#${token.id})`,
    })),
  ];
  if (
    data?.token_id &&
    !options.some((option) => option.value === data.token_id)
  ) {
    options.push({
      value: data.token_id,
      label: `${t('令牌不可用')} (#${data.token_id})`,
      disabled: true,
    });
  }
  return (
    <div className='flex flex-col gap-3'>
      <Typography.Title heading={5}>{t('模型介绍自动翻译')}</Typography.Title>
      <Typography.Text type='tertiary'>
        {t(
          '使用 qwen-mt-flash 和专用令牌，按现有渠道计费；已完成译文会缓存复用。',
        )}
      </Typography.Text>
      <Typography.Text>
        {t('先在令牌管理中创建仅允许 qwen-mt-flash 的令牌，再在此选择。')}
      </Typography.Text>
      <Space wrap>
        <Select
          aria-label={t('翻译专用令牌')}
          style={{ width: 300, maxWidth: '100%' }}
          value={tokenId}
          onChange={setTokenId}
          optionList={options}
          filter
          disabled={!data || busy}
        />
        <Button
          onClick={save}
          loading={busy}
          disabled={!data || tokenId === data.token_id}
        >
          {t('保存')}
        </Button>
        <Button onClick={load} disabled={busy}>
          {t('刷新')}
        </Button>
        <Button
          theme='borderless'
          onClick={() =>
            window.open('/console/token', '_blank', 'noopener,noreferrer')
          }
        >
          {t('令牌管理')}
        </Button>
      </Space>
      {data && (
        <Space wrap>
          <Tag>{t(stateLabels[data.state] || '等待翻译')}</Tag>
          {data.state === 'cooldown' && (
            <Typography.Text>
              {new Date(data.next_attempt_at * 1000).toLocaleString()}
            </Typography.Text>
          )}
          {data.reason && (
            <Typography.Text type='warning'>{data.reason}</Typography.Text>
          )}
        </Space>
      )}
    </div>
  );
}
