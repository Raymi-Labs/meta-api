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
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';

export default function SettingsCreditLimit(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    QuotaForNewUser: '',
    PreConsumedQuota: '',
    QuotaForInviter: '',
    QuotaForInvitee: '',
    InviterCommissionEnabled: false,
    InviterRechargeCommissionRate: '',
    InviterRechargeSecondLevelCommissionRate: '',
    InviterCommissionDailyCap: '',
    InvoiceServiceFeeRate: '',
    'quota_setting.enable_free_model_pre_consume': true,
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const inviterCommissionRate = Number.parseFloat(
    inputs.InviterRechargeCommissionRate,
  );
  const inviterCommissionPercent = Number.isFinite(inviterCommissionRate)
    ? (inviterCommissionRate * 100).toLocaleString(undefined, {
        maximumFractionDigits: 2,
      })
    : '0';
  const inviterSecondLevelCommissionRate = Number.parseFloat(
    inputs.InviterRechargeSecondLevelCommissionRate,
  );
  const inviterSecondLevelCommissionPercent = Number.isFinite(
    inviterSecondLevelCommissionRate,
  )
    ? (inviterSecondLevelCommissionRate * 100).toLocaleString(undefined, {
        maximumFractionDigits: 2,
      })
    : '0';
  const invoiceFeeRate = Number.parseFloat(inputs.InvoiceServiceFeeRate);
  const invoiceFeePercent = Number.isFinite(invoiceFeeRate)
    ? (invoiceFeeRate * 100).toLocaleString(undefined, {
        maximumFractionDigits: 2,
      })
    : '0';

  async function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    const firstLevelRate = Number(inputs.InviterRechargeCommissionRate);
    const secondLevelRate = Number(
      inputs.InviterRechargeSecondLevelCommissionRate,
    );
    if (
      !Number.isFinite(firstLevelRate) ||
      !Number.isFinite(secondLevelRate) ||
      firstLevelRate < 0 ||
      firstLevelRate > 1 ||
      secondLevelRate < 0 ||
      secondLevelRate > 1 ||
      firstLevelRate + secondLevelRate > 1
    ) {
      return showError(
        t('一级和二级返佣比例必须分别在 0 到 1 之间，且合计不能超过 1'),
      );
    }

    const rateKeys = new Set([
      'InviterRechargeCommissionRate',
      'InviterRechargeSecondLevelCommissionRate',
    ]);
    const makeRequest = async (item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        value = inputs[item.key];
      }
      const response = await API.put('/api/option/', {
        key: item.key,
        value,
      });
      if (!response?.data?.success) {
        throw new Error(response?.data?.message || t('保存失败，请重试'));
      }
      return response;
    };
    // 当一级下降、二级上升（或反向）时先保存下降项，避免中间态短暂超过 100%。
    const rateUpdates = updateArray
      .filter((item) => rateKeys.has(item.key))
      .sort((left, right) => {
        const leftDelta =
          Number(inputs[left.key]) - Number(inputsRow[left.key] || 0);
        const rightDelta =
          Number(inputs[right.key]) - Number(inputsRow[right.key] || 0);
        return leftDelta - rightDelta;
      });
    const otherUpdates = updateArray.filter((item) => !rateKeys.has(item.key));
    setLoading(true);
    try {
      for (const item of rateUpdates) {
        await makeRequest(item);
      }
      await Promise.all(otherUpdates.map(makeRequest));
      showSuccess(t('保存成功'));
      props.refresh();
    } catch (error) {
      showError(error?.message || t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    if (currentInputs.InviterRechargeSecondLevelCommissionRate === undefined) {
      currentInputs.InviterRechargeSecondLevelCommissionRate = '0';
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
          <Form.Section text={t('额度设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('新用户初始额度')}
                  field={'QuotaForNewUser'}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  placeholder={''}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      QuotaForNewUser: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('请求预扣费额度')}
                  field={'PreConsumedQuota'}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  extraText={t('请求结束后多退少补')}
                  placeholder={''}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      PreConsumedQuota: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('邀请新用户奖励额度')}
                  field={'QuotaForInviter'}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  extraText={''}
                  placeholder={t('例如：2000')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      QuotaForInviter: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={6}>
                <Form.InputNumber
                  label={t('新用户使用邀请码奖励额度')}
                  field={'QuotaForInvitee'}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  extraText={''}
                  placeholder={t('例如：1000')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      QuotaForInvitee: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={6}>
                <Form.InputNumber
                  label={t('一级邀请充值返佣比例')}
                  field={'InviterRechargeCommissionRate'}
                  step={0.01}
                  min={0}
                  max={1}
                  extraText={t(
                    '按实付金额计算，当前比例 {{ratePercent}}%，支付成功后自动结算',
                    {
                      ratePercent: inviterCommissionPercent,
                    },
                  )}
                  placeholder={t('例如：0.1')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      InviterRechargeCommissionRate: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={6}>
                <Form.InputNumber
                  label={t('二级邀请充值返佣比例')}
                  field={'InviterRechargeSecondLevelCommissionRate'}
                  step={0.01}
                  min={0}
                  max={1}
                  extraText={t(
                    '下级成功绑定后，其下级自动继承为二级关系；当前比例 {{ratePercent}}%，不重复抽取绑定概率',
                    {
                      ratePercent: inviterSecondLevelCommissionPercent,
                    },
                  )}
                  placeholder={t('例如：0.05')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      InviterRechargeSecondLevelCommissionRate: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={6}>
                <Form.InputNumber
                  label={t('邀请返佣单日上限')}
                  field={'InviterCommissionDailyCap'}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  extraText={t('0 表示不限制')}
                  placeholder={t('例如：500000')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      InviterCommissionDailyCap: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={6}>
                <Form.InputNumber
                  label={t('发票申请手续费费率')}
                  field={'InvoiceServiceFeeRate'}
                  step={0.01}
                  min={0}
                  max={1}
                  extraText={t(
                    '按发票开票金额计算，申请时从钱包额度扣除，当前比例 {{ratePercent}}%',
                    {
                      ratePercent: invoiceFeePercent,
                    },
                  )}
                  placeholder={t('例如：0.01')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      InvoiceServiceFeeRate: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col>
                <Form.Switch
                  label={t('启用邀请充值返佣')}
                  field={'InviterCommissionEnabled'}
                  extraText={t('仅统计成功充值订单，注册赠送额度不计入返佣')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      InviterCommissionEnabled: value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col>
                <Form.Switch
                  label={t('对免费模型启用预消耗')}
                  field={'quota_setting.enable_free_model_pre_consume'}
                  extraText={t(
                    '开启后，对免费模型（倍率为0，或者价格为0）的模型也会预消耗额度',
                  )}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'quota_setting.enable_free_model_pre_consume': value,
                    })
                  }
                />
              </Col>
            </Row>

            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存额度设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
