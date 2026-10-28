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

import React, { useState } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { showError } from '../../../helpers';
import CopyTokensModal from './modals/CopyTokensModal';
import DeleteTokensModal from './modals/DeleteTokensModal';
import ApiAddressesButton from './ApiAddressesButton';

const TokensActions = ({
  selectedKeys,
  setEditingToken,
  setShowEdit,
  batchEnableTokens,
  batchDisableTokens,
  batchCopyTokens,
  batchDeleteTokens,
  copyText,
  getTokenFullKey,
  loading = false,
  t,
}) => {
  // 弹窗状态
  const [showCopyModal, setShowCopyModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);

  // Handle copy selected tokens with options
  const handleCopySelectedTokens = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    setShowCopyModal(true);
  };

  // Handle delete selected tokens with confirmation
  const handleDeleteSelectedTokens = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    setShowDeleteModal(true);
  };

  // Handle delete confirmation
  const handleConfirmDelete = () => {
    batchDeleteTokens();
    setShowDeleteModal(false);
  };

  return (
    <>
      <div className='tokens-toolbar__actions' data-onboarding='tokens-actions'>
        <Button
          type='primary'
          theme='solid'
          className='flex-1 md:flex-initial'
          data-onboarding='create-token-button'
          onClick={() => {
            setEditingToken({
              id: undefined,
            });
            setShowEdit(true);
          }}
          size='small'
        >
          {t('添加令牌')}
        </Button>
        <ApiAddressesButton copyText={copyText} t={t} />
        {selectedKeys.length > 0 && (
          <div
            className='tokens-toolbar__batch'
            role='group'
            aria-label={t('批量操作')}
          >
            <span className='tokens-toolbar__count' role='status'>
              {t('已选择 {{count}} 个令牌', { count: selectedKeys.length })}
            </span>
            <Button
              type='tertiary'
              className='flex-1 md:flex-initial'
              onClick={batchEnableTokens}
              disabled={selectedKeys.length === 0 || loading}
              loading={loading}
              size='small'
            >
              {t('批量启用')}
            </Button>

            <Button
              type='tertiary'
              className='flex-1 md:flex-initial'
              onClick={batchDisableTokens}
              disabled={selectedKeys.length === 0 || loading}
              loading={loading}
              size='small'
            >
              {t('批量禁用')}
            </Button>

            <Button
              type='tertiary'
              className='flex-1 md:flex-initial'
              onClick={handleCopySelectedTokens}
              disabled={selectedKeys.length === 0 || loading}
              size='small'
            >
              {t('复制所选令牌')}
            </Button>

            <Button
              type='danger'
              className='w-full md:w-auto'
              onClick={handleDeleteSelectedTokens}
              disabled={selectedKeys.length === 0 || loading}
              loading={loading}
              size='small'
            >
              {t('删除所选令牌')}
            </Button>
          </div>
        )}
      </div>

      <CopyTokensModal
        visible={showCopyModal}
        onCancel={() => setShowCopyModal(false)}
        selectedKeys={selectedKeys}
        getTokenFullKey={getTokenFullKey}
        copyText={copyText}
        t={t}
      />

      <DeleteTokensModal
        visible={showDeleteModal}
        onCancel={() => setShowDeleteModal(false)}
        onConfirm={handleConfirmDelete}
        selectedKeys={selectedKeys}
        t={t}
      />
    </>
  );
};

export default TokensActions;
