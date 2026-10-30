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
import { Modal, Input } from '@douyinfe/semi-ui';
import { Plus, X, Search, Box } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function ModelTabs({
  models,
  tabs,
  activeModel,
  onSelect,
  onRemove,
  disabled,
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const filtered = models.filter(({ value }) =>
    value.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <>
      <div className='pg-model-tabs' role='tablist' aria-label={t('模型')}>
        {tabs.map((model) => (
          <div
            className={`pg-model-tab ${model === activeModel ? 'is-active' : ''}`}
            key={model}
          >
            <button
              role='tab'
              aria-selected={model === activeModel}
              disabled={disabled}
              onClick={() => onSelect(model)}
              title={model}
            >
              <Box size={15} />
              <span>{model}</span>
            </button>
            {tabs.length > 1 && (
              <button
                className='pg-tab-close'
                aria-label={`${t('移除')} ${model}`}
                disabled={disabled}
                onClick={() => onRemove(model)}
              >
                <X size={12} />
              </button>
            )}
          </div>
        ))}
        <button
          className='pg-add-model'
          aria-label={t('添加模型')}
          title={t('添加模型')}
          disabled={disabled || !models.length}
          onClick={() => {
            setQuery('');
            setOpen(true);
          }}
        >
          <Plus size={17} />
        </button>
      </div>
      <Modal
        title={t('添加模型')}
        visible={open}
        onCancel={() => setOpen(false)}
        footer={null}
        width={560}
        className='pg-model-modal'
      >
        <Input
          prefix={<Search size={16} />}
          value={query}
          onChange={setQuery}
          placeholder={t('搜索模型')}
          aria-label={t('搜索模型')}
          showClear
        />
        <div className='pg-model-options'>
          {filtered.map(({ value }) => (
            <button
              key={value}
              disabled={disabled}
              onClick={() => {
                onSelect(value);
                setOpen(false);
              }}
            >
              <Box size={17} />
              <span>{value}</span>
              {tabs.includes(value) && (
                <span className='pg-model-added'>✓</span>
              )}
            </button>
          ))}
          {!filtered.length && <p className='pg-muted'>{t('暂无可用模型')}</p>}
        </div>
      </Modal>
    </>
  );
}
