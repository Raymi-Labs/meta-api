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
import { Button, Col, Form, Row, Spin, Switch } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  parseHttpStatusCodeRules,
  toBoolean,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import HttpStatusCodeRulesInput from '../../../components/settings/HttpStatusCodeRulesInput';

export default function SettingsMonitoring(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ChannelDisableThreshold: '',
    QuotaRemindThreshold: '',
    AutomaticDisableChannelEnabled: false,
    AutomaticEnableChannelEnabled: false,
    AutomaticDisableKeywords: '',
    AutomaticDisableStatusCodes: '401',
    AutomaticRetryStatusCodes:
      '100-199,300-399,401-407,409-499,500-503,505-523,525-599',
    UserScopedCircuitBreakerEnabled: false,
    UserScopedCircuitBreakerStatusCodes: '503',
    UserScopedCircuitBreakerTTLSeconds: 60,
    UserScopedCircuitBreakerFailureThreshold: 2,
    'monitor_setting.auto_test_channel_enabled': false,
    'monitor_setting.auto_test_channel_minutes': 30,
    'monitor_setting.pre_disable_wait_enabled': false,
    'monitor_setting.pre_disable_wait_minutes': 10,
    'monitor_setting.pool_exhausted_notify_enabled': false,
    'monitor_setting.pool_exhausted_notify_emails': '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const parsedAutoDisableStatusCodes = parseHttpStatusCodeRules(
    inputs.AutomaticDisableStatusCodes || '',
  );
  const parsedAutoRetryStatusCodes = parseHttpStatusCodeRules(
    inputs.AutomaticRetryStatusCodes || '',
  );
  const parsedUserScopedCircuitBreakerStatusCodes = parseHttpStatusCodeRules(
    inputs.UserScopedCircuitBreakerStatusCodes || '',
  );

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    if (!parsedAutoDisableStatusCodes.ok) {
      const details =
        parsedAutoDisableStatusCodes.invalidTokens &&
        parsedAutoDisableStatusCodes.invalidTokens.length > 0
          ? `: ${parsedAutoDisableStatusCodes.invalidTokens.join(', ')}`
          : '';
      return showError(`${t('自动禁用状态码格式不正确')}${details}`);
    }
    if (!parsedAutoRetryStatusCodes.ok) {
      const details =
        parsedAutoRetryStatusCodes.invalidTokens &&
        parsedAutoRetryStatusCodes.invalidTokens.length > 0
          ? `: ${parsedAutoRetryStatusCodes.invalidTokens.join(', ')}`
          : '';
      return showError(`${t('自动重试状态码格式不正确')}${details}`);
    }
    if (!parsedUserScopedCircuitBreakerStatusCodes.ok) {
      const details =
        parsedUserScopedCircuitBreakerStatusCodes.invalidTokens &&
        parsedUserScopedCircuitBreakerStatusCodes.invalidTokens.length > 0
          ? `: ${parsedUserScopedCircuitBreakerStatusCodes.invalidTokens.join(', ')}`
          : '';
      return showError(`${t('用户维度短熔断状态码格式不正确')}${details}`);
    }
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        const normalizedMap = {
          AutomaticDisableStatusCodes: parsedAutoDisableStatusCodes.normalized,
          AutomaticRetryStatusCodes: parsedAutoRetryStatusCodes.normalized,
          UserScopedCircuitBreakerStatusCodes:
            parsedUserScopedCircuitBreakerStatusCodes.normalized,
        };
        value = normalizedMap[item.key] ?? inputs[item.key];
      }
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
    const currentInputs = { ...inputs };
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        if (
          key === 'AutomaticDisableChannelEnabled' ||
          key === 'AutomaticEnableChannelEnabled' ||
          key === 'UserScopedCircuitBreakerEnabled' ||
          key === 'monitor_setting.auto_test_channel_enabled' ||
          key === 'monitor_setting.pre_disable_wait_enabled' ||
          key === 'monitor_setting.pool_exhausted_notify_enabled'
        ) {
          currentInputs[key] = toBoolean(props.options[key]);
        } else if (
          key === 'ChannelDisableThreshold' ||
          key === 'QuotaRemindThreshold' ||
          key === 'UserScopedCircuitBreakerTTLSeconds' ||
          key === 'UserScopedCircuitBreakerFailureThreshold' ||
          key === 'monitor_setting.auto_test_channel_minutes' ||
          key === 'monitor_setting.pre_disable_wait_minutes'
        ) {
          currentInputs[key] = Number(props.options[key] ?? currentInputs[key]);
        } else {
          currentInputs[key] = props.options[key];
        }
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('监控设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'monitor_setting.auto_test_channel_enabled'}
                  label={t('定时测试所有通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.auto_test_channel_enabled': value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('自动测试所有通道间隔时间')}
                  step={1}
                  min={1}
                  suffix={t('分钟')}
                  extraText={t('每隔多少分钟测试一次所有通道')}
                  placeholder={''}
                  field={'monitor_setting.auto_test_channel_minutes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.auto_test_channel_minutes':
                        parseInt(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('测试所有渠道的最长响应时间')}
                  step={1}
                  min={0}
                  suffix={t('秒')}
                  extraText={t(
                    '当运行通道全部测试时，超过此时间将自动禁用通道',
                  )}
                  placeholder={''}
                  field={'ChannelDisableThreshold'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelDisableThreshold: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'flex-start',
                    gap: 8,
                    width: '100%',
                  }}
                >
                  <Form.InputNumber
                    label={
                      <span style={{ whiteSpace: 'nowrap' }}>
                        {t('禁用前等待时间')}
                      </span>
                    }
                    step={1}
                    min={1}
                    suffix={t('分钟')}
                    disabled={
                      !inputs['monitor_setting.pre_disable_wait_enabled']
                    }
                    extraText={t('触发自动禁用后，会等待并再测试一次')}
                    placeholder={''}
                    field={'monitor_setting.pre_disable_wait_minutes'}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'monitor_setting.pre_disable_wait_minutes':
                          parseInt(value),
                      })
                    }
                  />
                  <Switch
                    checked={inputs['monitor_setting.pre_disable_wait_enabled']}
                    size='default'
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'monitor_setting.pre_disable_wait_enabled': value,
                      })
                    }
                  />
                </div>
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('额度提醒阈值')}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  extraText={t('低于此额度时将发送邮件提醒用户')}
                  placeholder={''}
                  field={'QuotaRemindThreshold'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      QuotaRemindThreshold: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'AutomaticDisableChannelEnabled'}
                  label={t('失败时自动禁用通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      AutomaticDisableChannelEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'AutomaticEnableChannelEnabled'}
                  label={t('成功时自动启用通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      AutomaticEnableChannelEnabled: value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'monitor_setting.pool_exhausted_notify_enabled'}
                  label={t('账号池耗尽时通知管理员')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  extraText={t(
                    '多Key渠道的全部Key进入冷却或禁用时发送提醒（同一渠道6小时内最多一次）',
                  )}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.pool_exhausted_notify_enabled': value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={16} lg={16} xl={16}>
                <Form.TextArea
                  label={t('账号池耗尽通知邮箱')}
                  placeholder={'admin1@example.com, admin2@example.com'}
                  extraText={t(
                    '支持多个邮箱，使用逗号、分号或换行分隔；留空则使用超级管理员的通知方式',
                  )}
                  field={'monitor_setting.pool_exhausted_notify_emails'}
                  disabled={
                    !inputs['monitor_setting.pool_exhausted_notify_enabled']
                  }
                  autosize={{ minRows: 1, maxRows: 4 }}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.pool_exhausted_notify_emails': value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <HttpStatusCodeRulesInput
                  label={t('自动禁用状态码')}
                  placeholder={t('例如：401, 403, 429, 500-599')}
                  extraText={t(
                    '支持填写单个状态码或范围（含首尾），使用逗号分隔',
                  )}
                  field={'AutomaticDisableStatusCodes'}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticDisableStatusCodes: value })
                  }
                  parsed={parsedAutoDisableStatusCodes}
                  invalidText={t('自动禁用状态码格式不正确')}
                />
                <HttpStatusCodeRulesInput
                  label={t('自动重试状态码')}
                  placeholder={t('例如：401, 403, 429, 500-599')}
                  extraText={t(
                    '支持填写单个状态码或范围（含首尾），使用逗号分隔；504 和 524 始终不重试，不受此处配置影响',
                  )}
                  field={'AutomaticRetryStatusCodes'}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticRetryStatusCodes: value })
                  }
                  parsed={parsedAutoRetryStatusCodes}
                  invalidText={t('自动重试状态码格式不正确')}
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'UserScopedCircuitBreakerEnabled'}
                  label={t('用户维度短熔断')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  extraText={t(
                    '按用户ID + 渠道标签隔离短时过载，避免一个用户的瞬时 503 影响所有用户',
                  )}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      UserScopedCircuitBreakerEnabled: value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('短熔断 TTL')}
                  step={1}
                  min={1}
                  suffix={t('秒')}
                  disabled={!inputs.UserScopedCircuitBreakerEnabled}
                  extraText={t('命中后暂停该用户使用该标签渠道的秒数')}
                  field={'UserScopedCircuitBreakerTTLSeconds'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      UserScopedCircuitBreakerTTLSeconds: parseInt(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('短熔断连续失败次数')}
                  step={1}
                  min={1}
                  disabled={!inputs.UserScopedCircuitBreakerEnabled}
                  extraText={t('达到该次数后才开启短熔断，避免单次抖动误伤')}
                  field={'UserScopedCircuitBreakerFailureThreshold'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      UserScopedCircuitBreakerFailureThreshold: parseInt(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <HttpStatusCodeRulesInput
                  label={t('用户维度短熔断状态码')}
                  placeholder={t('例如：503, 502-503')}
                  extraText={t(
                    '这些 HTTP 状态码会按用户ID + 渠道标签记录连续失败并短时熔断，不会自动禁用渠道',
                  )}
                  field={'UserScopedCircuitBreakerStatusCodes'}
                  disabled={!inputs.UserScopedCircuitBreakerEnabled}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      UserScopedCircuitBreakerStatusCodes: value,
                    })
                  }
                  parsed={parsedUserScopedCircuitBreakerStatusCodes}
                  invalidText={t('用户维度短熔断状态码格式不正确')}
                />
                <Form.TextArea
                  label={t('自动禁用关键词')}
                  placeholder={t('一行一个，不区分大小写')}
                  extraText={t(
                    '当上游通道返回错误中包含这些关键词时（不区分大小写），自动禁用通道',
                  )}
                  field={'AutomaticDisableKeywords'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticDisableKeywords: value })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存监控设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
