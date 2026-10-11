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

import React, { useEffect, useMemo, useState, useRef } from 'react';
import {
  Banner,
  Button,
  Form,
  Row,
  Col,
  Typography,
  Spin,
} from '@douyinfe/semi-ui';
const { Text } = Typography;
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import PaymentMethodIcon from '../../../components/topup/PaymentMethodIcon';

export default function SettingsPaymentGateway(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    StripeApiSecret: '',
    StripeWebhookSecret: '',
    StripePriceId: '',
    StripeUnitPrice: 8.0,
    StripeMinTopUp: 1,
    StripePromotionCodesEnabled: false,
    StripeDisplayName: '',
    StripeDisplayIcon: '',
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);
  const legacyMethod = useMemo(() => {
    try {
      const methods = Array.isArray(props.options?.PayMethods)
        ? props.options.PayMethods
        : JSON.parse(props.options?.PayMethods || '[]');
      return methods.find((method) => method?.type === 'stripe') || {};
    } catch {
      return {};
    }
  }, [props.options?.PayMethods]);
  const previewMethod = {
    ...legacyMethod,
    type: 'stripe',
    name:
      inputs.StripeDisplayName?.trim() || legacyMethod.name?.trim() || 'Stripe',
    display_icon: inputs.StripeDisplayIcon || legacyMethod.display_icon || '',
  };

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        StripeApiSecret: props.options.StripeApiSecret || '',
        StripeWebhookSecret: props.options.StripeWebhookSecret || '',
        StripePriceId: props.options.StripePriceId || '',
        StripeDisplayName: props.options.StripeDisplayName || '',
        StripeDisplayIcon: props.options.StripeDisplayIcon || '',
        StripeUnitPrice:
          props.options.StripeUnitPrice !== undefined
            ? parseFloat(props.options.StripeUnitPrice)
            : 8.0,
        StripeMinTopUp:
          props.options.StripeMinTopUp !== undefined
            ? parseFloat(props.options.StripeMinTopUp)
            : 1,
        StripePromotionCodesEnabled:
          props.options.StripePromotionCodesEnabled !== undefined
            ? props.options.StripePromotionCodesEnabled
            : false,
      };
      setInputs(currentInputs);
      setOriginInputs({ ...currentInputs });
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  const submitStripeSetting = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    const values = formApiRef.current?.getValues() || inputs;
    setLoading(true);
    try {
      const options = [];

      for (const key of ['StripeDisplayName', 'StripeDisplayIcon']) {
        const value = (values[key] || '').trim();
        if (value !== (originInputs[key] || '')) {
          options.push({ key, value });
        }
      }

      if (values.StripeApiSecret && values.StripeApiSecret !== '') {
        options.push({ key: 'StripeApiSecret', value: values.StripeApiSecret });
      }
      if (values.StripeWebhookSecret && values.StripeWebhookSecret !== '') {
        options.push({
          key: 'StripeWebhookSecret',
          value: values.StripeWebhookSecret,
        });
      }
      if (values.StripePriceId !== '') {
        options.push({ key: 'StripePriceId', value: values.StripePriceId });
      }
      if (
        values.StripeUnitPrice !== undefined &&
        values.StripeUnitPrice !== null
      ) {
        options.push({
          key: 'StripeUnitPrice',
          value: values.StripeUnitPrice.toString(),
        });
      }
      if (
        values.StripeMinTopUp !== undefined &&
        values.StripeMinTopUp !== null
      ) {
        options.push({
          key: 'StripeMinTopUp',
          value: values.StripeMinTopUp.toString(),
        });
      }
      if (
        originInputs['StripePromotionCodesEnabled'] !==
          values.StripePromotionCodesEnabled &&
        values.StripePromotionCodesEnabled !== undefined
      ) {
        options.push({
          key: 'StripePromotionCodesEnabled',
          value: values.StripePromotionCodesEnabled ? 'true' : 'false',
        });
      }

      // 发送请求
      const requestQueue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );

      const results = await Promise.all(requestQueue);

      // 检查所有请求是否成功
      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        // 更新本地存储的原始值
        setOriginInputs({ ...values });
        props.refresh?.();
      }
    } catch (error) {
      showError(t('更新失败'));
    }
    setLoading(false);
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={t('Stripe 设置')}>
          <Text>
            Stripe 密钥、Webhook 等设置请
            <a
              href='https://dashboard.stripe.com/developers'
              target='_blank'
              rel='noreferrer'
            >
              点击此处
            </a>
            进行设置，最好先在
            <a
              href='https://dashboard.stripe.com/test/developers'
              target='_blank'
              rel='noreferrer'
            >
              测试环境
            </a>
            进行测试。
            <br />
          </Text>
          <Banner
            type='info'
            description={`Webhook 填：${props.options.ServerAddress ? removeTrailingSlash(props.options.ServerAddress) : t('网站地址')}/api/stripe/webhook`}
          />
          <Banner
            type='warning'
            description={`需要包含事件：checkout.session.completed 和 checkout.session.expired`}
          />
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} md={12}>
              <Form.Input
                field='StripeDisplayName'
                label={t('充值入口显示名称')}
                placeholder={t('例如：微信支付；留空沿用现有名称')}
                maxLength={40}
                showClear
              />
            </Col>
            <Col xs={24} md={12}>
              <Form.Select
                field='StripeDisplayIcon'
                label={t('充值入口显示图标')}
                placeholder={t('沿用现有图标')}
                style={{ width: '100%' }}
                optionList={[
                  { label: t('沿用现有图标'), value: '' },
                  { label: 'Stripe', value: 'stripe' },
                  { label: t('微信支付'), value: 'wxpay' },
                  { label: t('支付宝'), value: 'alipay' },
                  { label: t('银行卡'), value: 'card' },
                ]}
              />
            </Col>
          </Row>
          <div className='mb-4 space-y-2'>
            <div className='flex items-center gap-2'>
              <Text type='tertiary'>{t('用户端预览')}：</Text>
              <PaymentMethodIcon method={previewMethod} />
              <Text>{previewMethod.name}</Text>
            </div>
            <Text type='tertiary' size='small'>
              {t(
                '仅用于余额充值入口和确认页。实际支付方式由 Stripe 后台决定；显示为微信支付前，请确认结账页已支持微信支付。',
              )}
            </Text>
          </div>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='StripeApiSecret'
                label={t('API 密钥')}
                placeholder={t(
                  'sk_xxx 或 rk_xxx 的 Stripe 密钥，敏感信息不显示',
                )}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='StripeWebhookSecret'
                label={t('Webhook 签名密钥')}
                placeholder={t('whsec_xxx 的 Webhook 签名密钥，敏感信息不显示')}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='StripePriceId'
                label={t('商品价格 ID')}
                placeholder={t('price_xxx 的商品价格 ID，新建产品后可获得')}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='StripeUnitPrice'
                precision={2}
                label={t('充值价格（x元/美金）')}
                placeholder={t('例如：7，就是7元/美金')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='StripeMinTopUp'
                label={t('最低充值美元数量')}
                placeholder={t('例如：2，就是最低充值2$')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='StripePromotionCodesEnabled'
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                label={t('允许在 Stripe 支付中输入促销码')}
              />
            </Col>
          </Row>
          <Button onClick={submitStripeSetting}>{t('更新 Stripe 设置')}</Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
