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
import { useTranslation } from 'react-i18next';
import {
  API,
  downloadTextAsFile,
  showError,
  showSuccess,
  renderQuota,
  renderQuotaWithPrompt,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  Modal,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Form,
  Avatar,
  Row,
  Col,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconSave,
  IconClose,
  IconGift,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const EditRedemptionModal = (props) => {
  const { t } = useTranslation();
  const isEdit = props.editingRedemption.id !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const sessionRef = useRef(0);
  const formValuesRef = useRef(null);
  const submittingRef = useRef(false);
  const refreshRef = useRef(props.refresh);
  refreshRef.current = props.refresh;
  const [readySession, setReadySession] = useState(null);
  const formKey = `${props.visiable}:${props.editingRedemption.id ?? 'new'}`;
  const canSubmit = props.visiable && readySession === sessionRef.current;

  const getInitValues = () => ({
    name: '',
    benefit_type: 'quota',
    quota: 100000,
    plan_id: 0,
    subscription_purchase_mode: 'stack',
    subscription_purchase_quantity: 1,
    count: 1,
    expired_time: null,
  });

  const handleCancel = () => {
    sessionRef.current += 1;
    setReadySession(null);
    props.handleClose();
  };

  const loadRedemption = async (session) => {
    try {
      const res = await API.get(
        `/api/redemption/${props.editingRedemption.id}`,
        {
          disableDuplicate: true,
          skipErrorHandler: true,
        },
      );
      if (session !== sessionRef.current) return;
      const { success, message, data } = res.data;
      if (success) {
        const values = {
          ...getInitValues(),
          ...data,
          benefit_type: data.benefit_type || 'quota',
          subscription_purchase_mode:
            data.subscription_purchase_mode === 'renew_extend'
              ? 'renew'
              : data.subscription_purchase_mode || 'stack',
          subscription_purchase_quantity:
            data.subscription_purchase_quantity || 1,
          expired_time: data.expired_time
            ? new Date(data.expired_time * 1000)
            : null,
        };
        formValuesRef.current = { key: formKey, values };
        setReadySession(session);
      } else {
        showError(message);
      }
    } catch (error) {
      if (session === sessionRef.current) {
        showError(error.response ? error : error.message || t('操作失败'));
      }
    } finally {
      if (session === sessionRef.current) setLoading(false);
    }
  };

  // 管理端创建套餐兑换码时需要展示可选套餐列表。
  const loadSubscriptionPlans = async () => {
    const res = await API.get('/api/subscription/admin/plans');
    const { success, data } = res.data || {};
    if (success) {
      // 管理端只关心真正的套餐对象，兼容后端包一层 DTO 的返回结构。
      setSubscriptionPlans((data || []).map((item) => item.plan || item));
    }
  };

  useEffect(() => {
    loadSubscriptionPlans().catch(() => {});
  }, []);

  useEffect(() => {
    const session = ++sessionRef.current;
    submittingRef.current = false;
    setReadySession(null);
    const values = getInitValues();
    formValuesRef.current = { key: formKey, values };
    setLoading(Boolean(props.visiable && isEdit));
    if (props.visiable) {
      if (isEdit) {
        loadRedemption(session);
      } else {
        setReadySession(session);
      }
    }
    return () => {
      sessionRef.current += 1;
    };
  }, [props.visiable, props.editingRedemption.id]);

  const submit = async (values) => {
    if (
      !canSubmit ||
      readySession !== sessionRef.current ||
      submittingRef.current
    )
      return;
    const session = sessionRef.current;
    submittingRef.current = true;
    let name = values.name;
    if (!isEdit && (!name || name === '')) {
      name = renderQuota(values.quota);
    }
    setLoading(true);
    let localInputs = { ...values };
    localInputs.count = parseInt(localInputs.count) || 0;
    localInputs.quota = parseInt(localInputs.quota) || 0;
    localInputs.plan_id = parseInt(localInputs.plan_id) || 0;
    localInputs.subscription_purchase_quantity =
      parseInt(localInputs.subscription_purchase_quantity) || 1;
    localInputs.name = name;
    localInputs.benefit_type = localInputs.benefit_type || 'quota';
    if (localInputs.benefit_type === 'subscription') {
      // 套餐码不允许再携带余额，避免一个码同时发两类权益。
      localInputs.quota = 0;
    } else {
      // 余额码统一清空套餐字段，避免编辑历史数据时残留脏值。
      localInputs.plan_id = 0;
      localInputs.subscription_purchase_mode = 'stack';
      localInputs.subscription_purchase_quantity = 1;
    }
    if (!localInputs.expired_time) {
      localInputs.expired_time = 0;
    } else {
      localInputs.expired_time = Math.floor(
        localInputs.expired_time.getTime() / 1000,
      );
    }
    try {
      let res;
      if (isEdit) {
        res = await API.put(
          `/api/redemption/`,
          {
            ...localInputs,
            id: parseInt(props.editingRedemption.id),
          },
          { skipErrorHandler: true },
        );
      } else {
        res = await API.post(
          `/api/redemption/`,
          { ...localInputs },
          { skipErrorHandler: true },
        );
      }
      const { success, message, data } = res.data;
      if (success) refreshRef.current();
      if (session !== sessionRef.current) return;
      if (success) {
        showSuccess(isEdit ? t('兑换码更新成功！') : t('兑换码创建成功！'));
        handleCancel();
      } else {
        showError(message);
      }
      if (success && !isEdit && data) {
        const text = data.map((code) => code + '\n').join('');
        Modal.confirm({
          title: t('兑换码创建成功'),
          content: (
            <div>
              <p>{t('兑换码创建成功，是否下载兑换码？')}</p>
              <p>{t('兑换码将以文本文件的形式下载，文件名为兑换码的名称。')}</p>
            </div>
          ),
          onOk: () => {
            downloadTextAsFile(text, `${localInputs.name}.txt`);
          },
        });
      }
    } catch (error) {
      if (session === sessionRef.current) {
        showError(error.response ? error : error.message || t('操作失败'));
      }
    } finally {
      if (session === sessionRef.current) {
        submittingRef.current = false;
        setLoading(false);
      }
    }
  };

  return (
    <>
      <SideSheet
        placement={isEdit ? 'right' : 'left'}
        title={
          <Space>
            {isEdit ? (
              <Tag color='blue' shape='circle'>
                {t('更新')}
              </Tag>
            ) : (
              <Tag color='green' shape='circle'>
                {t('新建')}
              </Tag>
            )}
            <Title heading={4} className='m-0'>
              {isEdit ? t('更新兑换码信息') : t('创建新的兑换码')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: '0' }}
        visible={props.visiable}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
                loading={loading}
                disabled={!canSubmit}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                type='primary'
                onClick={handleCancel}
                icon={<IconClose />}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={() => handleCancel()}
      >
        <Spin spinning={loading}>
          <Form
            key={`${formKey}:${readySession}`}
            initValues={
              formValuesRef.current?.key === formKey
                ? formValuesRef.current.values
                : getInitValues()
            }
            getFormApi={(api) => (formApiRef.current = api)}
            onSubmit={submit}
          >
            {({ values }) => (
              <div className='p-2'>
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Basic Info */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='blue'
                      className='mr-2 shadow-md'
                    >
                      <IconGift size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('基本信息')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('设置兑换码的基本信息')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.Input
                        field='name'
                        label={t('名称')}
                        placeholder={t('请输入名称')}
                        style={{ width: '100%' }}
                        rules={
                          !isEdit
                            ? []
                            : [{ required: true, message: t('请输入名称') }]
                        }
                        showClear
                      />
                    </Col>
                    <Col span={24}>
                      <Form.Select
                        field='benefit_type'
                        label={t('权益类型')}
                        style={{ width: '100%' }}
                        optionList={[
                          { label: t('额度'), value: 'quota' },
                          { label: t('套餐'), value: 'subscription' },
                        ]}
                      />
                    </Col>
                    <Col span={24}>
                      <Form.DatePicker
                        field='expired_time'
                        label={t('过期时间')}
                        type='dateTime'
                        placeholder={t('选择过期时间（可选，留空为永久）')}
                        style={{ width: '100%' }}
                        showClear
                      />
                    </Col>
                  </Row>
                </Card>

                <Card className='!rounded-2xl shadow-sm border-0'>
                  {/* Header: Quota Settings */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='green'
                      className='mr-2 shadow-md'
                    >
                      <IconCreditCard size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('额度设置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('设置兑换码的额度和数量')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    {values.benefit_type === 'subscription' ? (
                      <>
                        <Col span={24}>
                          <Form.Select
                            field='plan_id'
                            label={t('套餐')}
                            placeholder={t('请选择套餐')}
                            style={{ width: '100%' }}
                            optionList={subscriptionPlans.map((plan) => ({
                              label: `${plan.title} (#${plan.id})`,
                              value: plan.id,
                            }))}
                            rules={[
                              { required: true, message: t('请选择套餐') },
                            ]}
                          />
                        </Col>
                        <Col span={12}>
                          <Form.InputNumber
                            field='subscription_purchase_quantity'
                            label={t('套餐份数')}
                            min={1}
                            style={{ width: '100%' }}
                            rules={[
                              { required: true, message: t('请输入套餐份数') },
                            ]}
                          />
                        </Col>
                      </>
                    ) : (
                      <Col span={12}>
                        <Form.AutoComplete
                          field='quota'
                          label={t('额度')}
                          placeholder={t('请输入额度')}
                          style={{ width: '100%' }}
                          type='number'
                          rules={[
                            { required: true, message: t('请输入额度') },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('额度必须大于0'));
                              },
                            },
                          ]}
                          extraText={renderQuotaWithPrompt(
                            Number(values.quota) || 0,
                          )}
                          data={[
                            { value: 500000, label: '1$' },
                            { value: 5000000, label: '10$' },
                            { value: 25000000, label: '50$' },
                            { value: 50000000, label: '100$' },
                            { value: 250000000, label: '500$' },
                            { value: 500000000, label: '1000$' },
                          ]}
                          showClear
                        />
                      </Col>
                    )}
                    {!isEdit && (
                      <Col span={12}>
                        <Form.InputNumber
                          field='count'
                          label={t('生成数量')}
                          min={1}
                          rules={[
                            { required: true, message: t('请输入生成数量') },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('生成数量必须大于0'));
                              },
                            },
                          ]}
                          style={{ width: '100%' }}
                          showClear
                        />
                      </Col>
                    )}
                  </Row>
                </Card>
              </div>
            )}
          </Form>
        </Spin>
      </SideSheet>
    </>
  );
};

export default EditRedemptionModal;
