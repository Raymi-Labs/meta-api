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

import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Dropdown,
  Input,
  Modal,
  Table,
  Tabs,
  Tag,
} from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import {
  enterpriseRequest,
  formatEnterpriseDate,
} from '../../helpers/enterprise';
import { isRoot } from '../../helpers/storage';
import { renderQuota } from '../../helpers/render';
import { showError, showSuccess } from '../../helpers/toast';
import EnterpriseFormModal from './EnterpriseFormModal';
import WalletModal from './WalletModal';
import Reservations from './Reservations';
import '../EnterpriseConsole/enterpriseMembers.css';
import './enterpriseAdmin.css';

function StatusModal({ enterprise, onClose, onDone }) {
  const { t } = useTranslation();
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => setReason(''), [enterprise?.id]);
  if (!enterprise) return null;
  const freeze = enterprise.status !== 'frozen';

  const submit = async () => {
    setBusy(true);
    try {
      await enterpriseRequest(
        'post',
        `/api/enterprise/admin/enterprises/${enterprise.id}/status`,
        { status: freeze ? 'frozen' : 'active', reason: reason.trim() },
      );
      showSuccess(freeze ? t('已冻结') : t('已解冻'));
      onDone();
      onClose();
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      className='ec-modal'
      title={
        freeze
          ? t('冻结 {{name}}？', { name: enterprise.name })
          : t('解冻 {{name}}？', { name: enterprise.name })
      }
      visible
      okText={freeze ? t('冻结') : t('解冻')}
      okButtonProps={freeze ? { type: 'danger' } : undefined}
      confirmLoading={busy}
      onOk={submit}
      onCancel={onClose}
    >
      <p className={freeze ? 'ea-warn' : 'ea-note'}>
        {freeze
          ? t(
              '冻结后，该企业的所有企业令牌立即无法发起新请求，待审批的额度申请全部作废。',
            )
          : t('解冻后企业令牌恢复可用，已作废的额度申请不会恢复。')}
      </p>
      <label className='ec-field'>
        {t('备注')}
        <Input maxLength={200} value={reason} onChange={setReason} />
      </label>
    </Modal>
  );
}

export default function EnterpriseAdmin() {
  const { t } = useTranslation();
  const root = isRoot();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [keyword, setKeyword] = useState('');
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [groups, setGroups] = useState([]);
  const [form, setForm] = useState(null);
  const [wallet, setWallet] = useState(null);
  const [status, setStatus] = useState(null);
  const [reviewCount, setReviewCount] = useState(0);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await enterpriseRequest(
        'get',
        '/api/enterprise/admin/enterprises',
        undefined,
        { p: page, page_size: pageSize, keyword: query },
      );
      setItems(data?.items || []);
      setTotal(data?.total || 0);
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, query]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    // “auto” 是按规则自动选组的占位名，不能作为企业的计费用户组。
    enterpriseRequest('get', '/api/group/')
      .then((list) => setGroups((list || []).filter((g) => g !== 'auto')))
      .catch((e) => showError(e.message));
  }, []);

  const search = (value) => {
    setPage(1);
    setQuery(value.trim());
  };

  const columns = [
    {
      title: t('企业'),
      dataIndex: 'name',
      render: (v, r) => (
        <div className='em-who'>
          <strong>
            {v}
            {r.status === 'frozen' && <Tag color='red'>{t('已冻结')}</Tag>}
          </strong>
          <span>{r.owner_name || `#${r.owner_user_id}`}</span>
        </div>
      ),
    },
    { title: t('计费用户组'), dataIndex: 'billing_group' },
    {
      title: t('未分配余额'),
      dataIndex: 'unallocated',
      align: 'right',
      render: (v) => (
        <span className={`em-num${v < 0 ? ' neg' : ''}`}>{renderQuota(v)}</span>
      ),
    },
    {
      title: t('累计入账'),
      dataIndex: 'net_credited',
      align: 'right',
      render: (v) => <span className='em-num'>{renderQuota(v)}</span>,
    },
    {
      title: t('开通日期'),
      dataIndex: 'created_at',
      render: (v) => <span className='em-num'>{formatEnterpriseDate(v)}</span>,
    },
    {
      title: '',
      dataIndex: 'id',
      align: 'right',
      render: (_, r) => (
        <div className='em-actions'>
          {root && (
            <Button
              size='small'
              theme='borderless'
              type='primary'
              onClick={() => setWallet({ mode: 'credit', enterprise: r })}
            >
              {t('入账')}
            </Button>
          )}
          <Button size='small' theme='borderless' onClick={() => setForm(r)}>
            {t('编辑')}
          </Button>
          <Dropdown
            trigger='click'
            clickToHide
            position='bottomRight'
            menu={[
              ...(root
                ? [
                    {
                      node: 'item',
                      key: 'adjust',
                      name: t('调账'),
                      onClick: () =>
                        setWallet({ mode: 'adjust', enterprise: r }),
                    },
                  ]
                : []),
              {
                node: 'item',
                key: 'status',
                name: r.status === 'frozen' ? t('解冻') : t('冻结'),
                type: r.status === 'frozen' ? undefined : 'danger',
                onClick: () => setStatus(r),
              },
            ]}
          >
            <Button size='small' theme='borderless'>
              {t('更多')}
            </Button>
          </Dropdown>
        </div>
      ),
    },
  ];

  return (
    <div className='em ea'>
      <div className='em-head'>
        <h1>{t('企业管理')}</h1>
        <div className='em-tools'>
          <Input
            prefix={<IconSearch />}
            showClear
            aria-label={t('搜索企业')}
            placeholder={t('搜索企业')}
            value={keyword}
            onChange={setKeyword}
            onEnterPress={() => search(keyword)}
            onClear={() => search('')}
          />
          <Button theme='solid' type='primary' onClick={() => setForm({})}>
            {t('开通企业')}
          </Button>
        </div>
      </div>

      <Tabs type='line'>
        <Tabs.TabPane itemKey='list' tab={t('企业')}>
          <div className='em-card ea-pane'>
            <Table
              rowKey='id'
              columns={columns}
              dataSource={items}
              loading={loading}
              empty={query ? t('搜索无结果') : t('还没有企业')}
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
        </Tabs.TabPane>
        <Tabs.TabPane
          itemKey='review'
          tab={
            <span className='ea-tab'>
              {t('待对账')}
              {reviewCount > 0 && (
                <Tag color='orange' size='small'>
                  {reviewCount}
                </Tag>
              )}
            </span>
          }
        >
          <div className='ea-pane'>
            <Reservations
              canResolve={root}
              onCount={setReviewCount}
              onDone={load}
            />
          </div>
        </Tabs.TabPane>
      </Tabs>

      <EnterpriseFormModal
        enterprise={form}
        groups={groups}
        onClose={() => setForm(null)}
        onDone={load}
      />
      <WalletModal
        mode={wallet?.mode}
        enterprise={wallet?.enterprise}
        onClose={() => setWallet(null)}
        onDone={load}
      />
      <StatusModal
        enterprise={status}
        onClose={() => setStatus(null)}
        onDone={load}
      />
    </div>
  );
}
