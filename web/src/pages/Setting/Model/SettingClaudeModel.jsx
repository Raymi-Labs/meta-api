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

import React, { useEffect, useState, useRef } from 'react';
import { Button, Col, Form, Row, Select, Spin } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import Text from '@douyinfe/semi-ui/lib/es/typography/text';

const CLAUDE_HEADER = {
  'claude-3-7-sonnet-20250219': {
    'anthropic-beta': ['your-beta-feature-token'],
  },
};

const CLAUDE_HEADER_APPEND_BEFORE = `anthropic-beta: existing-feature-token`;

const CLAUDE_HEADER_APPEND_AFTER = `anthropic-beta: existing-feature-token,your-beta-feature-token`;

const CLAUDE_DEFAULT_MAX_TOKENS = {
  default: 8192,
  'claude-3-haiku-20240307': 4096,
  'claude-3-opus-20240229': 4096,
  'claude-3-7-sonnet-20250219-thinking': 8192,
};

const DEFAULT_CLAUDE_TO_OPENAI_REASONING_MAP = {
  low: 'low',
  medium: 'medium',
  high: 'high',
  max: 'xhigh',
};

const OPENAI_REASONING_EFFORT_OPTIONS = [
  { label: 'minimal', value: 'minimal' },
  { label: 'low', value: 'low' },
  { label: 'medium', value: 'medium' },
  { label: 'high', value: 'high' },
  { label: 'xhigh', value: 'xhigh' },
  { label: 'max', value: 'max' },
];

const ALLOWED_OPENAI_REASONING_EFFORTS = new Set(
  OPENAI_REASONING_EFFORT_OPTIONS.map((option) => option.value),
);

const defaultClaudeSettingInputs = {
  'claude.model_headers_settings': '',
  'claude.thinking_adapter_enabled': true,
  'claude.default_max_tokens': '',
  'claude.thinking_adapter_budget_tokens_percentage': 0.8,
  ClaudeToOpenAIReasoningMap: JSON.stringify(
    DEFAULT_CLAUDE_TO_OPENAI_REASONING_MAP,
  ),
};

const normalizeClaudeToOpenAIReasoningMap = (raw) => {
  const normalized = { ...DEFAULT_CLAUDE_TO_OPENAI_REASONING_MAP };
  if (!raw || String(raw).trim() === '') {
    return normalized;
  }

  try {
    const parsed = JSON.parse(raw);
    for (const key of Object.keys(normalized)) {
      const value = parsed?.[key];
      if (ALLOWED_OPENAI_REASONING_EFFORTS.has(value)) {
        normalized[key] = value;
      }
    }
  } catch (error) {
    console.error('Invalid ClaudeToOpenAIReasoningMap:', error);
  }

  return normalized;
};

const stringifyClaudeToOpenAIReasoningMap = (mapping) =>
  JSON.stringify({
    low: mapping.low,
    medium: mapping.medium,
    high: mapping.high,
    max: mapping.max,
  });

export default function SettingClaudeModel(props) {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(defaultClaudeSettingInputs);
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(defaultClaudeSettingInputs);
  const [reasoningMap, setReasoningMap] = useState(
    DEFAULT_CLAUDE_TO_OPENAI_REASONING_MAP,
  );

  const updateReasoningMap = (level, value) => {
    setReasoningMap((prev) => {
      const next = {
        ...prev,
        [level]: value,
      };
      setInputs((current) => ({
        ...current,
        ClaudeToOpenAIReasoningMap: stringifyClaudeToOpenAIReasoningMap(next),
      }));
      return next;
    });
  };

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = String(inputs[item.key]);

      return API.put('/api/option/', {
        key: item.key,
        value,
      });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = { ...defaultClaudeSettingInputs };
    for (const key of Object.keys(defaultClaudeSettingInputs)) {
      if (props.options[key] !== undefined) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    setReasoningMap(
      normalizeClaudeToOpenAIReasoningMap(
        currentInputs.ClaudeToOpenAIReasoningMap,
      ),
    );
    if (refForm.current) {
      refForm.current.setValues(currentInputs);
    }
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('Claude设置')}>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.TextArea
                  label={t('Claude请求头追加')}
                  field={'claude.model_headers_settings'}
                  placeholder={
                    t('为一个 JSON 文本，例如：') +
                    '\n' +
                    JSON.stringify(CLAUDE_HEADER, null, 2)
                  }
                  extraText={
                    <div>
                      <div>
                        {t(
                          'Claude 会在已有请求头基础上合并并追加这些值，不会覆盖已有 anthropic-beta；重复值会自动去重。',
                        )}
                      </div>
                      <div className='mt-1'>
                        {t(
                          '注意：Claude Sonnet 4.6 / Opus 4.6+ 的 1M context 已不需要再追加 context-1m-2025-08-07。',
                        )}
                      </div>
                      <div className='mt-2 whitespace-pre-wrap font-mono text-xs'>
                        {`${t('前：')}\n${CLAUDE_HEADER_APPEND_BEFORE}\n\n${t('配置：')}\n${JSON.stringify(
                          CLAUDE_HEADER,
                          null,
                          2,
                        )}\n\n${t('后：')}\n${CLAUDE_HEADER_APPEND_AFTER}`}
                      </div>
                    </div>
                  }
                  autosize={{ minRows: 6, maxRows: 12 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'claude.model_headers_settings': value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.TextArea
                  label={t('缺省 MaxTokens')}
                  field={'claude.default_max_tokens'}
                  placeholder={
                    t('为一个 JSON 文本，例如：') +
                    '\n' +
                    JSON.stringify(CLAUDE_DEFAULT_MAX_TOKENS, null, 2)
                  }
                  extraText={
                    t('示例') +
                    '\n' +
                    JSON.stringify(CLAUDE_DEFAULT_MAX_TOKENS, null, 2)
                  }
                  autosize={{ minRows: 6, maxRows: 12 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  onChange={(value) =>
                    setInputs({ ...inputs, 'claude.default_max_tokens': value })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col span={16}>
                <Form.Switch
                  label={t('启用Claude思考适配（-thinking后缀）')}
                  field={'claude.thinking_adapter_enabled'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'claude.thinking_adapter_enabled': value,
                    })
                  }
                />
              </Col>
            </Row>
            <Form.Section
              text={
                <span style={{ fontSize: 14, fontWeight: 600 }}>
                  {t('Claude -> OpenAI 思考模式映射')}
                </span>
              }
            >
              <Row>
                <Col span={24}>
                  <Text>
                    {t(
                      '将 Claude 的思考档位映射为 OpenAI 的 reasoning_effort，用于 Claude 请求转换为 OpenAI 请求时。',
                    )}
                  </Text>
                </Col>
              </Row>
              <Row>
                <Col xs={24} sm={12} md={6} lg={6} xl={6}>
                  <Form.Slot label='low'>
                    <Select
                      style={{ width: '100%' }}
                      value={reasoningMap.low}
                      optionList={OPENAI_REASONING_EFFORT_OPTIONS}
                      onChange={(value) => updateReasoningMap('low', value)}
                    />
                  </Form.Slot>
                </Col>
                <Col xs={24} sm={12} md={6} lg={6} xl={6}>
                  <Form.Slot label='medium'>
                    <Select
                      style={{ width: '100%' }}
                      value={reasoningMap.medium}
                      optionList={OPENAI_REASONING_EFFORT_OPTIONS}
                      onChange={(value) => updateReasoningMap('medium', value)}
                    />
                  </Form.Slot>
                </Col>
                <Col xs={24} sm={12} md={6} lg={6} xl={6}>
                  <Form.Slot label='high'>
                    <Select
                      style={{ width: '100%' }}
                      value={reasoningMap.high}
                      optionList={OPENAI_REASONING_EFFORT_OPTIONS}
                      onChange={(value) => updateReasoningMap('high', value)}
                    />
                  </Form.Slot>
                </Col>
                <Col xs={24} sm={12} md={6} lg={6} xl={6}>
                  <Form.Slot label='max'>
                    <Select
                      style={{ width: '100%' }}
                      value={reasoningMap.max}
                      optionList={OPENAI_REASONING_EFFORT_OPTIONS}
                      onChange={(value) => updateReasoningMap('max', value)}
                    />
                  </Form.Slot>
                </Col>
              </Row>
            </Form.Section>
            <Row>
              <Col span={16}>
                <Text>{t('计算公式：BudgetTokens = MaxTokens × 百分比')}</Text>
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('思考适配 BudgetTokens 百分比')}
                  field={'claude.thinking_adapter_budget_tokens_percentage'}
                  initValue={''}
                  extraText={t('填写 0.1 以上的小数')}
                  min={0.1}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'claude.thinking_adapter_budget_tokens_percentage': value,
                    })
                  }
                />
              </Col>
            </Row>

            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
