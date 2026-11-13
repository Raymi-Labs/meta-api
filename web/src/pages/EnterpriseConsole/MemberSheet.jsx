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
  Modal,
  Radio,
  RadioGroup,
  Select,
  SideSheet,
  Table,
  Tabs,
  Tag,
} from '@douyinfe/semi-ui';
import {
  enterpriseRequest,
  formatEnterpriseTime,
} from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { showError, showSuccess } from '../../helpers/toast';

const TOKEN_STATUS = { 1: '已启用', 2: '已禁用', 3: '已过期', 4: '已耗尽' };
const LEDGER_KIND = {
  credit: '入账',
  adjust: '调账',
  allocate: '分配',
  reclaim: '回收',
  invite_grant: '邀请',
  request_grant: '申请',
  member_sweep: '停用',
};
const ACTOR = { platform: '平台', system: '系统' };

function ModelScope({ base, member, editable, onDone }) {
  const { t } = useTranslation();
  const [mode, setMode] = useState(member.model_mode || 'inherit');
  const [models, setModels] = useState(member.models || []);
  const [options, setOptions] = useState([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    // 成员能选的范围不超过企业自己的范围。
    Promise.all([
      enterpriseRequest('get', `${base}/model-options`),
      enterpriseRequest('get', `${base}/overview`),
    ])
      .then(([all, overview]) => {
        const ent = overview.enterprise;
        const allowed =
          ent.model_mode === 'custom' ? new Set(ent.models || []) : null;
        setOptions((all || []).filter((m) => !allowed || allowed.has(m)));
      })
      .catch((e) => showError(e.message));
  }, [base]);

  const save = async () => {
    // 指定模型但一个都不选，等于禁用全部模型，这里拦下来。
    if (mode === 'custom' && models.length === 0) {
      showError(t('请选择模型'));
      return;
    }
    setBusy(true);
    try {
      await enterpriseRequest('put', `${base}/members/${member.id}/models`, {
        model_mode: mode,
        models: mode === 'custom' ? models : [],
      });
      showSuccess(t('已保存'));
      onDone();
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className='ec-field'>
      <RadioGroup value={mode} onChange={(e) => setMode(e.target.value)}>
        <Radio value='inherit' disabled={!editable}>
          {t('跟随企业')}
        </Radio>
        <Radio value='custom' disabled={!editable}>
          {t('指定模型')}
        </Radio>
      </RadioGroup>
      {mode === 'custom' && (
        <Select
          multiple
          filter
          disabled={!editable}
          aria-label={t('选择模型')}
          placeholder={t('选择模型')}
          value={models}
          optionList={options.map((m) => ({ value: m, label: m }))}
          onChange={setModels}
          style={{ width: '100%' }}
        />
      )}
      {editable && (
        <div>
          <Button theme='solid' type='primary' loading={busy} onClick={save}>
            {t('保存')}
          </Button>
        </div>
      )}
    </div>
  );
}

function PagedTable({ url, params, columns, reloadKey }) {
  const { t } = useTranslation();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const memberId = params.member_id;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await enterpriseRequest('get', url, undefined, {
        member_id: memberId,
        p: page,
        page_size: 10,
      });
      setItems(data?.items || []);
      setTotal(data?.total || 0);
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  }, [url, memberId, page]);

  useEffect(() => {
    load();
  }, [load, reloadKey]);

  return (
    <Table
      rowKey='id'
      size='small'
      columns={columns}
      dataSource={items}
      loading={loading}
      empty={t('没有记录')}
      pagination={
        total > 10
          ? { currentPage: page, pageSize: 10, total, onPageChange: setPage }
          : false
      }
    />
  );
}

export default function MemberSheet({
  enterpriseId,
  member,
  timezone,
  editable,
  onClose,
  onDone,
}) {
  const { t } = useTranslation();
  const [reloadKey, setReloadKey] = useState(0);
  if (!member) return null;
  const base = `/api/enterprises/${enterpriseId}`;

  const revoke = (token) => {
    Modal.confirm({
      className: 'ec-modal',
      title: t('撤销令牌 {{name}}？', { name: token.name }),
      content: t('撤销后该令牌立即失效，且无法恢复。'),
      okText: t('撤销'),
      okButtonProps: { type: 'danger' },
      onOk: async () => {
        try {
          await enterpriseRequest(
            'post',
            `${base}/tokens/${token.id}/revoke`,
            {},
          );
          showSuccess(t('已撤销'));
          setReloadKey((k) => k + 1);
        } catch (e) {
          showError(e.message);
        }
      },
    });
  };

  const tokenColumns = [
    {
      title: t('令牌'),
      dataIndex: 'name',
      render: (v, r) => (
        <div className='em-who'>
          <strong>{v}</strong>
          <span>{r.masked_key}</span>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (v) => (
        <Tag color={v === 1 ? 'green' : 'grey'}>
          {t(TOKEN_STATUS[v] || '已禁用')}
        </Tag>
      ),
    },
    {
      title: t('已用额度'),
      dataIndex: 'used_quota',
      align: 'right',
      render: (v) => <span className='em-num'>{renderQuota(v)}</span>,
    },
    {
      title: '',
      dataIndex: 'id',
      align: 'right',
      render: (_, r) =>
        editable ? (
          <Button
            size='small'
            theme='borderless'
            type='danger'
            onClick={() => revoke(r)}
          >
            {t('撤销')}
          </Button>
        ) : null,
    },
  ];

  const ledgerColumns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => (
        <span className='em-num'>{formatEnterpriseTime(v, timezone)}</span>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'kind',
      render: (v, r) => (
        <div className='em-who'>
          <strong>{t(LEDGER_KIND[v] || v)}</strong>
          <span>{r.actor_name || t(ACTOR[r.actor_role] || '—')}</span>
        </div>
      ),
    },
    {
      title: t('金额'),
      dataIndex: 'member_delta',
      align: 'right',
      render: (v) => (
        <span className={`em-num${v < 0 ? ' neg' : ''}`}>
          {v > 0 ? '+' : ''}
          {renderQuota(v)}
        </span>
      ),
    },
    {
      title: t('可用额度'),
      dataIndex: 'member_after',
      align: 'right',
      render: (v) => <span className='em-num'>{renderQuota(v)}</span>,
    },
  ];

  return (
    <SideSheet
      title={member.display_name || member.username}
      visible
      width='min(560px, 100vw)'
      closeOnEsc
      onCancel={onClose}
    >
      <Tabs type='line'>
        <Tabs.TabPane itemKey='models' tab={t('可用模型')}>
          <div className='em-pane'>
            <ModelScope
              key={member.id}
              base={base}
              member={member}
              editable={editable}
              onDone={onDone}
            />
          </div>
        </Tabs.TabPane>
        <Tabs.TabPane itemKey='tokens' tab={t('令牌')}>
          <div className='em-pane'>
            <PagedTable
              url={`${base}/tokens`}
              params={{ member_id: member.id }}
              columns={tokenColumns}
              reloadKey={reloadKey}
            />
          </div>
        </Tabs.TabPane>
        <Tabs.TabPane itemKey='ledger' tab={t('额度流水')}>
          <div className='em-pane'>
            <PagedTable
              url={`${base}/ledger`}
              params={{ member_id: member.id }}
              columns={ledgerColumns}
            />
          </div>
        </Tabs.TabPane>
      </Tabs>
    </SideSheet>
  );
}
