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

import React, { useEffect, useMemo, useState } from 'react';
import { Button, Card, Select, Space, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

export default function ModelDiscountSettings({ options, refresh }) {
  const { t } = useTranslation();
  const [models, setModels] = useState([]);
  const [loading, setLoading] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const raw = options?.NoDiscountModels;
  useEffect(() => {
    if (raw === undefined) return;
    try {
      const names = JSON.parse(raw);
      if (
        !Array.isArray(names) ||
        names.some((name) => typeof name !== 'string')
      )
        throw new Error('Invalid model list');
      setModels(names);
      setInvalid(false);
    } catch {
      setInvalid(true);
    }
  }, [raw]);
  const candidates = useMemo(() => {
    const names = new Set(models);
    for (const key of [
      'ModelRatio',
      'ModelPrice',
      'AudioDurationPrice',
      'billing_setting.billing_mode',
    ]) {
      try {
        Object.keys(JSON.parse(options?.[key] || '{}')).forEach((name) =>
          names.add(name),
        );
      } catch {
        /* Other pricing editors report their own invalid settings. */
      }
    }
    return [...names].sort().map((name) => ({ label: name, value: name }));
  }, [models, options]);
  const save = async () => {
    setLoading(true);
    try {
      const { data } = await API.put('/api/option/', {
        key: 'NoDiscountModels',
        value: JSON.stringify(models),
      });
      if (!data.success) return showError(data.message || t('保存失败'));
      showSuccess(t('保存成功'));
      await refresh();
    } catch {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };
  return (
    <Card title={t('不参与折扣的模型')} className='mb-4'>
      <Space vertical align='start' style={{ width: '100%' }}>
        <Typography.Text type='tertiary'>
          {t(
            '所选模型在所有分组按本站基础价计费，不叠加个人、企业或时段倍率；不改变充值套餐折算和付费加速费用。精确匹配模型 ID，别名需单独选择。',
          )}
        </Typography.Text>
        {invalid && (
          <Typography.Text type='danger'>
            {t('加载失败，请重试')}
          </Typography.Text>
        )}
        <Select
          aria-label={t('不参与折扣的模型')}
          multiple
          filter
          allowCreate
          showClear
          style={{ width: '100%', minWidth: 240 }}
          optionList={candidates}
          value={models}
          onChange={setModels}
          disabled={loading || invalid || raw === undefined}
          placeholder={t('模型名称')}
        />
        <Button
          onClick={save}
          loading={loading}
          disabled={invalid || raw === undefined}
        >
          {t('保存')}
        </Button>
      </Space>
    </Card>
  );
}
