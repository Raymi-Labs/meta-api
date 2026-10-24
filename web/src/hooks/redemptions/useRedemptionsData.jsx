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

import { useState, useEffect } from 'react';
import { API, showError, showSuccess, copy } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import {
  REDEMPTION_ACTIONS,
  REDEMPTION_STATUS,
} from '../../constants/redemption.constants';
import { Modal } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useRedemptionsData = () => {
  const { t } = useTranslation();

  // 基础状态
  const [redemptions, setRedemptions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [tokenCount, setTokenCount] = useState(0);
  const [selectedKeys, setSelectedKeys] = useState([]);

  // 编辑状态
  const [editingRedemption, setEditingRedemption] = useState({
    id: undefined,
  });
  const [showEdit, setShowEdit] = useState(false);

  // Form API
  const [formApi, setFormApi] = useState(null);

  // UI 状态
  const [compactMode, setCompactMode] = useTableCompactMode('redemptions');

  // 表单状态
  const formInitValues = {
    searchKeyword: '',
  };

  // Get form values
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      searchKeyword: formValues.searchKeyword || '',
    };
  };

  // Set redemption data format
  const setRedemptionFormat = (redemptions) => {
    setRedemptions(redemptions);
  };

  // Load redemption list
  const loadRedemptions = async (page = 1, pageSize) => {
    setLoading(true);
    try {
      const res = await API.get(
        `/api/redemption/?p=${page}&page_size=${pageSize}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = data.items;
        setActivePage(data.page <= 0 ? 1 : data.page);
        setTokenCount(data.total);
        setRedemptionFormat(newPageData);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  // Search redemption codes
  const searchRedemptions = async () => {
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      await loadRedemptions(1, pageSize);
      return;
    }

    setSearching(true);
    try {
      const res = await API.get(
        `/api/redemption/search?keyword=${searchKeyword}&p=1&page_size=${pageSize}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = data.items;
        setActivePage(data.page || 1);
        setTokenCount(data.total);
        setRedemptionFormat(newPageData);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setSearching(false);
  };

  // Manage redemption codes (CRUD operations)
  const manageRedemption = async (id, action, record) => {
    setLoading(true);
    let data = { id };
    let res;

    try {
      switch (action) {
        case REDEMPTION_ACTIONS.DELETE:
          res = await API.delete(`/api/redemption/${id}/`);
          break;
        case REDEMPTION_ACTIONS.ENABLE:
          data.status = REDEMPTION_STATUS.UNUSED;
          res = await API.put('/api/redemption/?status_only=true', data);
          break;
        case REDEMPTION_ACTIONS.DISABLE:
          data.status = REDEMPTION_STATUS.DISABLED;
          res = await API.put('/api/redemption/?status_only=true', data);
          break;
        default:
          throw new Error('Unknown operation type');
      }

      const { success, message } = res.data;
      if (success) {
        showSuccess(t('操作成功完成！'));
        let redemption = res.data.data;
        let newRedemptions = [...redemptions];
        if (action !== REDEMPTION_ACTIONS.DELETE) {
          record.status = redemption.status;
        }
        setRedemptions(newRedemptions);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  // 刷新数据
  const refresh = async (page = activePage) => {
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      await loadRedemptions(page, pageSize);
    } else {
      await searchRedemptions();
    }
  };

  // 处理页码变更
  const handlePageChange = (page) => {
    setActivePage(page);
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      loadRedemptions(page, pageSize);
    } else {
      searchRedemptions();
    }
  };

  // 处理每页条数变更
  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setActivePage(1);
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      loadRedemptions(1, size);
    } else {
      searchRedemptions();
    }
  };

  // Row selection configuration
  const rowSelection = {
    onSelect: (record, selected) => {},
    onSelectAll: (selected, selectedRows) => {},
    onChange: (selectedRowKeys, selectedRows) => {
      setSelectedKeys(selectedRows);
    },
  };

  // Row style handling - using isExpired function
  const handleRow = (record, index) => {
    // Local isExpired function
    const isExpired = (rec) => {
      return (
        rec.status === REDEMPTION_STATUS.UNUSED &&
        rec.expired_time !== 0 &&
        rec.expired_time < Math.floor(Date.now() / 1000)
      );
    };

    if (record.status !== REDEMPTION_STATUS.UNUSED || isExpired(record)) {
      return {
        style: {
          background: 'var(--semi-color-disabled-border)',
        },
      };
    } else {
      return {};
    }
  };

  // Copy text
  const copyText = async (text) => {
    if (await copy(text)) {
      showSuccess('已复制到剪贴板！');
    } else {
      Modal.error({
        title: '无法复制到剪贴板，请手动复制',
        content: text,
        size: 'large',
      });
    }
  };

  // Batch copy redemption codes
  const batchCopyRedemptions = async () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个兑换码！'));
      return;
    }

    let keys = '';
    for (let i = 0; i < selectedKeys.length; i++) {
      keys += selectedKeys[i].name + '    ' + selectedKeys[i].key + '\n';
    }
    await copyText(keys);
  };

  // Batch delete redemption codes (clear invalid)
  const batchDeleteRedemptions = async () => {
    Modal.confirm({
      title: t('确定清除所有失效兑换码？'),
      content: t('将删除已使用、已禁用及过期的兑换码，此操作不可撤销。'),
      onOk: async () => {
        setLoading(true);
        const res = await API.delete('/api/redemption/invalid');
        const { success, message, data } = res.data;
        if (success) {
          showSuccess(t('已删除 {{count}} 条失效兑换码', { count: data }));
          await refresh();
        } else {
          showError(message);
        }
        setLoading(false);
      },
    });
  };

  // Batch manage selected administrator redemption codes.
  const batchManageSelectedRedemptions = async (action) => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个兑换码！'));
      return;
    }
    setLoading(true);
    try {
      const ids = selectedKeys.map((redemption) => redemption.id);
      const res = await API.post('/api/redemption/manage/batch', {
        ids,
        action,
      });
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
      } else if (action === REDEMPTION_ACTIONS.DISABLE) {
        showSuccess(
          t('已批量禁用 {{count}} 个兑换码', { count: successCount }),
        );
      } else {
        showSuccess(t('已删除 {{count}} 个兑换码', { count: successCount }));
      }
      setSelectedKeys([]);
      await refresh();
    } catch (error) {
      showError(error?.message || t('批量操作失败'));
    } finally {
      setLoading(false);
    }
  };

  const batchDisableSelectedRedemptions = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个兑换码！'));
      return;
    }
    Modal.confirm({
      title: t('批量禁用兑换码'),
      content: t('仅未使用的管理员兑换码会被禁用，钱包兑换码不会被修改。'),
      onOk: () => batchManageSelectedRedemptions(REDEMPTION_ACTIONS.DISABLE),
    });
  };

  const batchDeleteSelectedRedemptions = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个兑换码！'));
      return;
    }
    Modal.confirm({
      title: t('批量删除兑换码'),
      content: t(
        '选中的管理员兑换码将被删除，钱包兑换码不会被修改，此操作不可撤销。',
      ),
      onOk: () => batchManageSelectedRedemptions(REDEMPTION_ACTIONS.DELETE),
    });
  };

  // 关闭编辑弹窗
  const closeEdit = () => {
    setShowEdit(false);
  };

  // Remove record (for UI update after deletion)
  const removeRecord = (key) => {
    let newDataSource = [...redemptions];
    if (key != null) {
      let idx = newDataSource.findIndex((data) => data.key === key);
      if (idx > -1) {
        newDataSource.splice(idx, 1);
        setRedemptions(newDataSource);
      }
    }
  };

  // 初始化数据 loading
  useEffect(() => {
    loadRedemptions(1, pageSize)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  }, [pageSize]);

  return {
    // 数据状态
    redemptions,
    loading,
    searching,
    activePage,
    pageSize,
    tokenCount,
    selectedKeys,

    // 编辑状态
    editingRedemption,
    showEdit,

    // 表单状态
    formApi,
    formInitValues,

    // UI 状态
    compactMode,
    setCompactMode,

    // Data operations
    loadRedemptions,
    searchRedemptions,
    manageRedemption,
    refresh,
    copyText,
    removeRecord,

    // State updates
    setActivePage,
    setPageSize,
    setSelectedKeys,
    setEditingRedemption,
    setShowEdit,
    setFormApi,
    setLoading,

    // Event handlers
    handlePageChange,
    handlePageSizeChange,
    rowSelection,
    handleRow,
    closeEdit,
    getFormValues,

    // Batch operations
    batchCopyRedemptions,
    batchDeleteRedemptions,
    batchDisableSelectedRedemptions,
    batchDeleteSelectedRedemptions,

    // 国际化 function
    t,
  };
};
