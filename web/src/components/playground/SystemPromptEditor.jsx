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
import { Modal, TextArea } from '@douyinfe/semi-ui';
import { SlidersHorizontal, Maximize2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function SystemPromptEditor({ value = '', onChange, disabled }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        className='pg-system-prompt'
        onClick={() => setOpen(true)}
        disabled={disabled}
        aria-label={t('系统提示词')}
      >
        <SlidersHorizontal size={17} />
        <span>
          <strong>{t('系统提示词')}</strong>
          <span className={!value ? 'pg-muted' : ''}>
            {value || t('点击设置系统提示词')}
          </span>
        </span>
        <Maximize2 size={14} />
      </button>
      <Modal
        title={t('系统提示词')}
        visible={open}
        onCancel={() => setOpen(false)}
        footer={null}
        width={640}
        className='pg-model-modal'
      >
        <TextArea
          value={value}
          onChange={onChange}
          disabled={disabled}
          aria-label={t('系统提示词')}
          placeholder={t('点击设置系统提示词')}
          autosize={{ minRows: 8, maxRows: 16 }}
        />
        <div className='pg-prompt-footer'>
          <span>{t('自动保存')}</span>
          <span>{value.length}</span>
        </div>
      </Modal>
    </>
  );
}
