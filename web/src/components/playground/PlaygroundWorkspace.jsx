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

import React from 'react';
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
  return (
    <div className='pg-workspace'>
      <main className='pg-main'>{children}</main>
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
    </div>
  );
}
