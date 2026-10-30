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

import React, { useSyncExternalStore } from 'react';
import { SideSheet } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import './playgroundWorkspace.css';

const subscribeCompact = (callback) => {
  const query = window.matchMedia('(max-width: 1279px)');
  query.addEventListener('change', callback);
  return () => query.removeEventListener('change', callback);
};
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
  const compact = useSyncExternalStore(
    subscribeCompact,
    () => window.matchMedia('(max-width: 1279px)').matches,
    () => false,
  );
  return (
    <div className='pg-workspace'>
      {!isMobile && <aside className='pg-settings'>{settings}</aside>}
      <main className='pg-main'>{children}</main>
      {!compact && <aside className='pg-code'>{code}</aside>}
      {isMobile && (
        <SideSheet
          title={t('参数')}
          visible={showSettings}
          onCancel={onCloseSettings}
          placement='left'
          width='100%'
          bodyStyle={{ padding: 0 }}
          className='pg-sheet'
        >
          {settings}
        </SideSheet>
      )}
      {compact && (
        <SideSheet
          title={t('请求代码')}
          visible={showCode}
          onCancel={onCloseCode}
          placement='right'
          width={isMobile ? '100%' : 420}
          bodyStyle={{ padding: 0 }}
          className='pg-sheet'
        >
          {code}
        </SideSheet>
      )}
    </div>
  );
}
