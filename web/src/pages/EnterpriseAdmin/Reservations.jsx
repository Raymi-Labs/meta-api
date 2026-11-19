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

import React, { useCallback, useEffect, useRef, useState } from 'react';
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
import { renderQuota, getCurrencyConfig } from '../../helpers/render';
import { parseEnterpriseQuota } from '../../helpers/enterpriseQuota';
import { getQuotaPerUnit } from '../../helpers/quota';
import { quotaToDisplayAmount } from '../../helpers/quota';
import { showError } from '../../helpers/toast';
import { getStoredUser } from '../../helpers/storage';
import { listPendingEnterpriseReservations } from '../../helpers/enterpriseOperation';

import { useEnterpriseOperation } from '../../hooks/enterprise/useEnterpriseOperation';
import EnterpriseOperationStatus from '../../components/common/EnterpriseOperationStatus';

export function ResolveModal({ reservation, onClose, onDone }) {
  const { t } = useTranslation();
  const [settle, setSettle] = useState(true);
  const [amount, setAmount] = useState();
  const [reason, setReason] = useState('');
  const [rawUnits, setRawUnits] = useState(false);
  const operation = useEnterpriseOperation({
    scope: `reservation:${reservation?.id}`,
    url: `/api/enterprise/admin/reservations/${reservation?.id}/resolve`,
    lookupBase: reservation
      ? `/api/enterprise/admin/enterprises/${reservation.enterprise_id}`
      : null,
    recoveryContext: reservation
      ? {
          reservation: {
            id: reservation.id,
            enterprise_id: reservation.enterprise_id,
            enterprise_name: reservation.enterprise_name,
            member_name: reservation.member_name,
            request_id: reservation.request_id,
            quota: reservation.quota,
          },
        }
      : undefined,
    onDone,
    onClose,
  });
  const { busy, locked } = operation;
  const full = reservation ? quotaToDisplayAmount(reservation.quota) : 0;

  useEffect(() => {
    setSettle(true);
    setRawUnits(false);
    setAmount(full);
    setReason('');
  }, [reservation?.id]);

  useEffect(() => {
    const data = operation.pending?.data;
    if (!data) return;
    setSettle(data.settle);
    setRawUnits(true);
    setAmount(data.amount);
    setReason(data.reason || '');
  }, [operation.pending]);

  if (!reservation) return null;

  const submit = async () => {
    if (operation.pending) return operation.submit(operation.pending.data);
    if (busy) return;
    // 金额留空不能当成 0 结算，否则等于悄悄全额退回。
    if (settle && typeof amount !== 'number') return showError(t('请填写金额'));
    if (!reason.trim()) return showError(t('请填写原因'));
    // 金额没改就原样传预留额，避免换算取整差 1 个单位。
    const quota =
      !rawUnits && amount === full
        ? reservation.quota
        : parseEnterpriseQuota(amount, {
            raw: rawUnits,
            quotaPerUnit: getQuotaPerUnit(),
            ...getCurrencyConfig(),
          });
    if (settle && (!Number.isSafeInteger(quota) || quota < 0 || quota > 1e15))
      return showError(t('请填写金额'));
    await operation.submit({
      settle,
      amount: settle ? quota : 0,
      reason: reason.trim(),
    });
  };

  return (
    <Modal
      className='ec-modal ec-financial-modal'
      title={`${t('处理预留')} · ${reservation.enterprise_name}`}
      visible
      okText={
        operation.pending ? t('重试原操作') : settle ? t('结算') : t('退回')
      }
      okButtonProps={{ type: 'danger' }}
      confirmLoading={busy}
      onOk={submit}
      onCancel={() => !busy && onClose()}
      maskClosable={!busy}
      closeOnEsc={!busy}
      closable={!busy}
      cancelButtonProps={{ disabled: busy }}
    >
      <EnterpriseOperationStatus operation={operation} />
      <div className='ec-field'>
        <span className='ec-field-head'>
          {reservation.member_name}
          <em>
            {t('已预留 {{amount}}', {
              amount: renderQuota(reservation.quota, 8),
            })}
          </em>
        </span>
        <RadioGroup
          disabled={locked}
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
        <RadioGroup
          type='button'
          aria-label={t('金额单位')}
          value={rawUnits}
          disabled={locked}
          onChange={(e) => {
            setRawUnits(e.target.value);
            setAmount(e.target.value ? reservation.quota : full);
          }}
        >
          <Radio value={false}>{t('金额')}</Radio>
          <Radio value={true}>{t('原始额度')}</Radio>
        </RadioGroup>
      )}
      {settle && (
        <label className='ec-field'>
          {t('实际扣费')}
          <InputNumber
            min={0}
            precision={rawUnits ? 0 : 8}
            aria-label={rawUnits ? t('原始额度') : t('实际扣费')}
            disabled={locked}
            value={amount}
            onChange={setAmount}
            style={{ width: '100%' }}
          />
        </label>
      )}
      <label className='ec-field'>
        {t('原因')}
        <Input
          aria-label={t('原因')}
          disabled={locked}
          maxLength={200}
          value={reason}
          onChange={setReason}
        />
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
  const actorID = getStoredUser()?.id;
  const [recovery, setRecovery] = useState({ actorID: null, items: [] });
  const refreshRecovery = useCallback(() => {
    try {
      setRecovery({
        actorID,
        items: canResolve
          ? listPendingEnterpriseReservations(sessionStorage, actorID)
          : [],
      });
    } catch {
      setRecovery({ actorID, items: [] });
      showError(t('无法恢复操作，请联系管理员核对'));
    }
  }, [actorID, canResolve, t]);

  useEffect(() => {
    refreshRecovery();
  }, [refreshRecovery]);
  const pendingReservations =
    canResolve && recovery.actorID === actorID ? recovery.items : [];

  const [draft, setDraft] = useState({ enterprise_id: '', request_id: '' });
  const [filter, setFilter] = useState({});
  const sequence = useRef(0);
  const [loadError, setLoadError] = useState('');
  const load = useCallback(async () => {
    const seq = ++sequence.current;
    setLoadError('');
    setItems([]);
    setTotal(0);
    setLoading(true);
    try {
      const data = await enterpriseRequest(
        'get',
        '/api/enterprise/admin/reservations',
        undefined,
        { p: page, page_size: 20, ...filter },
      );
      if (seq !== sequence.current) return;
      setItems(data?.items || []);
      setTotal(data?.total || 0);
      if (!filter.enterprise_id && !filter.request_id)
        onCount(data?.total || 0);
    } catch (e) {
      if (seq === sequence.current) setLoadError(e.message);
    } finally {
      if (seq === sequence.current) setLoading(false);
    }
  }, [page, onCount, filter]);

  useEffect(() => {
    load();
    return () => {
      ++sequence.current;
    };
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
      {pendingReservations.length > 0 && (
        <div
          className='ec-pending-operations'
          role='group'
          aria-label={t('结果待确认')}
        >
          {pendingReservations.map((reservation) => (
            <Button
              key={reservation.id}
              theme='light'
              onClick={() => setTarget(reservation)}
            >
              {t('查询结果')} ·{' '}
              {reservation.enterprise_name || reservation.enterprise_id} ·{' '}
              {reservation.request_id || reservation.id}
            </Button>
          ))}
        </div>
      )}
      <form
        className='ea-review-filter'
        onSubmit={(e) => {
          e.preventDefault();
          setFilter(
            Object.fromEntries(
              Object.entries(draft)
                .map(([k, v]) => [k, v.trim()])
                .filter(([, v]) => v),
            ),
          );
          setPage(1);
        }}
      >
        <Input
          aria-label={`${t('企业')} ID`}
          placeholder={`${t('企业')} ID`}
          value={draft.enterprise_id}
          onChange={(v) => setDraft({ ...draft, enterprise_id: v })}
          showClear
        />
        <Input
          aria-label={t('请求 ID')}
          placeholder={t('请求 ID')}
          maxLength={96}
          value={draft.request_id}
          onChange={(v) => setDraft({ ...draft, request_id: v })}
          showClear
        />
        <Button htmlType='submit'>{t('查询')}</Button>
      </form>
      <div className='em-card'>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={items}
          loading={loading}
          empty={
            loadError ? (
              <span role='alert'>
                {loadError} <Button onClick={load}>{t('重试')}</Button>
              </span>
            ) : (
              t('没有待对账的预留')
            )
          }
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
        onClose={() => {
          setTarget(null);
          refreshRecovery();
        }}
        onDone={async () => {
          refreshRecovery();
          await load();
          await onDone();
        }}
      />
    </>
  );
}
