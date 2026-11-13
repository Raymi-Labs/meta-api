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
  Input,
  InputNumber,
  Modal,
  Select,
  Table,
  Tag,
} from '@douyinfe/semi-ui';
import {
  ENTERPRISE_REQUEST_STATUS,
  ENTERPRISE_REQUEST_STATUS_COLOR,
  enterpriseRequest,
  formatEnterpriseTime,
} from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../helpers/quota';
import { showError, showSuccess } from '../../helpers/toast';
import './enterpriseMembers.css';

function DecideModal({
  enterpriseId,
  mode,
  request,
  unallocated,
  onClose,
  onDone,
}) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState();
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const approve = mode === 'approve';
  const full = request ? quotaToDisplayAmount(request.amount) : undefined;

  useEffect(() => {
    setAmount(full);
    setNote('');
  }, [request?.id, mode]);

  if (!request) return null;

  const submit = async () => {
    const body = { note: note.trim() };
    if (approve) {
      const quota = displayAmountToQuota(amount);
      if (quota <= 0) {
        showError(t('请填写金额'));
        return;
      }
      // 金额没改就传 0，由后端按申请金额全额批准，避免换算取整多出 1 个单位。
      body.amount = amount === full ? 0 : Math.min(quota, request.amount);
    }
    setBusy(true);
    try {
      await enterpriseRequest(
        'post',
        `/api/enterprises/${enterpriseId}/quota-requests/${request.id}/${mode}`,
        body,
      );
      showSuccess(t('已完成'));
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
      title={`${approve ? t('批准') : t('拒绝')} · ${request.member_name}`}
      visible
      okText={approve ? t('批准') : t('拒绝')}
      okButtonProps={approve ? undefined : { type: 'danger' }}
      confirmLoading={busy}
      onOk={submit}
      onCancel={onClose}
    >
      <div className='ec-field'>
        <span className='ec-field-head'>
          {t('申请金额')}
          <em>{renderQuota(request.amount)}</em>
        </span>
        {request.reason && <span>{request.reason}</span>}
      </div>
      {approve && (
        <label className='ec-field'>
          <span className='ec-field-head'>
            {t('批准金额')}
            <em>
              {t('未分配余额 {{amount}}', { amount: renderQuota(unallocated) })}
            </em>
          </span>
          <InputNumber
            min={0}
            max={full}
            precision={2}
            value={amount}
            onChange={setAmount}
            style={{ width: '100%' }}
          />
        </label>
      )}
      <label className='ec-field'>
        {t('备注')}
        <Input maxLength={200} value={note} onChange={setNote} />
      </label>
    </Modal>
  );
}

export default function EnterpriseApprovals() {
  const { t } = useTranslation();
  const { enterpriseId, enterprise, role, memberId, unallocated, refreshSelf } =
    useOutletContext();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [status, setStatus] = useState('pending');
  const [loading, setLoading] = useState(false);
  const [dialog, setDialog] = useState(null);

  const loadSeq = useRef(0);
  const load = useCallback(async () => {
    const sequence = ++loadSeq.current;
    setLoading(true);
    try {
      const data = await enterpriseRequest(
        'get',
        `/api/enterprises/${enterpriseId}/quota-requests`,
        undefined,
        {
          p: page,
          page_size: pageSize,
          status: status === 'all' ? '' : status,
        },
      );
      if (sequence !== loadSeq.current) return;
      setItems(data?.items || []);
      setTotal(data?.total || 0);
    } catch (e) {
      if (sequence === loadSeq.current) showError(e.message);
    } finally {
      if (sequence === loadSeq.current) setLoading(false);
    }
  }, [enterpriseId, page, pageSize, status]);

  useEffect(() => {
    load();
    return () => {
      loadSeq.current++;
    };
  }, [load]);

  const refresh = () => {
    load();
    refreshSelf();
  };
  // Owner可处理包括本人在内的申请；Admin只能处理其他普通成员的申请。
  const actionable = (r) =>
    r.status === 'pending' &&
    (role === 'owner' ||
      (role === 'admin' &&
        r.member_id !== memberId &&
        r.member_role === 'member'));

  const columns = [
    {
      title: t('成员'),
      dataIndex: 'member_name',
      render: (_, r) => (
        <div className='em-who'>
          <strong>{r.member_name}</strong>
          <span>{r.member_email}</span>
        </div>
      ),
    },
    {
      title: t('申请金额'),
      dataIndex: 'amount',
      align: 'right',
      render: (value, r) => (
        <div className='em-who'>
          <strong className='em-num'>{renderQuota(value)}</strong>
        </div>
      ),
    },
    {
      title: t('原因'),
      dataIndex: 'reason',
      render: (value) => <div className='em-reason'>{value || '—'}</div>,
    },
    {
      title: t('可用额度'),
      dataIndex: 'member_available',
      align: 'right',
      render: (value, r) => (
        <div className='em-who'>
          <strong className={`em-num${value < 0 ? ' neg' : ''}`}>
            {renderQuota(value)}
          </strong>
          <span className='em-num'>
            {t('已消费')} {renderQuota(r.member_consumed)}
          </span>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (_, r) => {
        const s = r.status;
        const trail = [
          s === 'approved' ? renderQuota(r.approved_amount) : '',
          r.reviewer_name,
          r.review_note,
        ]
          .filter(Boolean)
          .join(' · ');
        return (
          <div className='em-who'>
            <strong>
              <Tag color={ENTERPRISE_REQUEST_STATUS_COLOR[s] || 'grey'}>
                {t(ENTERPRISE_REQUEST_STATUS[s] || s)}
              </Tag>
            </strong>
            {trail && <span>{trail}</span>}
          </div>
        );
      },
    },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (value) => (
        <span className='em-num'>
          {formatEnterpriseTime(value, enterprise.timezone)}
        </span>
      ),
    },
    {
      title: '',
      dataIndex: 'id',
      align: 'right',
      render: (_, r) =>
        actionable(r) ? (
          <div className='em-actions'>
            <Button
              size='small'
              theme='borderless'
              type='primary'
              onClick={() => setDialog({ mode: 'approve', request: r })}
            >
              {t('批准')}
            </Button>
            <Button
              size='small'
              theme='borderless'
              type='danger'
              onClick={() => setDialog({ mode: 'reject', request: r })}
            >
              {t('拒绝')}
            </Button>
          </div>
        ) : null,
    },
  ];

  return (
    <div className='em'>
      <div className='em-head'>
        <h1>{t('审批')}</h1>
        <div className='em-tools'>
          <Select
            aria-label={t('状态')}
            value={status}
            optionList={[
              { value: 'pending', label: t('待审批') },
              { value: 'all', label: t('全部') },
            ]}
            onChange={(v) => {
              setPage(1);
              setStatus(v);
            }}
          />
        </div>
      </div>

      <div className='em-card'>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={items}
          loading={loading}
          empty={t('没有待办')}
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

      <DecideModal
        enterpriseId={enterpriseId}
        mode={dialog?.mode}
        request={dialog?.request}
        unallocated={Number(unallocated) || 0}
        onClose={() => setDialog(null)}
        onDone={refresh}
      />
    </div>
  );
}
