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
import { Navigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  InputNumber,
  Modal,
  Table,
  Tag,
  TextArea,
  Tooltip,
} from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import { useEnterpriseSelf } from '../../hooks/enterprise/useEnterpriseSelf';
import {
  enterpriseRequest,
  ENTERPRISE_ROLE_LABEL,
  ENTERPRISE_REQUEST_STATUS,
  ENTERPRISE_REQUEST_STATUS_COLOR,
  formatEnterpriseDate,
} from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { displayAmountToQuota } from '../../helpers/quota';
import { showError, showSuccess } from '../../helpers/toast';
import './enterpriseQuota.css';

export default function EnterpriseQuota() {
  const { t } = useTranslation();
  const ent = useEnterpriseSelf();
  const [requests, setRequests] = useState([]);
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState();
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  const loadRequests = useCallback(async () => {
    try {
      const page = await enterpriseRequest(
        'get',
        '/api/enterprise/self/requests',
        undefined,
        { p: 1, page_size: 20 },
      );
      setRequests(page?.items || []);
    } catch (e) {
      showError(e.message);
    }
  }, []);

  useEffect(() => {
    if (ent.isMember) loadRequests();
  }, [ent.isMember, loadRequests]);

  const reload = () => Promise.all([ent.refresh(), loadRequests()]);

  const submit = async () => {
    const quota = displayAmountToQuota(amount);
    if (quota <= 0) {
      showError(t('请填写金额'));
      return;
    }
    setBusy(true);
    try {
      await enterpriseRequest('post', '/api/enterprise/self/requests', {
        amount: quota,
        reason: reason.trim(),
      });
      setOpen(false);
      setAmount(undefined);
      setReason('');
      await reload();
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const cancel = async (id) => {
    try {
      await enterpriseRequest(
        'post',
        `/api/enterprise/self/requests/${id}/cancel`,
      );
      showSuccess(t('已撤销'));
      await reload();
    } catch (e) {
      showError(e.message);
    }
  };

  if (!ent.enabled || ent.loading) return null;
  if (ent.error)
    return (
      <div className='p-6' role='alert'>
        <p>{t(ent.error)}</p>
        <Button onClick={ent.refresh}>{t('重试')}</Button>
      </div>
    );
  if (!ent.isMember) return <Navigate to='/console/token' replace />;

  const { enterprise, member, models, pending_request: pending } = ent.data;
  const columns = [
    {
      title: t('日期'),
      dataIndex: 'created_at',
      render: (v) => formatEnterpriseDate(v),
    },
    { title: t('用途'), dataIndex: 'reason', render: (v) => v || '—' },
    {
      title: t('申请'),
      dataIndex: 'amount',
      render: (v) => renderQuota(v),
    },
    {
      title: t('批准'),
      dataIndex: 'approved_amount',
      render: (v, r) => (r.status === 'approved' ? renderQuota(v) : '—'),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (v) => (
        <Tag color={ENTERPRISE_REQUEST_STATUS_COLOR[v] || 'grey'}>
          {t(ENTERPRISE_REQUEST_STATUS[v] || v)}
        </Tag>
      ),
    },
  ];

  return (
    <div className='enterprise-quota'>
      <div className='enterprise-quota-header'>
        <div className='enterprise-quota-title'>
          <h1>{t('我的额度')}</h1>
          <Tag size='large'>
            {enterprise.name} ·{' '}
            {t(ENTERPRISE_ROLE_LABEL[member.role] || member.role)}
          </Tag>
        </div>
        <Button
          theme='solid'
          type='primary'
          disabled={!!pending}
          onClick={() => setOpen(true)}
        >
          {t('申请额度')}
        </Button>
      </div>

      {pending && (
        <div className='enterprise-quota-pending'>
          <span>
            {t('申请 {{amount}} 待审批', {
              amount: renderQuota(pending.amount),
            })}
          </span>
          <Button size='small' onClick={() => cancel(pending.id)}>
            {t('撤销')}
          </Button>
        </div>
      )}

      <div className='enterprise-quota-card enterprise-quota-main'>
        <div
          className={`enterprise-quota-figure primary${member.available <= 0 ? ' low' : ''}`}
        >
          <span>{t('可用额度')}</span>
          <strong>{renderQuota(member.available)}</strong>
        </div>
        <div className='enterprise-quota-figure'>
          <span>
            {t('在途')}
            <Tooltip
              content={t('正在进行的请求暂时冻结的金额，结束后多退少补')}
            >
              <Info size={14} aria-label={t('在途')} />
            </Tooltip>
          </span>
          <strong>{renderQuota(member.reserved)}</strong>
        </div>
        <div className='enterprise-quota-figure'>
          <span>{t('已消费')}</span>
          <strong>{renderQuota(member.consumed)}</strong>
        </div>
      </div>

      {Array.isArray(models) && models.length > 0 && (
        <div className='enterprise-quota-card'>
          <h2>{t('可用模型')}</h2>
          <div className='enterprise-quota-models'>
            {models.map((name) => (
              <Tag key={name}>{name}</Tag>
            ))}
          </div>
        </div>
      )}

      <div className='enterprise-quota-card'>
        <h2>{t('我的申请')}</h2>
        <Table
          columns={columns}
          dataSource={requests}
          rowKey='id'
          pagination={false}
          empty={t('还没有申请')}
          size='small'
        />
      </div>

      <Modal
        className='ec-modal'
        title={t('申请额度')}
        visible={open}
        okText={t('提交申请')}
        confirmLoading={busy}
        onOk={submit}
        onCancel={() => setOpen(false)}
      >
        <label className='enterprise-quota-field'>
          {t('金额')}
          <InputNumber
            min={0}
            precision={2}
            value={amount}
            onChange={setAmount}
            style={{ width: '100%' }}
          />
        </label>
        <label className='enterprise-quota-field'>
          {t('用途')}
          <TextArea
            rows={3}
            maxCount={200}
            value={reason}
            onChange={setReason}
          />
        </label>
      </Modal>
    </div>
  );
}
