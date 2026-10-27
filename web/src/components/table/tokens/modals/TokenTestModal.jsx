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

import VideoPreflightResult from '../../../common/VideoPreflightResult';
import { isVideoTestModel } from '../../../../helpers/videoTest';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Modal,
  Space,
  Select,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconInfoCircle } from '@douyinfe/semi-icons';
import {
  API,
  getModelCategories,
  selectFilter,
  showError,
  showSuccess,
  showInfo,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import { useTranslation } from 'react-i18next';

const normalizeTokenModel = (model) => {
  if (typeof model === 'string') {
    return {
      name: model,
      supported_endpoint_types: [],
    };
  }
  return {
    name: model?.name || '',
    supported_endpoint_types: Array.isArray(model?.supported_endpoint_types)
      ? model.supported_endpoint_types
      : [],
  };
};

const endpointOptionMap = {
  'openai-video': {
    label: 'Video /v1/videos',
    helper: '视频模型仅检查本地配置，不提交生成任务。',
  },
  'openai-response': {
    label: 'OpenAI Responses /v1/responses',
    helper: 'Responses 是较新的 OpenAI 文本接口。',
  },
  'openai-response-compact': {
    label: 'OpenAI Responses Compact /v1/responses/compact',
    helper: '该模型使用 Responses Compact 接口。',
  },
  openai: {
    label: 'OpenAI Chat Completions /v1/chat/completions',
    helper: '兼容性更广，适合只支持 chat/completions 的上游。',
  },
  anthropic: {
    label: 'Anthropic Messages /v1/messages',
    helper: '该模型使用原生 Anthropic Messages 接口。',
  },
  gemini: {
    label: 'Gemini /v1beta/models/{model}:generateContent',
    helper: '该模型使用原生 Gemini 接口。',
  },
  embeddings: {
    label: 'Embeddings /v1/embeddings',
    helper: '该模型使用 Embeddings 接口。',
  },
  'jina-rerank': {
    label: 'Jina Rerank /v1/rerank',
    helper: '该模型使用 Rerank 接口。',
  },
  'image-generation': {
    label: '图像生成 /v1/images/generations',
    helper: '该模型使用图像生成接口。',
  },
};

const TokenTestModal = ({ visible, token, onCancel }) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [groupSelection, setGroupSelection] = useState({});
  const selectedGroup =
    groupSelection.tokenId === token?.id ? groupSelection.group : '';
  const [loadingModels, setLoadingModels] = useState(false);
  const [selectedModel, setSelectedModel] = useState('');
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState(null);
  const requestSerial = useRef(0);
  useEffect(() => {
    requestSerial.current++;
    setTesting(false);
    setResult(null);
    return () => {
      requestSerial.current++;
    };
  }, [visible, token?.id, selectedModel, selectedGroup]);

  useEffect(() => {
    if (!visible || !token?.id) {
      setModels([]);
      setSelectedModel('');
      setResult(null);
      setLoadingModels(false);
      setTesting(false);
      return;
    }

    let active = true;
    const loadModels = async () => {
      setLoadingModels(true);
      setModels([]);
      setSelectedModel('');
      try {
        const res = await API.get('/api/token/models', {
          params: { token_id: token.id, detail: true, group: selectedGroup },
        });
        if (!active) return;
        const { success, message, data } = res.data || {};
        setGroups(res.data?.groups || []);
        if (!success) {
          showError(t(message || '加载模型失败'));
          setModels([]);
          return;
        }
        const nextModels = (Array.isArray(data) ? data : [])
          .map(normalizeTokenModel)
          .filter((item) => item.name);
        setModels(nextModels);
        setSelectedModel((current) =>
          nextModels.some((item) => item.name === current)
            ? current
            : nextModels[0]?.name || '',
        );
      } catch (error) {
        if (!active) return;
        showError(error?.message || t('加载模型失败'));
        setModels([]);
      } finally {
        if (active) setLoadingModels(false);
      }
    };

    loadModels();
    return () => {
      active = false;
    };
  }, [visible, token?.id, selectedGroup, t]);

  const selectedModelMeta = useMemo(
    () => models.find((item) => item.name === selectedModel) || null,
    [models, selectedModel],
  );

  const availableEndpointTypes = useMemo(() => {
    return selectedModelMeta?.supported_endpoint_types || [];
  }, [selectedModelMeta]);

  useEffect(() => {
    setResult(null);
  }, [selectedModel]);

  const modelOptions = useMemo(() => {
    const categories = getModelCategories(t);
    return models.map((model) => {
      let icon = null;
      for (const [key, category] of Object.entries(categories)) {
        if (key !== 'all' && category.filter({ model_name: model.name })) {
          icon = category.icon;
          break;
        }
      }
      return {
        value: model.name,
        label: (
          <span className='flex items-center gap-1'>
            {icon}
            {model.name}
          </span>
        ),
      };
    });
  }, [models, t]);

  const endpointHelperText = useMemo(() => {
    if (availableEndpointTypes.length === 1) {
      return t(
        endpointOptionMap[availableEndpointTypes[0]]?.helper ||
          '系统会自动按模型能力选择测试接口。',
      );
    }
    if (availableEndpointTypes.includes('openai-response')) {
      return t(
        '系统会按实际命中的渠道自动选择测试接口；Codex 系列会固定使用 Responses。',
      );
    }
    return t('系统会按实际命中的渠道和模型能力自动选择测试接口。');
  }, [availableEndpointTypes, t]);

  const handleTest = async () => {
    if (!token?.id) {
      return;
    }
    if (!selectedModel) {
      showError(t('请选择模型'));
      return;
    }
    const serial = ++requestSerial.current;
    setTesting(true);
    setResult(null);
    try {
      const res = await API.post(`/api/token/test/${token.id}`, {
        model: selectedModel,
        group: selectedGroup,
      });
      if (serial !== requestSerial.current) return;
      const { success, message, time, test_kind, data } = res.data || {};
      const nextResult = {
        success: !!success,
        testKind: test_kind,
        preflight: data,
        message: message || '',
        time: Number(time || 0),
      };
      setResult(nextResult);
      if (test_kind === 'video_preflight') {
        showInfo(t('视频仅进行配置预检，未生成视频，也不会改变渠道健康状态。'));
      } else if (nextResult.success) {
        showSuccess(
          t('令牌测试成功，模型 {{model}} 耗时 {{time}} 秒', {
            model: selectedModel,
            time: nextResult.time.toFixed(2),
          }),
        );
      } else {
        showError(message || t('测试失败'));
      }
    } catch (error) {
      if (serial !== requestSerial.current) return;
      const message = error?.message || t('测试失败');
      setResult({
        success: false,
        message,
        time: 0,
      });
      showError(message);
    } finally {
      if (serial === requestSerial.current) setTesting(false);
    }
  };

  const endpointTagText = useMemo(() => {
    if (availableEndpointTypes.length === 1) {
      return t(endpointOptionMap[availableEndpointTypes[0]]?.label || '');
    }
    return t('自动匹配');
  }, [availableEndpointTypes, t]);

  const video = isVideoTestModel(
    selectedModel,
    0,
    availableEndpointTypes.includes('openai-video') ? 'openai-video' : '',
  );
  return (
    <Modal
      visible={visible}
      title={
        <div className='flex items-center gap-2'>
          <span>{t('测试令牌模型')}</span>
          {token?.name ? (
            <Tag color='blue' shape='circle'>
              {token.name}
            </Tag>
          ) : null}
        </div>
      }
      onCancel={onCancel}
      footer={
        <Space>
          <Button type='tertiary' onClick={onCancel} disabled={testing}>
            {t('关闭')}
          </Button>
          <Button
            theme='solid'
            type='primary'
            onClick={handleTest}
            loading={testing}
            disabled={loadingModels || models.length === 0}
          >
            {video ? t('检查配置') : t('开始测试')}
          </Button>
        </Space>
      }
      size={isMobile ? 'full-width' : 'medium'}
      maskClosable={!testing}
    >
      <div className='flex flex-col gap-3'>
        <Banner
          type='warning'
          closeIcon={null}
          icon={<IconInfoCircle />}
          description={
            video
              ? t('视频模型仅检查本地配置，不提交生成任务。')
              : t(
                  '该测试会按当前令牌与账户规则真实请求并正常计费，失败时按现有规则退款。',
                )
          }
        />

        <div className='flex flex-col gap-2'>
          <Typography.Text strong id='token-test-group-label'>
            {t('测试分组')}
          </Typography.Text>
          <Select
            aria-labelledby='token-test-group-label'
            value={selectedGroup}
            disabled={testing}
            onChange={(group) =>
              setGroupSelection({ tokenId: token.id, group })
            }
            optionList={[
              { value: '', label: t('按 Key 默认范围') },
              ...groups.map((group) => ({ value: group, label: group })),
            ]}
            style={{ width: '100%' }}
          />
          <Typography.Text type='tertiary'>
            {t(
              '选择具体分组后，仅展示并测试该分组内获准的模型，不会切换到其他分组。',
            )}
          </Typography.Text>
        </div>
        <div className='flex flex-col gap-2'>
          <Typography.Text strong>{t('测试模型')}</Typography.Text>
          <Select
            value={selectedModel}
            onChange={setSelectedModel}
            optionList={modelOptions}
            filter={selectFilter}
            searchable
            disabled={testing || loadingModels}
            loading={loadingModels}
            placeholder={t('请选择模型')}
            emptyContent={t('没有可用模型')}
            style={{ width: '100%' }}
          />
          <Typography.Text type='tertiary'>
            {t('仅展示该令牌当前分组与模型限制下实际可访问的模型。')}
          </Typography.Text>
        </div>

        <div className='flex flex-col gap-2'>
          <Typography.Text strong>{t('请求端点')}</Typography.Text>
          <Tag shape='circle' color='blue'>
            {endpointTagText}
          </Tag>
          <Typography.Text type='tertiary'>
            {endpointHelperText}
          </Typography.Text>
        </div>

        {result?.testKind === 'video_preflight' ? (
          <VideoPreflightResult data={result.preflight} />
        ) : result ? (
          <div className='rounded-xl border border-[var(--semi-color-border)] p-3 flex flex-col gap-2'>
            <div className='flex items-center gap-2'>
              <Typography.Text strong>{t('测试结果')}</Typography.Text>
              <Tag color={result.success ? 'green' : 'red'} shape='circle'>
                {result.success ? t('成功') : t('失败')}
              </Tag>
            </div>
            <Typography.Text>
              {result.success
                ? t('耗时 {{time}} 秒', { time: result.time.toFixed(2) })
                : result.message || t('测试失败')}
            </Typography.Text>
          </div>
        ) : null}
      </div>
    </Modal>
  );
};

export default TokenTestModal;
