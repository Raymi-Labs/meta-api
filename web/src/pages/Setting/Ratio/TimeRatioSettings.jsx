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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Button,
  Card,
  Descriptions,
  Form,
  Select,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

import {
  API,
  formatTimeRatioValue,
  showError,
  showSuccess,
  verifyJSON,
} from '../../../helpers';

const { Paragraph, Text } = Typography;

const DEFAULT_EXAMPLE = `[
  {
    "id": "offpeak-night",
    "enabled": true,
    "timezone": "Asia/Shanghai",
    "start": "00:00",
    "end": "08:00",
    "days": ["mon", "tue", "wed", "thu", "fri", "sat", "sun"],
    "ratio": 0.8,
    "models": ["*"],
    "groups": ["*"],
    "priority": 10
  },
  {
    "id": "peak-evening",
    "enabled": true,
    "timezone": "Asia/Shanghai",
    "start": "18:00",
    "end": "23:30",
    "ratio": 1.2,
    "models": ["gpt-*", "claude-*"],
    "groups": ["default"],
    "priority": 20
  }
]`;

const parseJSONObject = (raw) => {
  try {
    const parsed = JSON.parse(raw || '{}');
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed
      : {};
  } catch {
    return {};
  }
};

const parseRules = (raw) => {
  try {
    const parsed = JSON.parse(raw || '[]');
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};

export default function TimeRatioSettings({ options, refresh }) {
  const { t } = useTranslation();
  const formApiRef = useRef(null);
  const [loading, setLoading] = useState(false);
  const [rules, setRules] = useState('[]');
  const [previewModel, setPreviewModel] = useState('');
  const [previewGroup, setPreviewGroup] = useState('default');
  const [previewUserGroup, setPreviewUserGroup] = useState('');
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  const modelOptions = useMemo(() => {
    const candidates = new Set();
    [
      'ModelRatio',
      'ModelPrice',
      'CompletionRatio',
      'CacheRatio',
      'CreateCacheRatio',
      'ImageRatio',
      'AudioRatio',
      'AudioCompletionRatio',
    ].forEach((key) => {
      Object.keys(parseJSONObject(options?.[key])).forEach((model) =>
        candidates.add(model),
      );
    });
    parseRules(rules).forEach((rule) => {
      (rule?.models || []).forEach((model) => {
        if (model && model !== '*') candidates.add(model);
      });
    });
    return Array.from(candidates)
      .sort()
      .map((model) => ({ label: model, value: model }));
  }, [options, rules]);

  const groupOptions = useMemo(() => {
    const candidates = new Set(
      Object.keys(parseJSONObject(options?.GroupRatio)),
    );
    parseRules(rules).forEach((rule) => {
      (rule?.groups || []).forEach((group) => {
        if (group && group !== '*') candidates.add(group);
      });
      (rule?.user_groups || []).forEach((group) => {
        if (group && group !== '*') candidates.add(group);
      });
    });
    return Array.from(candidates)
      .sort()
      .map((group) => ({ label: group, value: group }));
  }, [options, rules]);

  useEffect(() => {
    const nextRules = options?.TimeRatioRules || '[]';
    setRules(nextRules);
    formApiRef.current?.setValues?.({ TimeRatioRules: nextRules });
  }, [options]);

  useEffect(() => {
    if (!previewModel && modelOptions.length > 0) {
      setPreviewModel(modelOptions[0].value);
    }
    if (
      previewGroup === 'default' &&
      groupOptions.length > 0 &&
      !groupOptions.some((item) => item.value === 'default')
    ) {
      setPreviewGroup(groupOptions[0].value);
    }
  }, [modelOptions, groupOptions, previewModel, previewGroup]);

  const validateRules = (value) => {
    const raw = value && value.trim() ? value : '[]';
    if (!verifyJSON(raw)) {
      return false;
    }
    try {
      const parsed = JSON.parse(raw);
      return Array.isArray(parsed);
    } catch {
      return false;
    }
  };

  const onSubmit = async () => {
    try {
      if (formApiRef.current?.validate) {
        await formApiRef.current.validate();
      } else if (!validateRules(rules)) {
        throw new Error('invalid rules');
      }
    } catch {
      showError(t('请检查输入'));
      return;
    }

    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'TimeRatioRules',
        value: rules,
      });
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('保存成功'));
      refresh?.();
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const loadPreview = useCallback(
    async (
      model = previewModel,
      group = previewGroup,
      userGroup = previewUserGroup,
    ) => {
      const normalizedModel = String(model || '').trim();
      const normalizedGroup = String(group || '').trim();
      const normalizedUserGroup = String(userGroup || '').trim();
      if (!normalizedModel || !normalizedGroup) {
        setPreview(null);
        return;
      }

      setPreviewLoading(true);
      try {
        const params = new URLSearchParams({
          model: normalizedModel,
          group: normalizedGroup,
          user_group: normalizedUserGroup,
        });
        const res = await API.get(
          `/api/option/time_ratio_preview?${params.toString()}`,
        );
        if (!res.data.success) {
          showError(res.data.message || t('预览失败'));
          return;
        }
        setPreview(res.data.data || null);
      } catch (error) {
        showError(error?.message || t('预览失败'));
      } finally {
        setPreviewLoading(false);
      }
    },
    [previewModel, previewGroup, previewUserGroup, t],
  );

  useEffect(() => {
    if (!previewModel || !previewGroup) {
      setPreview(null);
      return;
    }
    const timer = setTimeout(() => {
      loadPreview(previewModel, previewGroup, previewUserGroup);
    }, 350);
    return () => clearTimeout(timer);
  }, [previewModel, previewGroup, previewUserGroup, loadPreview]);

  const previewRows = preview
    ? [
        { key: t('模型'), value: preview.model || '-' },
        { key: t('使用分组'), value: preview.group || '-' },
        { key: t('用户分组'), value: preview.user_group || t('未指定') },
        {
          key: t('当前时间倍率'),
          value: (
            <Tag color={preview.matched ? 'orange' : 'white'} shape='circle'>
              {formatTimeRatioValue(preview.ratio)}x
            </Tag>
          ),
        },
        {
          key: t('命中规则'),
          value: preview.no_discount
            ? t('不参与折扣，按模型基础价计费')
            : preview.matched
              ? preview.rule_id || t('未命名规则')
              : t('未命中，使用 1x'),
        },
        { key: t('规则时区'), value: preview.timezone || '-' },
        {
          key: t('匹配时间'),
          value: preview.matched_at || preview.checked_at || '-',
        },
      ]
    : [];

  return (
    <Card>
      <Paragraph>
        {t(
          '时间倍率会在请求开始时按规则命中并冻结，后续预扣费、结算和日志都会使用同一个倍率。',
        )}
      </Paragraph>
      <Paragraph>
        <Text type='tertiary'>
          {t(
            '规则按 priority 从高到低匹配；start/end 使用 HH:MM；支持跨午夜；models/groups/user_groups 支持 * 通配。',
          )}
        </Text>
      </Paragraph>
      <Card
        title={t('当前规则预览')}
        bordered={false}
        style={{ marginBottom: 16, background: 'var(--semi-color-fill-0)' }}
      >
        <Paragraph>
          <Text type='tertiary'>
            {t(
              '选择模型和使用分组后，会按当前服务器时间和已保存规则展示实际命中的时间倍率规则。',
            )}
          </Text>
        </Paragraph>
        <Space wrap align='end' spacing={12} style={{ marginBottom: 12 }}>
          <div>
            <Text strong>{t('模型')}</Text>
            <Select
              allowCreate
              filter
              showClear
              value={previewModel}
              optionList={modelOptions}
              placeholder={t('选择或输入模型名')}
              style={{ width: 260, marginTop: 6 }}
              onChange={(value) => setPreviewModel(String(value || ''))}
            />
          </div>
          <div>
            <Text strong>{t('使用分组')}</Text>
            <Select
              allowCreate
              filter
              showClear
              value={previewGroup}
              optionList={groupOptions}
              placeholder={t('选择或输入分组')}
              style={{ width: 180, marginTop: 6 }}
              onChange={(value) => setPreviewGroup(String(value || ''))}
            />
          </div>
          <div>
            <Text strong>{t('用户分组')}</Text>
            <Select
              allowCreate
              filter
              showClear
              value={previewUserGroup}
              optionList={groupOptions}
              placeholder={t('可选')}
              style={{ width: 180, marginTop: 6 }}
              onChange={(value) => setPreviewUserGroup(String(value || ''))}
            />
          </div>
          <Button
            loading={previewLoading}
            disabled={!previewModel || !previewGroup}
            onClick={() => loadPreview()}
          >
            {t('刷新预览')}
          </Button>
        </Space>
        {previewRows.length > 0 ? (
          <Descriptions data={previewRows} />
        ) : (
          <Text type='tertiary'>{t('请先选择模型和使用分组')}</Text>
        )}
      </Card>
      <Form
        getFormApi={(formApi) => (formApiRef.current = formApi)}
        initValues={{ TimeRatioRules: rules }}
      >
        <Form.TextArea
          field='TimeRatioRules'
          label={t('时间倍率规则')}
          autosize={{ minRows: 16, maxRows: 30 }}
          placeholder={DEFAULT_EXAMPLE}
          rules={[
            {
              validator: (rule, value) => validateRules(value),
              message: t('必须是合法的 JSON 数组'),
            },
          ]}
          onChange={(value) => setRules(value)}
        />
        <Button theme='solid' loading={loading} onClick={onSubmit}>
          {t('保存时间倍率设置')}
        </Button>
      </Form>
    </Card>
  );
}
