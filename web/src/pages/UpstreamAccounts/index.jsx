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

import React, { useContext, useState } from 'react';
import { Card, Skeleton, Tabs, Typography } from '@douyinfe/semi-ui';
import { Server, ServerCog, WalletCards } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../../context/Status';
import OpenRouterAccountDashboard from './components/OpenRouterAccountDashboard';
import CPAAccountDashboard from './components/CPAAccountDashboard';
import CRSDashboardCard from './components/CRSDashboardCard';
import UpstreamWalletCard from './components/UpstreamWalletCard';
import { useUpstreamAccounts } from './hooks/useUpstreamAccounts';
import { formatMoney } from './utils';

const { Text, Title } = Typography;

const UpstreamAccountsPage = () => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const [activeTab, setActiveTab] = useState('wallet');
  const accountsHook = useUpstreamAccounts();

  return (
    <div className='space-y-4 px-2 pb-6'>
      <Card className='overflow-hidden' bodyStyle={{ padding: 0 }}>
        <div className='relative overflow-hidden px-5 py-5 sm:px-6'>
          <div className='absolute inset-0 bg-gradient-to-r from-cyan-500/10 via-emerald-500/5 to-transparent' />
          <div className='relative flex items-start gap-3'>
            <div className='rounded-2xl bg-cyan-500/10 p-3 text-cyan-600 dark:text-cyan-300'>
              <WalletCards size={24} />
            </div>
            <div>
              <Title heading={3} style={{ margin: 0 }}>
                {t('账户管理')}
              </Title>
              <Text type='tertiary' className='mt-1 block'>
                {t('上游账户') + ' · OpenRouter · CRS · CPA'}
              </Text>
            </div>
          </div>
        </div>
      </Card>

      <Tabs
        type='line'
        size='large'
        activeKey={activeTab}
        onChange={setActiveTab}
        keepDOM
      >
        <Tabs.TabPane
          itemKey='wallet'
          tab={
            <span className='flex items-center gap-1.5'>
              <WalletCards size={16} />
              {t('上游账户')}
            </span>
          }
        >
          <div className='mt-3'>
            {accountsHook.accountsLoading && !accountsHook.accounts.length ? (
              <Card>
                <Skeleton.Title style={{ width: 200 }} />
                <Skeleton.Paragraph rows={4} />
              </Card>
            ) : (
              <UpstreamWalletCard
                accounts={accountsHook.accounts.filter(
                  (a) => a.account_type !== 'openrouter',
                )}
                accountDraft={accountsHook.accountDraft}
                updateAccountDraftField={accountsHook.updateAccountDraftField}
                normalizeAccountDraftBaseUrl={
                  accountsHook.normalizeAccountDraftBaseUrl
                }
                touchAccountDraftField={accountsHook.touchAccountDraftField}
                accountDraftErrors={accountsHook.accountDraftErrors}
                accountDraftCanSave={accountsHook.accountDraftCanSave}
                accountDraftValidation={accountsHook.accountDraftValidation}
                editingAccountId={accountsHook.editingAccountId}
                editingAccount={accountsHook.editingAccount}
                accountTrend={accountsHook.accountTrend}
                accountTrendLoading={accountsHook.accountTrendLoading}
                saveAccount={accountsHook.saveAccount}
                syncAccount={accountsHook.syncAccount}
                syncAllAccounts={accountsHook.syncAllAccounts}
                deleteAccount={accountsHook.deleteAccount}
                savingAccount={accountsHook.savingAccount}
                syncingAccountId={accountsHook.syncingAccountId}
                syncingAllAccounts={accountsHook.syncingAllAccounts}
                deletingAccountId={accountsHook.deletingAccountId}
                sideSheetVisible={accountsHook.sideSheetVisible}
                detailSideSheetVisible={accountsHook.detailSideSheetVisible}
                openCreateSideSheet={accountsHook.openCreateSideSheet}
                openEditSideSheet={accountsHook.openEditSideSheet}
                closeSideSheet={accountsHook.closeSideSheet}
                openDetailSideSheet={accountsHook.openDetailSideSheet}
                closeDetailSideSheet={accountsHook.closeDetailSideSheet}
                formatMoney={formatMoney}
                status={statusState?.status}
                t={t}
              />
            )}
          </div>
        </Tabs.TabPane>
        <Tabs.TabPane itemKey='openrouter' tab='OpenRouter'>
          <OpenRouterAccountDashboard
            accounts={accountsHook.accounts}
            reloadAccounts={accountsHook.loadAccounts}
            active={activeTab === 'openrouter'}
            t={t}
          />
        </Tabs.TabPane>
        <Tabs.TabPane
          itemKey='crs'
          tab={
            <span className='flex items-center gap-1.5'>
              <Server size={16} />
              {t('CRS 账号')}
            </span>
          }
        >
          <div className='mt-3'>
            <CRSDashboardCard t={t} />
          </div>
        </Tabs.TabPane>
        <Tabs.TabPane
          itemKey='cpa'
          tab={
            <span className='flex items-center gap-1.5'>
              <ServerCog size={16} />
              {t('CPA 账号')}
            </span>
          }
        >
          <div className='mt-3'>
            <CPAAccountDashboard t={t} />
          </div>
        </Tabs.TabPane>
      </Tabs>
    </div>
  );
};

export default UpstreamAccountsPage;
