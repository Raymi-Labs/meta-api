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

import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@douyinfe/semi-ui';
import {
  API,
  copy,
  showError,
  showSuccess,
  encodeToBase64,
} from '../../helpers';
import { useTableCompactMode } from '../common/useTableCompactMode';
import { usePaginatedList } from '../common/usePaginatedList';

export const useTokensData = (openFluentNotification) => {
  const { t } = useTranslation();

  // 分页/加载/搜索/选择/竞态控制（共用基础状态）
  const {
    loading,
    setLoading,
    activePage,
    setActivePage,
    pageSize,
    setPageSize,
    searching,
    setSearching,
    selectedKeys,
    setSelectedKeys,
    nextRequestId,
    isLatestRequest,
  } = usePaginatedList();

  // 基础状态
  const [tokens, setTokens] = useState([]);

  const [tokenCount, setTokenCount] = useState(0);
  const [listError, setListError] = useState(false);
  const [searchMode, setSearchMode] = useState(false); // whether current list is search-result view
  const [groupOptions, setGroupOptions] = useState([]);

  // 选择状态（由 usePaginatedList 提供 selectedKeys/setSelectedKeys）

  // 编辑状态
  const [showEdit, setShowEdit] = useState(false);
  const [editingToken, setEditingToken] = useState({
    id: undefined,
  });
  const clearEditingTokenTimerRef = useRef(null);
  const aliveRef = useRef(false);
  const mutationBusyRef = useRef(false);
  const lastListRequestRef = useRef({ mode: 'list', page: 1, size: pageSize });
  // requestCounter 由 usePaginatedList 提供，通过 nextRequestId / isLatestRequest 使用

  // UI 状态
  const [compactMode, setCompactMode] = useTableCompactMode('tokens');
  const [showKeys, setShowKeys] = useState({});
  const [tokenFullKeys, setTokenFullKeys] = useState({});
  const [ccSwitchToken, setCCSwitchToken] = useState(null);

  // 表单状态
  const [formApi, setFormApi] = useState(null);
  const formInitValues = {
    searchKeyword: '',
    searchToken: '',
    searchGroup: '',
    searchPackageMode: '',
    searchBalanceMin: '',
    searchBalanceMax: '',
    searchUsedBalanceMin: '',
    searchUsedBalanceMax: '',
    searchAmountSort: '',
  };

  const normalizeTokenSortKey = (sortKey) => {
    if (sortKey === 'quota_asc' || sortKey === 'quota_desc') {
      return sortKey;
    }
    return '';
  };

  const resolveTokenSort = (sortKey) => {
    // Token 管理仅开放额度排序，隐藏 ID 排序选项。
    const normalized = normalizeTokenSortKey(sortKey);
    if (normalized === 'quota_asc') {
      return { sort_by: 'remain_quota', sort_order: 'asc' };
    }
    if (normalized === 'quota_desc') {
      return { sort_by: 'remain_quota', sort_order: 'desc' };
    }
    return {};
  };

  // 获取表单值的辅助函数
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      searchKeyword: formValues.searchKeyword || '',
      searchToken: formValues.searchToken || '',
      searchGroup: formValues.searchGroup || '',
      searchPackageMode: formValues.searchPackageMode || '',
      searchBalanceMin:
        formValues.searchBalanceMin === null ||
        formValues.searchBalanceMin === undefined ||
        formValues.searchBalanceMin === ''
          ? ''
          : formValues.searchBalanceMin,
      searchBalanceMax:
        formValues.searchBalanceMax === null ||
        formValues.searchBalanceMax === undefined ||
        formValues.searchBalanceMax === ''
          ? ''
          : formValues.searchBalanceMax,
      searchUsedBalanceMin:
        formValues.searchUsedBalanceMin === null ||
        formValues.searchUsedBalanceMin === undefined ||
        formValues.searchUsedBalanceMin === ''
          ? ''
          : formValues.searchUsedBalanceMin,
      searchUsedBalanceMax:
        formValues.searchUsedBalanceMax === null ||
        formValues.searchUsedBalanceMax === undefined ||
        formValues.searchUsedBalanceMax === ''
          ? ''
          : formValues.searchUsedBalanceMax,
      searchAmountSort: normalizeTokenSortKey(formValues.searchAmountSort),
    };
  };

  const hasTokenFilters = (values) => {
    // searchMode 依赖此判断，保证分页/刷新时保留生效筛选。
    return (
      values.searchKeyword !== '' ||
      values.searchToken !== '' ||
      values.searchGroup !== '' ||
      values.searchPackageMode !== '' ||
      values.searchBalanceMin !== '' ||
      values.searchBalanceMax !== '' ||
      values.searchUsedBalanceMin !== '' ||
      values.searchUsedBalanceMax !== '' ||
      values.searchAmountSort !== ''
    );
  };

  // 关闭编辑弹窗
  const closeEdit = () => {
    setShowEdit(false);
    if (clearEditingTokenTimerRef.current) {
      clearTimeout(clearEditingTokenTimerRef.current);
    }
    // 延迟到关闭动画结束后再清理编辑数据，
    // 避免 SideSheet 在退出过程中发生位置跳变。
    clearEditingTokenTimerRef.current = setTimeout(() => {
      setEditingToken({
        id: undefined,
      });
      clearEditingTokenTimerRef.current = null;
    }, 500);
  };

  useEffect(() => {
    if (!showEdit || !clearEditingTokenTimerRef.current) {
      return;
    }
    clearTimeout(clearEditingTokenTimerRef.current);
    clearEditingTokenTimerRef.current = null;
  }, [showEdit]);

  // 从 API 响应同步分页数据
  const syncPageData = (payload) => {
    setTokens(payload.items || []);
    setTokenCount(payload.total || 0);
    setActivePage(payload.page || 1);
    setPageSize(payload.page_size || pageSize);
  };

  // 加载 Token 列表
  const loadTokens = async (page = 1, size = pageSize) => {
    const reqId = nextRequestId();
    setLoading(true);
    setSearchMode(false);
    lastListRequestRef.current = { mode: 'list', page, size };
    setSelectedKeys([]);
    setShowKeys({});
    setListError(false);
    setSearching(false);
    try {
      const res = await API.get('/api/token/', {
        params: { p: page, size },
        disableDuplicate: true,
      });
      if (!isLatestRequest(reqId)) return;
      const { success, data } = res.data || {};
      if (!success || !Array.isArray(data?.items))
        throw new Error('Invalid token list');
      syncPageData(data);
    } catch {
      if (isLatestRequest(reqId)) setListError(true);
    } finally {
      if (isLatestRequest(reqId)) setLoading(false);
    }
  };

  // 刷新函数
  const refresh = async (page = activePage) => {
    if (searchMode) {
      await searchTokens(page, pageSize, lastListRequestRef.current.filters);
    } else {
      await loadTokens(page);
    }
  };

  // 复制文本函数
  const copyText = async (text) => {
    if (await copy(text)) {
      if (!aliveRef.current) return false;
      showSuccess(t('已复制到剪贴板！'));
      return true;
    } else {
      Modal.error({
        title: t('无法复制到剪贴板，请手动复制'),
        content: (
          <pre
            tabIndex={0}
            style={{
              maxHeight: '40vh',
              overflow: 'auto',
              whiteSpace: 'pre-wrap',
              overflowWrap: 'anywhere',
            }}
          >
            {text}
          </pre>
        ),
        size: 'large',
      });
      return false;
    }
  };

  const normalizeTokenKeyForDisplay = (key) => {
    if (!key) return '';
    return key.startsWith('sk-') ? key : `sk-${key}`;
  };

  const getTokenFullKey = async (record) => {
    if (!record?.id) return '';
    const cachedKey = tokenFullKeys[record.id];
    if (cachedKey) return cachedKey;

    try {
      // List API returns masked keys; fetch the full key only when needed.
      const res = await API.get(`/api/token/${record.id}/key`);
      if (!aliveRef.current) return '';
      const { success, message, data } = res.data || {};
      if (!success || !data?.key) {
        showError(message || t('获取令牌失败'));
        return '';
      }
      setTokenFullKeys((prev) => ({ ...prev, [record.id]: data.key }));
      return data.key;
    } catch (error) {
      showError(error?.message || t('获取令牌失败'));
      return '';
    }
  };

  const copyTokenKey = async (record) => {
    const key = await getTokenFullKey(record);
    if (!key) return;
    await copyText(normalizeTokenKeyForDisplay(key));
  };

  const toggleTokenKeyVisibility = async (record) => {
    const revealed = !!showKeys[record.id];
    if (revealed) {
      setShowKeys((prev) => ({ ...prev, [record.id]: false }));
      return;
    }
    const key = await getTokenFullKey(record);
    if (!key) return;
    setShowKeys((prev) => ({ ...prev, [record.id]: true }));
  };

  const getChatServerAddress = (serverAddress) =>
    serverAddress.replace(/\/+$/, '');

  // 打开聊天集成链接函数
  const onOpenLink = async (type, url, record) => {
    if (!url) return;

    if (url.startsWith('ccswitch')) {
      setCCSwitchToken(record);
      return;
    }

    const needsKey =
      url.startsWith('fluent') ||
      url.includes('{key}') ||
      url.includes('{cherryConfig}') ||
      url.includes('{aionuiConfig}') ||
      url.includes('{deepchatConfig}');
    let apiKey = '';
    let rawKey = '';
    if (needsKey) {
      rawKey = await getTokenFullKey(record);
      if (!rawKey) return;
      apiKey = normalizeTokenKeyForDisplay(rawKey);
    }

    if (url.startsWith('fluent')) {
      openFluentNotification(rawKey);
      return;
    }
    let status = localStorage.getItem('status');
    let serverAddress = '';
    if (status) {
      try {
        status = JSON.parse(status);
        serverAddress = status.server_address || '';
      } catch (_) {
        serverAddress = '';
      }
    }
    if (serverAddress === '') {
      serverAddress = window.location.origin;
    }
    serverAddress = getChatServerAddress(serverAddress);
    if (url.includes('{cherryConfig}') === true) {
      let cherryConfig = {
        id: 'new-api',
        baseUrl: serverAddress,
        apiKey,
      };
      let encodedConfig = encodeURIComponent(
        encodeToBase64(JSON.stringify(cherryConfig)),
      );
      url = url.replaceAll('{cherryConfig}', encodedConfig);
    } else if (url.includes('{aionuiConfig}') === true) {
      let aionuiConfig = {
        platform: 'new-api',
        baseUrl: serverAddress,
        apiKey,
      };
      let encodedConfig = encodeURIComponent(
        encodeToBase64(JSON.stringify(aionuiConfig)),
      );
      url = url.replaceAll('{aionuiConfig}', encodedConfig);
    } else if (url.includes('{deepchatConfig}') === true) {
      let deepchatConfig = {
        id: 'new-api',
        baseUrl: serverAddress,
        apiKey,
      };
      let encodedConfig = encodeURIComponent(
        encodeToBase64(JSON.stringify(deepchatConfig)),
      );
      url = url.replaceAll('{deepchatConfig}', encodedConfig);
    } else {
      let encodedServerAddress = encodeURIComponent(serverAddress);
      url = url.replaceAll('{address}', encodedServerAddress);
      url = url.replaceAll('{key}', apiKey);
    }

    window.open(url, '_blank');
  };

  // Token 管理函数（delete/enable/disable）
  const manageToken = async (id, action, record) => {
    if (mutationBusyRef.current) return;
    mutationBusyRef.current = true;
    setLoading(true);
    try {
      let res;
      if (action === 'delete') res = await API.delete(`/api/token/${id}/`);
      else if (action === 'enable' || action === 'disable')
        res = await API.put('/api/token/?status_only=true', {
          id,
          status: action === 'enable' ? 1 : 2,
        });
      else return;
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      showSuccess(t('操作成功完成！'));
      setTokens((current) =>
        action === 'delete'
          ? current.filter((item) => item.id !== id)
          : current.map((item) =>
              item.id === id ? { ...item, status: data.status } : item,
            ),
      );
    } catch {
      showError(t('操作失败'));
    } finally {
      mutationBusyRef.current = false;
      if (aliveRef.current) setLoading(false);
    }
  };

  // 搜索 Token 函数
  const searchTokens = async (page = 1, size = pageSize, appliedFilters) => {
    const normalizedPage = Number.isInteger(page) && page > 0 ? page : 1;
    const normalizedSize = Number.isInteger(size) && size > 0 ? size : pageSize;

    const {
      searchKeyword,
      searchToken,
      searchGroup,
      searchPackageMode,
      searchBalanceMin,
      searchBalanceMax,
      searchUsedBalanceMin,
      searchUsedBalanceMax,
      searchAmountSort,
    } = appliedFilters || getFormValues();
    const filters = appliedFilters || getFormValues();
    if (!hasTokenFilters(filters)) {
      setSearchMode(false);
      await loadTokens(normalizedPage, normalizedSize);
      return;
    }
    const reqId = nextRequestId();
    setSearching(true);
    // 通过 params 构造查询，避免手写 URL 拼接与转义遗漏。
    const params = {
      keyword: searchKeyword,
      token: searchToken,
      group: searchGroup,
      p: normalizedPage,
      size: normalizedSize,
      ...resolveTokenSort(searchAmountSort),
    };
    if (
      searchBalanceMin !== '' &&
      searchBalanceMin !== null &&
      searchBalanceMin !== undefined
    ) {
      params.balance_min = searchBalanceMin;
    }
    if (
      searchBalanceMax !== '' &&
      searchBalanceMax !== null &&
      searchBalanceMax !== undefined
    ) {
      params.balance_max = searchBalanceMax;
    }
    if (searchPackageMode !== '') {
      params.package_mode = searchPackageMode;
    }
    if (
      searchUsedBalanceMin !== '' &&
      searchUsedBalanceMin !== null &&
      searchUsedBalanceMin !== undefined
    ) {
      params.used_balance_min = searchUsedBalanceMin;
    }
    if (
      searchUsedBalanceMax !== '' &&
      searchUsedBalanceMax !== null &&
      searchUsedBalanceMax !== undefined
    ) {
      params.used_balance_max = searchUsedBalanceMax;
    }
    setSearchMode(true);
    lastListRequestRef.current = {
      mode: 'search',
      page: normalizedPage,
      size: normalizedSize,
      filters: { ...filters },
    };
    setSelectedKeys([]);
    setShowKeys({});
    setListError(false);
    setLoading(true);
    try {
      const res = await API.get('/api/token/search', {
        params,
        disableDuplicate: true,
      });
      if (!isLatestRequest(reqId)) return;
      const { success, data } = res.data || {};
      if (!success || !Array.isArray(data?.items))
        throw new Error('Invalid token search');
      syncPageData(data);
    } catch {
      if (isLatestRequest(reqId)) setListError(true);
    } finally {
      if (isLatestRequest(reqId)) {
        setSearching(false);
        setLoading(false);
      }
    }
  };

  const retryList = () => {
    const request = lastListRequestRef.current;
    return request.mode === 'search'
      ? searchTokens(request.page, request.size, request.filters)
      : loadTokens(request.page, request.size);
  };

  // 分页处理函数
  const handlePageChange = (page) => {
    if (searchMode) {
      searchTokens(page, pageSize, lastListRequestRef.current.filters).then();
    } else {
      loadTokens(page, pageSize).then();
    }
  };

  const handlePageSizeChange = async (size) => {
    setPageSize(size);
    if (searchMode) {
      await searchTokens(1, size, lastListRequestRef.current.filters);
    } else {
      await loadTokens(1, size);
    }
  };

  // 行选择处理函数
  const rowSelection = {
    selectedRowKeys: selectedKeys.map((token) => token.id),
    onSelect: (record, selected) => {},
    onSelectAll: (selected, selectedRows) => {},
    onChange: (selectedRowKeys, selectedRows) => {
      setSelectedKeys(selectedRows);
    },
  };

  // 行样式处理
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

  // 批量删除 Token
  const batchDeleteTokens = async () => {
    await batchManageTokens('delete');
  };

  // 统一 Token 批量管理（启用/禁用/删除）。
  const batchManageTokens = async (action) => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    if (mutationBusyRef.current) return;
    mutationBusyRef.current = true;
    setLoading(true);
    try {
      const ids = selectedKeys.map((token) => token.id);
      const res = await API.post('/api/token/manage/batch', { ids, action });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('批量操作失败'));
        return;
      }

      const successCount = Number(data?.success_count || 0);
      const failedCount = Number(data?.failed_count || 0);
      if (failedCount > 0) {
        showSuccess(
          t('批量操作完成: {{success}}个成功, {{failed}}个失败', {
            success: successCount,
            failed: failedCount,
          }),
        );
      } else if (action === 'enable') {
        showSuccess(t('已批量启用 {{count}} 个令牌', { count: successCount }));
      } else if (action === 'disable') {
        showSuccess(t('已批量禁用 {{count}} 个令牌', { count: successCount }));
      } else if (action === 'delete') {
        showSuccess(t('已删除 {{count}} 个令牌！', { count: successCount }));
      } else {
        showSuccess(t('操作成功完成！'));
      }

      await refresh();
    } catch (error) {
      showError(error?.message || t('批量操作失败'));
    } finally {
      mutationBusyRef.current = false;
      if (aliveRef.current) setLoading(false);
    }
  };

  const batchEnableTokens = async () => {
    await batchManageTokens('enable');
  };

  const batchDisableTokens = async () => {
    await batchManageTokens('disable');
  };

  // 批量复制 Token
  const batchCopyTokens = (copyType) => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }

    Modal.info({
      title: t('复制令牌'),
      icon: null,
      content: t('请选择你的复制方式'),
      footer: (
        <div className='flex gap-2'>
          <button
            className='px-3 py-1 bg-gray-200 rounded'
            onClick={async () => {
              let content = '';
              for (let i = 0; i < selectedKeys.length; i++) {
                const key = await getTokenFullKey(selectedKeys[i]);
                if (!key) return;
                content +=
                  selectedKeys[i].name +
                  '    ' +
                  normalizeTokenKeyForDisplay(key) +
                  '\n';
              }
              await copyText(content);
              Modal.destroyAll();
            }}
          >
            {t('名称+密钥')}
          </button>
          <button
            className='px-3 py-1 bg-blue-500 text-white rounded'
            onClick={async () => {
              let content = '';
              for (let i = 0; i < selectedKeys.length; i++) {
                const key = await getTokenFullKey(selectedKeys[i]);
                if (!key) return;
                content += normalizeTokenKeyForDisplay(key) + '\n';
              }
              await copyText(content);
              Modal.destroyAll();
            }}
          >
            {t('仅密钥')}
          </button>
        </div>
      ),
    });
  };

  // 初始化数据
  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
      nextRequestId();
      if (clearEditingTokenTimerRef.current) {
        clearTimeout(clearEditingTokenTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    const fetchGroups = async () => {
      try {
        const res = await API.get('/api/group/');
        if (!res?.data?.data) {
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

    loadTokens(1)
      .then()
      .catch((reason) => {
        showError(reason);
      });
    fetchGroups().then();
  }, []);

  return {
    listError,
    retryList,
    // 基础状态
    tokens,
    loading,
    activePage,
    tokenCount,
    pageSize,
    searching,
    groupOptions,

    // 选择状态
    selectedKeys,
    setSelectedKeys,

    // 编辑状态
    showEdit,
    setShowEdit,
    editingToken,
    setEditingToken,
    closeEdit,

    // UI 状态
    compactMode,
    setCompactMode,
    showKeys,
    setShowKeys,
    tokenFullKeys,
    ccSwitchToken,
    setCCSwitchToken,
    getTokenFullKey,
    copyTokenKey,
    toggleTokenKeyVisibility,

    // 表单状态
    formApi,
    setFormApi,
    formInitValues,
    getFormValues,

    // 函数集合
    loadTokens,
    refresh,
    copyText,
    onOpenLink,
    manageToken,
    searchTokens,
    handlePageChange,
    handlePageSizeChange,
    rowSelection,
    handleRow,
    batchDeleteTokens,
    batchManageTokens,
    batchEnableTokens,
    batchDisableTokens,
    batchCopyTokens,
    syncPageData,

    // 国际化
    t,
  };
};
