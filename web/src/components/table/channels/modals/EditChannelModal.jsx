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

import React, { useEffect, useState, useRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { CHANNEL_DISPLAY_VENDORS } from '../../../../helpers/channelCategory';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  verifyJSON,
} from '../../../../helpers';
import {
  currencyAmountToQuota,
  renderCurrencyQuota,
  quotaToCurrencyAmount,
} from '../../../../helpers/quota';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  CHANNEL_OPTIONS,
  MODEL_FETCHABLE_CHANNEL_TYPES,
} from '../../../../constants';
import {
  SideSheet,
  Space,
  Spin,
  Button,
  Select,
  Typography,
  Checkbox,
  Banner,
  Modal,
  ImagePreview,
  Card,
  Tag,
  Avatar,
  Form,
  Row,
  Col,
  Highlight,
  Input,
  Tooltip,
  Progress,
} from '@douyinfe/semi-ui';
import {
  getChannelModels,
  copy,
  getChannelIcon,
  getModelCategories,
  renderGroupOption,
  selectFilter,
} from '../../../../helpers';
import ModelSelectModal from './ModelSelectModal';
import SingleModelSelectModal from './SingleModelSelectModal';
import OllamaModelModal from './OllamaModelModal';
import CodexOAuthModal from './CodexOAuthModal';
import ParamOverrideEditorModal from './ParamOverrideEditorModal';
import JSONEditor from '../../../common/ui/JSONEditor';
import SecureVerificationModal from '../../../common/modals/SecureVerificationModal';
import StatusCodeRiskGuardModal from './StatusCodeRiskGuardModal';
import ChannelKeyDisplay from '../../../common/ui/ChannelKeyDisplay';
import { useSecureVerification } from '../../../../hooks/common/useSecureVerification';
import { parseChannelConnectionString } from '../../../../helpers/token';
import { createApiCalls } from '../../../../services/secureVerification';
import {
  buildTokenGroupVendorOptions,
  filterTokenGroupsByVendor,
  resolveTokenGroupVendor,
  sortTokenGroupOptions,
} from '../../tokens/tokenGroupUtils';
import {
  collectInvalidStatusCodeEntries,
  collectNewDisallowedStatusCodeRedirects,
} from './statusCodeRiskGuard';
import {
  areChannelGroupsEqual,
  mergeChannelFormValues,
  mergeChannelJsonObject,
  normalizeChannelGroups,
  parseChannelJsonObject,
} from './channelFormUtils';
import {
  IconSave,
  IconClose,
  IconServer,
  IconSetting,
  IconCode,
  IconGlobe,
  IconBolt,
  IconSearch,
  IconChevronUp,
  IconChevronDown,
  IconCalendar,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const MODEL_MAPPING_EXAMPLE = {
  'gpt-3.5-turbo': 'gpt-3.5-turbo-0125',
};

const STATUS_CODE_MAPPING_EXAMPLE = {
  400: '500',
};

const REGION_EXAMPLE = {
  default: 'global',
  'gemini-1.5-pro-002': 'europe-west2',
  'gemini-1.5-flash-002': 'europe-west2',
  'claude-3-5-sonnet-20240620': 'europe-west1',
};

const ADVANCED_SETTINGS_EXPANDED_KEY = 'channel-advanced-settings-expanded';

const CAPABILITY_MODE_OPTIONS = [
  { label: '继承默认', value: 'inherit' },
  { label: '强制开启', value: 'enabled' },
  { label: '强制关闭', value: 'disabled' },
];

function type2secretPrompt(type) {
  // inputs.type === 15 ? '按照如下格式输入：APIKey|SecretKey' : (inputs.type === 18 ? '按照如下格式输入：APPID|APISecret|APIKey' : '请输入渠道对应的鉴权密钥')
  switch (type) {
    case 15:
      return '按照如下格式输入：APIKey|SecretKey';
    case 18:
      return '按照如下格式输入：APPID|APISecret|APIKey';
    case 22:
      return '按照如下格式输入：APIKey-AppId，例如：fastgpt-0sp2gtvfdgyi4k30jwlgwf1i-64f335d84283f05518e9e041';
    case 23:
      return '按照如下格式输入：AppId|SecretId|SecretKey';
    case 33:
      return '按照如下格式输入：Ak|Sk|Region';
    case 45:
      return '请输入渠道对应的鉴权密钥, 豆包语音输入：AppId|AccessToken';
    case 50:
      return '按照如下格式输入: AccessKey|SecretKey, 如果上游是New API，则直接输ApiKey';
    case 51:
      return '按照如下格式输入: AccessKey|SecretAccessKey';
    case 57:
      return '请输入 JSON 格式的 OAuth 凭据（必须包含 access_token 和 account_id）';
    default:
      return '请输入渠道对应的鉴权密钥';
  }
}

const EditChannelModal = (props) => {
  const { t } = useTranslation();
  const channelId = props.editingChannel.id;
  const isEdit = channelId !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const isMobile = useIsMobile();
  const handleCancel = () => {
    props.handleClose();
  };
  const originInputs = {
    name: '',
    type: 1,
    channel_vendor: 'auto',
    key: '',
    openai_organization: '',
    max_input_tokens: 0,
    base_url: '',
    other: '',
    model_mapping: '',
    status_code_mapping: '',
    models: [],
    auto_ban: 1,
    test_model: '',
    groups: [],
    priority: 0,
    weight: 0,
    tag: '',
    multi_key_mode: 'random',
    // 渠道额外设置的默认值
    force_format: false,
    cpa_user_identity_enabled: false,
    cpa_instance_id: '',
    thinking_to_content: false,
    proxy: '',
    nonstream_response_header_timeout_sec: 0,
    pass_through_body_enabled: false,
    chat_completions_to_responses_mode: 'inherit',
    claude_incremental_cache_enabled: false,
    system_prompt: '',
    system_prompt_override: false,
    max_concurrency: 0,
    rpm_limit: 0,
    settings: '',
    // 客户端限制设置
    client_restriction_mode: '',
    client_restriction_clients: [],
    // 仅 Vertex: 密钥格式（存入 settings.vertex_key_type）
    vertex_key_type: 'json',
    vertex_project_id: '',
    // 仅 AWS: 密钥格式和区域（存入 settings.aws_key_type 和 settings.aws_region）
    aws_key_type: 'ak_sk',
    // 企业账户设置
    is_enterprise_account: false,
    // 字段透传控制默认值
    allow_service_tier: false,
    disable_store: false, // false = 允许透传（默认开启）
    allow_safety_identifier: false,
    allow_include_obfuscation: false,
    allow_inference_geo: false,
    chat_stream_options_mode: 'inherit',
    responses_api_mode: 'inherit',
    responses_stream_options_mode: 'inherit',
    claude_beta_query: false,
    upstream_model_update_check_enabled: false,
    upstream_model_update_auto_sync_enabled: false,
    upstream_model_update_last_check_time: 0,
    upstream_model_update_last_detected_models: [],
    upstream_model_update_ignored_models: '',
    // 周期配额限制
    quota_policy_enabled: false,
    quota_policy_period: 'day',
    quota_policy_quota_limit: 0,
    quota_policy_count_limit: 0,
  };
  const [batch, setBatch] = useState(false);
  const [multiToSingle, setMultiToSingle] = useState(false);
  const [multiKeyMode, setMultiKeyMode] = useState('random');
  const [autoBan, setAutoBan] = useState(true);
  const [inputs, setInputs] = useState(originInputs);
  const [originModelOptions, setOriginModelOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);
  const [groupOptions, setGroupOptions] = useState([]);
  const [groupVendor, setGroupVendor] = useState('');
  const [basicModels, setBasicModels] = useState([]);
  const [fullModels, setFullModels] = useState([]);
  const [modelGroups, setModelGroups] = useState([]);
  const [customModel, setCustomModel] = useState('');
  const [modelSearchValue, setModelSearchValue] = useState('');
  const [modalImageUrl, setModalImageUrl] = useState('');
  const [isModalOpenurl, setIsModalOpenurl] = useState(false);
  const [modelModalVisible, setModelModalVisible] = useState(false);
  const [fetchedModels, setFetchedModels] = useState([]);
  const [modelMappingValueModalVisible, setModelMappingValueModalVisible] =
    useState(false);
  const [modelMappingValueModalModels, setModelMappingValueModalModels] =
    useState([]);
  const [modelMappingValueKey, setModelMappingValueKey] = useState('');
  const [modelMappingValueSelected, setModelMappingValueSelected] =
    useState('');
  const [ollamaModalVisible, setOllamaModalVisible] = useState(false);
  const formApiRef = useRef(null);
  const [vertexKeys, setVertexKeys] = useState([]);
  const [vertexFileList, setVertexFileList] = useState([]);
  const vertexErroredNames = useRef(new Set()); // 避免重复报错
  const [isMultiKeyChannel, setIsMultiKeyChannel] = useState(false);
  const [channelSearchValue, setChannelSearchValue] = useState('');
  const [useManualInput, setUseManualInput] = useState(false); // 是否使用手动输入模式
  const [keyMode, setKeyMode] = useState('append'); // 密钥模式：replace（覆盖）或 append（追加）

  const orderedGroupOptions = useMemo(() => {
    return sortTokenGroupOptions(groupOptions);
  }, [groupOptions]);

  const groupVendorOptions = useMemo(
    () => buildTokenGroupVendorOptions(orderedGroupOptions, t('多厂商通用')),
    [orderedGroupOptions, t],
  );

  const selectedGroupValues = useMemo(() => {
    if (Array.isArray(inputs.groups)) return inputs.groups;
    if (typeof inputs.groups === 'string') {
      return inputs.groups
        .split(',')
        .map((group) => group.trim())
        .filter(Boolean);
    }
    return [];
  }, [inputs.groups]);

  const visibleGroupOptions = useMemo(
    () => filterTokenGroupsByVendor(orderedGroupOptions, groupVendor),
    [orderedGroupOptions, groupVendor],
  );

  const visibleSelectedGroupValues = useMemo(
    () =>
      selectedGroupValues.filter(
        (group) => resolveTokenGroupVendor(group) === groupVendor,
      ),
    [selectedGroupValues, groupVendor],
  );
  const [isEnterpriseAccount, setIsEnterpriseAccount] = useState(false); // 是否为企业账户
  const [doubaoApiEditUnlocked, setDoubaoApiEditUnlocked] = useState(false); // 豆包渠道自定义 API 地址隐藏入口
  const redirectModelList = useMemo(() => {
    const mapping = inputs.model_mapping;
    if (typeof mapping !== 'string') return [];
    const trimmed = mapping.trim();
    if (!trimmed) return [];
    try {
      const parsed = JSON.parse(trimmed);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return [];
      }
      const values = Object.values(parsed)
        .map((value) => (typeof value === 'string' ? value.trim() : undefined))
        .filter((value) => value);
      return Array.from(new Set(values));
    } catch (error) {
      return [];
    }
  }, [inputs.model_mapping]);
  const redirectModelKeyList = useMemo(() => {
    const mapping = inputs.model_mapping;
    if (typeof mapping !== 'string') return [];
    const trimmed = mapping.trim();
    if (!trimmed) return [];
    try {
      const parsed = JSON.parse(trimmed);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return [];
      }
      return Array.from(
        new Set(
          Object.keys(parsed)
            .map((value) => (typeof value === 'string' ? value.trim() : ''))
            .filter(Boolean),
        ),
      );
    } catch (error) {
      return [];
    }
  }, [inputs.model_mapping]);
  const modelSearchMatchedCount = useMemo(() => {
    const keyword = modelSearchValue.trim();
    if (!keyword) {
      return modelOptions.length;
    }
    return modelOptions.reduce(
      (count, option) => count + (selectFilter(keyword, option) ? 1 : 0),
      0,
    );
  }, [modelOptions, modelSearchValue]);
  const modelSearchHintText = useMemo(() => {
    const keyword = modelSearchValue.trim();
    if (!keyword || modelSearchMatchedCount !== 0) {
      return '';
    }
    return t('未匹配到模型，按回车键可将「{{name}}」作为自定义模型名添加', {
      name: keyword,
    });
  }, [modelSearchMatchedCount, modelSearchValue, t]);
  const [isIonetChannel, setIsIonetChannel] = useState(false);
  const [ionetMetadata, setIonetMetadata] = useState(null);
  const [codexOAuthModalVisible, setCodexOAuthModalVisible] = useState(false);
  const [codexCredentialRefreshing, setCodexCredentialRefreshing] =
    useState(false);
  const [paramOverrideEditorVisible, setParamOverrideEditorVisible] =
    useState(false);
  const [clipboardConfig, setClipboardConfig] = useState(null);
  const [advancedSettingsOpen, setAdvancedSettingsOpen] = useState(false);

  // 周期配额相关状态
  const [quotaUsage, setQuotaUsage] = useState(null);
  const [inheritedTagPolicy, setInheritedTagPolicy] = useState(null);
  const [quotaUsageRefreshTick, setQuotaUsageRefreshTick] = useState(0);

  // 密钥显示状态
  const [keyDisplayState, setKeyDisplayState] = useState({
    showModal: false,
    keyData: '',
  });

  // 专门的2FA验证状态（用于TwoFactorAuthModal）
  const [show2FAVerifyModal, setShow2FAVerifyModal] = useState(false);
  const [verifyCode, setVerifyCode] = useState('');

  useEffect(() => {
    if (!isEdit) {
      setIsIonetChannel(false);
      setIonetMetadata(null);
    }
  }, [isEdit]);

  const handleOpenIonetDeployment = () => {
    if (!ionetMetadata?.deployment_id) {
      return;
    }
    const targetUrl = `/console/deployment?deployment_id=${ionetMetadata.deployment_id}`;
    window.open(targetUrl, '_blank', 'noopener');
  };
  const [verifyLoading, setVerifyLoading] = useState(false);
  const statusCodeRiskConfirmResolverRef = useRef(null);
  const [statusCodeRiskConfirmVisible, setStatusCodeRiskConfirmVisible] =
    useState(false);
  const [statusCodeRiskDetailItems, setStatusCodeRiskDetailItems] = useState(
    [],
  );

  // 表单块导航相关状态
  const formSectionRefs = useRef({
    basicInfo: null,
    apiConfig: null,
    modelConfig: null,
    advancedSettings: null,
    quotaSettings: null,
    channelExtraSettings: null,
  });
  const [currentSectionIndex, setCurrentSectionIndex] = useState(0);
  const formSections = [
    'basicInfo',
    'apiConfig',
    'modelConfig',
    'advancedSettings',
    'quotaSettings',
    'channelExtraSettings',
  ];
  const formContainerRef = useRef(null);
  const doubaoApiClickCountRef = useRef(0);
  const initialModelsRef = useRef([]);
  const initialGroupsRef = useRef([]);
  const initialModelMappingRef = useRef('');
  const initialStatusCodeMappingRef = useRef('');

  const toggleAdvancedSettings = (open) => {
    setAdvancedSettingsOpen(open);
    localStorage.setItem(ADVANCED_SETTINGS_EXPANDED_KEY, String(open));
  };

  // 2FA状态更新辅助函数
  const updateTwoFAState = (updates) => {
    setTwoFAState((prev) => ({ ...prev, ...updates }));
  };
  // 使用通用安全验证 Hook
  const {
    isModalVisible,
    verificationMethods,
    verificationState,
    withVerification,
    executeVerification,
    cancelVerification,
    setVerificationCode,
    switchVerificationMethod,
  } = useSecureVerification({
    onSuccess: (result) => {
      // 验证成功后显示密钥
      console.log('Verification success, result:', result);
      if (result && result.success && result.data?.key) {
        showSuccess(t('密钥获取成功'));
        setKeyDisplayState({
          showModal: true,
          keyData: result.data.key,
        });
      } else if (result && result.key) {
        // 直接返回了 key（没有包装在 data 中）
        showSuccess(t('密钥获取成功'));
        setKeyDisplayState({
          showModal: true,
          keyData: result.key,
        });
      }
    },
  });

  // 重置密钥显示状态
  const resetKeyDisplayState = () => {
    setKeyDisplayState({
      showModal: false,
      keyData: '',
    });
  };

  // 重置2FA验证状态
  const reset2FAVerifyState = () => {
    setShow2FAVerifyModal(false);
    setVerifyCode('');
    setVerifyLoading(false);
  };

  // 表单导航功能
  const scrollToSection = (sectionKey) => {
    const sectionElement = formSectionRefs.current[sectionKey];
    if (sectionElement) {
      sectionElement.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
        inline: 'nearest',
      });
    }
  };

  const navigateToSection = (direction) => {
    const availableSections = formSections.filter((section) => {
      if (section === 'apiConfig') {
        return showApiConfigCard;
      }
      return true;
    });

    let newIndex;
    if (direction === 'up') {
      newIndex =
        currentSectionIndex > 0
          ? currentSectionIndex - 1
          : availableSections.length - 1;
    } else {
      newIndex =
        currentSectionIndex < availableSections.length - 1
          ? currentSectionIndex + 1
          : 0;
    }

    setCurrentSectionIndex(newIndex);
    scrollToSection(availableSections[newIndex]);
  };

  const handleApiConfigSecretClick = () => {
    if (inputs.type !== 45) return;
    const next = doubaoApiClickCountRef.current + 1;
    doubaoApiClickCountRef.current = next;
    if (next >= 10) {
      setDoubaoApiEditUnlocked((unlocked) => {
        if (!unlocked) {
          showInfo(t('已解锁豆包自定义 API 地址编辑'));
        }
        return true;
      });
    }
  };

  // 渠道额外设置状态
  const [channelSettings, setChannelSettings] = useState({
    nonstream_response_header_timeout_sec: 0,
    force_format: false,
    cpa_user_identity_enabled: false,
    cpa_instance_id: '',
    thinking_to_content: false,
    proxy: '',
    pass_through_body_enabled: false,
    chat_completions_to_responses_mode: 'inherit',
    system_prompt: '',
    client_restriction_mode: '',
    client_restriction_clients: [],
    crs_site_id: 0,
    crs_platform: 'openai-responses',
    crs_auto_manage: false,
  });
  const [crsSites, setCrsSites] = useState([]);

  useEffect(() => {
    if (!props.visible) return;
    let cancelled = false;
    API.get('/api/crs/sites')
      .then((res) => {
        if (!cancelled && res.data?.success) {
          setCrsSites(res.data.data || []);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [props.visible]);
  const showApiConfigCard = true; // 控制是否显示 API 配置卡片
  const getInitValues = () => ({ ...originInputs });

  // 处理渠道额外设置的更新
  const handleChannelSettingsChange = (key, value) => {
    // 更新内部状态
    setChannelSettings((prev) => ({ ...prev, [key]: value }));

    // 同步更新到表单字段
    if (formApiRef.current) {
      formApiRef.current.setValue(key, value);
    }

    // 基于原始 setting 合并，避免编辑一个字段时删除当前前端不认识的配置。
    setInputs((prev) => {
      const settingsJson = JSON.stringify(
        mergeChannelJsonObject(prev.setting, { [key]: value }),
      );
      if (formApiRef.current) {
        formApiRef.current.setValue('setting', settingsJson);
      }
      return { ...prev, [key]: value, setting: settingsJson };
    });
  };

  const handleChannelOtherSettingsChange = (key, value) => {
    // 更新内部状态
    setChannelSettings((prev) => ({ ...prev, [key]: value }));

    // 同步更新到表单字段
    if (formApiRef.current) {
      formApiRef.current.setValue(key, value);
    }

    // 基于原始 settings 合并，保留后端新增或当前页面尚未展示的字段。
    setInputs((prev) => {
      const settingsJson = JSON.stringify(
        mergeChannelJsonObject(prev.settings, { [key]: value }),
      );
      if (formApiRef.current) {
        formApiRef.current.setValue('settings', settingsJson);
      }
      return { ...prev, [key]: value, settings: settingsJson };
    });
  };

  const applyClipboardConfig = (config) => {
    if (!config) return;
    setInputs((prev) => ({
      ...prev,
      key: config.key,
      base_url: config.url,
    }));
    if (formApiRef.current) {
      formApiRef.current.setValue('key', config.key);
      formApiRef.current.setValue('base_url', config.url);
    }
    setClipboardConfig(null);
    showSuccess(t('连接信息已填入'));
  };

  const pasteFromClipboard = async () => {
    if (!navigator?.clipboard?.readText) {
      showError(t('无法读取剪贴板'));
      return;
    }
    try {
      const text = await navigator.clipboard.readText();
      const parsed = parseChannelConnectionString(text);
      if (parsed) {
        applyClipboardConfig(parsed);
      } else {
        showInfo(t('剪贴板中未检测到连接信息'));
      }
    } catch {
      showError(t('无法读取剪贴板'));
    }
  };

  const isIonetLocked = isIonetChannel && isEdit;

  const formatUnixTime = (timestamp) => {
    const value = Number(timestamp || 0);
    if (!value) {
      return t('暂无');
    }
    return new Date(value * 1000).toLocaleString();
  };

  const handleInputChange = (name, value) => {
    if (
      isIonetChannel &&
      isEdit &&
      ['type', 'key', 'base_url'].includes(name)
    ) {
      return;
    }
    if (formApiRef.current) {
      formApiRef.current.setValue(name, value);
    }
    if (name === 'models' && Array.isArray(value)) {
      value = Array.from(new Set(value.map((m) => (m || '').trim())));
    }

    if (name === 'base_url' && value.endsWith('/v1')) {
      Modal.confirm({
        title: '警告',
        content:
          '不需要在末尾加/v1，New API会自动处理，添加后可能导致请求失败，是否继续？',
        onOk: () => {
          setInputs((inputs) => ({ ...inputs, [name]: value }));
        },
      });
      return;
    }
    setInputs((inputs) => ({ ...inputs, [name]: value }));
    if (name === 'type') {
      let localModels = [];
      switch (value) {
        case 2:
          localModels = [
            'mj_imagine',
            'mj_variation',
            'mj_reroll',
            'mj_blend',
            'mj_upscale',
            'mj_describe',
            'mj_uploads',
          ];
          break;
        case 5:
          localModels = [
            'swap_face',
            'mj_imagine',
            'mj_video',
            'mj_edits',
            'mj_variation',
            'mj_reroll',
            'mj_blend',
            'mj_upscale',
            'mj_describe',
            'mj_zoom',
            'mj_shorten',
            'mj_modal',
            'mj_inpaint',
            'mj_custom_zoom',
            'mj_high_variation',
            'mj_low_variation',
            'mj_pan',
            'mj_uploads',
          ];
          break;
        case 36:
          localModels = ['suno_music', 'suno_lyrics'];
          break;
        case 45:
          localModels = getChannelModels(value);
          setInputs((prevInputs) => ({
            ...prevInputs,
            base_url: 'https://ark.cn-beijing.volces.com',
          }));
          break;
        default:
          localModels = getChannelModels(value);
          break;
      }
      if (inputs.models.length === 0) {
        setInputs((inputs) => ({ ...inputs, models: localModels }));
      }
      setBasicModels(localModels);

      // 重置手动输入模式状态
      setUseManualInput(false);

      if (value === 57) {
        setBatch(false);
        setMultiToSingle(false);
        setMultiKeyMode('random');
        setVertexKeys([]);
        setVertexFileList([]);
        if (formApiRef.current) {
          formApiRef.current.setValue('vertex_files', []);
        }
        setInputs((prev) => ({ ...prev, vertex_files: [] }));
      }
    }
    //setAutoBan
  };

  const formatJsonField = (fieldName) => {
    const rawValue = (inputs?.[fieldName] ?? '').trim();
    if (!rawValue) return;

    try {
      const parsed = JSON.parse(rawValue);
      handleInputChange(fieldName, JSON.stringify(parsed, null, 2));
    } catch (error) {
      showError(`${t('JSON格式错误')}: ${error.message}`);
    }
  };

  const clearModelMapping = () => {
    handleInputChange('model_mapping', '');
    if (formApiRef.current) {
      formApiRef.current.setValue('model_mapping', '');
    }
  };

  const loadChannel = async () => {
    setLoading(true);
    let res = await API.get(`/api/channel/${channelId}`);
    if (res === undefined) {
      setLoading(false);
      return;
    }
    const { success, message, data } = res.data;
    if (success) {
      if (data.models === '') {
        data.models = [];
      } else {
        data.models = data.models.split(',');
      }
      if (data.group === '') {
        data.groups = [];
      } else {
        data.groups = data.group.split(',');
      }
      initialGroupsRef.current = normalizeChannelGroups(data.groups);
      if (data.model_mapping !== '') {
        data.model_mapping = JSON.stringify(
          JSON.parse(data.model_mapping),
          null,
          2,
        );
      }
      const chInfo = data.channel_info || {};
      const isMulti = chInfo.is_multi_key === true;
      setIsMultiKeyChannel(isMulti);
      if (isMulti) {
        setBatch(true);
        setMultiToSingle(true);
        const modeVal = chInfo.multi_key_mode || 'random';
        setMultiKeyMode(modeVal);
        data.multi_key_mode = modeVal;
      } else {
        setBatch(false);
        setMultiToSingle(false);
      }
      // 解析渠道额外设置并合并到data中
      if (data.setting) {
        try {
          const parsedSettings = JSON.parse(data.setting);
          data.force_format = parsedSettings.force_format || false;
          data.cpa_user_identity_enabled = parsedSettings.cpa_user_identity_enabled === true;
          data.cpa_instance_id = parsedSettings.cpa_instance_id || '';
          data.thinking_to_content =
            parsedSettings.thinking_to_content || false;
          data.proxy = parsedSettings.proxy || '';
          data.nonstream_response_header_timeout_sec =
            parsedSettings.nonstream_response_header_timeout_sec ?? 0;
          data.pass_through_body_enabled =
            parsedSettings.pass_through_body_enabled || false;
          data.chat_completions_to_responses_mode =
            parsedSettings.chat_completions_to_responses_mode || 'inherit';
          data.claude_incremental_cache_enabled =
            parsedSettings.claude_incremental_cache_enabled || false;
          data.system_prompt = parsedSettings.system_prompt || '';
          data.system_prompt_override =
            parsedSettings.system_prompt_override || false;
          data.max_concurrency = Number(parsedSettings.max_concurrency) || 0;
          data.rpm_limit = Number(parsedSettings.rpm_limit) || 0;
          data.client_restriction_mode =
            parsedSettings.client_restriction_mode || '';
          data.client_restriction_clients =
            parsedSettings.client_restriction_clients || [];
          data.crs_site_id = Number(parsedSettings.crs_site_id) || 0;
          data.crs_platform =
            parsedSettings.crs_platform || 'openai-responses';
          data.crs_auto_manage = parsedSettings.crs_auto_manage === true;
          const qp = parsedSettings.quota_policy || {};
          data.quota_policy_enabled = !!qp.enabled;
          data.quota_policy_period = qp.period || 'day';
          data.quota_policy_quota_limit = quotaToCurrencyAmount(
            qp.quota_limit,
          );
          data.quota_policy_count_limit = Number(qp.count_limit) || 0;
        } catch (error) {
          console.error('解析渠道设置失败:', error);
          data.force_format = false;
          data.cpa_user_identity_enabled = false;
          data.cpa_instance_id = '';
          data.thinking_to_content = false;
          data.proxy = '';
          data.nonstream_response_header_timeout_sec = 0;
          data.pass_through_body_enabled = false;
          data.chat_completions_to_responses_mode = 'inherit';
          data.claude_incremental_cache_enabled = false;
          data.system_prompt = '';
          data.system_prompt_override = false;
          data.max_concurrency = 0;
          data.rpm_limit = 0;
          data.client_restriction_mode = '';
          data.client_restriction_clients = [];
          data.crs_site_id = 0;
          data.crs_platform = 'openai-responses';
          data.crs_auto_manage = false;
          data.quota_policy_enabled = false;
          data.quota_policy_period = 'day';
          data.quota_policy_quota_limit = 0;
          data.quota_policy_count_limit = 0;
        }
      } else {
        data.force_format = false;
          data.cpa_user_identity_enabled = false;
          data.cpa_instance_id = '';
        data.thinking_to_content = false;
        data.claude_incremental_cache_enabled = false;
        data.proxy = '';
        data.nonstream_response_header_timeout_sec = 0;
        data.pass_through_body_enabled = false;
        data.chat_completions_to_responses_mode = 'inherit';
        data.system_prompt = '';
        data.system_prompt_override = false;
        data.max_concurrency = 0;
        data.rpm_limit = 0;
        data.client_restriction_mode = '';
        data.client_restriction_clients = [];
        data.crs_site_id = 0;
        data.crs_platform = 'openai-responses';
        data.crs_auto_manage = false;
        data.quota_policy_enabled = false;
        data.quota_policy_period = 'day';
        data.quota_policy_quota_limit = 0;
        data.quota_policy_count_limit = 0;
      }

      if (data.settings) {
        try {
          const parsedSettings = JSON.parse(data.settings);
          data.azure_responses_version =
            parsedSettings.azure_responses_version || '';
          // 读取 Vertex 密钥格式
          data.vertex_key_type = parsedSettings.vertex_key_type || 'json';
          data.vertex_project_id = parsedSettings.vertex_project_id || '';
          // 读取 AWS 密钥格式和区域
          data.aws_key_type = parsedSettings.aws_key_type || 'ak_sk';
          // 读取企业账户设置
          data.is_enterprise_account =
            parsedSettings.openrouter_enterprise === true;
          // 读取字段透传控制设置
          data.allow_service_tier = parsedSettings.allow_service_tier || false;
          data.disable_store = parsedSettings.disable_store || false;
          data.allow_safety_identifier =
            parsedSettings.allow_safety_identifier || false;
          data.allow_include_obfuscation =
            parsedSettings.allow_include_obfuscation || false;
          data.allow_inference_geo =
            parsedSettings.allow_inference_geo || false;
          data.chat_stream_options_mode =
            parsedSettings.chat_stream_options_mode || 'inherit';
          data.responses_api_mode =
            parsedSettings.responses_api_mode || 'inherit';
          data.responses_stream_options_mode =
            parsedSettings.responses_stream_options_mode || 'inherit';
          data.claude_beta_query = parsedSettings.claude_beta_query || false;
          data.upstream_model_update_check_enabled =
            parsedSettings.upstream_model_update_check_enabled === true;
          data.upstream_model_update_auto_sync_enabled =
            parsedSettings.upstream_model_update_auto_sync_enabled === true;
          data.upstream_model_update_last_check_time =
            Number(parsedSettings.upstream_model_update_last_check_time) || 0;
          data.upstream_model_update_last_detected_models = Array.isArray(
            parsedSettings.upstream_model_update_last_detected_models,
          )
            ? parsedSettings.upstream_model_update_last_detected_models
            : [];
          data.upstream_model_update_ignored_models = Array.isArray(
            parsedSettings.upstream_model_update_ignored_models,
          )
            ? parsedSettings.upstream_model_update_ignored_models.join(',')
            : '';
        } catch (error) {
          console.error('解析其他设置失败:', error);
          data.azure_responses_version = '';
          data.region = '';
          data.vertex_key_type = 'json';
          data.vertex_project_id = '';
          data.aws_key_type = 'ak_sk';
          data.is_enterprise_account = false;
          data.allow_service_tier = false;
          data.disable_store = false;
          data.allow_safety_identifier = false;
          data.allow_include_obfuscation = false;
          data.allow_inference_geo = false;
          data.chat_stream_options_mode = 'inherit';
          data.responses_api_mode = 'inherit';
          data.responses_stream_options_mode = 'inherit';
          data.claude_beta_query = false;
          data.upstream_model_update_check_enabled = false;
          data.upstream_model_update_auto_sync_enabled = false;
          data.upstream_model_update_last_check_time = 0;
          data.upstream_model_update_last_detected_models = [];
          data.upstream_model_update_ignored_models = '';
        }
      } else {
        // 兼容历史数据：老渠道没有 settings 时，默认按 json 展示
        data.vertex_key_type = 'json';
        data.vertex_project_id = '';
        data.aws_key_type = 'ak_sk';
        data.is_enterprise_account = false;
        data.allow_service_tier = false;
        data.disable_store = false;
        data.allow_safety_identifier = false;
        data.allow_include_obfuscation = false;
        data.allow_inference_geo = false;
        data.chat_stream_options_mode = 'inherit';
        data.responses_api_mode = 'inherit';
        data.responses_stream_options_mode = 'inherit';
        data.claude_beta_query = false;
        data.upstream_model_update_check_enabled = false;
        data.upstream_model_update_auto_sync_enabled = false;
        data.upstream_model_update_last_check_time = 0;
        data.upstream_model_update_last_detected_models = [];
        data.upstream_model_update_ignored_models = '';
      }

      if (
        data.type === 45 &&
        (!data.base_url ||
          (typeof data.base_url === 'string' && data.base_url.trim() === ''))
      ) {
        data.base_url = 'https://ark.cn-beijing.volces.com';
      }

      data.channel_vendor = data.channel_vendor || 'auto';

      setGroupVendor('');
      setInputs(data);
      if (formApiRef.current) {
        formApiRef.current.setValues(data, { isOverride: true });
      }
      if (data.auto_ban === 0) {
        setAutoBan(false);
      } else {
        setAutoBan(true);
      }
      // 同步企业账户状态
      setIsEnterpriseAccount(data.is_enterprise_account || false);
      setBasicModels(getChannelModels(data.type));
      // 同步更新channelSettings状态显示
      setChannelSettings({
        force_format: data.force_format,
        thinking_to_content: data.thinking_to_content,
        proxy: data.proxy,
        nonstream_response_header_timeout_sec: data.nonstream_response_header_timeout_sec,
        pass_through_body_enabled: data.pass_through_body_enabled,
        chat_completions_to_responses_mode:
          data.chat_completions_to_responses_mode || 'inherit',
        claude_incremental_cache_enabled:
          data.claude_incremental_cache_enabled || false,
        system_prompt: data.system_prompt,
        system_prompt_override: data.system_prompt_override || false,
        max_concurrency: data.max_concurrency || 0,
        rpm_limit: data.rpm_limit || 0,
        crs_site_id: data.crs_site_id || 0,
        crs_platform: data.crs_platform || 'openai-responses',
        crs_auto_manage: data.crs_auto_manage || false,
      });
      initialModelsRef.current = (data.models || [])
        .map((model) => (model || '').trim())
        .filter(Boolean);
      initialModelMappingRef.current = data.model_mapping || '';
      initialStatusCodeMappingRef.current = data.status_code_mapping || '';

      let parsedIonet = null;
      if (data.other_info) {
        try {
          const maybeMeta = JSON.parse(data.other_info);
          if (
            maybeMeta &&
            typeof maybeMeta === 'object' &&
            maybeMeta.source === 'ionet'
          ) {
            parsedIonet = maybeMeta;
          }
        } catch (error) {
          // ignore parse error
        }
      }
      const managedByIonet = !!parsedIonet;
      setIsIonetChannel(managedByIonet);
      setIonetMetadata(parsedIonet);
      // console.log(data);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const fetchUpstreamModelList = async (name, options = {}) => {
    const silent = !!options.silent;
    // if (inputs['type'] !== 1) {
    //   showError(t('仅支持 OpenAI 接口格式'));
    //   return;
    // }
    setLoading(true);
    const models = [];
    let err = false;

    if (isEdit) {
      // 如果是编辑模式，使用已有的 channelId 获取模型列表
      const res = await API.get('/api/channel/fetch_models/' + channelId, {
        skipErrorHandler: true,
      });
      if (res && res.data && res.data.success) {
        models.push(...res.data.data);
      } else {
        err = true;
      }
    } else {
      // 如果是新建模式，通过后端代理获取模型列表
      if (!inputs?.['key']) {
        showError(t('请填写密钥'));
        err = true;
      } else {
        try {
          const res = await API.post(
            '/api/channel/fetch_models',
            {
              base_url: inputs['base_url'],
              type: inputs['type'],
              key: inputs['key'],
            },
            { skipErrorHandler: true },
          );

          if (res && res.data && res.data.success) {
            models.push(...res.data.data);
          } else {
            err = true;
          }
        } catch (error) {
          console.error('Error fetching models:', error);
          err = true;
        }
      }
    }

    if (!err) {
      const uniqueModels = Array.from(new Set(models));
      setFetchedModels(uniqueModels);
      if (!silent) {
        setModelModalVisible(true);
      }
      setLoading(false);
      return uniqueModels;
    } else {
      showError(t('获取模型列表失败'));
    }
    setLoading(false);
    return null;
  };

  const openModelMappingValueModal = async ({ pairKey, value }) => {
    const mappingKey = String(pairKey ?? '').trim();
    if (!mappingKey) return;

    if (!MODEL_FETCHABLE_CHANNEL_TYPES.has(inputs.type)) {
      return;
    }

    let modelsToUse = fetchedModels;
    if (!Array.isArray(modelsToUse) || modelsToUse.length === 0) {
      const fetched = await fetchUpstreamModelList('models', { silent: true });
      if (Array.isArray(fetched)) {
        modelsToUse = fetched;
      }
    }

    if (!Array.isArray(modelsToUse) || modelsToUse.length === 0) {
      showInfo(t('暂无模型'));
      return;
    }

    const normalizedModelsToUse = Array.from(
      new Set(
        modelsToUse.map((model) => String(model ?? '').trim()).filter(Boolean),
      ),
    );
    const currentValue = String(value ?? '').trim();

    setModelMappingValueModalModels(normalizedModelsToUse);
    setModelMappingValueKey(mappingKey);
    setModelMappingValueSelected(
      normalizedModelsToUse.includes(currentValue) ? currentValue : '',
    );
    setModelMappingValueModalVisible(true);
  };

  const fetchModels = async () => {
    try {
      let res = await API.get(`/api/channel/models`);
      const localModelOptions = res.data.data.map((model) => {
        const id = (model.id || '').trim();
        return {
          key: id,
          label: id,
          value: id,
        };
      });
      setOriginModelOptions(localModelOptions);
      setFullModels(res.data.data.map((model) => model.id));
      setBasicModels(
        res.data.data
          .filter((model) => {
            return model.id.startsWith('gpt-') || model.id.startsWith('text-');
          })
          .map((model) => model.id),
      );
    } catch (error) {
      showError(error.message);
    }
  };

  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      if (res === undefined) {
        return;
      }
      setGroupOptions(
        res.data.data.map((group) => ({
          label: group,
          value: group,
        })),
      );
    } catch (error) {
      showError(error.message);
    }
  };

  useEffect(() => {
    if (groupVendorOptions.length === 0) {
      setGroupVendor('');
      return;
    }

    if (groupVendorOptions.some((option) => option.value === groupVendor)) {
      return;
    }

    const firstSelectedVendor = selectedGroupValues
      .map((group) => resolveTokenGroupVendor(group))
      .find((vendor) =>
        groupVendorOptions.some((option) => option.value === vendor),
      );
    setGroupVendor(firstSelectedVendor || groupVendorOptions[0].value);
  }, [groupVendor, groupVendorOptions, selectedGroupValues]);

  const fetchModelGroups = async () => {
    try {
      const res = await API.get('/api/prefill_group?type=model');
      if (res?.data?.success) {
        setModelGroups(res.data.data || []);
      }
    } catch (error) {
      // ignore
    }
  };

  // 查看渠道密钥（透明验证）
  const handleShow2FAModal = async () => {
    try {
      // 使用 withVerification 包装，会自动处理需要验证的情况
      const result = await withVerification(
        createApiCalls.viewChannelKey(channelId),
        {
          title: t('查看渠道密钥'),
          description: t('为了保护账户安全，请验证您的身份。'),
          preferredMethod: 'passkey', // 优先使用 Passkey
        },
      );

      // 如果直接返回了结果（已验证），显示密钥
      if (result && result.success && result.data?.key) {
        showSuccess(t('密钥获取成功'));
        setKeyDisplayState({
          showModal: true,
          keyData: result.data.key,
        });
      }
    } catch (error) {
      console.error('Failed to view channel key:', error);
      showError(error.message || t('获取密钥失败'));
    }
  };

  const handleCodexOAuthGenerated = (key) => {
    handleInputChange('key', key);
    formatJsonField('key');
  };

  const handleRefreshCodexCredential = async () => {
    if (!isEdit) return;

    setCodexCredentialRefreshing(true);
    try {
      const res = await API.post(
        `/api/channel/${channelId}/codex/refresh`,
        {},
        { skipErrorHandler: true },
      );
      if (!res?.data?.success) {
        throw new Error(res?.data?.message || 'Failed to refresh credential');
      }
      showSuccess(t('凭证已刷新'));
    } catch (error) {
      showError(error.message || t('刷新失败'));
    } finally {
      setCodexCredentialRefreshing(false);
    }
  };

  useEffect(() => {
    if (inputs.type !== 45) {
      doubaoApiClickCountRef.current = 0;
      setDoubaoApiEditUnlocked(false);
    }
  }, [inputs.type]);

  useEffect(() => {
    const modelMap = new Map();

    originModelOptions.forEach((option) => {
      const v = (option.value || '').trim();
      if (!modelMap.has(v)) {
        modelMap.set(v, option);
      }
    });

    inputs.models.forEach((model) => {
      const v = (model || '').trim();
      if (!modelMap.has(v)) {
        modelMap.set(v, {
          key: v,
          label: v,
          value: v,
        });
      }
    });

    const categories = getModelCategories(t);
    const optionsWithIcon = Array.from(modelMap.values()).map((opt) => {
      const modelName = opt.value;
      let icon = null;
      for (const [key, category] of Object.entries(categories)) {
        if (key !== 'all' && category.filter({ model_name: modelName })) {
          icon = category.icon;
          break;
        }
      }
      return {
        ...opt,
        label: (
          <span className='flex items-center gap-1'>
            {icon}
            {modelName}
          </span>
        ),
      };
    });

    setModelOptions(optionsWithIcon);
  }, [originModelOptions, inputs.models, t]);

  useEffect(() => {
    fetchModels().then();
    fetchGroups().then();
    if (!isEdit) {
      setGroupVendor('');
      setInputs(originInputs);
      if (formApiRef.current) {
        formApiRef.current.setValues(originInputs);
      }
      let localModels = getChannelModels(inputs.type);
      setBasicModels(localModels);
      setInputs((inputs) => ({ ...inputs, models: localModels }));
    }
  }, [props.editingChannel.id]);

  useEffect(() => {
    if (formApiRef.current) {
      formApiRef.current.setValues(inputs);
    }
  }, [inputs]);

  useEffect(() => {
    setModelSearchValue('');
    if (props.visible) {
      if (isEdit) {
        loadChannel();
      } else {
        formApiRef.current?.setValues(getInitValues());
      }
      fetchModelGroups();
      // 重置手动输入模式状态
      setUseManualInput(false);
      // 重置导航状态
      setCurrentSectionIndex(0);
      setAdvancedSettingsOpen(
        isEdit &&
          localStorage.getItem(ADVANCED_SETTINGS_EXPANDED_KEY) === 'true',
      );
      if (!isEdit) {
        setGroupVendor('');
        try {
          navigator?.clipboard
            ?.readText?.()
            ?.then((text) => {
              const parsed = parseChannelConnectionString(text);
              if (parsed) {
                setClipboardConfig(parsed);
              }
            })
            .catch(() => {});
        } catch {}
      }
    } else {
      // 统一的模态框关闭重置逻辑
      resetModalState();
    }
  }, [props.visible, channelId]);

  useEffect(() => {
    if (!isEdit) {
      initialModelsRef.current = [];
      initialGroupsRef.current = [];
      initialModelMappingRef.current = '';
      initialStatusCodeMappingRef.current = '';
    }
  }, [isEdit, props.visible]);

  // 获取当前渠道的周期配额用量
  useEffect(() => {
    if (!props.visible || !isEdit || !channelId) {
      setQuotaUsage(null);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await API.get(`/api/channel/${channelId}/quota_usage`);
        if (!cancelled && res?.data?.success) {
          const payload = res.data.data || {};
          if (!payload.found) {
            setQuotaUsage(null);
            return;
          }
          const usage = payload.usage || {};
          const policy = payload.policy || {};
          setQuotaUsage({
            scope: usage.scope || 'channel',
            period: usage.period || policy.period || '',
            period_start: usage.period_start || 0,
            period_end: usage.period_end || 0,
            used_quota: Number(usage.used_quota) || 0,
            used_count: Number(usage.used_count) || 0,
            quota_limit: Number(policy.quota_limit) || 0,
            count_limit: Number(policy.count_limit) || 0,
          });
        }
      } catch (e) {
        // 静默失败，旧版本后端不存在该接口时不影响表单
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [props.visible, isEdit, channelId, quotaUsageRefreshTick]);

  // 自身未启用时，拉取标签策略用于"将继承"提示
  useEffect(() => {
    if (!props.visible || inputs.quota_policy_enabled) {
      setInheritedTagPolicy(null);
      return;
    }
    const tagName = (inputs.tag || '').trim();
    if (!tagName) {
      setInheritedTagPolicy(null);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await API.get(
          `/api/channel/tag/quota_policy?tag=${encodeURIComponent(tagName)}`,
        );
        if (!cancelled && res?.data?.success) {
          const payload = res.data.data || {};
          setInheritedTagPolicy(payload.found ? payload.quota_policy : null);
        }
      } catch (e) {
        // ignore
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [props.visible, inputs.tag, inputs.quota_policy_enabled]);

  useEffect(() => {
    return () => {
      if (statusCodeRiskConfirmResolverRef.current) {
        statusCodeRiskConfirmResolverRef.current(false);
        statusCodeRiskConfirmResolverRef.current = null;
      }
    };
  }, []);

  // 统一的模态框重置函数
  const resetModalState = () => {
    resolveStatusCodeRiskConfirm(false);
    formApiRef.current?.reset();
    // 重置渠道设置状态
    setChannelSettings({
      nonstream_response_header_timeout_sec: 0,
      force_format: false,
    cpa_user_identity_enabled: false,
    cpa_instance_id: '',
      thinking_to_content: false,
      proxy: '',
      pass_through_body_enabled: false,
      chat_completions_to_responses_mode: 'inherit',
      claude_incremental_cache_enabled: false,
      system_prompt: '',
      system_prompt_override: false,
      client_restriction_mode: '',
      client_restriction_clients: [],
      crs_site_id: 0,
      crs_platform: 'openai-responses',
      crs_auto_manage: false,
    });
    // 重置密钥模式状态
    setKeyMode('append');
    // 重置企业账户状态
    setIsEnterpriseAccount(false);
    // 重置豆包隐藏入口状态
    setDoubaoApiEditUnlocked(false);
    doubaoApiClickCountRef.current = 0;
    setModelSearchValue('');
    // 清空表单中的key_mode字段
    if (formApiRef.current) {
      formApiRef.current.setValue('key_mode', undefined);
    }
    // 重置本地输入，避免下次打开残留上一次的 JSON 字段值
    setInputs(getInitValues());
    // 重置密钥显示状态
    resetKeyDisplayState();
    setClipboardConfig(null);
  };

  const handleVertexUploadChange = ({ fileList }) => {
    vertexErroredNames.current.clear();
    (async () => {
      let validFiles = [];
      let keys = [];
      const errorNames = [];
      for (const item of fileList) {
        const fileObj = item.fileInstance;
        if (!fileObj) continue;
        try {
          const txt = await fileObj.text();
          keys.push(JSON.parse(txt));
          validFiles.push(item);
        } catch (err) {
          if (!vertexErroredNames.current.has(item.name)) {
            errorNames.push(item.name);
            vertexErroredNames.current.add(item.name);
          }
        }
      }

      // 非批量模式下只保留一个文件（最新选择的），避免重复叠加
      if (!batch && validFiles.length > 1) {
        validFiles = [validFiles[validFiles.length - 1]];
        keys = [keys[keys.length - 1]];
      }

      setVertexKeys(keys);
      setVertexFileList(validFiles);
      if (formApiRef.current) {
        formApiRef.current.setValue('vertex_files', validFiles);
      }
      setInputs((prev) => ({ ...prev, vertex_files: validFiles }));

      if (errorNames.length > 0) {
        showError(
          t('以下文件解析失败，已忽略：{{list}}', {
            list: errorNames.join(', '),
          }),
        );
      }
    })();
  };

  const confirmMissingModelMappings = (missingModels) =>
    new Promise((resolve) => {
      const modal = Modal.confirm({
        title: t('模型未加入列表，可能无法调用'),
        content: (
          <div className='text-sm leading-6'>
            <div>
              {t(
                '模型重定向里的下列模型尚未添加到“模型”列表，调用时会因为缺少可用模型而失败：',
              )}
            </div>
            <div className='font-mono text-xs break-all text-red-600 mt-1'>
              {missingModels.join(', ')}
            </div>
            <div className='mt-2'>
              {t(
                '你可以在“自定义模型名称”处手动添加它们，然后点击填入后再提交，或者直接使用下方操作自动处理。',
              )}
            </div>
          </div>
        ),
        centered: true,
        footer: (
          <Space align='center' className='w-full justify-end'>
            <Button
              type='tertiary'
              onClick={() => {
                modal.destroy();
                resolve('cancel');
              }}
            >
              {t('返回修改')}
            </Button>
            <Button
              type='primary'
              theme='light'
              onClick={() => {
                modal.destroy();
                resolve('submit');
              }}
            >
              {t('直接提交')}
            </Button>
            <Button
              type='primary'
              theme='solid'
              onClick={() => {
                modal.destroy();
                resolve('add');
              }}
            >
              {t('添加后提交')}
            </Button>
          </Space>
        ),
      });
    });

  const resolveStatusCodeRiskConfirm = (confirmed) => {
    setStatusCodeRiskConfirmVisible(false);
    setStatusCodeRiskDetailItems([]);
    if (statusCodeRiskConfirmResolverRef.current) {
      statusCodeRiskConfirmResolverRef.current(confirmed);
      statusCodeRiskConfirmResolverRef.current = null;
    }
  };

  const confirmStatusCodeRisk = (detailItems) =>
    new Promise((resolve) => {
      statusCodeRiskConfirmResolverRef.current = resolve;
      setStatusCodeRiskDetailItems(detailItems);
      setStatusCodeRiskConfirmVisible(true);
    });

  const hasModelConfigChanged = (normalizedModels, modelMappingStr) => {
    if (!isEdit) return true;
    const initialModels = initialModelsRef.current;
    if (normalizedModels.length !== initialModels.length) {
      return true;
    }
    for (let i = 0; i < normalizedModels.length; i++) {
      if (normalizedModels[i] !== initialModels[i]) {
        return true;
      }
    }
    const normalizedMapping = (modelMappingStr || '').trim();
    const initialMapping = (initialModelMappingRef.current || '').trim();
    return normalizedMapping !== initialMapping;
  };

  const submit = async () => {
    if (loading) {
      return;
    }
    const formValues = formApiRef.current ? formApiRef.current.getValues() : {};
    let localInputs = mergeChannelFormValues(formValues, inputs);

    if (localInputs.type === 57) {
      if (batch) {
        showInfo(t('Codex 渠道不支持批量创建'));
        return;
      }

      const rawKey = (localInputs.key || '').trim();
      if (!isEdit && rawKey === '') {
        showInfo(t('请输入密钥！'));
        return;
      }

      if (rawKey !== '') {
        if (!verifyJSON(rawKey)) {
          showInfo(t('密钥必须是合法的 JSON 格式！'));
          return;
        }
        try {
          const parsed = JSON.parse(rawKey);
          if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
            showInfo(t('密钥必须是 JSON 对象'));
            return;
          }
          const accessToken = String(parsed.access_token || '').trim();
          const accountId = String(parsed.account_id || '').trim();
          if (!accessToken) {
            showInfo(t('密钥 JSON 必须包含 access_token'));
            return;
          }
          if (!accountId) {
            showInfo(t('密钥 JSON 必须包含 account_id'));
            return;
          }
          localInputs.key = JSON.stringify(parsed);
        } catch (error) {
          showInfo(t('密钥必须是合法的 JSON 格式！'));
          return;
        }
      }
    }

    if (localInputs.type === 41) {
      const keyType = localInputs.vertex_key_type || 'json';
      if (keyType === 'api_key') {
        // 直接作为普通字符串密钥处理
        if (!isEdit && (!localInputs.key || localInputs.key.trim() === '')) {
          showInfo(t('请输入密钥！'));
          return;
        }
      } else {
        // JSON 服务账号密钥
        if (useManualInput) {
          if (localInputs.key && localInputs.key.trim() !== '') {
            try {
              const parsedKey = JSON.parse(localInputs.key);
              localInputs.key = JSON.stringify(parsedKey);
            } catch (err) {
              showError(t('密钥格式无效，请输入有效的 JSON 格式密钥'));
              return;
            }
          } else if (!isEdit) {
            showInfo(t('请输入密钥！'));
            return;
          }
        } else {
          // 文件上传模式
          let keys = vertexKeys;
          if (keys.length === 0 && vertexFileList.length > 0) {
            try {
              const parsed = await Promise.all(
                vertexFileList.map(async (item) => {
                  const fileObj = item.fileInstance;
                  if (!fileObj) return null;
                  const txt = await fileObj.text();
                  return JSON.parse(txt);
                }),
              );
              keys = parsed.filter(Boolean);
            } catch (err) {
              showError(t('解析密钥文件失败: {{msg}}', { msg: err.message }));
              return;
            }
          }
          if (keys.length === 0) {
            if (!isEdit) {
              showInfo(t('请上传密钥文件！'));
              return;
            } else {
              delete localInputs.key;
            }
          } else {
            localInputs.key = batch
              ? JSON.stringify(keys)
              : JSON.stringify(keys[0]);
          }
        }
      }
    }

    // 如果是编辑模式且 key 为空字符串，避免提交空值覆盖旧密钥
    if (isEdit && (!localInputs.key || localInputs.key.trim() === '')) {
      delete localInputs.key;
    }
    delete localInputs.vertex_files;

    if (!isEdit && (!localInputs.name || !localInputs.key)) {
      showInfo(t('请填写渠道名称和渠道密钥！'));
      return;
    }
    if (!Array.isArray(localInputs.models) || localInputs.models.length === 0) {
      showInfo(t('请至少选择一个模型！'));
      return;
    }
    localInputs.groups = normalizeChannelGroups(localInputs.groups);
    if (localInputs.groups.length === 0) {
      showInfo(t('请选择分组'));
      return;
    }
    if (
      localInputs.type === 45 &&
      (!localInputs.base_url || localInputs.base_url.trim() === '')
    ) {
      showInfo(t('请输入API地址！'));
      return;
    }
    const hasModelMapping =
      typeof localInputs.model_mapping === 'string' &&
      localInputs.model_mapping.trim() !== '';
    let parsedModelMapping = null;
    if (hasModelMapping) {
      if (!verifyJSON(localInputs.model_mapping)) {
        showInfo(t('模型映射必须是合法的 JSON 格式！'));
        return;
      }
      try {
        parsedModelMapping = JSON.parse(localInputs.model_mapping);
      } catch (error) {
        showInfo(t('模型映射必须是合法的 JSON 格式！'));
        return;
      }
    }

    const normalizedModels = (localInputs.models || [])
      .map((model) => (model || '').trim())
      .filter(Boolean);
    localInputs.models = normalizedModels;

    if (
      parsedModelMapping &&
      typeof parsedModelMapping === 'object' &&
      !Array.isArray(parsedModelMapping)
    ) {
      const modelSet = new Set(normalizedModels);
      const missingModels = Object.keys(parsedModelMapping)
        .map((key) => (key || '').trim())
        .filter((key) => key && !modelSet.has(key));
      const shouldPromptMissing =
        missingModels.length > 0 &&
        hasModelConfigChanged(normalizedModels, localInputs.model_mapping);
      if (shouldPromptMissing) {
        const confirmAction = await confirmMissingModelMappings(missingModels);
        if (confirmAction === 'cancel') {
          return;
        }
        if (confirmAction === 'add') {
          const updatedModels = Array.from(
            new Set([...normalizedModels, ...missingModels]),
          );
          localInputs.models = updatedModels;
          handleInputChange('models', updatedModels);
        }
      }
    }

    const invalidStatusCodeEntries = collectInvalidStatusCodeEntries(
      localInputs.status_code_mapping,
    );
    if (invalidStatusCodeEntries.length > 0) {
      showError(
        `${t('状态码复写包含无效的状态码')}: ${invalidStatusCodeEntries.join(', ')}`,
      );
      return;
    }

    const riskyStatusCodeRedirects = collectNewDisallowedStatusCodeRedirects(
      initialStatusCodeMappingRef.current,
      localInputs.status_code_mapping,
    );
    if (riskyStatusCodeRedirects.length > 0) {
      const confirmed = await confirmStatusCodeRisk(riskyStatusCodeRedirects);
      if (!confirmed) {
        return;
      }
    }

    if (localInputs.base_url && localInputs.base_url.endsWith('/')) {
      localInputs.base_url = localInputs.base_url.slice(
        0,
        localInputs.base_url.length - 1,
      );
    }
    if (localInputs.type === 18 && localInputs.other === '') {
      localInputs.other = 'v2.1';
    }

    const normalizedClientRestrictionClients = (
      localInputs.client_restriction_clients || []
    )
      .map((client) => (typeof client === 'string' ? client.trim() : ''))
      .filter(Boolean);
    localInputs.client_restriction_clients = normalizedClientRestrictionClients;
    if (
      localInputs.client_restriction_mode === 'allowlist' &&
      normalizedClientRestrictionClients.length === 0
    ) {
      showError(t('白名单模式至少选择一个客户端'));
      return;
    }
    if (localInputs.crs_auto_manage && !(Number(localInputs.crs_site_id) > 0)) {
      showError(t('开启 CRS 自动管理前请选择 CRS 站点'));
      return;
    }

    // 基于原始 setting 合并，避免无修改提交或编辑单个字段时丢失未知配置。
    const originalChannelExtraSettings = parseChannelJsonObject(
      localInputs.setting,
    );
    const originalQuotaPolicy =
      originalChannelExtraSettings.quota_policy &&
      typeof originalChannelExtraSettings.quota_policy === 'object' &&
      !Array.isArray(originalChannelExtraSettings.quota_policy)
        ? originalChannelExtraSettings.quota_policy
        : {};

    const nonstreamHeaderTimeout = Number(
      localInputs.nonstream_response_header_timeout_sec ?? 0,
    );
    if (!Number.isInteger(nonstreamHeaderTimeout) || nonstreamHeaderTimeout < 0 || nonstreamHeaderTimeout > 600) {
      showError(t('非流式响应头超时必须为 0 到 600 的整数'));
      return;
    }
    // 生成渠道额外设置JSON
    const channelExtraSettings = {
      ...originalChannelExtraSettings,
      force_format: localInputs.force_format || false,
      cpa_user_identity_enabled: localInputs.cpa_user_identity_enabled === true,
      cpa_instance_id: (localInputs.cpa_instance_id || '').trim(),
      thinking_to_content: localInputs.thinking_to_content || false,
      proxy: localInputs.proxy || '',
      nonstream_response_header_timeout_sec: nonstreamHeaderTimeout,
      pass_through_body_enabled: localInputs.pass_through_body_enabled || false,
      chat_completions_to_responses_mode:
        localInputs.chat_completions_to_responses_mode || 'inherit',
      claude_incremental_cache_enabled:
        localInputs.claude_incremental_cache_enabled || false,
      system_prompt: localInputs.system_prompt || '',
      system_prompt_override: localInputs.system_prompt_override || false,
      max_concurrency: Math.max(0, Number(localInputs.max_concurrency) || 0),
      rpm_limit: Math.max(0, Number(localInputs.rpm_limit) || 0),
      client_restriction_mode: localInputs.client_restriction_mode || '',
      client_restriction_clients: normalizedClientRestrictionClients,
      crs_site_id: Math.max(0, Number(localInputs.crs_site_id) || 0),
      crs_platform: localInputs.crs_platform || 'openai-responses',
      crs_auto_manage: localInputs.crs_auto_manage === true,
      quota_policy: {
        ...originalQuotaPolicy,
        enabled: !!localInputs.quota_policy_enabled,
        period: localInputs.quota_policy_period || 'day',
        quota_limit: currencyAmountToQuota(
          localInputs.quota_policy_quota_limit,
        ),
        count_limit: Math.max(0, Number(localInputs.quota_policy_count_limit) || 0),
      },
    };
    localInputs.setting = JSON.stringify(channelExtraSettings);

    // 处理 settings 字段（包括企业账户设置和字段透传控制）
    let settings = parseChannelJsonObject(localInputs.settings);

    // type === 20: 设置企业账户标识，无论是true还是false都要传到后端
    if (localInputs.type === 20) {
      settings.openrouter_enterprise =
        localInputs.is_enterprise_account === true;
    }

    // type === 33 (AWS): 保存 aws_key_type 到 settings
    if (localInputs.type === 33) {
      settings.aws_key_type = localInputs.aws_key_type || 'ak_sk';
    }

    // type === 41 (Vertex): 始终保存 vertex_key_type 到 settings，避免编辑时被重置
    if (localInputs.type === 41) {
      settings.vertex_key_type = localInputs.vertex_key_type || 'json';
      settings.vertex_project_id = (localInputs.vertex_project_id || '').trim();
    } else {
      delete settings.vertex_key_type;
      delete settings.vertex_project_id;
    }

    const capabilityModes = [
      'chat_stream_options_mode',
      'responses_api_mode',
      'responses_stream_options_mode',
    ];
    capabilityModes.forEach((key) => {
      const value = localInputs[key] || 'inherit';
      if (value === 'inherit') {
        delete settings[key];
      } else {
        settings[key] = value;
      }
    });

    // type === 1 (OpenAI) 或 type === 14 (Claude): 设置字段透传控制（显式保存布尔值）
    if (localInputs.type === 1 || localInputs.type === 14) {
      settings.allow_service_tier = localInputs.allow_service_tier === true;
      // 仅 OpenAI 渠道需要 store / safety_identifier / include_obfuscation
      if (localInputs.type === 1) {
        settings.disable_store = localInputs.disable_store === true;
        settings.allow_safety_identifier =
          localInputs.allow_safety_identifier === true;
        settings.allow_include_obfuscation =
          localInputs.allow_include_obfuscation === true;
      }
      if (localInputs.type === 14) {
        settings.allow_inference_geo = localInputs.allow_inference_geo === true;
        settings.claude_beta_query = localInputs.claude_beta_query === true;
      }
    }

    settings.upstream_model_update_check_enabled =
      localInputs.upstream_model_update_check_enabled === true;
    settings.upstream_model_update_auto_sync_enabled =
      settings.upstream_model_update_check_enabled &&
      localInputs.upstream_model_update_auto_sync_enabled === true;
    settings.upstream_model_update_ignored_models = Array.from(
      new Set(
        String(localInputs.upstream_model_update_ignored_models || '')
          .split(',')
          .map((model) => model.trim())
          .filter(Boolean),
      ),
    );
    if (
      !Array.isArray(settings.upstream_model_update_last_detected_models) ||
      !settings.upstream_model_update_check_enabled
    ) {
      settings.upstream_model_update_last_detected_models = [];
    }
    if (
      !Array.isArray(settings.upstream_model_update_last_removed_models) ||
      !settings.upstream_model_update_check_enabled
    ) {
      settings.upstream_model_update_last_removed_models = [];
    }
    if (typeof settings.upstream_model_update_last_check_time !== 'number') {
      settings.upstream_model_update_last_check_time = 0;
    }

    localInputs.settings = JSON.stringify(settings);

    // 清理不需要发送到后端的字段
    delete localInputs.force_format;
    delete localInputs.cpa_user_identity_enabled;
    delete localInputs.cpa_instance_id;
    delete localInputs.thinking_to_content;
    delete localInputs.client_restriction_mode;
    delete localInputs.client_restriction_clients;
    delete localInputs.proxy;
    delete localInputs.nonstream_response_header_timeout_sec;
    delete localInputs.pass_through_body_enabled;
    delete localInputs.system_prompt;
    delete localInputs.system_prompt_override;
    delete localInputs.is_enterprise_account;
    delete localInputs.quota_policy_enabled;
    delete localInputs.quota_policy_period;
    delete localInputs.quota_policy_quota_limit;
    delete localInputs.quota_policy_count_limit;
    // 顶层的 vertex_key_type 不应发送给后端
    delete localInputs.vertex_key_type;
    delete localInputs.vertex_project_id;
    // 顶层的 aws_key_type 不应发送给后端
    delete localInputs.aws_key_type;
    // 清理字段透传控制的临时字段
    delete localInputs.allow_service_tier;
    delete localInputs.disable_store;
    delete localInputs.allow_safety_identifier;
    delete localInputs.allow_include_obfuscation;
    delete localInputs.allow_inference_geo;
    delete localInputs.chat_stream_options_mode;
    delete localInputs.responses_api_mode;
    delete localInputs.responses_stream_options_mode;
    delete localInputs.claude_beta_query;
    delete localInputs.upstream_model_update_check_enabled;
    delete localInputs.upstream_model_update_auto_sync_enabled;
    delete localInputs.upstream_model_update_last_check_time;
    delete localInputs.upstream_model_update_last_detected_models;
    delete localInputs.upstream_model_update_ignored_models;

    let res;
    localInputs.auto_ban = localInputs.auto_ban ? 1 : 0;
    localInputs.models = localInputs.models.join(',');
    localInputs.group_touched = !areChannelGroupsEqual(
      localInputs.groups,
      initialGroupsRef.current,
    );
    localInputs.group = (localInputs.groups || []).join(',');

    let mode = 'single';
    if (batch) {
      mode = multiToSingle ? 'multi_to_single' : 'batch';
    }

    if (isEdit) {
      res = await API.put(`/api/channel/`, {
        ...localInputs,
        id: parseInt(channelId),
        key_mode: isMultiKeyChannel ? keyMode : undefined, // 只在多key模式下传递
      });
    } else {
      res = await API.post(`/api/channel/`, {
        mode: mode,
        multi_key_mode: mode === 'multi_to_single' ? multiKeyMode : undefined,
        channel: localInputs,
      });
    }
    const { success, message } = res.data;
    if (success) {
      if (isEdit) {
        showSuccess(t('渠道更新成功！'));
      } else {
        showSuccess(t('渠道创建成功！'));
        setInputs(originInputs);
      }
      props.refresh();
      props.handleClose();
    } else {
      showError(message);
    }
  };

  // 密钥去重函数
  const deduplicateKeys = () => {
    const currentKey = formApiRef.current?.getValue('key') || inputs.key || '';

    if (!currentKey.trim()) {
      showInfo(t('请先输入密钥'));
      return;
    }

    // 按行分割密钥
    const keyLines = currentKey.split('\n');
    const beforeCount = keyLines.length;

    // 使用哈希表去重，保持原有顺序
    const keySet = new Set();
    const deduplicatedKeys = [];

    keyLines.forEach((line) => {
      const trimmedLine = line.trim();
      if (trimmedLine && !keySet.has(trimmedLine)) {
        keySet.add(trimmedLine);
        deduplicatedKeys.push(trimmedLine);
      }
    });

    const afterCount = deduplicatedKeys.length;
    const deduplicatedKeyText = deduplicatedKeys.join('\n');

    // 更新表单和状态
    if (formApiRef.current) {
      formApiRef.current.setValue('key', deduplicatedKeyText);
    }
    handleInputChange('key', deduplicatedKeyText);

    // 显示去重结果
    const message = t(
      '去重完成：去重前 {{before}} 个密钥，去重后 {{after}} 个密钥',
      {
        before: beforeCount,
        after: afterCount,
      },
    );

    if (beforeCount === afterCount) {
      showInfo(t('未发现重复密钥'));
    } else {
      showSuccess(message);
    }
  };

  const addCustomModels = () => {
    if (customModel.trim() === '') return;
    const modelArray = customModel.split(',').map((model) => model.trim());

    let localModels = [...inputs.models];
    let localModelOptions = [...modelOptions];
    const addedModels = [];

    modelArray.forEach((model) => {
      if (model && !localModels.includes(model)) {
        localModels.push(model);
        localModelOptions.push({
          key: model,
          label: model,
          value: model,
        });
        addedModels.push(model);
      }
    });

    setModelOptions(localModelOptions);
    setCustomModel('');
    handleInputChange('models', localModels);

    if (addedModels.length > 0) {
      showSuccess(
        t('已新增 {{count}} 个模型：{{list}}', {
          count: addedModels.length,
          list: addedModels.join(', '),
        }),
      );
    } else {
      showInfo(t('未发现新增模型'));
    }
  };

  const batchAllowed = (!isEdit || isMultiKeyChannel) && inputs.type !== 57;
  const batchExtra = batchAllowed ? (
    <Space>
      {!isEdit && (
        <Checkbox
          disabled={isEdit}
          checked={batch}
          onChange={(e) => {
            const checked = e.target.checked;

            if (!checked && vertexFileList.length > 1) {
              Modal.confirm({
                title: t('切换为单密钥模式'),
                content: t(
                  '将仅保留第一个密钥文件，其余文件将被移除，是否继续？',
                ),
                onOk: () => {
                  const firstFile = vertexFileList[0];
                  const firstKey = vertexKeys[0] ? [vertexKeys[0]] : [];

                  setVertexFileList([firstFile]);
                  setVertexKeys(firstKey);

                  formApiRef.current?.setValue('vertex_files', [firstFile]);
                  setInputs((prev) => ({ ...prev, vertex_files: [firstFile] }));

                  setBatch(false);
                  setMultiToSingle(false);
                  setMultiKeyMode('random');
                },
                onCancel: () => {
                  setBatch(true);
                },
                centered: true,
              });
              return;
            }

            setBatch(checked);
            if (!checked) {
              setMultiToSingle(false);
              setMultiKeyMode('random');
            } else {
              // 批量模式下禁用手动输入，并清空手动输入的内容
              setUseManualInput(false);
              if (inputs.type === 41) {
                // 清空手动输入的密钥内容
                if (formApiRef.current) {
                  formApiRef.current.setValue('key', '');
                }
                handleInputChange('key', '');
              }
            }
          }}
        >
          {t('批量创建')}
        </Checkbox>
      )}
      {batch && (
        <>
          <Checkbox
            disabled={isEdit}
            checked={multiToSingle}
            onChange={() => {
              setMultiToSingle((prev) => {
                const nextValue = !prev;
                setInputs((prevInputs) => {
                  const newInputs = { ...prevInputs };
                  if (nextValue) {
                    newInputs.multi_key_mode = multiKeyMode;
                  } else {
                    delete newInputs.multi_key_mode;
                  }
                  return newInputs;
                });
                return nextValue;
              });
            }}
          >
            {t('密钥聚合模式')}
          </Checkbox>

          {inputs.type !== 41 && (
            <Button
              size='small'
              type='tertiary'
              theme='outline'
              onClick={deduplicateKeys}
              style={{ textDecoration: 'underline' }}
            >
              {t('密钥去重')}
            </Button>
          )}
        </>
      )}
    </Space>
  ) : null;

  const channelOptionList = useMemo(
    () =>
      CHANNEL_OPTIONS.map((opt) => ({
        ...opt,
        // 保持 label 为纯文本以支持搜索
        label: opt.label,
      })),
    [],
  );

  const renderChannelOption = (renderProps) => {
    const {
      disabled,
      selected,
      label,
      value,
      focused,
      className,
      style,
      onMouseEnter,
      onClick,
      ...rest
    } = renderProps;

    const searchWords = channelSearchValue ? [channelSearchValue] : [];

    // 构建样式类名
    const optionClassName = [
      'flex items-center gap-3 px-3 py-2 transition-all duration-200 rounded-lg mx-2 my-1',
      focused && 'bg-blue-50 shadow-sm',
      selected &&
        'bg-blue-100 text-blue-700 shadow-lg ring-2 ring-blue-200 ring-opacity-50',
      disabled && 'opacity-50 cursor-not-allowed',
      !disabled && 'hover:bg-gray-50 hover:shadow-md cursor-pointer',
      className,
    ]
      .filter(Boolean)
      .join(' ');

    return (
      <div
        style={style}
        className={optionClassName}
        onClick={() => !disabled && onClick()}
        onMouseEnter={(e) => onMouseEnter()}
      >
        <div className='flex items-center gap-3 w-full'>
          <div className='flex-shrink-0 w-5 h-5 flex items-center justify-center'>
            {getChannelIcon(value)}
          </div>
          <div className='flex-1 min-w-0'>
            <Highlight
              sourceString={label}
              searchWords={searchWords}
              className='text-sm font-medium truncate'
            />
          </div>
          {selected && (
            <div className='flex-shrink-0 text-blue-600'>
              <svg
                width='16'
                height='16'
                viewBox='0 0 16 16'
                fill='currentColor'
              >
                <path d='M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z' />
              </svg>
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <>
      <SideSheet
        placement={isEdit ? 'right' : 'left'}
        title={
          <Space>
            <Tag color='blue' shape='circle'>
              {isEdit ? t('编辑') : t('新建')}
            </Tag>
            <Title heading={4} className='m-0'>
              {isEdit ? t('更新渠道信息') : t('创建新的渠道')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: '0' }}
        visible={props.visible}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-between items-center bg-white'>
            <div className='flex gap-2'>
              {!isEdit && (
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconBolt />}
                  onClick={pasteFromClipboard}
                >
                  {t('从剪贴板粘贴配置')}
                </Button>
              )}
              <Button
                size='small'
                type='tertiary'
                icon={<IconChevronUp />}
                onClick={() => navigateToSection('up')}
                style={{
                  borderRadius: '50%',
                  width: '32px',
                  height: '32px',
                  padding: 0,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
                title={t('上一个表单块')}
              />
              <Button
                size='small'
                type='tertiary'
                icon={<IconChevronDown />}
                onClick={() => navigateToSection('down')}
                style={{
                  borderRadius: '50%',
                  width: '32px',
                  height: '32px',
                  padding: 0,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
                title={t('下一个表单块')}
              />
            </div>
            <Space>
              <Button
                theme='solid'
                loading={loading}
                disabled={loading}
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
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
        <Form
          key={isEdit ? 'edit' : 'new'}
          initValues={originInputs}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          {() => (
            <Spin spinning={loading}>
              <div className='p-2 space-y-3' ref={formContainerRef}>
                {!isEdit && clipboardConfig && (
                  <Banner
                    type='info'
                    description={
                      <div className='flex items-center justify-between gap-2'>
                        <span>{t('检测到剪贴板中的连接信息')}</span>
                        <div className='flex gap-1'>
                          <Button
                            size='small'
                            theme='solid'
                            type='primary'
                            onClick={() => applyClipboardConfig(clipboardConfig)}
                          >
                            {t('自动填入')}
                          </Button>
                          <Button
                            size='small'
                            type='tertiary'
                            onClick={() => setClipboardConfig(null)}
                          >
                            {t('忽略')}
                          </Button>
                        </div>
                      </div>
                    }
                  />
                )}
                <div ref={(el) => (formSectionRefs.current.basicInfo = el)}>
                  <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                    {/* Header: Basic Info */}
                    <div className='flex items-center mb-2'>
                      <Avatar
                        size='small'
                        color='blue'
                        className='mr-2 shadow-md'
                      >
                        <IconServer size={16} />
                      </Avatar>
                      <div>
                        <Text className='text-lg font-medium'>
                          {t('基本信息')}
                        </Text>
                        <div className='text-xs text-gray-600'>
                          {t('渠道的基本配置信息')}
                        </div>
                      </div>
                    </div>

                    {isIonetChannel && (
                      <Banner
                        type='info'
                        closeIcon={null}
                        className='mb-4 rounded-xl'
                        description={t(
                          '此渠道由 IO.NET 自动同步，类型、密钥和 API 地址已锁定。',
                        )}
                      >
                        <Space>
                          {ionetMetadata?.deployment_id && (
                            <Button
                              size='small'
                              theme='light'
                              type='primary'
                              icon={<IconGlobe />}
                              onClick={handleOpenIonetDeployment}
                            >
                              {t('查看关联部署')}
                            </Button>
                          )}
                        </Space>
                      </Banner>
                    )}

                    <Form.Select
                      field='type'
                      label={t('类型')}
                      placeholder={t('请选择渠道类型')}
                      rules={[{ required: true, message: t('请选择渠道类型') }]}
                      optionList={channelOptionList}
                      style={{ width: '100%' }}
                      filter={selectFilter}
                      autoClearSearchValue={false}
                      searchPosition='dropdown'
                      onSearch={(value) => setChannelSearchValue(value)}
                      renderOptionItem={renderChannelOption}
                      onChange={(value) => handleInputChange('type', value)}
                      disabled={isIonetLocked}
                    />

                    <Form.Select
                      field='channel_vendor'
                      label={t('展示厂商')}
                      optionList={[
                        { label: t('自动识别'), value: 'auto' },
                        { label: t('跟随渠道类型'), value: 'protocol' },
                        ...CHANNEL_DISPLAY_VENDORS.map(({ label, value }) => ({
                          label: t(label),
                          value,
                        })),
                      ]}
                      style={{ width: '100%' }}
                      onChange={(value) =>
                        handleInputChange('channel_vendor', value)
                      }
                      extraText={t(
                        '仅当全部模型属于同一已知厂商时才会自动归类，混合模型渠道仍按渠道类型展示。',
                      )}
                    />

                    {inputs.type === 57 && (
                      <Banner
                        type='warning'
                        closeIcon={null}
                        className='mb-4 rounded-xl'
                        description={t(
                          '免责声明：仅限个人使用，请勿分发或共享任何凭证。该渠道存在前置条件与使用门槛，请在充分了解流程与风险后使用，并遵守 OpenAI 的相关条款与政策。相关凭证与配置仅限接入 Codex CLI 使用，不适用于其他客户端、平台或渠道。',
                        )}
                      />
                    )}

                    {inputs.type === 20 && (
                      <Form.Switch
                        field='is_enterprise_account'
                        label={t('是否为企业账户')}
                        checkedText={t('是')}
                        uncheckedText={t('否')}
                        onChange={(value) => {
                          setIsEnterpriseAccount(value);
                          handleInputChange('is_enterprise_account', value);
                        }}
                        extraText={t(
                          '企业账户为特殊返回格式，需要特殊处理，如果非企业账户，请勿勾选',
                        )}
                        initValue={inputs.is_enterprise_account}
                      />
                    )}

                    <Form.Input
                      field='name'
                      label={t('名称')}
                      placeholder={t('请为渠道命名')}
                      rules={[{ required: true, message: t('请为渠道命名') }]}
                      showClear
                      onChange={(value) => handleInputChange('name', value)}
                      autoComplete='new-password'
                    />

                    {inputs.type === 33 && (
                      <>
                        <Form.Select
                          field='aws_key_type'
                          label={t('密钥格式')}
                          placeholder={t('请选择密钥格式')}
                          optionList={[
                            {
                              label: 'AccessKey / SecretAccessKey',
                              value: 'ak_sk',
                            },
                            { label: 'API Key', value: 'api_key' },
                          ]}
                          style={{ width: '100%' }}
                          value={inputs.aws_key_type || 'ak_sk'}
                          onChange={(value) => {
                            handleChannelOtherSettingsChange(
                              'aws_key_type',
                              value,
                            );
                          }}
                          extraText={t(
                            'AK/SK 模式：使用 AccessKey 和 SecretAccessKey；API Key 模式：使用 API Key',
                          )}
                        />
                      </>
                    )}

                    {inputs.type === 41 && (
                      <Form.Select
                        field='vertex_key_type'
                        label={t('密钥格式')}
                        placeholder={t('请选择密钥格式')}
                        optionList={[
                          { label: 'JSON', value: 'json' },
                          { label: 'API Key', value: 'api_key' },
                        ]}
                        style={{ width: '100%' }}
                        value={inputs.vertex_key_type || 'json'}
                        onChange={(value) => {
                          // 更新设置中的 vertex_key_type
                          handleChannelOtherSettingsChange(
                            'vertex_key_type',
                            value,
                          );
                          // 切换为 api_key 时，关闭批量与手动/文件切换，并清理已选文件
                          if (value === 'api_key') {
                            setBatch(false);
                            setUseManualInput(false);
                            setVertexKeys([]);
                            setVertexFileList([]);
                            if (formApiRef.current) {
                              formApiRef.current.setValue('vertex_files', []);
                            }
                          }
                        }}
                        extraText={
                          inputs.vertex_key_type === 'api_key'
                            ? t('API Key 模式下不支持批量创建')
                            : t('JSON 模式支持手动输入或上传服务账号 JSON')
                        }
                      />
                    )}
                    {inputs.type === 41 && inputs.vertex_key_type === 'api_key' && (
                      <Form.Input
                        field='vertex_project_id'
                        label={t('Vertex 项目 ID（可选）')}
                        placeholder='my-project-id'
                        value={inputs.vertex_project_id || ''}
                        onChange={(value) =>
                          handleChannelOtherSettingsChange('vertex_project_id', value)
                        }
                        extraText={t(
                          '填写后使用项目端点；留空沿用 Express 模式。JSON 密钥仍使用凭据中的项目。',
                        )}
                      />
                    )}
                    {batch ? (
                      inputs.type === 41 &&
                      (inputs.vertex_key_type || 'json') === 'json' ? (
                        <Form.Upload
                          field='vertex_files'
                          label={t('密钥文件 (.json)')}
                          accept='.json'
                          multiple
                          draggable
                          dragIcon={<IconBolt />}
                          dragMainText={t('点击上传文件或拖拽文件到这里')}
                          dragSubText={t('仅支持 JSON 文件，支持多文件')}
                          style={{ marginTop: 10 }}
                          uploadTrigger='custom'
                          beforeUpload={() => false}
                          onChange={handleVertexUploadChange}
                          fileList={vertexFileList}
                          rules={
                            isEdit
                              ? []
                              : [
                                  {
                                    required: true,
                                    message: t('请上传密钥文件'),
                                  },
                                ]
                          }
                          extraText={batchExtra}
                        />
                      ) : (
                        <Form.TextArea
                          field='key'
                          label={t('密钥')}
                          placeholder={
                            inputs.type === 33
                              ? inputs.aws_key_type === 'api_key'
                                ? t(
                                    '请输入 API Key，一行一个，格式：APIKey|Region',
                                  )
                                : t(
                                    '请输入密钥，一行一个，格式：AccessKey|SecretAccessKey|Region',
                                  )
                              : t('请输入密钥，一行一个')
                          }
                          rules={
                            isEdit
                              ? []
                              : [{ required: true, message: t('请输入密钥') }]
                          }
                          autosize
                          autoComplete='new-password'
                          onChange={(value) => handleInputChange('key', value)}
                          disabled={isIonetLocked}
                          extraText={
                            <div className='flex items-center gap-2 flex-wrap'>
                              {isEdit &&
                                isMultiKeyChannel &&
                                keyMode === 'append' && (
                                  <Text type='warning' size='small'>
                                    {t(
                                      '追加模式：新密钥将添加到现有密钥列表的末尾',
                                    )}
                                  </Text>
                                )}
                              {isEdit && (
                                <Button
                                  size='small'
                                  type='primary'
                                  theme='outline'
                                  onClick={handleShow2FAModal}
                                >
                                  {t('查看密钥')}
                                </Button>
                              )}
                              {batchExtra}
                            </div>
                          }
                          showClear
                        />
                      )
                    ) : (
                      <>
                        {inputs.type === 57 ? (
                          <>
                            <Form.TextArea
                              field='key'
                              label={
                                isEdit
                                  ? t('密钥（编辑模式下，保存的密钥不会显示）')
                                  : t('密钥')
                              }
                              placeholder={t(
                                '请输入 JSON 格式的 OAuth 凭据，例如：\n{\n  "access_token": "...",\n  "account_id": "..." \n}',
                              )}
                              rules={
                                isEdit
                                  ? []
                                  : [
                                      {
                                        required: true,
                                        message: t('请输入密钥'),
                                      },
                                    ]
                              }
                              autoComplete='new-password'
                              onChange={(value) =>
                                handleInputChange('key', value)
                              }
                              disabled={isIonetLocked}
                              extraText={
                                <div className='flex flex-col gap-2'>
                                  <Text type='tertiary' size='small'>
                                    {t(
                                      '仅支持 JSON 对象，必须包含 access_token 与 account_id',
                                    )}
                                  </Text>

                                  <Space wrap spacing='tight'>
                                    <Button
                                      size='small'
                                      type='primary'
                                      theme='outline'
                                      onClick={() =>
                                        setCodexOAuthModalVisible(true)
                                      }
                                      disabled={isIonetLocked}
                                    >
                                      {t('Codex 授权')}
                                    </Button>
                                    {isEdit && (
                                      <Button
                                        size='small'
                                        type='primary'
                                        theme='outline'
                                        onClick={handleRefreshCodexCredential}
                                        loading={codexCredentialRefreshing}
                                        disabled={isIonetLocked}
                                      >
                                        {t('刷新凭证')}
                                      </Button>
                                    )}
                                    <Button
                                      size='small'
                                      type='primary'
                                      theme='outline'
                                      onClick={() => formatJsonField('key')}
                                      disabled={isIonetLocked}
                                    >
                                      {t('格式化')}
                                    </Button>
                                    {isEdit && (
                                      <Button
                                        size='small'
                                        type='primary'
                                        theme='outline'
                                        onClick={handleShow2FAModal}
                                        disabled={isIonetLocked}
                                      >
                                        {t('查看密钥')}
                                      </Button>
                                    )}
                                    {batchExtra}
                                  </Space>
                                </div>
                              }
                              autosize
                              showClear
                            />

                            <CodexOAuthModal
                              visible={codexOAuthModalVisible}
                              onCancel={() => setCodexOAuthModalVisible(false)}
                              onSuccess={handleCodexOAuthGenerated}
                            />
                          </>
                        ) : inputs.type === 41 &&
                          (inputs.vertex_key_type || 'json') === 'json' ? (
                          <>
                            {!batch && (
                              <div className='flex items-center justify-between mb-3'>
                                <Text className='text-sm font-medium'>
                                  {t('密钥输入方式')}
                                </Text>
                                <Space>
                                  <Button
                                    size='small'
                                    type={
                                      !useManualInput ? 'primary' : 'tertiary'
                                    }
                                    onClick={() => {
                                      setUseManualInput(false);
                                      // 切换到文件上传模式时清空手动输入的密钥
                                      if (formApiRef.current) {
                                        formApiRef.current.setValue('key', '');
                                      }
                                      handleInputChange('key', '');
                                    }}
                                  >
                                    {t('文件上传')}
                                  </Button>
                                  <Button
                                    size='small'
                                    type={
                                      useManualInput ? 'primary' : 'tertiary'
                                    }
                                    onClick={() => {
                                      setUseManualInput(true);
                                      // 切换到手动输入模式时清空文件上传相关状态
                                      setVertexKeys([]);
                                      setVertexFileList([]);
                                      if (formApiRef.current) {
                                        formApiRef.current.setValue(
                                          'vertex_files',
                                          [],
                                        );
                                      }
                                      setInputs((prev) => ({
                                        ...prev,
                                        vertex_files: [],
                                      }));
                                    }}
                                  >
                                    {t('手动输入')}
                                  </Button>
                                </Space>
                              </div>
                            )}

                            {batch && (
                              <Banner
                                type='info'
                                description={t(
                                  '批量创建模式下仅支持文件上传，不支持手动输入',
                                )}
                                className='!rounded-lg mb-3'
                              />
                            )}

                            {useManualInput && !batch ? (
                              <Form.TextArea
                                field='key'
                                label={
                                  isEdit
                                    ? t(
                                        '密钥（编辑模式下，保存的密钥不会显示）',
                                      )
                                    : t('密钥')
                                }
                                placeholder={t(
                                  '请输入 JSON 格式的密钥内容，例如：\n{\n  "type": "service_account",\n  "project_id": "your-project-id",\n  "private_key_id": "...",\n  "private_key": "...",\n  "client_email": "...",\n  "client_id": "...",\n  "auth_uri": "...",\n  "token_uri": "...",\n  "auth_provider_x509_cert_url": "...",\n  "client_x509_cert_url": "..."\n}',
                                )}
                                rules={
                                  isEdit
                                    ? []
                                    : [
                                        {
                                          required: true,
                                          message: t('请输入密钥'),
                                        },
                                      ]
                                }
                                autoComplete='new-password'
                                onChange={(value) =>
                                  handleInputChange('key', value)
                                }
                                extraText={
                                  <div className='flex items-center gap-2'>
                                    <Text type='tertiary' size='small'>
                                      {t('请输入完整的 JSON 格式密钥内容')}
                                    </Text>
                                    {isEdit &&
                                      isMultiKeyChannel &&
                                      keyMode === 'append' && (
                                        <Text type='warning' size='small'>
                                          {t(
                                            '追加模式：新密钥将添加到现有密钥列表的末尾',
                                          )}
                                        </Text>
                                      )}
                                    {isEdit && (
                                      <Button
                                        size='small'
                                        type='primary'
                                        theme='outline'
                                        onClick={handleShow2FAModal}
                                      >
                                        {t('查看密钥')}
                                      </Button>
                                    )}
                                    {batchExtra}
                                  </div>
                                }
                                autosize
                                showClear
                              />
                            ) : (
                              <Form.Upload
                                field='vertex_files'
                                label={t('密钥文件 (.json)')}
                                accept='.json'
                                draggable
                                dragIcon={<IconBolt />}
                                dragMainText={t('点击上传文件或拖拽文件到这里')}
                                dragSubText={t('仅支持 JSON 文件')}
                                style={{ marginTop: 10 }}
                                uploadTrigger='custom'
                                beforeUpload={() => false}
                                onChange={handleVertexUploadChange}
                                fileList={vertexFileList}
                                rules={
                                  isEdit
                                    ? []
                                    : [
                                        {
                                          required: true,
                                          message: t('请上传密钥文件'),
                                        },
                                      ]
                                }
                                extraText={batchExtra}
                              />
                            )}
                          </>
                        ) : (
                          <Form.Input
                            field='key'
                            label={
                              isEdit
                                ? t('密钥（编辑模式下，保存的密钥不会显示）')
                                : t('密钥')
                            }
                            placeholder={
                              inputs.type === 33
                                ? inputs.aws_key_type === 'api_key'
                                  ? t('请输入 API Key，格式：APIKey|Region')
                                  : t(
                                      '按照如下格式输入：AccessKey|SecretAccessKey|Region',
                                    )
                                : t(type2secretPrompt(inputs.type))
                            }
                            rules={
                              isEdit
                                ? []
                                : [{ required: true, message: t('请输入密钥') }]
                            }
                            autoComplete='new-password'
                            onChange={(value) =>
                              handleInputChange('key', value)
                            }
                            extraText={
                              <div className='flex items-center gap-2'>
                                {isEdit &&
                                  isMultiKeyChannel &&
                                  keyMode === 'append' && (
                                    <Text type='warning' size='small'>
                                      {t(
                                        '追加模式：新密钥将添加到现有密钥列表的末尾',
                                      )}
                                    </Text>
                                  )}
                                {isEdit && (
                                  <Button
                                    size='small'
                                    type='primary'
                                    theme='outline'
                                    onClick={handleShow2FAModal}
                                  >
                                    {t('查看密钥')}
                                  </Button>
                                )}
                                {batchExtra}
                              </div>
                            }
                            showClear
                          />
                        )}
                      </>
                    )}

                    {isEdit && isMultiKeyChannel && (
                      <Form.Select
                        field='key_mode'
                        label={t('密钥更新模式')}
                        placeholder={t('请选择密钥更新模式')}
                        optionList={[
                          { label: t('追加到现有密钥'), value: 'append' },
                          { label: t('覆盖现有密钥'), value: 'replace' },
                        ]}
                        style={{ width: '100%' }}
                        value={keyMode}
                        onChange={(value) => setKeyMode(value)}
                        extraText={
                          <Text type='tertiary' size='small'>
                            {keyMode === 'replace'
                              ? t('覆盖模式：将完全替换现有的所有密钥')
                              : t('追加模式：将新密钥添加到现有密钥列表末尾')}
                          </Text>
                        }
                      />
                    )}
                    {batch && multiToSingle && (
                      <>
                        <Form.Select
                          field='multi_key_mode'
                          label={t('密钥聚合模式')}
                          placeholder={t('请选择多密钥使用策略')}
                          optionList={[
                            { label: t('随机'), value: 'random' },
                            { label: t('轮询'), value: 'polling' },
                          ]}
                          style={{ width: '100%' }}
                          value={inputs.multi_key_mode || 'random'}
                          onChange={(value) => {
                            setMultiKeyMode(value);
                            handleInputChange('multi_key_mode', value);
                          }}
                        />
                        {inputs.multi_key_mode === 'polling' && (
                          <Banner
                            type='warning'
                            description={t(
                              '轮询模式必须搭配Redis和内存缓存功能使用，否则性能将大幅降低，并且无法实现轮询功能',
                            )}
                            className='!rounded-lg mt-2'
                          />
                        )}
                      </>
                    )}

                    {inputs.type === 18 && (
                      <Form.Input
                        field='other'
                        label={t('模型版本')}
                        placeholder={
                          '请输入星火大模型版本，注意是接口地址中的版本号，例如：v2.1'
                        }
                        onChange={(value) => handleInputChange('other', value)}
                        showClear
                      />
                    )}

                    {inputs.type === 41 && (
                      <JSONEditor
                        key={`region-${isEdit ? channelId : 'new'}`}
                        field='other'
                        label={t('部署地区')}
                        placeholder={t(
                          '请输入部署地区，例如：us-central1\n支持使用模型映射格式\n{\n    "default": "us-central1",\n    "claude-3-5-sonnet-20240620": "europe-west1"\n}',
                        )}
                        value={inputs.other || ''}
                        onChange={(value) => handleInputChange('other', value)}
                        rules={[
                          { required: true, message: t('请填写部署地区') },
                        ]}
                        template={REGION_EXAMPLE}
                        templateLabel={t('填入模板')}
                        editorType='region'
                        formApi={formApiRef.current}
                        extraText={t('设置默认地区和特定模型的专用地区')}
                      />
                    )}

                    {inputs.type === 21 && (
                      <Form.Input
                        field='other'
                        label={t('知识库 ID')}
                        placeholder={'请输入知识库 ID，例如：123456'}
                        onChange={(value) => handleInputChange('other', value)}
                        showClear
                      />
                    )}

                    {inputs.type === 39 && (
                      <Form.Input
                        field='other'
                        label='Account ID'
                        placeholder={
                          '请输入Account ID，例如：d6b5da8hk1awo8nap34ube6gh'
                        }
                        onChange={(value) => handleInputChange('other', value)}
                        showClear
                      />
                    )}

                    {inputs.type === 49 && (
                      <Form.Input
                        field='other'
                        label={t('智能体ID')}
                        placeholder={'请输入智能体ID，例如：7342866812345'}
                        onChange={(value) => handleInputChange('other', value)}
                        showClear
                      />
                    )}

                    {inputs.type === 1 && (
                      <Form.Input
                        field='openai_organization'
                        label={t('组织')}
                        placeholder={t('请输入组织org-xxx')}
                        showClear
                        helpText={t('组织，不填则为默认组织')}
                        onChange={(value) =>
                          handleInputChange('openai_organization', value)
                        }
                      />
                    )}
                  </Card>
                </div>

                {/* API Configuration Card */}
                {showApiConfigCard && (
                  <div ref={(el) => (formSectionRefs.current.apiConfig = el)}>
                    <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                      {/* Header: API Config */}
                      <div
                        className='flex items-center mb-2'
                        onClick={handleApiConfigSecretClick}
                      >
                        <Avatar
                          size='small'
                          color='green'
                          className='mr-2 shadow-md'
                        >
                          <IconGlobe size={16} />
                        </Avatar>
                        <div>
                          <Text className='text-lg font-medium'>
                            {t('API 配置')}
                          </Text>
                          <div className='text-xs text-gray-600'>
                            {t('API 地址和相关配置')}
                          </div>
                        </div>
                      </div>

                      {inputs.type === 40 && (
                        <Banner
                          type='info'
                          description={
                            <div>
                              <Text strong>{t('邀请链接')}:</Text>
                              <Text
                                link
                                underline
                                className='ml-2 cursor-pointer'
                                onClick={() =>
                                  window.open(
                                    'https://cloud.siliconflow.cn/i/hij0YNTZ',
                                  )
                                }
                              >
                                https://cloud.siliconflow.cn/i/hij0YNTZ
                              </Text>
                            </div>
                          }
                          className='!rounded-lg'
                        />
                      )}

                      {inputs.type === 3 && (
                        <>
                          <Banner
                            type='warning'
                            description={t(
                              '2025年5月10日后添加的渠道，不需要再在部署的时候移除模型名称中的"."',
                            )}
                            className='!rounded-lg'
                          />
                          <div>
                            <Form.Input
                              field='base_url'
                              label='AZURE_OPENAI_ENDPOINT'
                              placeholder={t(
                                '请输入 AZURE_OPENAI_ENDPOINT，例如：https://docs-test-001.openai.azure.com',
                              )}
                              onChange={(value) =>
                                handleInputChange('base_url', value)
                              }
                              showClear
                              disabled={isIonetLocked}
                            />
                          </div>
                          <div>
                            <Form.Input
                              field='other'
                              label={t('默认 API 版本')}
                              placeholder={t(
                                '请输入默认 API 版本，例如：2025-04-01-preview',
                              )}
                              onChange={(value) =>
                                handleInputChange('other', value)
                              }
                              showClear
                            />
                          </div>
                          <div>
                            <Form.Input
                              field='azure_responses_version'
                              label={t(
                                '默认 Responses API 版本，为空则使用上方版本',
                              )}
                              placeholder={t('例如：preview')}
                              onChange={(value) =>
                                handleChannelOtherSettingsChange(
                                  'azure_responses_version',
                                  value,
                                )
                              }
                              showClear
                            />
                          </div>
                        </>
                      )}

                      {inputs.type === 8 && (
                        <>
                          <Banner
                            type='warning'
                            description={t(
                              '如果你对接的是上游One API或者New API等转发项目，请使用OpenAI类型，不要使用此类型，除非你知道你在做什么。',
                            )}
                            className='!rounded-lg'
                          />
                          <div>
                            <Form.Input
                              field='base_url'
                              label={t('完整的 Base URL，支持变量{model}')}
                              placeholder={t(
                                '请输入完整的URL，例如：https://api.openai.com/v1/chat/completions',
                              )}
                              onChange={(value) =>
                                handleInputChange('base_url', value)
                              }
                              showClear
                              disabled={isIonetLocked}
                            />
                          </div>
                        </>
                      )}

                      {inputs.type === 37 && (
                        <Banner
                          type='warning'
                          description={t(
                            'Dify渠道只适配chatflow和agent，并且agent不支持图片！',
                          )}
                          className='!rounded-lg'
                        />
                      )}

                      {inputs.type !== 3 &&
                        inputs.type !== 8 &&
                        inputs.type !== 22 &&
                        inputs.type !== 36 &&
                        (inputs.type !== 45 || doubaoApiEditUnlocked) && (
                          <div>
                            <Form.Input
                              field='base_url'
                              label={t('API地址')}
                              placeholder={t(
                                '此项可选，用于通过自定义API地址来进行 API 调用，末尾不要带/v1和/',
                              )}
                              onChange={(value) =>
                                handleInputChange('base_url', value)
                              }
                              showClear
                              disabled={isIonetLocked}
                              extraText={t(
                                '对于官方渠道，new-api已经内置地址，除非是第三方代理站点或者Azure的特殊接入地址，否则不需要填写',
                              )}
                            />
                          </div>
                        )}

                      {inputs.type === 22 && (
                        <div>
                          <Form.Input
                            field='base_url'
                            label={t('私有部署地址')}
                            placeholder={t(
                              '请输入私有部署地址，格式为：https://fastgpt.run/api/openapi',
                            )}
                            onChange={(value) =>
                              handleInputChange('base_url', value)
                            }
                            showClear
                            disabled={isIonetLocked}
                          />
                        </div>
                      )}

                      {inputs.type === 36 && (
                        <div>
                          <Form.Input
                            field='base_url'
                            label={t(
                              '注意非Chat API，请务必填写正确的API地址，否则可能导致无法使用',
                            )}
                            placeholder={t(
                              '请输入到 /suno 前的路径，通常就是域名，例如：https://api.example.com',
                            )}
                            onChange={(value) =>
                              handleInputChange('base_url', value)
                            }
                            showClear
                            disabled={isIonetLocked}
                          />
                        </div>
                      )}

                      {inputs.type === 45 && !doubaoApiEditUnlocked && (
                        <div>
                          <Form.Select
                            field='base_url'
                            label={t('API地址')}
                            placeholder={t('请选择API地址')}
                            onChange={(value) =>
                              handleInputChange('base_url', value)
                            }
                            optionList={[
                              {
                                value: 'https://ark.cn-beijing.volces.com',
                                label: 'https://ark.cn-beijing.volces.com',
                              },
                              {
                                value:
                                  'https://ark.ap-southeast.bytepluses.com',
                                label:
                                  'https://ark.ap-southeast.bytepluses.com',
                              },
                              {
                                value: 'doubao-coding-plan',
                                label: 'Doubao Coding Plan',
                              },
                            ]}
                            defaultValue='https://ark.cn-beijing.volces.com'
                            disabled={isIonetLocked}
                          />
                        </div>
                      )}
                    </Card>
                  </div>
                )}

                {/* Model Configuration Card */}
                <div ref={(el) => (formSectionRefs.current.modelConfig = el)}>
                  <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                    {/* Header: Model Config */}
                    <div className='flex items-center mb-2'>
                      <Avatar
                        size='small'
                        color='purple'
                        className='mr-2 shadow-md'
                      >
                        <IconCode size={16} />
                      </Avatar>
                      <div>
                        <Text className='text-lg font-medium'>
                          {t('模型配置')}
                        </Text>
                        <div className='text-xs text-gray-600'>
                          {t('模型选择和映射设置')}
                        </div>
                      </div>
                    </div>

                    <Form.Select
                      field='models'
                      label={t('模型')}
                      placeholder={t('请选择该渠道所支持的模型')}
                      rules={[{ required: true, message: t('请选择模型') }]}
                      multiple
                      filter={selectFilter}
                      allowCreate
                      autoClearSearchValue={false}
                      searchPosition='dropdown'
                      optionList={modelOptions}
                      onSearch={(value) => setModelSearchValue(value)}
                      innerBottomSlot={
                        modelSearchHintText ? (
                          <Text className='px-3 py-2 block text-xs !text-semi-color-text-2'>
                            {modelSearchHintText}
                          </Text>
                        ) : null
                      }
                      style={{ width: '100%' }}
                      onChange={(value) => handleInputChange('models', value)}
                      renderSelectedItem={(optionNode) => {
                        const modelName = String(optionNode?.value ?? '');
                        return {
                          isRenderInTag: true,
                          content: (
                            <span
                              className='cursor-pointer select-none'
                              role='button'
                              tabIndex={0}
                              title={t('点击复制模型名称')}
                              onClick={async (e) => {
                                e.stopPropagation();
                                const ok = await copy(modelName);
                                if (ok) {
                                  showSuccess(
                                    t('已复制：{{name}}', { name: modelName }),
                                  );
                                } else {
                                  showError(t('复制失败'));
                                }
                              }}
                            >
                              {optionNode.label || modelName}
                            </span>
                          ),
                        };
                      }}
                      extraText={
                        <Space wrap>
                          <Button
                            size='small'
                            type='primary'
                            onClick={() =>
                              handleInputChange('models', basicModels)
                            }
                          >
                            {t('填入相关模型')}
                          </Button>
                          <Button
                            size='small'
                            type='secondary'
                            onClick={() =>
                              handleInputChange('models', fullModels)
                            }
                          >
                            {t('填入所有模型')}
                          </Button>
                          {MODEL_FETCHABLE_CHANNEL_TYPES.has(inputs.type) && (
                            <Button
                              size='small'
                              type='tertiary'
                              onClick={() => fetchUpstreamModelList('models')}
                            >
                              {t('获取模型列表')}
                            </Button>
                          )}
                          {inputs.type === 4 && isEdit && (
                            <Button
                              size='small'
                              type='primary'
                              theme='light'
                              onClick={() => setOllamaModalVisible(true)}
                            >
                              {t('Ollama 模型管理')}
                            </Button>
                          )}
                          <Button
                            size='small'
                            type='warning'
                            onClick={() => handleInputChange('models', [])}
                          >
                            {t('清除所有模型')}
                          </Button>
                          <Button
                            size='small'
                            type='tertiary'
                            onClick={() => {
                              if (inputs.models.length === 0) {
                                showInfo(t('没有模型可以复制'));
                                return;
                              }
                              try {
                                copy(inputs.models.join(','));
                                showSuccess(t('模型列表已复制到剪贴板'));
                              } catch (error) {
                                showError(t('复制失败'));
                              }
                            }}
                          >
                            {t('复制所有模型')}
                          </Button>
                          {modelGroups &&
                            modelGroups.length > 0 &&
                            modelGroups.map((group) => (
                              <Button
                                key={group.id}
                                size='small'
                                type='primary'
                                onClick={() => {
                                  let items = [];
                                  try {
                                    if (Array.isArray(group.items)) {
                                      items = group.items;
                                    } else if (
                                      typeof group.items === 'string'
                                    ) {
                                      const parsed = JSON.parse(
                                        group.items || '[]',
                                      );
                                      if (Array.isArray(parsed)) items = parsed;
                                    }
                                  } catch {}
                                  const current =
                                    formApiRef.current?.getValue('models') ||
                                    inputs.models ||
                                    [];
                                  const merged = Array.from(
                                    new Set(
                                      [...current, ...items]
                                        .map((m) => (m || '').trim())
                                        .filter(Boolean),
                                    ),
                                  );
                                  handleInputChange('models', merged);
                                }}
                              >
                                {group.name}
                              </Button>
                            ))}
                        </Space>
                      }
                    />

                    <Form.Input
                      field='custom_model'
                      label={t('自定义模型名称')}
                      placeholder={t('输入自定义模型名称')}
                      onChange={(value) => setCustomModel(value.trim())}
                      value={customModel}
                      suffix={
                        <Button
                          size='small'
                          type='primary'
                          onClick={addCustomModels}
                        >
                          {t('填入')}
                        </Button>
                      }
                    />

                    <Form.Input
                      field='test_model'
                      label={t('默认测试模型')}
                      placeholder={t('不填则为模型列表第一个')}
                      onChange={(value) =>
                        handleInputChange('test_model', value)
                      }
                      showClear
                    />

                    <JSONEditor
                      key={`model_mapping-${isEdit ? channelId : 'new'}`}
                      field='model_mapping'
                      label={t('模型重定向')}
                      placeholder={
                        t(
                          '此项可选，用于修改请求体中的模型名称，为一个 JSON 字符串，键为请求中模型名称，值为要替换的模型名称，例如：',
                        ) +
                        `\n${JSON.stringify(MODEL_MAPPING_EXAMPLE, null, 2)}`
                      }
                      value={inputs.model_mapping || ''}
                      onChange={(value) =>
                        handleInputChange('model_mapping', value)
                      }
                      template={MODEL_MAPPING_EXAMPLE}
                      templateLabel={t('填入模板')}
                      editorType='keyValue'
                      formApi={formApiRef.current}
                      renderStringValueSuffix={({ pairKey, value }) => {
                        if (!MODEL_FETCHABLE_CHANNEL_TYPES.has(inputs.type)) {
                          return null;
                        }
                        const disabled = !String(pairKey ?? '').trim();
                        return (
                          <Tooltip content={t('选择模型')}>
                            <Button
                              type='tertiary'
                              theme='borderless'
                              size='small'
                              icon={<IconSearch size={14} />}
                              disabled={disabled}
                              onClick={(e) => {
                                e.stopPropagation();
                                openModelMappingValueModal({ pairKey, value });
                              }}
                            />
                          </Tooltip>
                        );
                      }}
                      extraText={t(
                        '键为请求中的模型名称，值为要替换的模型名称',
                      )}
                      extraFooter={
                        <Space>
                          <Button
                            size='small'
                            type='danger'
                            theme='borderless'
                            onClick={clearModelMapping}
                          >
                            {t('清空重定向')}
                          </Button>
                        </Space>
                      }
                    />
                  </Card>
                </div>

                {/* Advanced Settings Card */}
                <div
                  ref={(el) => (formSectionRefs.current.advancedSettings = el)}
                >
                  <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                    {/* Header: Advanced Settings */}
                    <div className='flex items-center justify-between mb-2'>
                      <div className='flex items-center'>
                        <Avatar
                          size='small'
                          color='orange'
                          className='mr-2 shadow-md'
                        >
                          <IconSetting size={16} />
                        </Avatar>
                        <div>
                          <Text className='text-lg font-medium'>
                            {t('高级设置')}
                          </Text>
                          <div className='text-xs text-gray-600'>
                            {t('渠道的高级配置选项')}
                          </div>
                        </div>
                      </div>
                      <Button
                        size='small'
                        type='tertiary'
                        theme='borderless'
                        icon={
                          advancedSettingsOpen ? (
                            <IconChevronUp size={14} />
                          ) : (
                            <IconChevronDown size={14} />
                          )
                        }
                        onClick={() =>
                          toggleAdvancedSettings(!advancedSettingsOpen)
                        }
                      >
                        {advancedSettingsOpen ? t('收起') : t('展开')}
                      </Button>
                    </div>

                    {advancedSettingsOpen && (
                      <>
                        <Form.Slot label={t('分组')}>
                          <Space
                            vertical
                            align='start'
                            spacing='tight'
                            style={{ width: '100%' }}
                          >
                            <Select
                              value={groupVendor}
                              placeholder={t('先选择供应商')}
                              optionList={groupVendorOptions}
                              filter={selectFilter}
                              onChange={setGroupVendor}
                              style={{ width: '100%' }}
                            />
                            <Select
                              value={visibleSelectedGroupValues}
                              placeholder={
                                groupVendor
                                  ? t('请选择该供应商下的分组')
                                  : t('先选择供应商')
                              }
                              multiple
                              disabled={!groupVendor}
                              allowAdditions
                              additionLabel={t(
                                '请在系统设置页面编辑分组倍率以添加新的分组：',
                              )}
                              optionList={visibleGroupOptions}
                              filter={selectFilter}
                              renderOptionItem={renderGroupOption}
                              onChange={(value) => {
                                const hiddenGroups = selectedGroupValues.filter(
                                  (group) =>
                                    resolveTokenGroupVendor(group) !==
                                    groupVendor,
                                );
                                handleInputChange('groups', [
                                  ...hiddenGroups,
                                  ...value,
                                ]);
                              }}
                              style={{ width: '100%' }}
                            />
                          </Space>
                        </Form.Slot>

                    <Form.Input
                      field='tag'
                      label={t('渠道标签')}
                      placeholder={t('渠道标签')}
                      showClear
                      onChange={(value) => handleInputChange('tag', value)}
                    />
                    <Form.TextArea
                      field='remark'
                      label={t('备注')}
                      placeholder={t('请输入备注（仅管理员可见）')}
                      maxLength={255}
                      showClear
                      onChange={(value) => handleInputChange('remark', value)}
                    />

                    <Row gutter={12}>
                      <Col span={12}>
                        <Form.InputNumber
                          field='priority'
                          label={t('渠道优先级')}
                          placeholder={t('渠道优先级')}
                          min={0}
                          onNumberChange={(value) =>
                            handleInputChange('priority', value)
                          }
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col span={12}>
                        <Form.InputNumber
                          field='weight'
                          label={t('渠道权重')}
                          placeholder={t('渠道权重')}
                          min={0}
                          onNumberChange={(value) =>
                            handleInputChange('weight', value)
                          }
                          style={{ width: '100%' }}
                        />
                      </Col>
                    </Row>

                    <Form.Switch
                      field='auto_ban'
                      label={t('是否自动禁用')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) => {
                        setAutoBan(value);
                        handleInputChange('auto_ban', value);
                      }}
                      extraText={t(
                        '仅当自动禁用开启时有效，关闭后不会自动禁用该渠道',
                      )}
                      initValue={autoBan}
                    />

                    {MODEL_FETCHABLE_CHANNEL_TYPES.has(inputs.type) && (
                      <>
                        <Form.Switch
                          field='upstream_model_update_check_enabled'
                          label={t('是否检测上游模型更新')}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'upstream_model_update_check_enabled',
                              value,
                            )
                          }
                          extraText={t(
                            '开启后由后端定时任务检测该渠道上游模型变化',
                          )}
                        />

                        <Form.Switch
                          field='upstream_model_update_auto_sync_enabled'
                          label={t('是否自动同步上游模型更新')}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          disabled={!inputs.upstream_model_update_check_enabled}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'upstream_model_update_auto_sync_enabled',
                              value,
                            )
                          }
                          extraText={t(
                            '开启后检测到新增模型会自动加入当前渠道模型列表',
                          )}
                        />

                        <div className='text-xs text-gray-500 mb-2'>
                          {t('上次检测时间')}:{' '}
                          {formatUnixTime(
                            inputs.upstream_model_update_last_check_time,
                          )}
                        </div>

                        <Form.Input
                          field='upstream_model_update_ignored_models'
                          label={t('已忽略模型')}
                          placeholder={t(
                            '例如：gpt-4.1-nano,regex:^claude-.*$,regex:^sora-.*$',
                          )}
                          onChange={(value) =>
                            handleInputChange(
                              'upstream_model_update_ignored_models',
                              value,
                            )
                          }
                          extraText={t(
                            '支持精确匹配；使用 regex: 开头可按正则匹配。',
                          )}
                          showClear
                        />

                        <div className='text-xs text-gray-500 mb-3 break-all'>
                          {t('待加入模型')}:{' '}
                          {Array.isArray(
                            inputs.upstream_model_update_last_detected_models,
                          ) &&
                          inputs.upstream_model_update_last_detected_models
                            .length > 0
                            ? inputs.upstream_model_update_last_detected_models
                                .map((model) => String(model || '').trim())
                                .filter(Boolean)
                                .join(', ')
                            : t('暂无')}
                        </div>
                      </>
                    )}

                    <Form.TextArea
                      field='param_override'
                      label={t('参数覆盖')}
                      placeholder={
                        t(
                          '此项可选，用于覆盖请求参数。不支持覆盖 stream 参数',
                        ) +
                        '\n' +
                        t('旧格式（直接覆盖）：') +
                        '\n{\n  "temperature": 0,\n  "max_tokens": 1000\n}' +
                        '\n\n' +
                        t('新格式（支持条件判断与json自定义）：') +
                        '\n{\n  "operations": [\n    {\n      "path": "temperature",\n      "mode": "set",\n      "value": 0.7,\n      "conditions": [\n        {\n          "path": "model",\n          "mode": "prefix",\n          "value": "gpt"\n        }\n      ]\n    }\n  ]\n}'
                      }
                      autosize
                      onChange={(value) =>
                        handleInputChange('param_override', value)
                      }
                      extraText={
                        <div className='flex flex-col gap-2'>
                          <div className='flex gap-2 flex-wrap items-center'>
                            <Button
                              size='small'
                              type='primary'
                              icon={<IconCode size={14} />}
                              onClick={() =>
                                setParamOverrideEditorVisible(true)
                              }
                            >
                              {t('可视化编辑')}
                            </Button>
                            <Text
                              className='!text-semi-color-primary cursor-pointer'
                              onClick={() =>
                                handleInputChange(
                                  'param_override',
                                  JSON.stringify({ temperature: 0 }, null, 2),
                                )
                              }
                            >
                              {t('旧格式模板')}
                            </Text>
                            <Text
                              className='!text-semi-color-primary cursor-pointer'
                              onClick={() =>
                                handleInputChange(
                                  'param_override',
                                  JSON.stringify(
                                    {
                                      operations: [
                                        {
                                          path: 'temperature',
                                          mode: 'set',
                                          value: 0.7,
                                          conditions: [
                                            {
                                              path: 'model',
                                              mode: 'prefix',
                                              value: 'gpt',
                                            },
                                          ],
                                          logic: 'AND',
                                        },
                                      ],
                                    },
                                    null,
                                    2,
                                  ),
                                )
                              }
                            >
                              {t('新格式模板')}
                            </Text>
                            <Text
                              className='!text-semi-color-primary cursor-pointer'
                              onClick={() => formatJsonField('param_override')}
                            >
                              {t('格式化')}
                            </Text>
                          </div>
                          <Text type='tertiary' size='small'>
                            {t('默认推荐使用可视化编辑；下方原始 JSON 输入可作为回退方式。')}
                          </Text>
                        </div>
                      }
                      showClear
                    />

                    <Form.TextArea
                      field='header_override'
                      label={t('请求头覆盖')}
                      placeholder={
                        t('此项可选，用于覆盖请求头参数') +
                        '\n' +
                        t('格式示例：') +
                        '\n{\n  "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36 Edg/139.0.0.0",\n  "Authorization": "Bearer {api_key}"\n}'
                      }
                      autosize
                      onChange={(value) =>
                        handleInputChange('header_override', value)
                      }
                      extraText={
                        <div className='flex flex-col gap-1'>
                          <div className='flex gap-2 flex-wrap items-center'>
                            <Text
                              className='!text-semi-color-primary cursor-pointer'
                              onClick={() =>
                                handleInputChange(
                                  'header_override',
                                  JSON.stringify(
                                    {
                                      '*': true,
                                      're:^X-Trace-.*$': true,
                                      'X-Foo': '{client_header:X-Foo}',
                                      Authorization: 'Bearer {api_key}',
                                      'User-Agent':
                                        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36 Edg/139.0.0.0',
                                    },
                                    null,
                                    2,
                                  ),
                                )
                              }
                            >
                              {t('填入模板')}
                            </Text>
                            <Text
                              className='!text-semi-color-primary cursor-pointer'
                              onClick={() =>
                                handleInputChange(
                                  'header_override',
                                  JSON.stringify(
                                    {
                                      '*': true,
                                    },
                                    null,
                                    2,
                                  ),
                                )
                              }
                            >
                              {t('填入透传模版')}
                            </Text>
                            <Text
                              className='!text-semi-color-primary cursor-pointer'
                              onClick={() => formatJsonField('header_override')}
                            >
                              {t('格式化')}
                            </Text>
                          </div>
                          <div>
                            <Text type='tertiary' size='small'>
                              {t('支持变量：')}
                            </Text>
                            <div className='text-xs text-tertiary ml-2'>
                              <div>
                                {t('渠道密钥')}: {'{api_key}'}
                              </div>
                            </div>
                          </div>
                        </div>
                      }
                      showClear
                    />

                    <JSONEditor
                      key={`status_code_mapping-${isEdit ? channelId : 'new'}`}
                      field='status_code_mapping'
                      label={t('状态码复写')}
                      placeholder={
                        t(
                          '此项可选，用于复写返回的状态码，仅影响本地判断，不修改返回到上游的状态码，比如将claude渠道的400错误复写为500（用于重试），请勿滥用该功能，例如：',
                        ) +
                        '\n' +
                        JSON.stringify(STATUS_CODE_MAPPING_EXAMPLE, null, 2)
                      }
                      value={inputs.status_code_mapping || ''}
                      onChange={(value) =>
                        handleInputChange('status_code_mapping', value)
                      }
                      template={STATUS_CODE_MAPPING_EXAMPLE}
                      templateLabel={t('填入模板')}
                      editorType='keyValue'
                      formApi={formApiRef.current}
                      extraText={t(
                        '键为原状态码，值为要复写的状态码，仅影响本地判断',
                      )}
                    />

                    {/* 字段透传控制 - OpenAI 渠道 */}
                    {inputs.type === 1 && (
                      <>
                        <div className='mt-4 mb-2 text-sm font-medium text-gray-700'>
                          {t('字段透传控制')}
                        </div>

                        <Form.Select
                          field='chat_stream_options_mode'
                          label={t('Chat stream_options 支持')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={inputs.chat_stream_options_mode || 'inherit'}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'chat_stream_options_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制 chat/completions 请求是否允许保留或自动补全 stream_options。第三方 OpenAI 兼容上游不完整时，建议保持继承或强制关闭。',
                          )}
                        />

                        <Form.Select
                          field='responses_api_mode'
                          label={t('Responses API 支持')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={inputs.responses_api_mode || 'inherit'}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'responses_api_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制该渠道是否允许 chat -> responses 桥接以及直接走 /v1/responses。未知兼容站建议保持继承或强制关闭。',
                          )}
                        />

                        <Form.Select
                          field='chat_completions_to_responses_mode'
                          label={t('Chat 转 Responses 桥接')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={
                            inputs.chat_completions_to_responses_mode ||
                            'inherit'
                          }
                          onChange={(value) =>
                            handleChannelSettingsChange(
                              'chat_completions_to_responses_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制 /v1/chat/completions 请求是否自动桥接到 /v1/responses。上游只支持 Chat Completions 时请强制关闭。',
                          )}
                        />

                        <Form.Select
                          field='responses_stream_options_mode'
                          label={t('Responses stream_options 支持')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={
                            inputs.responses_stream_options_mode || 'inherit'
                          }
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'responses_stream_options_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制 /v1/responses 的 stream_options 是否允许保留。若上游会报 Unsupported parameter: stream_options，可强制关闭。',
                          )}
                        />

                        <Form.Switch
                          field='allow_service_tier'
                          label={t('允许 service_tier 透传')}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'allow_service_tier',
                              value,
                            )
                          }
                          extraText={t(
                            'service_tier 字段用于指定服务层级，允许透传可能导致实际计费高于预期。默认关闭以避免额外费用',
                          )}
                        />

                        <Form.Switch
                          field='disable_store'
                          label={t('禁用 store 透传')}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'disable_store',
                              value,
                            )
                          }
                          extraText={t(
                            'store 字段用于授权 OpenAI 存储请求数据以评估和优化产品。默认关闭，开启后可能导致 Codex 无法正常使用',
                          )}
                        />

                        <Form.Switch
                          field='allow_safety_identifier'
                          label={t('允许 safety_identifier 透传')}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'allow_safety_identifier',
                              value,
                            )
                          }
                          extraText={t(
                            'safety_identifier 字段用于帮助 OpenAI 识别可能违反使用政策的应用程序用户。默认关闭以保护用户隐私',
                          )}
                        />

                        <Form.Switch
                          field='allow_include_obfuscation'
                          label={t(
                            '允许 stream_options.include_obfuscation 透传',
                          )}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'allow_include_obfuscation',
                              value,
                            )
                          }
                          extraText={t(
                            'include_obfuscation 用于控制 Responses 流混淆字段。默认关闭以避免客户端关闭该安全保护',
                          )}
                        />
                      </>
                    )}

                    {/* 字段透传控制 - Claude 渠道 */}
                    {inputs.type === 14 && (
                      <>
                        <div className='mt-4 mb-2 text-sm font-medium text-gray-700'>
                          {t('字段透传控制')}
                        </div>

                        <Form.Select
                          field='chat_stream_options_mode'
                          label={t('Chat stream_options 支持')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={inputs.chat_stream_options_mode || 'inherit'}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'chat_stream_options_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制桥接到 OpenAI chat 兼容格式时是否允许保留或自动补全 stream_options。',
                          )}
                        />

                        <Form.Select
                          field='responses_api_mode'
                          label={t('Responses API 支持')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={inputs.responses_api_mode || 'inherit'}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'responses_api_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制 chat -> responses 桥接是否启用。若上游不支持 /v1/responses，可强制关闭。',
                          )}
                        />

                        <Form.Select
                          field='chat_completions_to_responses_mode'
                          label={t('Chat 转 Responses 桥接')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={
                            inputs.chat_completions_to_responses_mode ||
                            'inherit'
                          }
                          onChange={(value) =>
                            handleChannelSettingsChange(
                              'chat_completions_to_responses_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制 /v1/chat/completions 请求是否自动桥接到 /v1/responses。上游只支持 Chat Completions 时请强制关闭。',
                          )}
                        />

                        <Form.Select
                          field='responses_stream_options_mode'
                          label={t('Responses stream_options 支持')}
                          optionList={CAPABILITY_MODE_OPTIONS}
                          value={
                            inputs.responses_stream_options_mode || 'inherit'
                          }
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'responses_stream_options_mode',
                              value,
                            )
                          }
                          extraText={t(
                            '控制桥接到 /v1/responses 时是否允许保留 stream_options。',
                          )}
                        />

                        <Form.Switch
                          field='allow_service_tier'
                          label={t('允许 service_tier 透传')}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'allow_service_tier',
                              value,
                            )
                          }
                          extraText={t(
                            'service_tier 字段用于指定服务层级，允许透传可能导致实际计费高于预期。默认关闭以避免额外费用',
                          )}
                        />

                        <Form.Switch
                          field='allow_inference_geo'
                          label={t('允许 inference_geo 透传')}
                          checkedText={t('开')}
                          uncheckedText={t('关')}
                          onChange={(value) =>
                            handleChannelOtherSettingsChange(
                              'allow_inference_geo',
                              value,
                            )
                          }
                          extraText={t(
                            'inference_geo 字段用于控制 Claude 数据驻留推理区域。默认关闭以避免未经授权透传地域信息',
                          )}
                        />
                      </>
                    )}
                      </>
                    )}
                  </Card>
                </div>

                {/* Quota Policy Card */}
                <div
                  ref={(el) => (formSectionRefs.current.quotaSettings = el)}
                >
                  <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                    <div className='flex items-center mb-2'>
                      <Avatar
                        size='small'
                        color='cyan'
                        className='mr-2 shadow-md'
                      >
                        <IconCalendar size={16} />
                      </Avatar>
                      <div>
                        <Text className='text-lg font-medium'>
                          {t('周期配额限制')}
                        </Text>
                        <div className='text-xs text-gray-600'>
                          {t('按周期限制本渠道的最大消耗或调用次数，达到上限自动禁用，下个周期开始自动恢复')}
                        </div>
                      </div>
                    </div>

                    <Form.Switch
                      field='quota_policy_enabled'
                      label={t('启用周期限额')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) =>
                        handleInputChange('quota_policy_enabled', value)
                      }
                      extraText={t(
                        '开启后将忽略所属标签的共享池配额；关闭则继承所属标签的策略（若有）',
                      )}
                    />

                    {inputs.quota_policy_enabled && (
                      <>
                        <Form.Select
                          field='quota_policy_period'
                          label={t('统计周期')}
                          optionList={[
                            { label: t('日（每日 0 点重置）'), value: 'day' },
                            { label: t('周（每 7 天重置）'), value: 'week' },
                            { label: t('月（每月重置）'), value: 'month' },
                          ]}
                          onChange={(value) =>
                            handleInputChange('quota_policy_period', value)
                          }
                          style={{ width: '100%' }}
                        />

                        <Row gutter={12}>
                          <Col span={12}>
                            <Form.InputNumber
                              field='quota_policy_quota_limit'
                              label={t('周期内金额上限（0=无限）')}
                              placeholder={t('按系统货币填写')}
                              min={0}
                              precision={2}
                              onNumberChange={(value) =>
                                handleInputChange(
                                  'quota_policy_quota_limit',
                                  value,
                                )
                              }
                              style={{ width: '100%' }}
                            />
                          </Col>
                          <Col span={12}>
                            <Form.InputNumber
                              field='quota_policy_count_limit'
                              label={t('周期内调用次数上限（0=无限）')}
                              placeholder={t('每次成功请求计 1 次')}
                              min={0}
                              precision={0}
                              onNumberChange={(value) =>
                                handleInputChange(
                                  'quota_policy_count_limit',
                                  value,
                                )
                              }
                              style={{ width: '100%' }}
                            />
                          </Col>
                        </Row>
                      </>
                    )}

                    {!inputs.quota_policy_enabled &&
                      inheritedTagPolicy?.enabled && (
                        <Banner
                          type='info'
                          closeIcon={null}
                          className='!rounded-lg mt-4'
                          description={t(
                            '将继承标签 「{{tag}}」 的共享池配额：周期 {{period}}，金额上限 {{quota}}，次数上限 {{count}}',
                            {
                              tag: (inputs.tag || '').trim(),
                              period:
                                inheritedTagPolicy.period === 'day'
                                  ? t('日')
                                  : inheritedTagPolicy.period === 'week'
                                    ? t('周')
                                    : t('月'),
                              quota:
                                Number(inheritedTagPolicy.quota_limit) > 0
                                  ? renderCurrencyQuota(
                                      Number(inheritedTagPolicy.quota_limit),
                                    )
                                  : t('无限'),
                              count:
                                Number(inheritedTagPolicy.count_limit) > 0
                                  ? inheritedTagPolicy.count_limit
                                  : t('无限'),
                            },
                          )}
                        />
                      )}

                    {isEdit && quotaUsage && (
                      <div className='mt-4 p-3 rounded-xl bg-semi-color-fill-0'>
                        <div className='flex items-center justify-between mb-2'>
                          <Text className='text-sm font-medium'>
                            {t('当前周期用量')}
                            {quotaUsage.scope === 'tag' && (
                              <Tag
                                size='small'
                                color='blue'
                                shape='circle'
                                className='ml-2'
                              >
                                {t('标签共享池')}
                              </Tag>
                            )}
                          </Text>
                          <Button
                            size='small'
                            theme='borderless'
                            onClick={() =>
                              setQuotaUsageRefreshTick((tick) => tick + 1)
                            }
                          >
                            {t('刷新')}
                          </Button>
                        </div>
                        {Number(quotaUsage.quota_limit) > 0 && (
                          <div className='mb-2'>
                            <div className='flex justify-between text-xs mb-1'>
                              <span>{t('周期内已用金额')}</span>
                              <span>
                                {renderCurrencyQuota(
                                  Number(quotaUsage.used_quota || 0),
                                )}{' '}
                                /{' '}
                                {renderCurrencyQuota(
                                  Number(quotaUsage.quota_limit),
                                )}
                              </span>
                            </div>
                            <Progress
                              percent={Math.min(
                                100,
                                Math.floor(
                                  (Number(quotaUsage.used_quota || 0) /
                                    Math.max(
                                      1,
                                      Number(quotaUsage.quota_limit),
                                    )) *
                                    100,
                                ),
                              )}
                              showInfo={false}
                              size='small'
                            />
                          </div>
                        )}
                        {Number(quotaUsage.count_limit) > 0 && (
                          <div className='mb-2'>
                            <div className='flex justify-between text-xs mb-1'>
                              <span>{t('周期内已调用次数')}</span>
                              <span>
                                {Number(quotaUsage.used_count || 0)} /{' '}
                                {Number(quotaUsage.count_limit)}
                              </span>
                            </div>
                            <Progress
                              percent={Math.min(
                                100,
                                Math.floor(
                                  (Number(quotaUsage.used_count || 0) /
                                    Math.max(
                                      1,
                                      Number(quotaUsage.count_limit),
                                    )) *
                                    100,
                                ),
                              )}
                              showInfo={false}
                              size='small'
                            />
                          </div>
                        )}
                        <div className='text-xs text-gray-500 mt-2'>
                          {t('周期窗口')}:{' '}
                          {formatUnixTime(quotaUsage.period_start)}{' '}
                          {t('至')} {formatUnixTime(quotaUsage.period_end)}
                        </div>
                      </div>
                    )}
                  </Card>
                </div>

                {/* Channel Extra Settings Card */}
                <div
                  ref={(el) =>
                    (formSectionRefs.current.channelExtraSettings = el)
                  }
                >
                  <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                    {/* Header: Channel Extra Settings */}
                    <div className='flex items-center mb-2'>
                      <Avatar
                        size='small'
                        color='violet'
                        className='mr-2 shadow-md'
                      >
                        <IconBolt size={16} />
                      </Avatar>
                      <div>
                        <Text className='text-lg font-medium'>
                          {t('渠道额外设置')}
                        </Text>
                      </div>
                    </div>

                    <Form.Switch
                      field='cpa_user_identity_enabled'
                      label={t('CPA 用户身份转发')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) => handleChannelSettingsChange('cpa_user_identity_enabled', value)}
                      extraText={t('仅对可信 CPA 渠道开启，使用登录用户 ID 覆盖身份请求头，用于跨令牌识别同一用户。')}
                    />
                    {inputs.cpa_user_identity_enabled && (
                      <Form.Input
                        field='cpa_instance_id'
                        label={t('New API 实例标识')}
                        placeholder='newapi-main'
                        maxLength={64}
                        onChange={(value) => handleChannelSettingsChange('cpa_instance_id', value)}
                        extraText={t('同一站点的 CPA 渠道填写相同标识，不同站点不要重复；使用字母或数字开头，允许字母、数字、点、下划线和短横线，最多 64 位。')}
                      />
                    )}

                    {inputs.type === 14 && (
                      <Form.Switch
                        field='claude_beta_query'
                        label={t('Claude 强制 beta=true')}
                        checkedText={t('开')}
                        uncheckedText={t('关')}
                        onChange={(value) =>
                          handleChannelOtherSettingsChange(
                            'claude_beta_query',
                            value,
                          )
                        }
                        extraText={t(
                          '开启后，该渠道请求 Claude 时将强制追加 ?beta=true（无需客户端手动传参）',
                        )}
                      />
                    )}

                    {inputs.type === 14 && (
                      <Form.Switch
                        field='claude_incremental_cache_enabled'
                        label={t('Claude 增量缓存')}
                        checkedText={t('开')}
                        uncheckedText={t('关')}
                        onChange={(value) =>
                          handleChannelSettingsChange(
                            'claude_incremental_cache_enabled',
                            value,
                          )
                        }
                        extraText={t(
                          '仅对 OpenAI Chat Completions 转 Claude 的请求生效；每轮在最后一个用户内容块追加缓存断点，使历史缓存随对话增长。首次请求可能产生缓存创建费用',
                        )}
                      />
                    )}

                    {inputs.type === 1 && (
                      <Form.Switch
                        field='force_format'
                        label={t('强制格式化')}
                        checkedText={t('开')}
                        uncheckedText={t('关')}
                        onChange={(value) =>
                          handleChannelSettingsChange('force_format', value)
                        }
                        extraText={t(
                          '强制将响应格式化为 OpenAI 标准格式（只适用于OpenAI渠道类型）',
                        )}
                      />
                    )}

                    <Form.Switch
                      field='thinking_to_content'
                      label={t('思考内容转换')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) =>
                        handleChannelSettingsChange(
                          'thinking_to_content',
                          value,
                        )
                      }
                      extraText={t(
                        '将 reasoning_content 转换为 <think> 标签拼接到内容中',
                      )}
                    />

                    <Form.Switch
                      field='pass_through_body_enabled'
                      label={t('透传请求体')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) =>
                        handleChannelSettingsChange(
                          'pass_through_body_enabled',
                          value,
                        )
                      }
                      extraText={t('启用请求体透传功能')}
                    />

                    <Form.InputNumber
                      field='nonstream_response_header_timeout_sec'
                      label={t('非流式响应头超时（秒）')}
                      min={0}
                      max={600}
                      precision={0}
                      onChange={(value) =>
                        handleChannelSettingsChange('nonstream_response_header_timeout_sec', value ?? 0)
                      }
                      style={{ width: '100%' }}
                      extraText={t('0 为继承全局；仅非流式文本请求生效，最长 600 秒。流式和生图不变，总超时及反向代理限制仍生效。')}
                    />
                    <Form.Input
                      field='proxy'
                      label={t('代理地址')}
                      placeholder={t('例如: socks5://user:pass@host:port')}
                      onChange={(value) =>
                        handleChannelSettingsChange('proxy', value)
                      }
                      showClear
                      extraText={t('用于配置网络代理，支持 socks5 协议')}
                    />

                    <Form.InputNumber
                      field='max_concurrency'
                      label={t('渠道最大并发')}
                      min={0}
                      onChange={(value) =>
                        handleChannelSettingsChange('max_concurrency', value)
                      }
                      style={{ width: '100%' }}
                      placeholder={t('0 表示使用全局默认')}
                      extraText={t(
                        '该渠道的在途并发上限，0 表示使用全局默认值。需在「运营设置」开启渠道并发控制后生效；超过上限时请求会切换到其它未满渠道，全部满时进入有界等待或返回 503',
                      )}
                    />

                    <Form.InputNumber
                      field='rpm_limit'
                      label={t('渠道每分钟请求上限')}
                      min={0}
                      onChange={(value) =>
                        handleChannelSettingsChange('rpm_limit', value)
                      }
                      style={{ width: '100%' }}
                      placeholder={t('0 表示使用全局默认')}
                      extraText={t(
                        '该渠道在 RPM 统计窗口内允许进入的请求数，0 表示使用全局默认值；全局也为 0 时不限制',
                      )}
                    />

                    <Form.Select
                      field='crs_site_id'
                      label={t('绑定 CRS 站点')}
                      placeholder={t('不绑定')}
                      optionList={crsSites.map((site) => ({
                        label: site.name || site.host,
                        value: site.id,
                      }))}
                      showClear
                      filter
                      onChange={(value) => {
                        const siteId = Number(value) || 0;
                        handleChannelSettingsChange('crs_site_id', siteId);
                        if (!siteId) {
                          handleChannelSettingsChange('crs_auto_manage', false);
                        }
                      }}
                      extraText={t('显式绑定账号池，避免按域名猜测渠道归属')}
                    />

                    <Form.Select
                      field='crs_platform'
                      label={t('CRS 账号池类型')}
                      optionList={[
                        { label: 'OpenAI Responses', value: 'openai-responses' },
                        { label: 'OpenAI', value: 'openai' },
                      ]}
                      onChange={(value) =>
                        handleChannelSettingsChange('crs_platform', value)
                      }
                    />

                    <Form.Switch
                      field='crs_auto_manage'
                      label={t('CRS 自动禁用与恢复')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      disabled={!inputs.crs_site_id}
                      onChange={(value) =>
                        handleChannelSettingsChange('crs_auto_manage', value)
                      }
                      extraText={t(
                        '账号池连续两次不可用时禁用，连续两次恢复后仅恢复由 CRS 禁用的渠道',
                      )}
                    />

                    <Form.TextArea
                      field='system_prompt'
                      label={t('系统提示词')}
                      placeholder={t(
                        '输入系统提示词，用户的系统提示词将优先于此设置',
                      )}
                      onChange={(value) =>
                        handleChannelSettingsChange('system_prompt', value)
                      }
                      autosize
                      showClear
                      extraText={t(
                        '用户优先：如果用户在请求中指定了系统提示词，将优先使用用户的设置',
                      )}
                    />
                    <Form.Switch
                      field='system_prompt_override'
                      label={t('系统提示词拼接')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) =>
                        handleChannelSettingsChange(
                          'system_prompt_override',
                          value,
                        )
                      }
                      extraText={t(
                        '如果用户请求中包含系统提示词，则使用此设置拼接到用户的系统提示词前面',
                      )}
                    />

                    <Form.Select
                      field='client_restriction_mode'
                      label={t('客户端限制')}
                      placeholder={t('不限制')}
                      optionList={[
                        { label: t('不限制'), value: '' },
                        { label: t('仅允许（白名单）'), value: 'allowlist' },
                        { label: t('禁止（黑名单）'), value: 'blocklist' },
                      ]}
                      onChange={(value) =>
                        handleChannelSettingsChange(
                          'client_restriction_mode',
                          value,
                        )
                      }
                      extraText={t(
                        '限制哪些客户端工具可以使用该渠道',
                      )}
                    />

                    {inputs.client_restriction_mode && (
                      <Form.Select
                        field='client_restriction_clients'
                        label={t('客户端列表')}
                        placeholder={t('选择客户端工具')}
                        multiple
                        filter
                        optionList={[
                          { label: 'Claude Code CLI', value: 'claude-code' },
                          { label: 'Codex CLI', value: 'codex-cli' },
                          { label: 'Gemini CLI', value: 'gemini-cli' },
                          { label: 'Droid CLI', value: 'droid-cli' },
                          { label: 'Aider', value: 'aider' },
                          { label: 'Cursor', value: 'cursor' },
                          { label: 'Windsurf', value: 'windsurf' },
                          { label: 'Cline', value: 'cline' },
                          { label: 'Roo Code', value: 'roo-code' },
                          { label: 'Continue', value: 'continue' },
                          { label: 'Trae', value: 'trae' },
                          { label: 'Zed', value: 'zed' },
                          { label: 'JetBrains AI', value: 'jetbrains' },
                          { label: 'Augment', value: 'augment' },
                          { label: 'Cherry Studio', value: 'cherry-studio' },
                          { label: 'ChatBox', value: 'chatbox' },
                          { label: 'NextChat', value: 'nextchat' },
                          { label: 'LobeChat', value: 'lobechat' },
                          { label: 'Open WebUI', value: 'open-webui' },
                          { label: 'LibreChat', value: 'librechat' },
                          { label: 'TypingMind', value: 'typingmind' },
                          { label: 'OpenCat', value: 'opencat' },
                          { label: 'BotGem', value: 'botgem' },
                          { label: 'Jan', value: 'jan' },
                          { label: t('API 直连'), value: 'api-direct' },
                        ]}
                        style={{ width: '100%' }}
                        onChange={(value) =>
                          handleChannelSettingsChange(
                            'client_restriction_clients',
                            value,
                          )
                        }
                      />
                    )}
                  </Card>
                </div>
              </div>
            </Spin>
          )}
        </Form>
        <ImagePreview
          src={modalImageUrl}
          visible={isModalOpenurl}
          onVisibleChange={(visible) => setIsModalOpenurl(visible)}
        />
      </SideSheet>
      <StatusCodeRiskGuardModal
        visible={statusCodeRiskConfirmVisible}
        detailItems={statusCodeRiskDetailItems}
        onCancel={() => resolveStatusCodeRiskConfirm(false)}
        onConfirm={() => resolveStatusCodeRiskConfirm(true)}
      />
      {/* 使用通用安全验证模态框 */}
      <SecureVerificationModal
        visible={isModalVisible}
        verificationMethods={verificationMethods}
        verificationState={verificationState}
        onVerify={executeVerification}
        onCancel={cancelVerification}
        onCodeChange={setVerificationCode}
        onMethodSwitch={switchVerificationMethod}
        title={verificationState.title}
        description={verificationState.description}
      />

      {/* 使用ChannelKeyDisplay组件显示密钥 */}
      <Modal
        title={
          <div className='flex items-center'>
            <div className='w-8 h-8 rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center mr-3'>
              <svg
                className='w-4 h-4 text-green-600 dark:text-green-400'
                fill='currentColor'
                viewBox='0 0 20 20'
              >
                <path
                  fillRule='evenodd'
                  d='M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z'
                  clipRule='evenodd'
                />
              </svg>
            </div>
            {t('渠道密钥信息')}
          </div>
        }
        visible={keyDisplayState.showModal}
        onCancel={resetKeyDisplayState}
        footer={
          <Button type='primary' onClick={resetKeyDisplayState}>
            {t('完成')}
          </Button>
        }
        width={700}
        style={{ maxWidth: '90vw' }}
      >
        <ChannelKeyDisplay
          keyData={keyDisplayState.keyData}
          showSuccessIcon={true}
          successText={t('密钥获取成功')}
          showWarning={true}
          warningText={t(
            '请妥善保管密钥信息，不要泄露给他人。如有安全疑虑，请及时更换密钥。',
          )}
        />
      </Modal>

      <ParamOverrideEditorModal
        visible={paramOverrideEditorVisible}
        value={inputs.param_override || ''}
        onCancel={() => setParamOverrideEditorVisible(false)}
        onSave={(nextValue) => {
          handleInputChange('param_override', nextValue);
          setParamOverrideEditorVisible(false);
        }}
      />

      <ModelSelectModal
        visible={modelModalVisible}
        models={fetchedModels}
        selected={inputs.models}
        redirectModels={redirectModelList}
        redirectSourceModels={redirectModelKeyList}
        onConfirm={(selectedModels) => {
          handleInputChange('models', selectedModels);
          showSuccess(t('模型列表已更新'));
          setModelModalVisible(false);
        }}
        onCancel={() => setModelModalVisible(false)}
      />

      <SingleModelSelectModal
        visible={modelMappingValueModalVisible}
        models={modelMappingValueModalModels}
        selected={modelMappingValueSelected}
        onConfirm={(selectedModel) => {
          const modelName = String(selectedModel ?? '').trim();
          if (!modelName) {
            showError(t('请先选择模型！'));
            return;
          }

          const mappingKey = String(modelMappingValueKey ?? '').trim();
          if (!mappingKey) {
            setModelMappingValueModalVisible(false);
            return;
          }

          let parsed = {};
          const currentMapping = inputs.model_mapping;
          if (typeof currentMapping === 'string' && currentMapping.trim()) {
            try {
              parsed = JSON.parse(currentMapping);
            } catch (error) {
              parsed = {};
            }
          } else if (
            currentMapping &&
            typeof currentMapping === 'object' &&
            !Array.isArray(currentMapping)
          ) {
            parsed = currentMapping;
          }
          if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
            parsed = {};
          }

          parsed[mappingKey] = modelName;
          const nextMapping = JSON.stringify(parsed, null, 2);
          handleInputChange('model_mapping', nextMapping);
          if (formApiRef.current) {
            formApiRef.current.setValue('model_mapping', nextMapping);
          }
          setModelMappingValueModalVisible(false);
        }}
        onCancel={() => setModelMappingValueModalVisible(false)}
      />

      <OllamaModelModal
        visible={ollamaModalVisible}
        onCancel={() => setOllamaModalVisible(false)}
        channelId={channelId}
        channelInfo={inputs}
        onModelsUpdate={(options = {}) => {
          // 当模型更新后，重新获取模型列表以更新表单
          fetchUpstreamModelList('models', { silent: !!options.silent });
        }}
        onApplyModels={({ mode, modelIds } = {}) => {
          if (!Array.isArray(modelIds) || modelIds.length === 0) {
            return;
          }
          const existingModels = Array.isArray(inputs.models)
            ? inputs.models.map(String)
            : [];
          const incoming = modelIds.map(String);
          const nextModels = Array.from(
            new Set([...existingModels, ...incoming]),
          );

          handleInputChange('models', nextModels);
          if (formApiRef.current) {
            formApiRef.current.setValue('models', nextModels);
          }
          showSuccess(t('模型列表已追加更新'));
        }}
      />
    </>
  );
};

export default EditChannelModal;
