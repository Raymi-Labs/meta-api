import React, { useEffect, useRef, useState } from 'react';
import { Button, TextArea, Modal, Select, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import VideoPreflightResult from '../../../common/VideoPreflightResult';

export default function VideoPreflightModal({
  channel,
  model,
  onCancel,
  onResult,
}) {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [data, setData] = useState(null);
  const [group, setGroup] = useState('');
  const [parameters, setParameters] = useState('');
  const [loading, setLoading] = useState(false);
  const serial = useRef(0);
  const inspect = async (quote = false) => {
    const current = ++serial.current;
    let parsed;
    if (quote) {
      try {
        parsed = JSON.parse(parameters);
        if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object')
          throw new Error();
      } catch {
        showError(t('请输入 JSON 对象形式的真实请求参数'));
        return;
      }
    }
    setLoading(true);
    setData(null);
    try {
      const res = await API.post(`/api/channel/${channel.id}/video/preflight`, {
        model,
        group,
        ...(quote ? { parameters: parsed } : {}),
      });
      if (current !== serial.current) return;
      if (res.data?.test_kind !== 'video_preflight') {
        showError(res.data?.message || t('检查失败'));
        return;
      }
      setData(res.data.data);
      onResult?.(res.data.data);
    } catch (err) {
      if (current === serial.current) showError(err?.message || t('检查失败'));
    } finally {
      if (current === serial.current) setLoading(false);
    }
  };
  useEffect(() => {
    setGroup(channel?.group?.split(',')[0]?.trim() || '');
    setParameters('');
    setData(null);
    if (model && channel) inspect();
    return () => {
      serial.current++;
    };
  }, [model, channel?.id]);
  const invalidate = () => {
    serial.current++;
    setLoading(false);
    setData(null);
  };
  return (
    <Modal
      visible={Boolean(model)}
      title={`${model || ''} · ${t('视频配置检查')}`}
      onCancel={onCancel}
      size={isMobile ? 'full-width' : 'medium'}
      footer={<Button onClick={onCancel}>{t('关闭')}</Button>}
    >
      <div className='flex flex-col gap-4'>
        <Typography.Text>
          {t(
            '检查配置和查看记录不生成视频。需要真实验收时，请使用正式视频 API；本页不会创建付费任务。',
          )}
        </Typography.Text>
        <Button onClick={() => inspect()} loading={loading}>
          {t('检查配置与最近记录')}
        </Button>
        <div className='flex flex-col gap-2'>
          <Typography.Text strong>
            {t('可选：按当前价格表达式估价')}
          </Typography.Text>
          <Select
            aria-label={t('估价分组')}
            value={group}
            onChange={(value) => {
              invalidate();
              setGroup(value);
            }}
            optionList={(channel?.group || '')
              .split(',')
              .filter(Boolean)
              .map((value) => ({ value: value.trim(), label: value.trim() }))}
          />
          <TextArea
            aria-label={t('请求参数 JSON')}
            value={parameters}
            onChange={(value) => {
              invalidate();
              setParameters(value);
            }}
            autosize={{ minRows: 4, maxRows: 10 }}
            placeholder={t(
              '填写计划使用的 duration、resolution 等请求参数；请按具体模型文档填写，不使用统一的 1 秒/480p 默认值。',
            )}
          />
          <Typography.Text type='tertiary'>
            {t(
              '使用当前管理员账户的分组优惠和时段倍率。修改参数后需重新估价；不保存价格，不提交生成。',
            )}
          </Typography.Text>
          <Button
            onClick={() => inspect(true)}
            loading={loading}
            disabled={!parameters.trim() || !group}
          >
            {t('本地估价（不生成）')}
          </Button>
        </div>
        <VideoPreflightResult data={data} />
      </div>
    </Modal>
  );
}
