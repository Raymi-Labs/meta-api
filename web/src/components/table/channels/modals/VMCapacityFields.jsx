import React from 'react';
import { InputNumber, Select, Switch, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { normalizeVMCapacitySettings } from '../../../../helpers/channelCapacity';

export default function VMCapacityFields({ value, onChange, channelType }) {
  const { t } = useTranslation();
  const settings = normalizeVMCapacitySettings(value);
  if (Number(channelType) !== 14 && !settings.enabled) return null;
  const update = (patch) => onChange({ ...settings, ...patch });
  return (
    <section aria-label={t('VM 实时容量')} style={{ marginBottom: 20 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          minHeight: 44,
        }}
      >
        <Switch
          id='vm-capacity-enabled'
          checked={settings.enabled}
          aria-label={t('VM 实时容量')}
          onChange={(enabled) => update({ enabled })}
        />
        <label htmlFor='vm-capacity-enabled'>{t('VM 实时容量')}</label>
      </div>
      <Typography.Paragraph type='tertiary'>
        {t(
          '仅限单 Key Claude 渠道，需开启渠道并发控制。快照不可用时暂停派发；静态上限仍生效。',
        )}
      </Typography.Paragraph>
      {settings.enabled && (
        <div style={{ display: 'grid', gap: 12 }}>
          <label htmlFor='vm-capacity-policy'>{t('满载处理')}</label>
          <Select
            id='vm-capacity-policy'
            aria-label={t('满载处理')}
            value={settings.overflow_policy}
            onChange={(overflow_policy) => update({ overflow_policy })}
            optionList={[
              { value: 'wait', label: t('缓存优先：有界等待当前渠道') },
              { value: 'reroute', label: t('速度优先：尝试其他渠道') },
            ]}
            style={{ width: '100%', minHeight: 44 }}
          />
          <label htmlFor='vm-capacity-wait'>{t('最大等待时间（毫秒）')}</label>
          <InputNumber
            id='vm-capacity-wait'
            aria-label={t('最大等待时间（毫秒）')}
            min={1}
            max={120000}
            precision={0}
            value={settings.max_wait_ms}
            disabled={settings.overflow_policy === 'reroute'}
            onChange={(max_wait_ms) => update({ max_wait_ms })}
            style={{ width: '100%', minHeight: 44 }}
          />
        </div>
      )}
    </section>
  );
}
