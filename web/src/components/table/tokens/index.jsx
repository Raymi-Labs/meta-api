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

import React, {
  useEffect,
  useRef,
  useState,
  useMemo,
  useCallback,
  useContext,
} from 'react';
import { UserContext } from '../../../context/User';
import { Button } from '@douyinfe/semi-ui';
import { API } from '../../../helpers';
import CardPro from '../../common/ui/CardPro';
import TokensTable from './TokensTable';
import TokensActions from './TokensActions';
import TokensFilters from './TokensFilters';
import TokensDescription from './TokensDescription';
import './tokensToolbar.css';
import EditTokenModal from './modals/EditTokenModal';
import TokenTestModal from './modals/TokenTestModal';
import CCSwitchModal from './modals/CCSwitchModal';
import { useTokensData } from '../../../hooks/tokens/useTokensData';
import { useFluentIntegration } from '../../../hooks/tokens/useFluentIntegration';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

function TokensPageContent() {
  // openFluentNotificationRef 用于打破 TDZ 循环依赖：
  // useTokensData 需要在初始化时接收回调，但回调本身来自后续的 useFluentIntegration
  const openFluentNotificationRef = useRef(null);
  const tokensData = useTokensData((key) =>
    openFluentNotificationRef.current?.(key),
  );
  const isMobile = useIsMobile();

  // FluentRead 集成（MutationObserver、Notification、模型加载全部在此 Hook 内管理）
  const { openFluentNotification } = useFluentIntegration({
    tokens: tokensData.tokens,
    selectedKeys: tokensData.selectedKeys,
    getTokenFullKey: tokensData.getTokenFullKey,
    t: tokensData.t,
  });
  // 每次渲染后同步最新版本
  openFluentNotificationRef.current = openFluentNotification;

  const [showTestModal, setShowTestModal] = useState(false);
  const [testingToken, setTestingToken] = useState(null);

  const {
    // 编辑状态
    showEdit,
    editingToken,
    closeEdit,
    refresh,

    // 操作函数 state
    selectedKeys,
    setEditingToken,
    setShowEdit,
    batchEnableTokens,
    batchDisableTokens,
    batchCopyTokens,
    batchDeleteTokens,
    copyText,

    // Filters state
    formInitValues,
    setFormApi,
    searchTokens,
    loading,
    searching,
    groupOptions,

    // Description state
    compactMode,
    setCompactMode,

    // 国际化
    t,
  } = tokensData;

  // Fetch runtime status for tokens that have runtime limits
  const [runtimeStatusMap, setRuntimeStatusMap] = useState({});
  const runtimeRequestRef = useRef(0);

  const fetchRuntimeStatus = useCallback(async (tokensList) => {
    const requestId = ++runtimeRequestRef.current;
    const ids = tokensList
      .filter(
        (tk) =>
          Number(tk.max_concurrency) > 0 ||
          (Number(tk.window_request_limit) > 0 &&
            Number(tk.window_seconds) > 0),
      )
      .map((tk) => tk.id);
    if (ids.length === 0) {
      setRuntimeStatusMap({});
      return;
    }
    try {
      const res = await API.post('/api/token/runtime_status', {
        token_ids: ids,
      });
      if (
        requestId === runtimeRequestRef.current &&
        res.data?.success &&
        res.data?.data
      ) {
        setRuntimeStatusMap(res.data.data);
      }
    } catch {
      // silently ignore
    }
  }, []);

  useEffect(() => {
    setRuntimeStatusMap({});
    if (!tokensData.tokens || tokensData.tokens.length === 0) {
      runtimeRequestRef.current++;
      return;
    }
    fetchRuntimeStatus(tokensData.tokens);
    const interval = setInterval(
      () => fetchRuntimeStatus(tokensData.tokens),
      30000,
    );
    return () => {
      clearInterval(interval);
      runtimeRequestRef.current++;
    };
  }, [tokensData.tokens, fetchRuntimeStatus]);

  // Merge runtime status into tokens
  const tokensWithRuntime = useMemo(() => {
    if (!tokensData.tokens || Object.keys(runtimeStatusMap).length === 0) {
      return tokensData.tokens;
    }
    return tokensData.tokens.map((tk) => {
      const rs = runtimeStatusMap[String(tk.id)];
      if (rs) {
        return { ...tk, runtime_status: rs };
      }
      return tk;
    });
  }, [tokensData.tokens, runtimeStatusMap]);

  // Override tokens in tokensData for downstream components
  const enhancedTokensData = useMemo(
    () => ({ ...tokensData, tokens: tokensWithRuntime }),
    [tokensData, tokensWithRuntime],
  );

  return (
    <div className='tokens-page'>
      <EditTokenModal
        refresh={refresh}
        editingToken={editingToken}
        visiable={showEdit}
        handleClose={closeEdit}
      />
      <TokenTestModal
        visible={showTestModal}
        token={testingToken}
        onCancel={() => {
          setShowTestModal(false);
          setTestingToken(null);
        }}
      />
      <CCSwitchModal
        visible={!!tokensData.ccSwitchToken}
        token={tokensData.ccSwitchToken}
        getTokenFullKey={tokensData.getTokenFullKey}
        onCancel={() => tokensData.setCCSwitchToken(null)}
        t={tokensData.t}
      />
      <CardPro
        mobileActionsInitiallyVisible
        type='type1'
        descriptionArea={
          <TokensDescription
            tokenCount={tokensData.tokenCount}
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        }
        actionsArea={
          <div className='tokens-toolbar'>
            <TokensActions
              selectedKeys={selectedKeys}
              setEditingToken={setEditingToken}
              setShowEdit={setShowEdit}
              batchEnableTokens={batchEnableTokens}
              batchDisableTokens={batchDisableTokens}
              batchCopyTokens={batchCopyTokens}
              batchDeleteTokens={batchDeleteTokens}
              getTokenFullKey={tokensData.getTokenFullKey}
              copyText={copyText}
              loading={loading}
              t={t}
            />

            <div className='tokens-toolbar__filters'>
              <TokensFilters
                formInitValues={formInitValues}
                setFormApi={setFormApi}
                searchTokens={searchTokens}
                groupOptions={groupOptions}
                loading={loading}
                searching={searching}
                t={t}
              />
            </div>
          </div>
        }
        paginationArea={createCardProPagination({
          currentPage: tokensData.activePage,
          pageSize: tokensData.pageSize,
          total: tokensData.tokenCount,
          onPageChange: tokensData.handlePageChange,
          onPageSizeChange: tokensData.handlePageSizeChange,
          isMobile: isMobile,
          t: tokensData.t,
        })}
        t={tokensData.t}
      >
        {tokensData.listError ? (
          <div className='tokens-list-error' role='alert'>
            <p>{t('加载失败，请重试')}</p>
            <Button onClick={tokensData.retryList}>{t('重试')}</Button>
          </div>
        ) : (
          <TokensTable
            {...enhancedTokensData}
            openTestModal={(token) => {
              setTestingToken(token);
              setShowTestModal(true);
            }}
          />
        )}
      </CardPro>
    </div>
  );
}
function TokensPage() {
  const [state] = useContext(UserContext);
  const user = state?.user;
  if (!user?.id) return null;
  return (
    <TokensPageContent key={`${user.id}:${user.role}:${user.group || ''}`} />
  );
}
export default TokensPage;
