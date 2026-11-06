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

import React, { useEffect } from 'react';
import { X } from 'lucide-react';
import { SideSheet } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import './playgroundWorkspace.css';

export default function PlaygroundWorkspace({
  settings,
  code,
  children,
  showSettings,
  showCode,
  onCloseSettings,
  onCloseCode,
}) {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  useEffect(() => {
    if (isMobile || (!showSettings && !showCode)) return;
    const onKey = (e) => {
      if (e.key === 'Escape' && !e.defaultPrevented) {
        e.preventDefault();
        if (showSettings) onCloseSettings();
        else onCloseCode();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isMobile, showSettings, showCode, onCloseSettings, onCloseCode]);
  return (
    <div className='pg-workspace'>
      {!isMobile && showSettings && (
        <aside
          className='pg-docked-panel pg-docked-settings'
          aria-label={t('参数')}
        >
          <header>
            <strong>{t('参数')}</strong>
            <button onClick={onCloseSettings} aria-label={t('关闭设置')}>
              <X size={18} />
            </button>
          </header>
          {settings}
        </aside>
      )}
      <main className='pg-main'>{children}</main>
      {!isMobile && showCode && (
        <aside
          className='pg-docked-panel pg-docked-code'
          aria-label={t('请求代码')}
        >
          <header>
            <strong>{t('请求代码')}</strong>
            <button onClick={onCloseCode} aria-label={t('关闭代码')}>
              <X size={18} />
            </button>
          </header>
          {code}
        </aside>
      )}
      {isMobile && (
        <SideSheet
          closeOnEsc
          title={t('参数')}
          visible={showSettings}
          onCancel={onCloseSettings}
          placement='left'
          width={isMobile ? '100%' : 360}
          bodyStyle={{ padding: 0 }}
          className='pg-sheet'
        >
          {settings}
        </SideSheet>
      )}
      {isMobile && (
        <SideSheet
          closeOnEsc
          title={t('请求代码')}
          visible={showCode}
          onCancel={onCloseCode}
          placement='right'
          width={isMobile ? '100%' : 480}
          bodyStyle={{ padding: 0 }}
          className='pg-sheet'
        >
          {code}
        </SideSheet>
      )}
    </div>
  );
}
