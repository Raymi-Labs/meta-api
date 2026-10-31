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

import React, { useState, useEffect, useRef, useMemo } from 'react';
import { omitUnchangedModelMetadata } from '../../../../helpers/modelMetadataPatch';
import ModelDescriptionEditor from './ModelDescriptionEditor';
import ModelFAQEditor from './ModelFAQEditor';
import ModelCapabilitiesEditor from './ModelCapabilitiesEditor';
import ModelFunctionsEditor, {
  ModelFunctionsPreview,
} from './ModelFunctionsEditor';
import OfficialPricingEditor from './OfficialPricingEditor';
import JSONEditor from '../../../common/ui/JSONEditor';
import {
  Banner,
  SideSheet,
  Form,
  Button,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Avatar,
  Col,
  Row,
} from '@douyinfe/semi-ui';
import { Save, X, FileText } from 'lucide-react';
import { IconAlertTriangle, IconLink } from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../../helpers';
import { useTranslation } from 'react-i18next';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';

const { Text, Title } = Typography;

// Example endpoint template for quick fill
const ENDPOINT_TEMPLATE = {
  openai: { path: '/v1/chat/completions', method: 'POST' },
  'openai-response': { path: '/v1/responses', method: 'POST' },
  'openai-response-compact': { path: '/v1/responses/compact', method: 'POST' },
  anthropic: { path: '/v1/messages', method: 'POST' },
  gemini: { path: '/v1beta/models/{model}:generateContent', method: 'POST' },
  'jina-rerank': { path: '/v1/rerank', method: 'POST' },
  'image-generation': { path: '/v1/images/generations', method: 'POST' },
};

const nameRuleOptions = [
  { label: '精确名称匹配', value: 0 },
  { label: '前缀名称匹配', value: 1 },
  { label: '包含名称匹配', value: 2 },
  { label: '后缀名称匹配', value: 3 },
];

const modalityOptions = [
  { label: '文本', value: 'text' },
  { label: '图像', value: 'image' },
  { label: '音频', value: 'audio' },
  { label: '视频', value: 'video' },
  { label: '文件', value: 'file' },
];

const EditModelModal = (props) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const sessionRef = useRef(0);
  const formValuesRef = useRef(null);
  const submittingRef = useRef(false);
  const refreshRef = useRef(props.refresh);
  refreshRef.current = props.refresh;
  const [readySession, setReadySession] = useState(null);
  const isEdit = props.editingModel && props.editingModel.id !== undefined;
  const formKey = `${props.visiable}:${props.editingModel?.id ?? 'new'}:${props.editingModel?.model_name || ''}`;
  const canSubmit = props.visiable && readySession === sessionRef.current;
  const placement = useMemo(() => (isEdit ? 'right' : 'left'), [isEdit]);

  // 供应商列表
  const [vendors, setVendors] = useState([]);
  const vendorRequestRef = useRef(0);

  // 预填组（标签、端点）
  const [tagGroups, setTagGroups] = useState([]);
  const [endpointGroups, setEndpointGroups] = useState([]);

  // 获取供应商列表
  const fetchVendors = async (session) => {
    const requestId = ++vendorRequestRef.current;
    try {
      const res = await API.get('/api/vendors/?page_size=1000', {
        disableDuplicate: true,
      }); // 获取全部供应商
      if (
        res.data.success &&
        requestId === vendorRequestRef.current &&
        (session === undefined || session === sessionRef.current)
      ) {
        const items = res.data.data.items || res.data.data || [];
        setVendors(Array.isArray(items) ? items : []);
      }
    } catch (error) {
      // ignore
    }
  };

  // 获取预填组（标签、端点）
  const fetchPrefillGroups = async () => {
    try {
      const [tagRes, endpointRes] = await Promise.all([
        API.get('/api/prefill_group?type=tag'),
        API.get('/api/prefill_group?type=endpoint'),
      ]);
      if (tagRes?.data?.success) {
        setTagGroups(tagRes.data.data || []);
      }
      if (endpointRes?.data?.success) {
        setEndpointGroups(endpointRes.data.data || []);
      }
    } catch (error) {
      // ignore
    }
  };

  useEffect(() => {
    if (props.visiable) {
      fetchVendors();
      fetchPrefillGroups();
    }
  }, [props.visiable]);

  const getInitValues = () => ({
    model_name: props.editingModel?.model_name || '',
    display_name: '',
    release_date: '',
    context_length: 0,
    max_output_tokens: 0,
    capabilities: {},
    function_overrides: {},
    input_modalities: [],
    output_modalities: [],
    description: '',
    description_overrides: {},
    faq: [],
    official_pricing: {},
    icon: '',
    tags: [],
    vendor_id: null,
    vendor: '',
    vendor_icon: '',
    endpoints: '',
    name_rule: props.editingModel?.model_name ? 0 : undefined, // 通过未配置模型过来的固定为精确匹配
    status: true,
    sync_official: true,
  });

  const handleCancel = () => {
    sessionRef.current += 1;
    setReadySession(null);
    props.handleClose();
  };

  const loadModel = async (session) => {
    try {
      const res = await API.get(
        `/api/models/${props.editingModel.id}?refresh_metadata=true`,
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
          display_name: data.display_name || '',
          official_pricing: data.official_pricing || {},
          faq: Array.isArray(data.faq) ? data.faq : [],
          release_date: data.release_date || '',
          context_length: data.context_length || 0,
          max_output_tokens: data.max_output_tokens || 0,
          capabilities: data.capabilities || {},
          function_overrides: data.function_overrides || {},
          input_modalities: Array.isArray(data.input_modalities)
            ? data.input_modalities
            : [],
          output_modalities: Array.isArray(data.output_modalities)
            ? data.output_modalities
            : [],
          tags: data.tags ? data.tags.split(',').filter(Boolean) : [],
          vendor_id: data.vendor_id || null,
          endpoints: data.endpoints || '',
          status: data.status === 1,
          sync_official: (data.sync_official ?? 1) === 1,
        };
        await fetchVendors(session);
        if (session !== sessionRef.current) return;
        formValuesRef.current = {
          key: formKey,
          values: structuredClone(values),
        };
        setReadySession(session);
      } else {
        showError(message);
      }
    } catch (error) {
      if (session === sessionRef.current) {
        showError(error.response ? error : t('加载模型信息失败'));
      }
    } finally {
      if (session === sessionRef.current) setLoading(false);
    }
  };

  useEffect(() => {
    const session = ++sessionRef.current;
    submittingRef.current = false;
    setReadySession(null);
    const values = getInitValues();
    formValuesRef.current = { key: formKey, values };
    setLoading(Boolean(props.visiable && isEdit));
    if (props.visiable) {
      if (isEdit) {
        loadModel(session);
      } else {
        setReadySession(session);
      }
    }
    return () => {
      sessionRef.current += 1;
    };
  }, [props.visiable, props.editingModel?.id, props.editingModel?.model_name]);

  const submit = async (values) => {
    if (
      !canSubmit ||
      readySession !== sessionRef.current ||
      submittingRef.current
    )
      return;
    const session = sessionRef.current;
    submittingRef.current = true;
    setLoading(true);
    try {
      const submitData = {
        ...values,
        display_name: (values.display_name || '').trim(),
        official_pricing: values.official_pricing || {},
        faq: (values.faq || []).map(({ question, answer }) => ({
          question: question.trim(),
          answer: answer.trim(),
        })),
        release_date:
          values.name_rule === 0 ? (values.release_date || '').trim() : '',
        context_length: values.context_length || 0,
        max_output_tokens: values.max_output_tokens || 0,
        capabilities: values.capabilities || {},
        function_overrides: values.function_overrides || {},
        input_modalities: values.input_modalities || [],
        output_modalities: values.output_modalities || [],
        tags: Array.isArray(values.tags) ? values.tags.join(',') : values.tags,
        endpoints: values.endpoints || '',
        vendor_id: values.vendor_id || 0,
        status: values.status ? 1 : 0,
        sync_official: values.sync_official ? 1 : 0,
      };

      if (isEdit) {
        submitData.id = props.editingModel.id;
        const res = await API.put(
          '/api/models/',
          omitUnchangedModelMetadata(
            submitData,
            values,
            formValuesRef.current?.values,
          ),
          {
            skipErrorHandler: true,
          },
        );
        const { success, message } = res.data;
        if (success) refreshRef.current();
        if (session !== sessionRef.current) return;
        if (success) {
          showSuccess(t('模型更新成功！'));
          handleCancel();
        } else {
          showError(t(message));
        }
      } else {
        const res = await API.post(
          '/api/models/',
          omitUnchangedModelMetadata(submitData, values),
          {
            skipErrorHandler: true,
          },
        );
        const { success, message } = res.data;
        if (success) refreshRef.current();
        if (session !== sessionRef.current) return;
        if (success) {
          showSuccess(t('模型创建成功！'));
          handleCancel();
        } else {
          showError(t(message));
        }
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
    <SideSheet
      placement={placement}
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
            {isEdit ? t('更新模型信息') : t('创建新的模型')}
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
              className='!rounded-lg'
              onClick={() => formApiRef.current?.submitForm()}
              icon={<Save size={16} />}
              loading={loading}
              disabled={!canSubmit}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              className='!rounded-lg'
              type='primary'
              onClick={handleCancel}
              icon={<X size={16} />}
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
              {values.sync_official && (
                <Banner
                  type='info'
                  closeIcon={null}
                  className='mb-3'
                  description={t(
                    '模型资料自动更新，官方信息优先；人工修改保留，尚未获取的内容保持未知。',
                  )}
                />
              )}
              {/* 基本信息 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='green' className='mr-2 shadow-md'>
                    <FileText size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('基本信息')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置模型的基本信息')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Input
                      field='model_name'
                      label={t('模型名称')}
                      placeholder={t('请输入模型名称，如：gpt-4')}
                      rules={[{ required: true, message: t('请输入模型名称') }]}
                      showClear
                    />
                  </Col>

                  <Col span={24}>
                    <Form.Input
                      field='display_name'
                      label={t('显示名称')}
                      placeholder={t('留空时使用模型 ID')}
                      maxLength={128}
                      showClear
                    />
                  </Col>

                  <Col span={24}>
                    <Form.Select
                      field='name_rule'
                      label={t('名称匹配类型')}
                      placeholder={t('请选择名称匹配类型')}
                      optionList={nameRuleOptions.map((o) => ({
                        label: t(o.label),
                        value: o.value,
                      }))}
                      rules={[
                        { required: true, message: t('请选择名称匹配类型') },
                      ]}
                      extraText={t(
                        '根据模型名称和匹配规则查找模型元数据，优先级：精确 > 前缀 > 后缀 > 包含',
                      )}
                      style={{ width: '100%' }}
                    />
                  </Col>

                  <Col span={24}>
                    <Form.Input
                      field='icon'
                      label={t('模型图标')}
                      placeholder={t('请输入图标名称')}
                      extraText={
                        <span>
                          {t(
                            "图标使用@lobehub/icons库，如：OpenAI、Claude.Color，支持链式参数：OpenAI.Avatar.type={'platform'}、OpenRouter.Avatar.shape={'square'}，查询所有可用图标请 ",
                          )}
                          <Typography.Text
                            link={{
                              href: 'https://icons.lobehub.com/components/lobe-hub',
                              target: '_blank',
                            }}
                            icon={<IconLink />}
                            underline
                          >
                            {t('请点击我')}
                          </Typography.Text>
                        </span>
                      }
                      showClear
                    />
                  </Col>

                  <Col span={24}>
                    <Form.TextArea
                      field='description'
                      label={t('介绍原文')}
                      placeholder={t('请输入模型描述')}
                      extraText={t(
                        '原文用于自动生成全站语言的介绍，不决定用户看到的语言。',
                      )}
                      rows={5}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    <ModelDescriptionEditor
                      field='description_overrides'
                      label={t('多语言介绍')}
                      automatic={
                        formValuesRef.current?.values?.description_automatic ||
                        {}
                      }
                    />
                  </Col>
                  <Col span={24}>
                    <ModelFAQEditor
                      field='faq'
                      label={t('常见问题')}
                      rules={[
                        {
                          validator: (_, items) =>
                            (items || []).every(
                              (item) =>
                                item.question?.trim() && item.answer?.trim(),
                            ),
                          message: t('FAQ 的问题和答案不能为空'),
                        },
                      ]}
                    />
                  </Col>
                  <Col span={24}>
                    <OfficialPricingEditor
                      field='official_pricing'
                      label={t('官方参考价')}
                      disabled={values.name_rule !== 0}
                      extraText={
                        values.name_rule !== 0
                          ? t('官方参考价仅支持精确匹配模型')
                          : undefined
                      }
                    />
                  </Col>
                  <Col span={24}>
                    <Form.Input
                      field='release_date'
                      label={t('发布日期')}
                      placeholder='YYYY-MM-DD / YYYY-MM'
                      maxLength={10}
                      disabled={values.name_rule !== 0}
                      extraText={t(
                        values.name_rule === 0
                          ? '支持 YYYY-MM-DD 或 YYYY-MM；留空自动同步，人工填写优先保留。'
                          : '发布日期仅支持精确匹配模型',
                      )}
                      showClear
                    />
                  </Col>
                  <Col span={isMobile ? 24 : 12}>
                    <Form.InputNumber
                      field='context_length'
                      label={t('上下文长度')}
                      extraText={t(
                        '单位为 Token。开启同步时，0 或空模态可自动补齐；人工填写的值优先保留。',
                      )}
                      min={0}
                      max={Number.MAX_SAFE_INTEGER}
                      precision={0}
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={isMobile ? 24 : 12}>
                    <Form.InputNumber
                      field='max_output_tokens'
                      label={t('最大输出')}
                      min={0}
                      max={Number.MAX_SAFE_INTEGER}
                      precision={0}
                      style={{ width: '100%' }}
                      extraText={t('单位为 Token；0 表示未知，可由同步补齐。')}
                    />
                  </Col>
                  <Col span={isMobile ? 24 : 12}>
                    <Form.Select
                      field='input_modalities'
                      label={t('输入模态')}
                      placeholder={t('未配置')}
                      multiple
                      showClear
                      optionList={modalityOptions.map((option) => ({
                        ...option,
                        label: t(option.label),
                      }))}
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={isMobile ? 24 : 12}>
                    <Form.Select
                      field='output_modalities'
                      label={t('输出模态')}
                      placeholder={t('未配置')}
                      multiple
                      showClear
                      optionList={modalityOptions.map((option) => ({
                        ...option,
                        label: t(option.label),
                      }))}
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <ModelCapabilitiesEditor
                      field='capabilities'
                      label={t('能力')}
                      extraText={t(
                        '未知可自动补齐；人工配置的支持或不支持优先保留。',
                      )}
                    />
                  </Col>
                  <Col span={24}>
                    <ModelFunctionsEditor
                      field='function_overrides'
                      label={t('广场功能分类')}
                      model={{ ...values, tags: (values.tags || []).join(',') }}
                      extraText={t(
                        '自动使用模型资料；包含或排除为人工设置，不会被同步覆盖。',
                      )}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.TagInput
                      field='tags'
                      label={t('标签')}
                      placeholder={t('输入标签或使用","分隔多个标签')}
                      addOnBlur
                      showClear
                      onChange={(newTags) => {
                        if (!formApiRef.current) return;
                        const normalize = (tags) => {
                          if (!Array.isArray(tags)) return [];
                          return [
                            ...new Set(
                              tags.flatMap((tag) =>
                                tag
                                  .split(',')
                                  .map((t) => t.trim())
                                  .filter(Boolean),
                              ),
                            ),
                          ];
                        };
                        const normalized = normalize(newTags);
                        formApiRef.current.setValue('tags', normalized);
                      }}
                      style={{ width: '100%' }}
                      {...(tagGroups.length > 0 && {
                        extraText: (
                          <Space wrap>
                            {tagGroups.map((group) => (
                              <Button
                                key={group.id}
                                size='small'
                                type='primary'
                                onClick={() => {
                                  if (formApiRef.current) {
                                    const currentTags =
                                      formApiRef.current.getValue('tags') || [];
                                    const newTags = [
                                      ...currentTags,
                                      ...(group.items || []),
                                    ];
                                    const uniqueTags = [...new Set(newTags)];
                                    formApiRef.current.setValue(
                                      'tags',
                                      uniqueTags,
                                    );
                                  }
                                }}
                              >
                                {group.name}
                              </Button>
                            ))}
                          </Space>
                        ),
                      })}
                    />
                  </Col>
                  <Col span={24}>
                    <ModelFunctionsPreview
                      model={{ ...values, tags: (values.tags || []).join(',') }}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.Select
                      field='vendor_id'
                      label={t('供应商')}
                      placeholder={t('选择模型供应商')}
                      optionList={vendors.map((v) => ({
                        label: v.name,
                        value: v.id,
                      }))}
                      filter
                      showClear
                      onChange={(value) => {
                        const vendorInfo = vendors.find((v) => v.id === value);
                        if (vendorInfo && formApiRef.current) {
                          formApiRef.current.setValue(
                            'vendor',
                            vendorInfo.name,
                          );
                        }
                      }}
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <Banner
                      type='warning'
                      closeIcon={null}
                      icon={
                        <IconAlertTriangle
                          size='large'
                          style={{ color: 'var(--semi-color-warning)' }}
                        />
                      }
                      description={t(
                        '提示：此处配置仅用于控制「模型广场」基础信息展示，不会影响模型的实际调用与路由。若需限制模型可见范围或调用权限，请前往「系统设置」的「模型/价格设置」配置。',
                      )}
                      style={{ marginBottom: 12 }}
                    />
                    <JSONEditor
                      field='endpoints'
                      label={t('在模型广场向用户展示的端点')}
                      placeholder={
                        '{\n  "openai": {"path": "/v1/chat/completions", "method": "POST"}\n}'
                      }
                      value={values.endpoints}
                      onChange={(val) =>
                        formApiRef.current?.setValue('endpoints', val)
                      }
                      formApi={formApiRef.current}
                      editorType='object'
                      template={ENDPOINT_TEMPLATE}
                      templateLabel={t('填入模板')}
                      extraText={t('留空则使用默认端点；支持 {path, method}')}
                      extraFooter={
                        endpointGroups.length > 0 && (
                          <Space wrap>
                            {endpointGroups.map((group) => (
                              <Button
                                key={group.id}
                                size='small'
                                type='primary'
                                onClick={() => {
                                  try {
                                    const current =
                                      formApiRef.current?.getValue(
                                        'endpoints',
                                      ) || '';
                                    let base = {};
                                    if (current && current.trim())
                                      base = JSON.parse(current);
                                    const groupObj =
                                      typeof group.items === 'string'
                                        ? JSON.parse(group.items || '{}')
                                        : group.items || {};
                                    const merged = { ...base, ...groupObj };
                                    formApiRef.current?.setValue(
                                      'endpoints',
                                      JSON.stringify(merged, null, 2),
                                    );
                                  } catch (e) {
                                    try {
                                      const groupObj =
                                        typeof group.items === 'string'
                                          ? JSON.parse(group.items || '{}')
                                          : group.items || {};
                                      formApiRef.current?.setValue(
                                        'endpoints',
                                        JSON.stringify(groupObj, null, 2),
                                      );
                                    } catch {}
                                  }
                                }}
                              >
                                {group.name}
                              </Button>
                            ))}
                          </Space>
                        )
                      }
                    />
                  </Col>
                  <Col span={24}>
                    <Form.Switch
                      field='sync_official'
                      label={t('参与官方同步')}
                      extraText={t(
                        '开启后自动补齐模型属性和 OpenAI 官方文本价格；人工设置优先，清空后可重新同步。',
                      )}
                      size='large'
                    />
                  </Col>
                  <Col span={24}>
                    <Form.Switch
                      field='status'
                      label={t('状态')}
                      size='large'
                    />
                  </Col>
                </Row>
              </Card>
            </div>
          )}
        </Form>
      </Spin>
    </SideSheet>
  );
};

export default EditModelModal;
