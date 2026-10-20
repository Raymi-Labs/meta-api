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

import {
  isLiveTestSuccess,
  summarizeModelTests,
} from '../../helpers/videoTest';
import { useState, useEffect, useRef, useMemo } from 'react';
import {
  initialChannelUsage,
  sumChannelUsage,
} from '../../helpers/channelUsage';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  loadChannelModels,
  copy,
  toBoolean,
} from '../../helpers';
import {
  CHANNEL_OPTIONS,
  ITEMS_PER_PAGE,
  MODEL_TABLE_PAGE_SIZE,
} from '../../constants';
import { useIsMobile } from '../common/useIsMobile';
import { useTableCompactMode } from '../common/useTableCompactMode';
import { useChannelUpstreamUpdates } from './useChannelUpstreamUpdates';
import { parseUpstreamUpdateMeta } from './upstreamUpdateUtils';
import { getTagAggregationStatus } from '../../components/table/channels/tagAggregationStatus';
import {
  buildTagTestSummary,
  collectTagTestModels,
  resolveTagTestTargets,
  shouldPromptEnableChannelAfterManualTest,
} from '../../components/table/channels/tagTestUtils';
import { Modal, Button, Checkbox } from '@douyinfe/semi-ui';
import { openCodexUsageModal } from '../../components/table/channels/modals/CodexUsageModal';
import {
  buildChannelCategoryQuery,
  CHANNEL_CATEGORY_ALL,
} from '../../helpers/channelCategory';

const EMPTY_CHANNEL_CONCURRENCY_SNAPSHOT = {
  loaded: false,
  enabled: false,
  defaultMaxConcurrency: 100,
  byChannelId: {},
};

export const useChannelsData = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();

  const [usageOptions, setUsageOptions] = useState(() =>
    initialChannelUsage(window.location.search),
  );
  const [usageMeta, setUsageMeta] = useState(null);
  const [usageError, setUsageError] = useState('');
  const usageOptionsMounted = useRef(false);

  // 基础状态s
  const [channels, setChannels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [idSort, setIdSort] = useState(false);
  const [searching, setSearching] = useState(false);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [channelCount, setChannelCount] = useState(0);
  const [groupOptions, setGroupOptions] = useState([]);

  // UI 状态s
  const [showEdit, setShowEdit] = useState(false);
  const [enableBatchDelete, setEnableBatchDelete] = useState(false);
  const [editingChannel, setEditingChannel] = useState({ id: undefined });
  const [showEditTag, setShowEditTag] = useState(false);
  const [editingTag, setEditingTag] = useState('');
  const [selectedChannels, setSelectedChannels] = useState([]);
  const [enableTagMode, setEnableTagMode] = useState(false);
  const [showBatchSetTag, setShowBatchSetTag] = useState(false);
  const [batchSetTagValue, setBatchSetTagValue] = useState('');
  const [compactMode, setCompactMode] = useTableCompactMode('channels');

  // Column visibility states
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Status filter
  const [statusFilter, setStatusFilter] = useState(
    localStorage.getItem('channel-status-filter') || 'all',
  );

  // Display categories are independent from Channel.Type, which remains the
  // relay protocol used by API clients and provider adapters.
  const [activeCategoryKey, setActiveCategoryKey] =
    useState(CHANNEL_CATEGORY_ALL);
  const [categoryCounts, setCategoryCounts] = useState({});

  // Model test states
  const [showModelTestModal, setShowModelTestModal] = useState(false);
  const [currentTestChannel, setCurrentTestChannel] = useState(null);
  const [modelSearchKeyword, setModelSearchKeyword] = useState('');
  const [modelTestResults, setModelTestResults] = useState({});
  const [testingModels, setTestingModels] = useState(new Set());
  const [selectedModelKeys, setSelectedModelKeys] = useState([]);
  const [isBatchTesting, setIsBatchTesting] = useState(false);
  const [modelTablePage, setModelTablePage] = useState(1);
  const [selectedEndpointType, setSelectedEndpointType] = useState('');
  const [isStreamTest, setIsStreamTest] = useState(false);
  const [showTagTestModal, setShowTagTestModal] = useState(false);
  const [currentTagTestGroup, setCurrentTagTestGroup] = useState(null);
  const [tagTestMode, setTagTestMode] = useState('default');
  const [tagTestScope, setTagTestScope] = useState('all');
  const [tagTestLoading, setTagTestLoading] = useState(false);
  const [tagTestSubmitting, setTagTestSubmitting] = useState(false);
  const [tagTestChannels, setTagTestChannels] = useState([]);
  const [selectedTagTestChannelIds, setSelectedTagTestChannelIds] = useState(
    [],
  );
  const [selectedTagTestModel, setSelectedTagTestModel] = useState('');
  const [globalPassThroughEnabled, setGlobalPassThroughEnabled] =
    useState(false);
  const [channelConcurrencySnapshot, setChannelConcurrencySnapshot] = useState(
    EMPTY_CHANNEL_CONCURRENCY_SNAPSHOT,
  );
  const channelConcurrencyRequestRef = useRef(0);

  const fetchGlobalPassThroughEnabled = async () => {
    try {
      const res = await API.get('/api/option/');
      const { success, data } = res?.data || {};
      if (!success || !Array.isArray(data)) {
        return;
      }
      const option = data.find(
        (item) => item?.key === 'global.pass_through_request_enabled',
      );
      if (option) {
        setGlobalPassThroughEnabled(toBoolean(option.value));
      }
    } catch (error) {
      setGlobalPassThroughEnabled(false);
    }
  };

  const fetchChannelConcurrencyStats = async () => {
    const reqId = ++channelConcurrencyRequestRef.current;
    try {
      const res = await API.get('/api/channel_concurrency/stats');
      if (res === undefined || reqId !== channelConcurrencyRequestRef.current) {
        return;
      }
      const { success, data } = res.data || {};
      if (!success || !data) {
        setChannelConcurrencySnapshot((current) => ({
          ...current,
          loaded: false,
        }));
        return;
      }

      const configuredDefault = Number(data.config?.default_max_concurrency);
      const defaultMaxConcurrency =
        Number.isInteger(configuredDefault) && configuredDefault > 0
          ? configuredDefault
          : 100;
      const byChannelId = {};
      for (const item of data.channels || []) {
        const channelId = Number(item?.channel_id);
        if (!Number.isInteger(channelId) || channelId <= 0) continue;
        byChannelId[channelId] = {
          inflight: Math.max(0, Number(item.inflight) || 0),
          maxConcurrency:
            Number(item.max_concurrency) > 0
              ? Number(item.max_concurrency)
              : defaultMaxConcurrency,
        };
      }
      setChannelConcurrencySnapshot({
        loaded: true,
        enabled: data.enabled === true,
        defaultMaxConcurrency,
        byChannelId,
      });
    } catch (error) {
      if (reqId === channelConcurrencyRequestRef.current) {
        setChannelConcurrencySnapshot((current) => ({
          ...current,
          loaded: false,
        }));
      }
    }
  };

  // 使用 ref 来避免闭包问题，类似旧版实现
  const shouldStopBatchTestingRef = useRef(false);

  // Multi-key management states
  const [showMultiKeyManageModal, setShowMultiKeyManageModal] = useState(false);
  const [currentMultiKeyChannel, setCurrentMultiKeyChannel] = useState(null);

  // Refs
  const requestCounter = useRef(0);
  const allSelectingRef = useRef(false);
  const [formApi, setFormApi] = useState(null);

  const formInitValues = {
    searchKeyword: '',
    searchGroup: '',
    searchModel: '',
  };

  // Column keys
  const COLUMN_KEYS = {
    ID: 'id',
    NAME: 'name',
    GROUP: 'group',
    TYPE: 'type',
    STATUS: 'status',
    CONCURRENCY: 'concurrency',
    RESPONSE_TIME: 'response_time',
    BALANCE: 'balance',
    PRIORITY: 'priority',
    WEIGHT: 'weight',
    QUOTA_USAGE: 'quota_usage',
    OPERATE: 'operate',
  };

  // Initialize from localStorage
  useEffect(() => {
    const localIdSort = localStorage.getItem('id-sort') === 'true';
    const localPageSize =
      parseInt(localStorage.getItem('page-size')) || ITEMS_PER_PAGE;
    const localEnableTagMode =
      localStorage.getItem('enable-tag-mode') === 'true';
    const localEnableBatchDelete =
      localStorage.getItem('enable-batch-delete') === 'true';

    setIdSort(localIdSort);
    setPageSize(localPageSize);
    setEnableTagMode(localEnableTagMode);
    setEnableBatchDelete(localEnableBatchDelete);

    loadChannels(1, localPageSize, localIdSort, localEnableTagMode)
      .then()
      .catch((reason) => {
        showError(reason);
      });
    fetchGroups().then();
    loadChannelModels().then();
    fetchGlobalPassThroughEnabled().then();
    fetchChannelConcurrencyStats().then();
  }, []);

  // Column visibility management
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.ID]: true,
      [COLUMN_KEYS.NAME]: true,
      [COLUMN_KEYS.GROUP]: true,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.STATUS]: true,
      [COLUMN_KEYS.CONCURRENCY]: true,
      [COLUMN_KEYS.RESPONSE_TIME]: true,
      [COLUMN_KEYS.BALANCE]: true,
      [COLUMN_KEYS.PRIORITY]: true,
      [COLUMN_KEYS.WEIGHT]: true,
      [COLUMN_KEYS.QUOTA_USAGE]: false,
      [COLUMN_KEYS.OPERATE]: true,
    };
  };

  const initDefaultColumns = () => {
    const defaults = getDefaultColumnVisibility();
    setVisibleColumns(defaults);
  };

  // Load saved column preferences
  useEffect(() => {
    const savedColumns = localStorage.getItem('channels-table-columns');
    if (savedColumns) {
      try {
        const parsed = JSON.parse(savedColumns);
        const defaults = getDefaultColumnVisibility();
        const merged = { ...defaults, ...parsed };
        setVisibleColumns(merged);
      } catch (e) {
        console.error('Failed to parse saved column preferences', e);
        initDefaultColumns();
      }
    } else {
      initDefaultColumns();
    }
  }, []);

  // Save column preferences
  useEffect(() => {
    if (Object.keys(visibleColumns).length > 0) {
      localStorage.setItem(
        'channels-table-columns',
        JSON.stringify(visibleColumns),
      );
    }
  }, [visibleColumns]);

  const handleColumnVisibilityChange = (columnKey, checked) => {
    const updatedColumns = { ...visibleColumns, [columnKey]: checked };
    setVisibleColumns(updatedColumns);
  };

  const handleSelectAll = (checked) => {
    const allKeys = Object.keys(COLUMN_KEYS).map((key) => COLUMN_KEYS[key]);
    const updatedColumns = {};
    allKeys.forEach((key) => {
      updatedColumns[key] = checked;
    });
    setVisibleColumns(updatedColumns);
  };

  // Data formatting
  const decorateChannelRecord = (channel) => {
    if (!channel || typeof channel !== 'object') {
      return channel;
    }
    return {
      ...channel,
      upstreamUpdateMeta: parseUpstreamUpdateMeta(channel.settings),
    };
  };

  const setChannelFormat = (channels, enableTagMode) => {
    let channelDates = [];
    let channelTags = {};

    for (let i = 0; i < channels.length; i++) {
      const currentChannel = decorateChannelRecord(channels[i]);
      currentChannel.key = '' + currentChannel.id;
      if (!enableTagMode) {
        channelDates.push(currentChannel);
      } else {
        let tag = currentChannel.tag ? currentChannel.tag : '';
        let tagIndex = channelTags[tag];
        let tagChannelDates = undefined;

        if (tagIndex === undefined) {
          channelTags[tag] = 1;
          tagChannelDates = {
            key: tag,
            id: tag,
            tag: tag,
            name: '标签：' + tag,
            group: '',
            used_quota: 0,
            response_time: 0,
            priority: -1,
            weight: -1,
          };
          tagChannelDates.children = [];
          channelDates.push(tagChannelDates);
        } else {
          tagChannelDates = channelDates.find((item) => item.key === tag);
        }

        if (tagChannelDates.priority === -1) {
          tagChannelDates.priority = currentChannel.priority;
        } else {
          if (tagChannelDates.priority !== currentChannel.priority) {
            tagChannelDates.priority = '';
          }
        }

        if (tagChannelDates.weight === -1) {
          tagChannelDates.weight = currentChannel.weight;
        } else {
          if (tagChannelDates.weight !== currentChannel.weight) {
            tagChannelDates.weight = '';
          }
        }

        if (tagChannelDates.group === '') {
          tagChannelDates.group = currentChannel.group;
        } else {
          let channelGroupsStr = currentChannel.group;
          channelGroupsStr.split(',').forEach((item, index) => {
            if (tagChannelDates.group.indexOf(item) === -1) {
              tagChannelDates.group += ',' + item;
            }
          });
        }

        tagChannelDates.children.push(currentChannel);
        if (currentChannel.upstreamUpdateMeta?.enabled) {
          tagChannelDates.upstreamUpdateMeta =
            tagChannelDates.upstreamUpdateMeta || {
              enabled: true,
              pendingAddModels: [],
              pendingRemoveModels: [],
            };
          tagChannelDates.upstreamUpdateMeta.pendingAddModels = Array.from(
            new Set([
              ...tagChannelDates.upstreamUpdateMeta.pendingAddModels,
              ...(currentChannel.upstreamUpdateMeta.pendingAddModels || []),
            ]),
          );
          tagChannelDates.upstreamUpdateMeta.pendingRemoveModels = Array.from(
            new Set([
              ...tagChannelDates.upstreamUpdateMeta.pendingRemoveModels,
              ...(currentChannel.upstreamUpdateMeta.pendingRemoveModels || []),
            ]),
          );
        }
        tagChannelDates.used_quota += currentChannel.used_quota;
        tagChannelDates.response_time += currentChannel.response_time;
        tagChannelDates.response_time = tagChannelDates.response_time / 2;
      }
    }
    if (enableTagMode) {
      channelDates = channelDates.map((item) => {
        if (item?.children === undefined) {
          return item;
        }
        const tagAggregationStatus = getTagAggregationStatus(item.children);
        let status = item.status;
        if (tagAggregationStatus.isAllEnabled) {
          status = 1;
        } else if (tagAggregationStatus.isAllDisabled) {
          status = 2;
        } else if (tagAggregationStatus.isMixed) {
          // Mixed tag rows should stay visually available, but render as a compact progress state.
          status = 1;
        }
        return {
          ...item,
          status,
          usage: sumChannelUsage(item.children),
          effective_available: tagAggregationStatus.enabledCount > 0,
          tagAggregationStatus,
        };
      });
    }
    setChannels(channelDates);
  };

  // Get form values helper
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      searchKeyword: formValues.searchKeyword || '',
      searchGroup: formValues.searchGroup || '',
      searchModel: formValues.searchModel || '',
    };
  };

  const changeUsageOptions = (options) => {
    requestCounter.current += 1;
    setUsageMeta(null);
    setUsageError('');
    setChannels([]);
    setActivePage(1);
    setUsageOptions(options);
    const url = new URL(window.location.href);
    if (options.enabled) {
      url.searchParams.set('view', 'usage');
      url.searchParams.set('start_time', options.range[0]);
      url.searchParams.set('end_time', options.range[1]);
    } else {
      ['view', 'start_time', 'end_time'].forEach((key) =>
        url.searchParams.delete(key),
      );
    }
    window.history.replaceState(null, '', url);
  };

  const loadUsageChannels = async (page, size, tagMode, category, status) => {
    const reqId = ++requestCounter.current;
    setLoading(true);
    setUsageError('');
    setUsageMeta(null);
    setChannels([]);
    const values = getFormValues();
    try {
      const res = await API.get('/api/channel/usage', {
        params: {
          p: page,
          page_size: size,
          tag_mode: tagMode,
          category: tagMode ? 'all' : category,
          status,
          keyword: values.searchKeyword,
          group: values.searchGroup,
          model: values.searchModel,
          start_time: usageOptions.range[0],
          end_time: usageOptions.range[1],
          usage_order: usageOptions.order,
        },
      });
      if (reqId !== requestCounter.current) return;
      const { success, data, message } = res.data || {};
      if (!success || !Array.isArray(data?.items) || !data?.usage_summary)
        throw new Error(message || t('渠道用量不可用，请确认后端已升级'));
      setChannelFormat(data.items, tagMode);
      setChannelCount(data.total);
      setCategoryCounts(data.category_counts || {});
      setUsageMeta({
        ...data,
        items: undefined,
        group: values.searchGroup,
        model: values.searchModel,
      });
      setActivePage(page);
    } catch (error) {
      if (reqId === requestCounter.current) {
        setChannelCount(0);
        setUsageError(
          error.response?.status === 404
            ? t('渠道用量不可用，请确认后端已升级')
            : t('加载渠道用量失败，请重试'),
        );
      }
    } finally {
      if (reqId === requestCounter.current) {
        setLoading(false);
        setSearching(false);
      }
    }
  };

  useEffect(() => {
    if (!usageOptionsMounted.current) {
      usageOptionsMounted.current = true;
      return;
    }
    loadChannels(1, pageSize, idSort, enableTagMode).catch(showError);
  }, [usageOptions]);

  // Load channels
  const loadChannels = async (
    page,
    pageSize,
    idSort,
    enableTagMode,
    categoryKey = activeCategoryKey,
    statusF,
  ) => {
    if (statusF === undefined) statusF = statusFilter;
    if (usageOptions.enabled)
      return loadUsageChannels(
        page,
        pageSize,
        enableTagMode,
        categoryKey,
        statusF,
      );

    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    if (searchKeyword !== '' || searchGroup !== '' || searchModel !== '') {
      await searchChannels(
        enableTagMode,
        categoryKey,
        statusF,
        page,
        pageSize,
        idSort,
      );
      return;
    }

    const reqId = ++requestCounter.current;
    setLoading(true);
    const statusParam = statusF !== 'all' ? `&status=${statusF}` : '';
    const categoryParam = buildChannelCategoryQuery(categoryKey, enableTagMode);
    const res = await API.get(
      `/api/channel/?p=${page}&page_size=${pageSize}&id_sort=${idSort}&tag_mode=${enableTagMode}${statusParam}${categoryParam}`,
    );

    if (res === undefined || reqId !== requestCounter.current) {
      return;
    }

    const { success, message, data } = res.data;
    if (success) {
      const { items, total, category_counts } = data;
      if (category_counts) {
        setCategoryCounts(category_counts);
      }
      setChannelFormat(items, enableTagMode);
      setChannelCount(total);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // Search channels
  const searchChannels = async (
    enableTagMode,
    categoryKey = activeCategoryKey,
    statusF = statusFilter,
    page = 1,
    pageSz = pageSize,
    sortFlag = idSort,
  ) => {
    if (usageOptions.enabled)
      return loadUsageChannels(
        page,
        pageSz,
        enableTagMode,
        categoryKey,
        statusF,
      );
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    const reqId = ++requestCounter.current;
    setLoading(true);
    setSearching(true);
    try {
      if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
        await loadChannels(
          page,
          pageSz,
          sortFlag,
          enableTagMode,
          categoryKey,
          statusF,
        );
        return;
      }

      const statusParam = statusF !== 'all' ? `&status=${statusF}` : '';
      const categoryParam = buildChannelCategoryQuery(
        categoryKey,
        enableTagMode,
      );
      const res = await API.get(
        `/api/channel/search?keyword=${searchKeyword}&group=${searchGroup}&model=${searchModel}&id_sort=${sortFlag}&tag_mode=${enableTagMode}&p=${page}&page_size=${pageSz}${statusParam}${categoryParam}`,
      );
      if (reqId !== requestCounter.current) {
        return;
      }
      const { success, message, data } = res.data;
      if (success) {
        const { items = [], total = 0, category_counts = {} } = data;
        setCategoryCounts(category_counts);
        setChannelFormat(items, enableTagMode);
        setChannelCount(total);
        setActivePage(page);
      } else {
        showError(message);
      }
    } finally {
      if (reqId === requestCounter.current) {
        setSearching(false);
        setLoading(false);
      }
    }
  };

  const runAfterClearingGroupFilter = (callback) => {
    const selectedGroup = formApi?.getValue('searchGroup');
    if (!selectedGroup) {
      callback();
      return;
    }
    formApi.setValue('searchGroup', null);
    setTimeout(callback, 0);
  };

  const handleCategoryChange = (categoryKey) => {
    setActiveCategoryKey(categoryKey);
    setActivePage(1);
    runAfterClearingGroupFilter(() =>
      loadChannels(1, pageSize, idSort, enableTagMode, categoryKey),
    );
  };

  const handleTagModeChange = (enabled) => {
    localStorage.setItem('enable-tag-mode', enabled + '');
    setEnableTagMode(enabled);
    setActivePage(1);
    runAfterClearingGroupFilter(() =>
      loadChannels(1, pageSize, idSort, enabled, activeCategoryKey),
    );
  };

  // Refresh
  const refresh = async (page = activePage) => {
    if (typeof page !== 'number') page = activePage;
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    let channelsRequest;
    if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
      channelsRequest = loadChannels(page, pageSize, idSort, enableTagMode);
    } else {
      channelsRequest = searchChannels(
        enableTagMode,
        activeCategoryKey,
        statusFilter,
        page,
        pageSize,
        idSort,
      );
    }
    await Promise.all([channelsRequest, fetchChannelConcurrencyStats()]);
  };

  // Channel management
  const manageChannel = async (id, action, record, value) => {
    let data = { id };
    let res;
    switch (action) {
      case 'delete':
        res = await API.delete(`/api/channel/${id}/`);
        break;
      case 'enable':
        data.status = 1;
        res = await API.put('/api/channel/', data);
        break;
      case 'disable':
        data.status = 2;
        res = await API.put('/api/channel/', data);
        break;
      case 'priority':
        if (value === '') return;
        data.priority = parseInt(value);
        res = await API.put('/api/channel/', data);
        break;
      case 'weight':
        if (value === '') return;
        data.weight = parseInt(value);
        if (data.weight < 0) data.weight = 0;
        res = await API.put('/api/channel/', data);
        break;
      case 'enable_all':
        data.channel_info = record.channel_info;
        data.channel_info.multi_key_status_list = {};
        res = await API.put('/api/channel/', data);
        break;
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('操作成功完成！'));
      if (action === 'enable' || action === 'enable_all') {
        await refresh();
        return;
      }
      let channel = res.data.data;
      let newChannels = [...channels];
      if (action !== 'delete') {
        record.status = channel.status;
      }
      setChannels(newChannels);
    } else {
      showError(message);
    }
  };

  // Tag management
  const manageTag = async (tag, action) => {
    let res;
    switch (action) {
      case 'enable':
        res = await API.post('/api/channel/tag/enabled', { tag: tag });
        break;
      case 'disable':
        res = await API.post('/api/channel/tag/disabled', { tag: tag });
        break;
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('操作成功完成！'));
      if (action === 'enable') {
        await refresh();
        return;
      }
      let newChannels = [...channels];
      for (let i = 0; i < newChannels.length; i++) {
        if (newChannels[i].tag === tag) {
          let status = action === 'enable' ? 1 : 2;
          newChannels[i]?.children?.forEach((channel) => {
            channel.status = status;
          });
          newChannels[i].status = status;
        }
      }
      setChannels(newChannels);
    } else {
      showError(message);
    }
  };

  // 分页处理函数
  const handlePageChange = (page) => {
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    setActivePage(page);
    if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
      loadChannels(page, pageSize, idSort, enableTagMode).then(() => {});
    } else {
      searchChannels(
        enableTagMode,
        activeCategoryKey,
        statusFilter,
        page,
        pageSize,
        idSort,
      );
    }
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    const { searchKeyword, searchGroup, searchModel } = getFormValues();
    if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
      loadChannels(1, size, idSort, enableTagMode)
        .then()
        .catch((reason) => {
          showError(reason);
        });
    } else {
      searchChannels(
        enableTagMode,
        activeCategoryKey,
        statusFilter,
        1,
        size,
        idSort,
      );
    }
  };

  // Fetch groups
  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      if (res === undefined) return;
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

  // Copy channel
  const copySelectedChannel = async (record) => {
    try {
      const res = await API.post(`/api/channel/copy/${record.id}`);
      if (res?.data?.success) {
        showSuccess(t('渠道复制成功'));
        await refresh();
      } else {
        showError(res?.data?.message || t('渠道复制失败'));
      }
    } catch (error) {
      showError(
        t('渠道复制失败: ') +
          (error?.response?.data?.message || error?.message || error),
      );
    }
  };

  // Update channel property
  const updateChannelProperty = (channelId, updateFn) => {
    const newChannels = [...channels];
    let updated = false;

    newChannels.forEach((channel) => {
      if (channel.children !== undefined) {
        channel.children.forEach((child) => {
          if (child.id === channelId) {
            updateFn(child);
            updated = true;
          }
        });
      } else if (channel.id === channelId) {
        updateFn(channel);
        updated = true;
      }
    });

    if (updated) {
      setChannels(newChannels);
    }
  };

  // Tag edit
  const submitTagEdit = async (type, data) => {
    switch (type) {
      case 'priority':
        if (data.priority === undefined || data.priority === '') {
          showInfo('优先级必须是整数！');
          return;
        }
        data.priority = parseInt(data.priority);
        break;
      case 'weight':
        if (
          data.weight === undefined ||
          data.weight < 0 ||
          data.weight === ''
        ) {
          showInfo('权重必须是非负整数！');
          return;
        }
        data.weight = parseInt(data.weight);
        break;
    }

    try {
      const res = await API.put('/api/channel/tag', data);
      if (res?.data?.success) {
        showSuccess('更新成功！');
        await refresh();
      }
    } catch (error) {
      showError(error);
    }
  };

  // Close edit
  const closeEdit = () => {
    setShowEdit(false);
  };

  // Row style
  const handleRow = (record, index) => {
    if (record.status !== 1) {
      return {
        style: {
          background: 'var(--semi-color-disabled-border)',
        },
      };
    } else {
      return {};
    }
  };

  // Batch operations
  const batchSetChannelTag = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要设置标签的渠道！'));
      return;
    }
    if (batchSetTagValue === '') {
      showError(t('标签不能为空！'));
      return;
    }
    let ids = selectedChannels.map((channel) => channel.id);
    const res = await API.post('/api/channel/batch/tag', {
      ids: ids,
      tag: batchSetTagValue === '' ? null : batchSetTagValue,
    });
    if (res.data.success) {
      showSuccess(
        t('已为 ${count} 个渠道设置标签！').replace('${count}', res.data.data),
      );
      await refresh();
      setShowBatchSetTag(false);
    } else {
      showError(res.data.message);
    }
  };

  const batchDeleteChannels = async () => {
    if (selectedChannels.length === 0) {
      showError(t('请先选择要删除的通道！'));
      return;
    }
    setLoading(true);
    let ids = [];
    selectedChannels.forEach((channel) => {
      ids.push(channel.id);
    });
    const res = await API.post(`/api/channel/batch`, { ids: ids });
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(t('已删除 ${data} 个通道！').replace('${data}', data));
      await refresh();
      setTimeout(() => {
        if (channels.length === 0 && activePage > 1) {
          refresh(activePage - 1);
        }
      }, 100);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const tagTestModelOptions = useMemo(
    () =>
      collectTagTestModels(tagTestChannels).map((model) => ({
        label: model,
        value: model,
      })),
    [tagTestChannels],
  );

  const closeTagTestModal = () => {
    setShowTagTestModal(false);
    setCurrentTagTestGroup(null);
    setTagTestMode('default');
    setTagTestScope('all');
    setTagTestLoading(false);
    setTagTestSubmitting(false);
    setTagTestChannels([]);
    setSelectedTagTestChannelIds([]);
    setSelectedTagTestModel('');
  };

  const openTagTestModal = async (record, mode = 'default') => {
    const tag = record?.tag || record?.key;
    if (!tag) {
      showError(t('标签不能为空'));
      return;
    }

    setCurrentTagTestGroup(record);
    setTagTestMode(mode);
    setTagTestScope('all');
    setSelectedTagTestChannelIds([]);
    setSelectedTagTestModel('');
    setTagTestChannels([]);
    setTagTestLoading(true);
    setShowTagTestModal(true);

    try {
      const res = await API.get(
        `/api/channel/tag/channels?tag=${encodeURIComponent(tag)}`,
      );
      const { success, message, data } = res?.data || {};
      if (!success) {
        showError(message || t('获取标签渠道失败'));
        closeTagTestModal();
        return;
      }
      const channelList = Array.isArray(data)
        ? data.map((channel) => decorateChannelRecord(channel))
        : [];
      setTagTestChannels(channelList);
    } catch (error) {
      showError(
        error?.response?.data?.message ||
          error?.message ||
          t('获取标签渠道失败'),
      );
      closeTagTestModal();
    } finally {
      setTagTestLoading(false);
    }
  };

  const manualEnableChannelsByIds = async (channelIds = []) => {
    let enabledCount = 0;

    for (const channelId of channelIds) {
      try {
        const res = await API.post(`/api/channel/${channelId}/manual_enable`);
        const { success, message } = res?.data || {};
        if (success) {
          enabledCount++;
        } else {
          showError(message || t('开启渠道失败'));
        }
      } catch (error) {
        showError(
          error?.response?.data?.message || error?.message || t('开启渠道失败'),
        );
      }
    }

    if (enabledCount > 0) {
      showSuccess(
        t('已开启 ${count} 个渠道').replace('${count}', enabledCount),
      );
      await refresh();
    }

    return enabledCount;
  };

  const promptEnableChannelsAfterSuccess = async (channelsToEnable = []) => {
    if (channelsToEnable.length === 0) {
      return false;
    }

    if (channelsToEnable.length === 1) {
      const channel = channelsToEnable[0];
      return new Promise((resolve) => {
        Modal.confirm({
          title: t('测试成功，是否开启该渠道？'),
          content: channel.name || `#${channel.id}`,
          centered: true,
          onOk: async () => {
            const enabledCount = await manualEnableChannelsByIds([channel.id]);
            resolve(enabledCount > 0);
          },
          onCancel: () => resolve(false),
        });
      });
    }

    let selectedIds = channelsToEnable.map((channel) => String(channel.id));
    return new Promise((resolve) => {
      Modal.confirm({
        title: t('以下渠道测试成功，是否开启？'),
        centered: true,
        content: (
          <Checkbox.Group
            defaultValue={selectedIds}
            onChange={(values) => {
              selectedIds = values.map((value) => String(value));
            }}
          >
            <div
              className='flex flex-col gap-2 overflow-y-auto'
              style={{ maxHeight: 280 }}
            >
              {channelsToEnable.map((channel) => (
                <Checkbox key={channel.id} value={String(channel.id)}>
                  {channel.name || `#${channel.id}`}
                </Checkbox>
              ))}
            </div>
          </Checkbox.Group>
        ),
        onOk: async () => {
          const enabledCount = await manualEnableChannelsByIds(selectedIds);
          resolve(enabledCount > 0);
        },
        onCancel: () => resolve(false),
      });
    });
  };

  // Channel operations
  const testAllChannels = async () => {
    const res = await API.get(`/api/channel/test`);
    const { success, message } = res.data;
    if (success) {
      showInfo(t('已成功开始测试所有已启用通道，请刷新页面查看结果。'));
    } else {
      showError(message);
    }
  };

  const deleteAllDisabledChannels = async () => {
    const res = await API.delete(`/api/channel/disabled`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(
        t('已删除所有禁用渠道，共计 ${data} 个').replace('${data}', data),
      );
      await refresh();
    } else {
      showError(message);
    }
  };

  const updateAllChannelsBalance = async () => {
    const res = await API.get(`/api/channel/update_balance`);
    const { success, message } = res.data;
    if (success) {
      showInfo(t('已更新完毕所有已启用通道余额！'));
    } else {
      showError(message);
    }
  };

  const updateChannelBalance = async (record) => {
    if (record?.type === 57) {
      openCodexUsageModal({
        t,
        record,
        onCopy: async (text) => {
          const ok = await copy(text);
          if (ok) showSuccess(t('已复制'));
          else showError(t('复制失败'));
        },
      });
      return;
    }

    const res = await API.get(`/api/channel/update_balance/${record.id}/`);
    const { success, message, balance } = res.data;
    if (success) {
      updateChannelProperty(record.id, (channel) => {
        channel.balance = balance;
        channel.balance_updated_time = Date.now() / 1000;
      });
      showInfo(
        t('通道 ${name} 余额更新成功！').replace('${name}', record.name),
      );
    } else {
      showError(message);
    }
  };

  const fixChannelsAbilities = async () => {
    const res = await API.post(`/api/channel/fix`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(
        t('已修复 ${success} 个通道，失败 ${fails} 个通道。')
          .replace('${success}', data.success)
          .replace('${fails}', data.fails),
      );
      await refresh();
    } else {
      showError(message);
    }
  };

  const checkOllamaVersion = async (record) => {
    try {
      const res = await API.get(`/api/channel/ollama/version/${record.id}`);
      const { success, message, data } = res.data;

      if (success) {
        const version = data?.version || '-';
        const infoMessage = t('当前 Ollama 版本为 ${version}').replace(
          '${version}',
          version,
        );

        const handleCopyVersion = async () => {
          if (!version || version === '-') {
            showInfo(t('暂无可复制的版本信息'));
            return;
          }

          const copied = await copy(version);
          if (copied) {
            showSuccess(t('已复制版本号'));
          } else {
            showError(t('复制失败，请手动复制'));
          }
        };

        Modal.info({
          title: t('Ollama 版本信息'),
          content: infoMessage,
          centered: true,
          footer: (
            <div className='flex justify-end gap-2'>
              <Button type='tertiary' onClick={handleCopyVersion}>
                {t('复制版本号')}
              </Button>
              <Button
                type='primary'
                theme='solid'
                onClick={() => Modal.destroyAll()}
              >
                {t('关闭')}
              </Button>
            </div>
          ),
          hasCancel: false,
          hasOk: false,
          closable: true,
          maskClosable: true,
        });
      } else {
        showError(message || t('获取 Ollama 版本失败'));
      }
    } catch (error) {
      const errMsg =
        error?.response?.data?.message ||
        error?.message ||
        t('获取 Ollama 版本失败');
      showError(errMsg);
    }
  };

  const performChannelTest = async (
    record,
    model,
    endpointType = '',
    stream = false,
    options = {},
  ) => {
    const {
      silent = false,
      trackModelState = true,
      promptEnableOnSuccess = false,
    } = options;
    const normalizedModel = model || '';
    const testKey = `${record.id}-${normalizedModel}`;

    if (shouldStopBatchTestingRef.current && isBatchTesting) {
      return Promise.resolve({
        success: false,
        skipped: true,
        channel: record,
      });
    }

    if (trackModelState) {
      setTestingModels((prev) => new Set([...prev, normalizedModel]));
    }

    try {
      let url = `/api/channel/test/${record.id}?model=${encodeURIComponent(normalizedModel)}`;
      if (endpointType) {
        url += `&endpoint_type=${encodeURIComponent(endpointType)}`;
      }
      if (stream) {
        url += `&stream=true`;
      }
      const res = await API.get(url);

      if (shouldStopBatchTestingRef.current && isBatchTesting) {
        return Promise.resolve({
          success: false,
          skipped: true,
          channel: record,
        });
      }

      const {
        success: liveSuccess,
        message,
        time,
        error_code,
        test_kind,
        data,
      } = res.data;
      const success =
        test_kind === 'video_preflight' ? data?.passed === true : liveSuccess;
      const result = {
        success,
        message,
        time: time || 0,
        errorCode: error_code || null,
        testKind: test_kind,
        preflight: data,
        channel: record,
        model: normalizedModel,
      };

      if (trackModelState) {
        setModelTestResults((prev) => ({
          ...prev,
          [testKey]: {
            success,
            message,
            time: time || 0,
            timestamp: Date.now(),
            errorCode: error_code || null,
            testKind: test_kind,
            preflight: data,
          },
        }));
      }

      if (test_kind === 'video_preflight') {
        if (!silent)
          showInfo(
            t('视频仅进行配置预检，未生成视频，也不会改变渠道健康状态。'),
          );
      } else if (success) {
        updateChannelProperty(record.id, (channel) => {
          channel.response_time = time * 1000;
          channel.test_time = Date.now() / 1000;
        });

        if (!silent) {
          if (!normalizedModel) {
            showInfo(
              t('通道 ${name} 测试成功，耗时 ${time.toFixed(2)} 秒。')
                .replace('${name}', record.name)
                .replace('${time.toFixed(2)}', time.toFixed(2)),
            );
          } else {
            showInfo(
              t(
                '通道 ${name} 测试成功，模型 ${model} 耗时 ${time.toFixed(2)} 秒。',
              )
                .replace('${name}', record.name)
                .replace('${model}', normalizedModel)
                .replace('${time.toFixed(2)}', time.toFixed(2)),
            );
          }
        }

        if (
          promptEnableOnSuccess &&
          shouldPromptEnableChannelAfterManualTest(record)
        ) {
          await promptEnableChannelsAfterSuccess([record]);
        }
      } else if (!silent) {
        showError(message);
      }

      return result;
    } catch (error) {
      const failedResult = {
        success: false,
        message: error.message || t('网络错误'),
        time: 0,
        errorCode: null,
        channel: record,
        model: normalizedModel,
      };

      if (trackModelState) {
        setModelTestResults((prev) => ({
          ...prev,
          [testKey]: {
            success: false,
            message: failedResult.message,
            time: 0,
            timestamp: Date.now(),
            errorCode: null,
          },
        }));
      }

      if (!silent) {
        showError(error.message || t('测试失败'));
      }

      return failedResult;
    } finally {
      if (trackModelState) {
        setTestingModels((prev) => {
          const newSet = new Set(prev);
          newSet.delete(normalizedModel);
          return newSet;
        });
      }
    }
  };

  // Test channel - 单个模型测试，参考旧版实现
  const testChannel = async (
    record,
    model,
    endpointType = '',
    stream = false,
    options = {},
  ) => performChannelTest(record, model, endpointType, stream, options);

  const runTagChannelsTest = async () => {
    const targets = resolveTagTestTargets(
      tagTestChannels,
      tagTestScope,
      selectedTagTestChannelIds,
    );

    if (targets.length === 0) {
      showError(t('请先选择要测试的渠道'));
      return;
    }

    if (tagTestMode === 'model' && !selectedTagTestModel) {
      showError(t('请选择要测试的模型'));
      return;
    }

    setTagTestSubmitting(true);
    try {
      const results = [];
      for (const channel of targets) {
        const result = await performChannelTest(
          channel,
          tagTestMode === 'model' ? selectedTagTestModel : '',
          '',
          false,
          {
            silent: true,
            trackModelState: false,
            promptEnableOnSuccess: false,
          },
        );
        results.push(result);
      }

      const successChannels = results
        .filter(isLiveTestSuccess)
        .map((result) => result.channel);
      const preflightCount = results.filter(
        (result) => result.testKind === 'video_preflight',
      ).length;
      const summary = buildTagTestSummary(
        currentTagTestGroup?.tag ||
          currentTagTestGroup?.key ||
          currentTagTestGroup?.name,
        successChannels.length,
        targets.length - preflightCount,
      );

      if (preflightCount > 0)
        showInfo(
          t('视频配置预检 {{count}} 项，不计入生成成功或失败。', {
            count: preflightCount,
          }),
        );
      if (preflightCount === targets.length) {
        closeTagTestModal();
        return;
      }
      if (summary.tone === 'success') {
        showSuccess(summary.message);
      } else if (summary.tone === 'error') {
        showError(summary.message);
      } else {
        showInfo(summary.message);
      }

      closeTagTestModal();
      if (successChannels.length > 0) {
        await refresh();
      }

      const channelsToEnable = successChannels.filter((channel) =>
        shouldPromptEnableChannelAfterManualTest(channel),
      );
      if (channelsToEnable.length > 0) {
        await promptEnableChannelsAfterSuccess(channelsToEnable);
      }
    } finally {
      setTagTestSubmitting(false);
    }
  };

  // 批量测试单个渠道的所有模型，参考旧版实现
  const batchTestModels = async () => {
    if (!currentTestChannel || !currentTestChannel.models) {
      showError(t('渠道模型信息不完整'));
      return;
    }

    const models = currentTestChannel.models
      .split(',')
      .filter((model) =>
        model.toLowerCase().includes(modelSearchKeyword.toLowerCase()),
      );

    if (models.length === 0) {
      showError(t('没有找到匹配的模型'));
      return;
    }

    setIsBatchTesting(true);
    shouldStopBatchTestingRef.current = false; // 重置停止标志

    // 清空该渠道之前的测试结果
    setModelTestResults((prev) => {
      const newResults = { ...prev };
      models.forEach((model) => {
        const testKey = `${currentTestChannel.id}-${model}`;
        delete newResults[testKey];
      });
      return newResults;
    });

    try {
      showInfo(
        t('开始批量测试 ${count} 个模型，已清空上次结果...').replace(
          '${count}',
          models.length,
        ),
      );

      // 提高并发数量以加快测试速度，参考旧版的并发限制
      const concurrencyLimit = 5;
      const results = [];

      for (let i = 0; i < models.length; i += concurrencyLimit) {
        // 检查是否应该停止
        if (shouldStopBatchTestingRef.current) {
          showInfo(t('批量测试已停止'));
          break;
        }

        const batch = models.slice(i, i + concurrencyLimit);
        showInfo(
          t('正在测试第 ${current} - ${end} 个模型 (共 ${total} 个)')
            .replace('${current}', i + 1)
            .replace('${end}', Math.min(i + concurrencyLimit, models.length))
            .replace('${total}', models.length),
        );

        const batchPromises = batch.map((model) =>
          testChannel(
            currentTestChannel,
            model,
            selectedEndpointType,
            isStreamTest,
          ),
        );
        const batchResults = await Promise.allSettled(batchPromises);
        results.push(...batchResults);

        // 再次检查是否应该停止
        if (shouldStopBatchTestingRef.current) {
          showInfo(t('批量测试已停止'));
          break;
        }

        // 短暂延迟避免过于频繁的请求
        if (i + concurrencyLimit < models.length) {
          await new Promise((resolve) => setTimeout(resolve, 100));
        }
      }

      if (!shouldStopBatchTestingRef.current) {
        // 等待一小段时间确保所有结果都已更新
        await new Promise((resolve) => setTimeout(resolve, 300));

        // 使用当前状态重新计算结果统计
        setModelTestResults((currentResults) => {
          const counts = summarizeModelTests(
            models.map(
              (model) => currentResults[`${currentTestChannel.id}-${model}`],
            ),
          );
          setTimeout(() => {
            showInfo(
              t(
                '批量检查完成：真实测试成功 {{success}}，失败 {{failed}}，视频配置预检 {{preflight}}。',
                counts,
              ),
            );
          }, 100);

          return currentResults; // 不修改状态，只是为了获取最新值
        });
      }
    } catch (error) {
      showError(t('批量测试过程中发生错误: ') + error.message);
    } finally {
      setIsBatchTesting(false);
    }
  };

  // 停止批量测试
  const stopBatchTesting = () => {
    shouldStopBatchTestingRef.current = true;
    setIsBatchTesting(false);
    setTestingModels(new Set());
    showInfo(t('已停止批量测试'));
  };

  // 清空测试结果
  const clearTestResults = () => {
    setModelTestResults({});
    showInfo(t('已清空测试结果'));
  };

  // Handle close modal
  const handleCloseModal = () => {
    // 如果正在批量测试，先停止测试
    if (isBatchTesting) {
      shouldStopBatchTestingRef.current = true;
      showInfo(t('关闭弹窗，已停止批量测试'));
    }

    setShowModelTestModal(false);
    setModelSearchKeyword('');
    setIsBatchTesting(false);
    setTestingModels(new Set());
    setSelectedModelKeys([]);
    setModelTablePage(1);
    setSelectedEndpointType('');
    setIsStreamTest(false);
    // 可选择性保留测试结果，这里不清空以便用户查看
  };

  const upstreamUpdates = useChannelUpstreamUpdates({
    t,
    refresh,
  });

  const availableCategoryKeys = useMemo(() => {
    const keys = [CHANNEL_CATEGORY_ALL];
    Object.entries(categoryCounts).forEach(([k, v]) => {
      if (k !== CHANNEL_CATEGORY_ALL && v > 0) keys.push(String(k));
    });
    return keys;
  }, [categoryCounts]);

  return {
    usageOptions,
    changeUsageOptions,
    usageMeta,
    usageError,
    // 基础状态s
    channels,
    loading,
    searching,
    activePage,
    pageSize,
    channelCount,
    groupOptions,
    idSort,
    enableTagMode,
    enableBatchDelete,
    statusFilter,
    compactMode,
    globalPassThroughEnabled,
    channelConcurrencySnapshot,

    // UI 状态s
    showEdit,
    setShowEdit,
    editingChannel,
    setEditingChannel,
    showEditTag,
    setShowEditTag,
    editingTag,
    setEditingTag,
    selectedChannels,
    setSelectedChannels,
    showBatchSetTag,
    setShowBatchSetTag,
    batchSetTagValue,
    setBatchSetTagValue,

    // Column states
    visibleColumns,
    showColumnSelector,
    setShowColumnSelector,
    COLUMN_KEYS,

    // Display category states
    activeCategoryKey,
    handleCategoryChange,
    categoryCounts,
    availableCategoryKeys,

    // Model test states
    showModelTestModal,
    setShowModelTestModal,
    currentTestChannel,
    setCurrentTestChannel,
    modelSearchKeyword,
    setModelSearchKeyword,
    modelTestResults,
    setModelTestResults,
    testingModels,
    selectedModelKeys,
    setSelectedModelKeys,
    isBatchTesting,
    modelTablePage,
    setModelTablePage,
    selectedEndpointType,
    setSelectedEndpointType,
    isStreamTest,
    setIsStreamTest,
    allSelectingRef,
    showTagTestModal,
    currentTagTestGroup,
    tagTestMode,
    tagTestScope,
    setTagTestScope,
    tagTestLoading,
    tagTestSubmitting,
    tagTestChannels,
    selectedTagTestChannelIds,
    setSelectedTagTestChannelIds,
    tagTestModelOptions,
    selectedTagTestModel,
    setSelectedTagTestModel,

    // Multi-key management states
    showMultiKeyManageModal,
    setShowMultiKeyManageModal,
    currentMultiKeyChannel,
    setCurrentMultiKeyChannel,

    // Form
    formApi,
    setFormApi,
    formInitValues,

    // Helpers
    t,
    isMobile,

    // Upstream updates
    ...upstreamUpdates,

    // 函数集合
    loadChannels,
    searchChannels,
    refresh,
    manageChannel,
    manageTag,
    handlePageChange,
    handlePageSizeChange,
    copySelectedChannel,
    updateChannelProperty,
    submitTagEdit,
    closeEdit,
    handleRow,
    batchSetChannelTag,
    batchDeleteChannels,
    testAllChannels,
    deleteAllDisabledChannels,
    updateAllChannelsBalance,
    updateChannelBalance,
    fixChannelsAbilities,
    checkOllamaVersion,
    testChannel,
    openTagTestModal,
    closeTagTestModal,
    runTagChannelsTest,
    batchTestModels,
    handleCloseModal,
    getFormValues,

    // Column functions
    handleColumnVisibilityChange,
    handleSelectAll,
    initDefaultColumns,
    getDefaultColumnVisibility,

    // Setters
    setIdSort,
    handleTagModeChange,
    setEnableBatchDelete,
    setStatusFilter,
    setCompactMode,
    setActivePage,
  };
};
