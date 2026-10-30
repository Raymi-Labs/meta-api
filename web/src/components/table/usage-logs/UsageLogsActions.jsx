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
import { Button } from '@douyinfe/semi-ui';
import { RefreshCw, Columns3, Download } from 'lucide-react';
import CompactModeToggle from '../../common/ui/CompactModeToggle';

const LogsActions = ({
  exporting,
  exportLogs,
  loadingStat,
  compactMode,
  setCompactMode,
  isAdminUser,
  openTopUsersDrawer,
  openGroupSummary,
  refresh,
  loading,
  columnViewMode,
  setColumnViewMode,
  openColumnSelector,
  t,
}) => {
  return (
    <>
      <div className='usage-logs-toolbar'>
        <div className='usage-logs-tools'>
          <Button
            size='small'
            theme='light'
            icon={<RefreshCw size={15} />}
            onClick={refresh}
            loading={loading || loadingStat}
          >
            {t('刷新日志')}
          </Button>
          {!isAdminUser && (
            <>
              <Button
                size='small'
                theme='light'
                className={
                  columnViewMode === 'summary'
                    ? 'console-control-active'
                    : undefined
                }
                aria-pressed={columnViewMode === 'summary'}
                onClick={() => setColumnViewMode('summary')}
              >
                {t('摘要视图')}
              </Button>
              <Button
                size='small'
                theme='light'
                className={
                  columnViewMode === 'full'
                    ? 'console-control-active'
                    : undefined
                }
                aria-pressed={columnViewMode === 'full'}
                onClick={() => setColumnViewMode('full')}
              >
                {t('完整视图')}
              </Button>
            </>
          )}
          <Button
            size='small'
            theme='light'
            icon={<Columns3 size={15} />}
            onClick={openColumnSelector}
          >
            {t('列设置')}
          </Button>
        </div>
        <div className='usage-logs-tools'>
          {isAdminUser && (
            <>
              <Button size='small' theme='light' onClick={openGroupSummary}>
                {t('分组汇总')}
              </Button>
              <Button size='small' theme='light' onClick={openTopUsersDrawer}>
                {t('大用户榜单')}
              </Button>
            </>
          )}
          <Button
            size='small'
            theme='light'
            icon={<Download size={15} />}
            loading={exporting}
            disabled={exporting}
            onClick={exportLogs}
          >
            {t('导出日志')}
          </Button>
          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        </div>
      </div>
    </>
  );
};
export default LogsActions;
