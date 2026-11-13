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

import React, { useCallback, useEffect, useState, useRef } from 'react';
import { useOutletContext } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Dropdown,
  Input,
  Modal,
  Select,
  Table,
  Tag,
} from '@douyinfe/semi-ui';
import { Search, UserPlus } from 'lucide-react';
import {
  ENTERPRISE_ROLE_LABEL,
  enterpriseRequest,
} from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { showError, showSuccess } from '../../helpers/toast';
import AmountModal from './AmountModal';
import InviteModal from './InviteModal';
import MemberSheet from './MemberSheet';
import InvitationList from './InvitationList';
import './enterpriseMembers.css';

export default function EnterpriseMembers() {
  const { t } = useTranslation();
  const { enterpriseId, enterprise, role, unallocated, refreshSelf } =
    useOutletContext();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [status, setStatus] = useState('active');
  const [keyword, setKeyword] = useState('');
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(false);
  const [dialog, setDialog] = useState(null);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [sheet, setSheet] = useState(null);
  const [version, setVersion] = useState(0);

  const loadSeq = useRef(0);
  const load = useCallback(async () => {
    const sequence = ++loadSeq.current;
    setLoading(true);
    try {
      const data = await enterpriseRequest(
        'get',
        `/api/enterprises/${enterpriseId}/members`,
        undefined,
        { p: page, page_size: pageSize, status, keyword },
      );
      if (sequence !== loadSeq.current) return;
      setItems(data?.items || []);
      setTotal(data?.total || 0);
    } catch (e) {
      if (sequence === loadSeq.current) showError(e.message);
    } finally {
      if (sequence === loadSeq.current) setLoading(false);
    }
  }, [enterpriseId, page, pageSize, status, keyword]);

  useEffect(() => {
    load();
    return () => {
      loadSeq.current++;
    };
  }, [load]);

  const refresh = () => {
    load();
    refreshSelf();
    setVersion((v) => v + 1);
  };
  const search = () => {
    setPage(1);
    setKeyword(draft.trim());
  };
  const manageable = (m) =>
    m.status === 'active' && (role === 'owner' || m.role === 'member');

  const disable = (m) => {
    const name = m.display_name || m.username;
    Modal.confirm({
      className: 'ec-modal',
      title: t('停用成员 {{name}}？', { name }),
      content:
        m.available < 0
          ? t(
              '停用后该成员的令牌立即失效，待审批的申请作废，欠额 {{amount}} 从未分配余额扣除。',
              { amount: renderQuota(-m.available) },
            )
          : t(
              '停用后该成员的令牌立即失效，待审批的申请作废，剩余额度退回未分配余额。',
            ),
      okText: t('停用'),
      okButtonProps: { type: 'danger' },
      onOk: async () => {
        try {
          await enterpriseRequest(
            'post',
            `/api/enterprises/${enterpriseId}/members/${m.id}/disable`,
            {},
          );
          showSuccess(t('已停用'));
          refresh();
        } catch (e) {
          showError(e.message);
        }
      },
    });
  };

  const setMemberRole = async (m, next) => {
    try {
      await enterpriseRequest(
        'put',
        `/api/enterprises/${enterpriseId}/members/${m.id}/role`,
        { role: next },
      );
      showSuccess(t('已保存'));
      refresh();
    } catch (e) {
      showError(e.message);
    }
  };

  const transfer = (m) => {
    const name = m.display_name || m.username;
    Modal.confirm({
      className: 'ec-modal',
      title: t('将所有权转移给 {{name}}？', { name }),
      content: t(
        '转移后你将成为管理员，只有新所有者可以修改企业设置和调整角色。',
      ),
      okText: t('转移所有权'),
      okButtonProps: { type: 'danger' },
      onOk: async () => {
        try {
          await enterpriseRequest(
            'post',
            `/api/enterprises/${enterpriseId}/members/${m.id}/transfer-ownership`,
            {},
          );
          showSuccess(t('已完成'));
          setSheet(null);
          refresh();
        } catch (e) {
          showError(e.message);
        }
      },
    });
  };

  const STATUS_LABEL = { disabled: t('已停用'), left: t('已退出') };

  const columns = [
    {
      title: t('成员'),
      dataIndex: 'username',
      render: (_, m) => (
        <div className='em-who'>
          <strong>
            {m.display_name || m.username}
            {m.status !== 'active' && (
              <Tag size='small'>{STATUS_LABEL[m.status] || m.status}</Tag>
            )}
          </strong>
          <span>{m.email}</span>
        </div>
      ),
    },
    {
      title: t('角色'),
      dataIndex: 'role',
      width: 110,
      render: (value) => (
        <Tag color={value === 'member' ? 'white' : 'blue'}>
          {t(ENTERPRISE_ROLE_LABEL[value] || value)}
        </Tag>
      ),
    },
    {
      title: t('可用额度'),
      dataIndex: 'available',
      align: 'right',
      render: (value) => (
        <span className={`em-num${value < 0 ? ' neg' : ''}`}>
          {renderQuota(value)}
        </span>
      ),
    },
    {
      title: t('已消费'),
      dataIndex: 'consumed',
      align: 'right',
      render: (value) => <span className='em-num'>{renderQuota(value)}</span>,
    },
    {
      title: '',
      dataIndex: 'id',
      align: 'right',
      render: (_, m) => (
        <div className='em-actions'>
          <Button size='small' theme='borderless' onClick={() => setSheet(m)}>
            {t('详情')}
          </Button>
          {manageable(m) && (
            <>
              <Button
                size='small'
                theme='borderless'
                type='primary'
                onClick={() => setDialog({ mode: 'allocate', member: m })}
              >
                {t('分配')}
              </Button>
              <Button
                size='small'
                theme='borderless'
                disabled={m.available <= 0}
                onClick={() => setDialog({ mode: 'reclaim', member: m })}
              >
                {t('回收')}
              </Button>
              {role === 'owner' && m.role !== 'owner' && (
                <Dropdown
                  trigger='click'
                  clickToHide
                  position='bottomRight'
                  menu={[
                    {
                      node: 'item',
                      key: 'role',
                      name:
                        m.role === 'admin' ? t('设为成员') : t('设为管理员'),
                      onClick: () =>
                        setMemberRole(
                          m,
                          m.role === 'admin' ? 'member' : 'admin',
                        ),
                    },
                    {
                      node: 'item',
                      key: 'transfer',
                      name: t('转移所有权'),
                      type: 'danger',
                      onClick: () => transfer(m),
                    },
                  ]}
                >
                  <Button size='small' theme='borderless'>
                    {t('更多')}
                  </Button>
                </Dropdown>
              )}
              {m.role !== 'owner' && (
                <Button
                  size='small'
                  theme='borderless'
                  type='danger'
                  onClick={() => disable(m)}
                >
                  {t('停用')}
                </Button>
              )}
            </>
          )}
        </div>
      ),
    },
  ];

  return (
    <div className='em'>
      <div className='em-head'>
        <h1>{t('成员')}</h1>
        <div className='em-tools'>
          <Input
            prefix={<Search size={16} aria-hidden='true' />}
            aria-label={t('搜索成员')}
            placeholder={t('搜索成员')}
            showClear
            value={draft}
            onChange={setDraft}
            onEnterPress={search}
            onClear={() => {
              setDraft('');
              setPage(1);
              setKeyword('');
            }}
          />
          <Select
            aria-label={t('状态')}
            value={status}
            optionList={[
              { value: 'active', label: t('有效') },
              { value: 'all', label: t('全部') },
            ]}
            onChange={(v) => {
              setPage(1);
              setStatus(v);
            }}
          />
          <Button
            theme='solid'
            type='primary'
            icon={<UserPlus size={16} aria-hidden='true' />}
            onClick={() => setInviteOpen(true)}
          >
            {t('邀请成员')}
          </Button>
        </div>
      </div>

      <div className='em-card'>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={items}
          loading={loading}
          pagination={{
            currentPage: page,
            pageSize,
            total,
            pageSizeOpts: [10, 20, 50],
            showSizeChanger: true,
            onPageChange: setPage,
            onPageSizeChange: (size) => {
              setPage(1);
              setPageSize(size);
            },
          }}
        />
      </div>

      <InvitationList enterpriseId={enterpriseId} version={version} />

      <MemberSheet
        enterpriseId={enterpriseId}
        member={sheet}
        timezone={enterprise.timezone}
        editable={!!sheet && manageable(sheet)}
        onClose={() => setSheet(null)}
        onDone={refresh}
      />
      <AmountModal
        enterpriseId={enterpriseId}
        mode={dialog?.mode}
        member={dialog?.member}
        unallocated={Number(unallocated) || 0}
        onClose={() => setDialog(null)}
        onDone={refresh}
      />
      <InviteModal
        enterpriseId={enterpriseId}
        visible={inviteOpen}
        isOwner={role === 'owner'}
        onClose={() => setInviteOpen(false)}
        onDone={refresh}
      />
    </div>
  );
}
