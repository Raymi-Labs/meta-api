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

import { useState, useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { requestModelCapabilitySync } from '../../helpers/modelCapabilitySync';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useModelsData = () => {
  const { t } = useTranslation();
  const [compactMode, setCompactMode] = useTableCompactMode('models');

  // 状态管理
  const [models, setModels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [searching, setSearching] = useState(false);
  const [modelCount, setModelCount] = useState(0);

  // 弹窗状态
  const [showEdit, setShowEdit] = useState(false);
  const [editingModel, setEditingModel] = useState({
    id: undefined,
  });

  // Row selection
  const [selectedKeys, setSelectedKeys] = useState([]);
  const rowSelection = {
    getCheckboxProps: (record) => ({
      name: record.model_name,
    }),
    selectedRowKeys: selectedKeys.map((model) => model.id),
    onChange: (selectedRowKeys, selectedRows) => {
      setSelectedKeys(selectedRows);
    },
  };

  // 表单初始值
  const formInitValues = {
    searchKeyword: '',
    searchVendor: '',
  };

  // ---------- helpers ----------
  // Safely extract array items from API payload
  const extractItems = (payload) => {
    const items = payload?.items || payload || [];
    return Array.isArray(items) ? items : [];
  };

  // 表单 API 引用
  const [formApi, setFormApi] = useState(null);

  // 获取表单值的辅助函数
  const getFormValues = () => formApi?.getValues() || formInitValues;

  // 关闭编辑弹窗
  const closeEdit = () => {
    setShowEdit(false);
  };

  // Set model format with key field
  const setModelFormat = (models) => {
    for (let i = 0; i < models.length; i++) {
      models[i].key = models[i].id;
    }
    setModels(models);
  };

  // Vendor list
  const [vendors, setVendors] = useState([]);
  const [vendorCounts, setVendorCounts] = useState({});
  const [activeVendorKey, setActiveVendorKey] = useState('all');
  const [showAddVendor, setShowAddVendor] = useState(false);
  const [showEditVendor, setShowEditVendor] = useState(false);
  const [editingVendor, setEditingVendor] = useState({ id: undefined });
  const [syncing, setSyncing] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [syncingCapabilities, setSyncingCapabilities] = useState(false);
  const syncInFlight = useRef(false);

  const vendorMap = useMemo(() => {
    const map = {};
    vendors.forEach((v) => {
      map[v.id] = v;
    });
    return map;
  }, [vendors]);

  // Load vendor list
  const loadVendors = async () => {
    try {
      const res = await API.get('/api/vendors/?page_size=1000');
      if (res.data.success) {
        const items = res.data.data.items || res.data.data || [];
        setVendors(Array.isArray(items) ? items : []);
      }
    } catch (_) {
      // ignore
    }
  };

  // Load models data
  const loadModels = async (
    page = 1,
    size = pageSize,
    vendorKey = activeVendorKey,
  ) => {
    setLoading(true);
    try {
      let url = `/api/models/?p=${page}&page_size=${size}`;
      if (vendorKey && vendorKey !== 'all') {
        // Filter by vendor ID
        url = `/api/models/search?vendor=${vendorKey}&p=${page}&page_size=${size}`;
      }

      const res = await API.get(url);
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = extractItems(data);
        setActivePage(data.page || page);
        setModelCount(data.total || newPageData.length);
        setModelFormat(newPageData);

        if (data.vendor_counts) {
          const sumAll = Object.values(data.vendor_counts).reduce(
            (acc, v) => acc + v,
            0,
          );
          setVendorCounts({ ...data.vendor_counts, all: sumAll });
        }
      } else {
        showError(message);
        setModels([]);
      }
    } catch (error) {
      console.error(error);
      showError(t('获取模型列表失败'));
      setModels([]);
    }
    setLoading(false);
  };

  // 刷新数据
  const refresh = async (page = activePage) => {
    await loadModels(page, pageSize);
  };

  // Sync upstream models/vendors for missing models only
  const syncUpstream = async (opts = {}) => {
    if (syncInFlight.current) return false;
    syncInFlight.current = true;
    const locale = opts?.locale;
    setSyncing(true);
    try {
      const body = {};
      if (locale) body.locale = locale;
      const res = await API.post('/api/models/sync_upstream', body);
      const { success, message, data } = res.data || {};
      if (success) {
        const createdModels = data?.created_models || 0;
        const createdVendors = data?.created_vendors || 0;
        const skipped = (data?.skipped_models || []).length || 0;
        showSuccess(
          t(
            `已同步：新增 ${createdModels} 模型，新增 ${createdVendors} 供应商，跳过 ${skipped} 项`,
          ),
        );
        await loadVendors();
        await refresh();
      } else {
        showError(message || t('同步失败'));
      }
    } catch (e) {
      showError(t('同步失败'));
    } finally {
      syncInFlight.current = false;
      setSyncing(false);
    }
  };

  // Preview upstream differences
  const previewUpstreamDiff = async (opts = {}) => {
    if (syncInFlight.current) return null;
    syncInFlight.current = true;
    const locale = opts?.locale;
    setPreviewing(true);
    try {
      const url = `/api/models/sync_upstream/preview${locale ? `?locale=${locale}` : ''}`;
      const res = await API.get(url);
      const { success, message, data } = res.data || {};
      if (success) {
        return data || { missing: [], conflicts: [] };
      }
      showError(message || t('预览失败'));
      return null;
    } catch (e) {
      showError(t('预览失败'));
      return null;
    } finally {
      syncInFlight.current = false;
      setPreviewing(false);
    }
  };

  // Apply selected overwrite
  const applyUpstreamOverwrite = async (payloadOrArray = []) => {
    if (syncInFlight.current) return false;
    syncInFlight.current = true;
    const isArray = Array.isArray(payloadOrArray);
    const overwrite = isArray ? payloadOrArray : payloadOrArray.overwrite || [];
    const locale = isArray ? undefined : payloadOrArray.locale;
    setSyncing(true);
    try {
      const body = { overwrite };
      if (locale) body.locale = locale;
      const res = await API.post('/api/models/sync_upstream', body);
      const { success, message, data } = res.data || {};
      if (success) {
        const createdModels = data?.created_models || 0;
        const updatedModels = data?.updated_models || 0;
        const createdVendors = data?.created_vendors || 0;
        const skipped = (data?.skipped_models || []).length || 0;
        showSuccess(
          t(
            `完成：新增 ${createdModels} 模型，更新 ${updatedModels} 模型，新增 ${createdVendors} 供应商，跳过 ${skipped} 项`,
          ),
        );
        await loadVendors();
        await refresh();
        return true;
      }
      showError(message || t('同步失败'));
      return false;
    } catch (e) {
      showError(t('同步失败'));
      return false;
    } finally {
      syncInFlight.current = false;
      setSyncing(false);
    }
  };

  const syncCapabilities = async () => {
    if (syncInFlight.current) return false;
    syncInFlight.current = true;
    setSyncingCapabilities(true);
    try {
      const result = await requestModelCapabilitySync(API);
      showSuccess(
        t('模型能力已同步：新增 {{created}} 个，更新 {{updated}} 个', result),
      );
      await refresh();
      return true;
    } catch (error) {
      showError(
        error.response?.data?.message || t('模型能力同步失败，现有数据已保留'),
      );
      return false;
    } finally {
      syncInFlight.current = false;
      setSyncingCapabilities(false);
    }
  };

  // Search models with keyword and vendor
  const searchModels = async () => {
    const { searchKeyword = '', searchVendor = '' } = getFormValues();

    if (searchKeyword === '' && searchVendor === '') {
      // If keyword is blank, load models instead
      await loadModels(1, pageSize);
      return;
    }

    setSearching(true);
    try {
      const res = await API.get(
        `/api/models/search?keyword=${searchKeyword}&vendor=${searchVendor}&p=1&page_size=${pageSize}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = extractItems(data);
        setActivePage(data.page || 1);
        setModelCount(data.total || newPageData.length);
        setModelFormat(newPageData);
        if (data.vendor_counts) {
          const sumAll = Object.values(data.vendor_counts).reduce(
            (acc, v) => acc + v,
            0,
          );
          setVendorCounts({ ...data.vendor_counts, all: sumAll });
        }
      } else {
        showError(message);
        setModels([]);
      }
    } catch (error) {
      console.error(error);
      showError(t('搜索模型失败'));
      setModels([]);
    }
    setSearching(false);
  };

  // Manage model (enable/disable/delete)
  const manageModel = async (id, action, record) => {
    let res;
    switch (action) {
      case 'delete':
        res = await API.delete(`/api/models/${id}`);
        break;
      case 'enable':
        res = await API.put('/api/models/?status_only=true', { id, status: 1 });
        break;
      case 'disable':
        res = await API.put('/api/models/?status_only=true', { id, status: 0 });
        break;
      default:
        return;
    }

    const { success, message } = res.data;
    if (success) {
      showSuccess(t('操作成功完成！'));
      if (action === 'delete') {
        await refresh();
      } else {
        // Update local state for enable/disable
        setModels((prevModels) =>
          prevModels.map((model) =>
            model.id === id
              ? { ...model, status: action === 'enable' ? 1 : 0 }
              : model,
          ),
        );
      }
    } else {
      showError(message);
    }
  };

  // 处理页码变更
  const handlePageChange = (page) => {
    setActivePage(page);
    loadModels(page, pageSize, activeVendorKey);
  };

  // Reload models when activeVendorKey changes
  useEffect(() => {
    loadModels(1, pageSize, activeVendorKey);
  }, [activeVendorKey]);

  // 处理每页条数变更
  const handlePageSizeChange = async (size) => {
    setPageSize(size);
    setActivePage(1);
    await loadModels(1, size, activeVendorKey);
  };

  // Handle row click and styling
  const handleRow = (record, index) => {
    const rowStyle =
      record.status !== 1
        ? {
            style: {
              background: 'var(--semi-color-disabled-border)',
            },
          }
        : {};

    return {
      ...rowStyle,
      onClick: (event) => {
        // Don't trigger row selection when clicking on buttons
        if (event.target.closest('button, .semi-button')) {
          return;
        }
        const newSelectedKeys = selectedKeys.some(
          (item) => item.id === record.id,
        )
          ? selectedKeys.filter((item) => item.id !== record.id)
          : [...selectedKeys, record];
        setSelectedKeys(newSelectedKeys);
      },
    };
  };

  // Batch delete models
  const batchDeleteModels = async () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个模型'));
      return;
    }

    try {
      const deletePromises = selectedKeys.map((model) =>
        API.delete(`/api/models/${model.id}`),
      );

      const results = await Promise.all(deletePromises);
      let successCount = 0;

      results.forEach((res, index) => {
        if (res.data.success) {
          successCount++;
        } else {
          showError(
            `删除模型 ${selectedKeys[index].model_name} 失败: ${res.data.message}`,
          );
        }
      });

      if (successCount > 0) {
        showSuccess(t(`成功删除 ${successCount} 个模型`));
        setSelectedKeys([]);
        await refresh();
      }
    } catch (error) {
      showError(t('批量删除失败'));
    }
  };

  // Copy text helper
  const copyText = async (text) => {
    try {
      await navigator.clipboard.writeText(text);
      showSuccess(t('复制成功'));
    } catch (error) {
      console.error('Copy failed:', error);
      showError(t('复制失败'));
    }
  };

  // Initial load
  useEffect(() => {
    (async () => {
      await loadVendors();
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
    // 数据状态
    models,
    loading,
    searching,
    activePage,
    pageSize,
    modelCount,

    // 选择状态
    selectedKeys,
    rowSelection,
    handleRow,
    setSelectedKeys,

    // 弹窗状态
    showEdit,
    editingModel,
    setEditingModel,
    setShowEdit,
    closeEdit,

    // 表单状态
    formInitValues,
    setFormApi,

    // 操作函数
    loadModels,
    searchModels,
    refresh,
    manageModel,
    batchDeleteModels,
    copyText,

    // Pagination
    setActivePage,
    handlePageChange,
    handlePageSizeChange,

    // UI 状态
    compactMode,
    setCompactMode,

    // Vendor data
    vendors,
    vendorMap,
    vendorCounts,
    activeVendorKey,
    setActiveVendorKey,
    showAddVendor,
    setShowAddVendor,
    showEditVendor,
    setShowEditVendor,
    editingVendor,
    setEditingVendor,
    loadVendors,

    // 国际化
    t,

    // Upstream sync
    syncing,
    previewing,
    syncingCapabilities,
    syncCapabilities,
    syncUpstream,
    previewUpstreamDiff,
    applyUpstreamOverwrite,
  };
};
