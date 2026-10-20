import React from 'react';
import { Banner, Tag, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

export default function VideoPreflightResult({ data, showHistory = true }) {
  const { t } = useTranslation();
  if (!data) return null;
  const labels = {
    model_declared: t('渠道已声明该模型'),
    model_mapping: t('模型映射格式有效'),
    credential_configured: t('已填写凭据（未验证有效性）'),
    base_url: t('上游地址格式有效'),
    task_adapter: t('存在视频任务适配器'),
    explicit_task_price: t('已明确配置任务价格'),
    channel_group: t('已配置渠道分组'),
  };
  const statuses = {
    SUCCESS: t('生成成功'),
    FAILURE: t('生成失败'),
    SUBMITTED: t('已受理'),
    QUEUED: t('排队中'),
    IN_PROGRESS: t('生成中'),
    NOT_START: t('未开始'),
    UNKNOWN: t('结果待确认'),
  };
  return (
    <div className='flex flex-col gap-3'>
      <Banner
        type='info'
        closeIcon={null}
        description={t(
          '本次仅检查本地配置，未调用上游、未产生视频生成费用；不证明凭据、参数或模型实际可用。',
        )}
      />
      <div className='flex flex-wrap gap-2'>
        {(data.checks || []).map((check) => (
          <Tag key={check.name} color={check.ok ? 'blue' : 'orange'}>
            {labels[check.name] || check.name} ·{' '}
            {check.ok ? t('通过') : t('待完善')}
          </Tag>
        ))}
      </div>
      {data.quote && (
        <div className='rounded-lg border border-[var(--semi-color-border)] p-3'>
          <Typography.Text strong>
            {t('本站估算扣费（USD）')} $
            {Number(data.quote.amount_usd).toFixed(6)}
          </Typography.Text>
          <div>
            {t('分组')}：{data.quote.group} · {t('分组倍率')}：
            {data.quote.group_ratio} · {t('时段倍率')}：{data.quote.time_ratio}
          </div>
          <Typography.Text type='tertiary'>
            {t(
              '仅按输入参数和当前表达式估算，不验证供应商参数；不是扣费授权。上游实际成本未知，本站零价不代表上游免费。',
            )}
          </Typography.Text>
        </div>
      )}
      {data.quote_unavailable && (
        <Banner
          type='warning'
          closeIcon={null}
          description={t(data.quote_unavailable)}
        />
      )}
      {showHistory && (
        <div className='flex flex-col gap-2'>
          <Typography.Text strong>
            {t('最近真实任务（历史参考）')}
          </Typography.Text>
          <Typography.Text type='tertiary'>
            {t(
              '仅查看最近 200 条渠道任务中的同名模型，最多展示 10 条。历史记录没有当前配置校验，不能证明现在可用。',
            )}
          </Typography.Text>
          {data.history_unavailable ? (
            <Typography.Text type='warning'>
              {t('暂时无法读取历史记录')}
            </Typography.Text>
          ) : data.history?.length ? (
            data.history.map((item, index) => (
              <div key={index} className='flex flex-wrap gap-2'>
                <Tag color={item.status === 'SUCCESS' ? 'blue' : 'grey'}>
                  {statuses[item.status] || t('结果待确认')}
                </Tag>
                <span>
                  {item.submit_time
                    ? new Date(item.submit_time * 1000).toLocaleString()
                    : t('时间未知')}
                </span>
              </div>
            ))
          ) : (
            <Typography.Text type='tertiary'>
              {t('扫描范围内暂无同名模型记录；不会自动生成测试视频。')}
            </Typography.Text>
          )}
        </div>
      )}
    </div>
  );
}
