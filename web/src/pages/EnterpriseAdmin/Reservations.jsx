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
  Input,
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
  Table,
} from '@douyinfe/semi-ui';
import {
  enterpriseRequest,
  formatEnterpriseTime,
} from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../helpers/quota';
import { showError, showSuccess } from '../../helpers/toast';

function ResolveModal({ reservation, onClose, onDone }) {
  const { t } = useTranslation();
  const [settle, setSettle] = useState(true);
  const [amount, setAmount] = useState();
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const full = reservation ? quotaToDisplayAmount(reservation.quota) : 0;

  useEffect(() => {
    setSettle(true);
    setAmount(full);
    setReason('');
  }, [reservation?.id]);

  if (!reservation) return null;

  const submit = async () => {
    // 金额留空不能当成 0 结算，否则等于悄悄全额退回。
    if (settle && typeof amount !== 'number') return showError(t('请填写金额'));
    if (!reason.trim()) return showError(t('请填写原因'));
    // 金额没改就原样传预留额，避免换算取整差 1 个单位。
    const quota =
      amount === full ? reservation.quota : displayAmountToQuota(amount);
    setBusy(true);
    try {
      await enterpriseRequest(
        'post',
        `/api/enterprise/admin/reservations/${reservation.id}/resolve`,
        { settle, amount: settle ? quota : 0, reason: reason.trim() },
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
      title={`${t('处理预留')} · ${reservation.enterprise_name}`}
      visible
      okText={settle ? t('结算') : t('退回')}
      okButtonProps={{ type: 'danger' }}
      confirmLoading={busy}
      onOk={submit}
      onCancel={onClose}
    >
      <div className='ec-field'>
        <span className='ec-field-head'>
          {reservation.member_name}
          <em>
            {t('已预留 {{amount}}', { amount: renderQuota(reservation.quota) })}
          </em>
        </span>
        <RadioGroup
          type='button'
          aria-label={t('处理方式')}
          value={settle}
          onChange={(e) => setSettle(e.target.value)}
        >
          <Radio value={true}>{t('按金额结算')}</Radio>
          <Radio value={false}>{t('全部退回成员')}</Radio>
        </RadioGroup>
      </div>
      {settle && (
        <label className='ec-field'>
          {t('实际扣费')}
          <InputNumber
            min={0}
            precision={2}
            value={amount}
            onChange={setAmount}
            style={{ width: '100%' }}
          />
        </label>
      )}
      <label className='ec-field'>
        {t('原因')}
        <Input maxLength={200} value={reason} onChange={setReason} />
      </label>
      <p className='ea-warn'>{t('处理后不能更改。')}</p>
    </Modal>
  );
}

// 结果不明的请求留下的预留：上游是否扣费无法自动判断，由平台核对后结算或退回。
export default function Reservations({ canResolve, onCount, onDone }) {
  const { t } = useTranslation();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [target, setTarget] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await enterpriseRequest(
        'get',
        '/api/enterprise/admin/reservations',
        undefined,
        { p: page, page_size: 20 },
      );
      setItems(data?.items || []);
      setTotal(data?.total || 0);
      onCount(data?.total || 0);
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  }, [page, onCount]);

  useEffect(() => {
    load();
  }, [load]);

  const columns = [
    {
      title: t('企业'),
      dataIndex: 'enterprise_name',
      render: (v, r) => (
        <div className='em-who'>
          <strong>{v}</strong>
          <span>{r.member_name}</span>
        </div>
      ),
    },
    {
      title: t('模型'),
      dataIndex: 'model_name',
      render: (v, r) => (
        <div className='em-who'>
          <strong>{v || '—'}</strong>
          <span>{r.token_name}</span>
        </div>
      ),
    },
    {
      title: t('请求 ID'),
      dataIndex: 'request_id',
      render: (v) => <span className='em-reason'>{v}</span>,
    },
    {
      title: t('预留金额'),
      dataIndex: 'quota',
      align: 'right',
      render: (v) => <span className='em-num'>{renderQuota(v)}</span>,
    },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => <span className='em-num'>{formatEnterpriseTime(v)}</span>,
    },
    {
      title: '',
      dataIndex: 'id',
      align: 'right',
      render: (_, r) =>
        canResolve ? (
          <Button
            size='small'
            theme='borderless'
            type='primary'
            onClick={() => setTarget(r)}
          >
            {t('处理')}
          </Button>
        ) : null,
    },
  ];

  return (
    <>
      <div className='em-card'>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={items}
          loading={loading}
          empty={t('没有待对账的预留')}
          pagination={
            total > 20
              ? {
                  currentPage: page,
                  pageSize: 20,
                  total,
                  onPageChange: setPage,
                }
              : false
          }
        />
      </div>
      <ResolveModal
        reservation={target}
        onClose={() => setTarget(null)}
        onDone={() => {
          load();
          onDone();
        }}
      />
    </>
  );
}
