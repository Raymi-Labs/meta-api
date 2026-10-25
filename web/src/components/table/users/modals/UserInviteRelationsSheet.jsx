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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  SideSheet,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { API, renderGroup, renderQuota, showError } from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import CardTable from '../../../common/ui/CardTable';
import InvitePromotionStats from '../../../common/invitation/InvitePromotionStats';

const { Text } = Typography;

const initialRelations = {
  user: null,
  inviter: null,
  invitees: {
    items: [],
    total: 0,
    page: 1,
    page_size: 10,
  },
  invite_income_summary: {
    direct_total_quota: 0,
    recharge_total_quota: 0,
    total_quota: 0,
  },
};

const UserInviteRelationsSheet = ({
  visible,
  onCancel,
  user,
  onNavigateUser,
  onBack,
  canGoBack = false,
  t,
}) => {
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(false);
  const [relations, setRelations] = useState(initialRelations);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const latestRequestRef = useRef(0);

  // 按用户维度查询邀请关系：
  // 1. 当前用户基本信息
  // 2. 当前用户的邀请人信息（若存在）
  // 3. 当前用户邀请的下游用户列表（分页）
  const loadRelations = async (targetPage = page, targetPageSize = pageSize) => {
    if (!user?.id) {
      return;
    }
    const requestId = latestRequestRef.current + 1;
    latestRequestRef.current = requestId;
    setLoading(true);
    try {
      const res = await API.get(`/api/user/${user.id}/invite-relations`, {
        params: {
          p: targetPage,
          page_size: targetPageSize,
        },
      });
      const { success, message, data } = res.data || {};
      if (latestRequestRef.current !== requestId) {
        return;
      }
      if (!success) {
        showError(message || t('加载失败'));
        return;
      }
      setRelations({
        user: data?.user || null,
        inviter: data?.inviter || null,
        invitees: data?.invitees || {
          items: [],
          total: 0,
          page: targetPage,
          page_size: targetPageSize,
        },
        invite_income_summary: data?.invite_income_summary || {
          direct_total_quota: 0,
          recharge_total_quota: 0,
          total_quota: 0,
        },
      });
    } catch (error) {
      if (latestRequestRef.current === requestId) {
        showError(t('请求失败'));
      }
    } finally {
      if (latestRequestRef.current === requestId) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (!visible) {
      latestRequestRef.current += 1;
      setRelations(initialRelations);
      setLoading(false);
      return;
    }
    // 每次打开抽屉重置分页，确保不同用户之间不会共享上一次翻页状态。
    setPage(1);
    setPageSize(10);
    setRelations(initialRelations);
    loadRelations(1, 10);
  }, [visible, user?.id]);

  const columns = useMemo(
    () => [
      {
        title: 'ID',
        dataIndex: 'id',
        width: 90,
      },
      {
        title: t('用户名'),
        dataIndex: 'username',
      },
      {
        title: t('显示名称'),
        dataIndex: 'display_name',
        render: (text) => text || '-',
      },
      {
        title: t('分组'),
        dataIndex: 'group',
        render: (group) => renderGroup(group),
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 110,
        render: (status) => {
          if (status === 1) {
            return (
              <Tag color='green' shape='circle' size='small'>
                {t('已启用')}
              </Tag>
            );
          }
          if (status === 2) {
            return (
              <Tag color='red' shape='circle' size='small'>
                {t('已禁用')}
              </Tag>
            );
          }
          return (
            <Tag color='grey' shape='circle' size='small'>
              {t('未知状态')}
            </Tag>
          );
        },
      },
      {
        // 余额展示的是该被邀请人的当前剩余额度（quota 字段）。
        title: t('余额'),
        dataIndex: 'quota',
        render: (quota) => renderQuota(quota || 0),
      },
      {
        // “产生收益”按你要求展示总和：
        // 总收益 = 直接收益（配置口径） + 充值返佣（台账结算口径）。
        title: t('产生收益'),
        dataIndex: 'invite_total_income_quota',
        render: (totalIncome, record) => {
          const direct = Number(record?.invite_direct_income_quota || 0);
          const recharge = Number(record?.invite_recharge_commission_quota || 0);
          const total = Number(totalIncome || 0);
          const tooltipContent = (
            <div style={{ whiteSpace: 'nowrap', lineHeight: 1.4 }}>
              {t('直接收益')}: {renderQuota(direct)} + {t('充值返佣')}:{' '}
              {renderQuota(recharge)}
            </div>
          );
          return (
            <Tooltip content={tooltipContent} position='top'>
              <Tag color='white' shape='circle'>
                {renderQuota(total)}
              </Tag>
            </Tooltip>
          );
        },
      },
    ],
    [t],
  );

  const invitees = (relations?.invitees?.items || []).map((item) => ({
    ...item,
    key: item?.id,
  }));
  const inviteIncomeSummary = relations?.invite_income_summary || {
    direct_total_quota: 0,
    recharge_total_quota: 0,
    total_quota: 0,
  };
  const inviteesTotal = Number(relations?.invitees?.total || 0);
  const relationUser = relations?.user || user || null;
  const relationInviter = relations?.inviter || null;
  const inviterDisplayName = relationInviter
    ? `${relationInviter?.id} / ${relationInviter?.username || '-'}`
    : '';

  return (
    <SideSheet
      visible={visible}
      onCancel={onCancel}
      title={t('邀请关系')}
      placement='right'
      width={isMobile ? '100%' : 900}
      bodyStyle={{ padding: 0 }}
    >
      <Spin spinning={loading}>
        <div className='p-3 space-y-3'>
          {visible && user?.id && (
            <Card className='!rounded-2xl'>
              <InvitePromotionStats
                endpoint={`/api/user/${user.id}/invite-promotion`}
              />
            </Card>
          )}
          <Card className='!rounded-2xl shadow-sm border-0'>
            <div className='flex flex-col gap-2'>
              {canGoBack ? (
                <div>
                  <Button
                    type='tertiary'
                    theme='borderless'
                    size='small'
                    className='!px-0'
                    onClick={onBack}
                  >
                    {t('返回上一级')}
                  </Button>
                </div>
              ) : null}
              <Text strong>
                {t('当前用户')}：{relationUser?.id || '-'} /{' '}
                {relationUser?.username || '-'}
              </Text>
              {relationInviter ? (
                <Text>
                  {t('邀请人')}：
                  <Button
                    type='tertiary'
                    theme='borderless'
                    size='small'
                    className='!px-0'
                    onClick={() => onNavigateUser?.(relationInviter)}
                  >
                    {inviterDisplayName}
                  </Button>
                </Text>
              ) : (
                <Text type='tertiary'>{t('无邀请人')}</Text>
              )}
              <Space>
                <Tag color='white' shape='circle'>
                  {t('直接邀请人数')}：{relationUser?.aff_count || 0}
                </Tag>
                <Tag color='white' shape='circle'>
                  {t('本次列表总数')}：{inviteesTotal}
                </Tag>
                <Tag color='white' shape='circle'>
                  {t('直接收益汇总')}：{renderQuota(
                    inviteIncomeSummary.direct_total_quota || 0,
                  )}
                </Tag>
                <Tag color='white' shape='circle'>
                  {t('充值返佣汇总')}：{renderQuota(
                    inviteIncomeSummary.recharge_total_quota || 0,
                  )}
                </Tag>
                <Tag color='white' shape='circle'>
                  {t('总收益汇总')}：{renderQuota(
                    inviteIncomeSummary.total_quota || 0,
                  )}
                </Tag>
              </Space>
            </div>
          </Card>

          <Card className='!rounded-2xl shadow-sm border-0'>
            <CardTable
              columns={columns}
              dataSource={invitees}
              pagination={{
                currentPage: page,
                pageSize: pageSize,
                total: inviteesTotal,
                pageSizeOpts: [10, 20, 50, 100],
                showSizeChanger: true,
                onPageChange: (nextPage) => {
                  setPage(nextPage);
                  loadRelations(nextPage, pageSize);
                },
                onPageSizeChange: (nextPageSize) => {
                  setPage(1);
                  setPageSize(nextPageSize);
                  loadRelations(1, nextPageSize);
                },
              }}
              size='middle'
              hidePagination={false}
              empty={
                <Empty
                  image={
                    <IllustrationNoResult style={{ width: 150, height: 150 }} />
                  }
                  darkModeImage={
                    <IllustrationNoResultDark
                      style={{ width: 150, height: 150 }}
                    />
                  }
                  description={t('暂无被邀请用户')}
                  style={{ padding: 30 }}
                />
              }
            />
          </Card>
        </div>
      </Spin>
    </SideSheet>
  );
};

export default UserInviteRelationsSheet;
