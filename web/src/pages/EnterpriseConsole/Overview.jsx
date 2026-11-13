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
import { useNavigate, useOutletContext } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button, Tag, Tooltip } from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import { enterpriseRequest } from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { showError } from '../../helpers/toast';
import './enterpriseOverview.css';

export default function EnterpriseOverview() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { enterpriseId } = useOutletContext();
  const [data, setData] = useState(null);
  const [pending, setPending] = useState([]);

  const load = useCallback(async () => {
    try {
      const [overview, requests] = await Promise.all([
        enterpriseRequest('get', `/api/enterprises/${enterpriseId}/overview`),
        enterpriseRequest(
          'get',
          `/api/enterprises/${enterpriseId}/quota-requests`,
          undefined,
          { status: 'pending', p: 1, page_size: 5 },
        ),
      ]);
      setData(overview);
      setPending(requests?.items || []);
    } catch (e) {
      showError(e.message);
    }
  }, [enterpriseId]);

  useEffect(() => {
    load();
  }, [load]);

  if (!data) return null;
  const { balance, top_spenders: spenders = [] } = data;
  const parts = [
    { key: 'unallocated', label: t('未分配'), value: balance.unallocated },
    { key: 'available', label: t('成员可用'), value: balance.member_available },
    {
      key: 'reserved',
      label: t('在途'),
      value: balance.member_reserved,
      hint: t('正在进行的请求暂时冻结的金额，结束后多退少补'),
    },
    { key: 'consumed', label: t('已消费'), value: balance.consumed },
  ];
  const sum = parts.reduce((acc, p) => acc + Math.max(p.value, 0), 0);
  const maxSpent = Math.max(1, ...spenders.map((s) => s.consumed));

  return (
    <div className='eo'>
      <div className='eo-head'>
        <div className='eo-title'>
          <h1>{t('总览')}</h1>
          <Tag size='large'>
            {t('累计入账')} {renderQuota(balance.net_credited)}
          </Tag>
        </div>
        <Button
          theme='solid'
          type='primary'
          onClick={() => navigate('/enterprise/console/members')}
        >
          {t('分配额度')}
        </Button>
      </div>

      <div className='eo-card'>
        <div className='eo-stats'>
          {parts.map((p) => (
            <div key={p.key} className='eo-stat'>
              <span>
                <i className={`eo-dot ${p.key}`} aria-hidden='true' />
                {p.label}
                {p.hint && (
                  <Tooltip content={p.hint}>
                    <Info size={14} aria-label={p.label} />
                  </Tooltip>
                )}
              </span>
              <strong className={p.value < 0 ? 'neg' : ''}>
                {renderQuota(p.value)}
              </strong>
            </div>
          ))}
        </div>
        {sum > 0 && (
          <div className='eo-bar' role='img' aria-label={t('额度分布')}>
            {parts.map((p) =>
              p.value > 0 ? (
                <i
                  key={p.key}
                  className={`eo-seg ${p.key}`}
                  style={{ width: `${(p.value / sum) * 100}%` }}
                />
              ) : null,
            )}
          </div>
        )}
      </div>

      <div className='eo-cols'>
        <section className='eo-card'>
          <h2>{t('待审批')}</h2>
          {pending.length === 0 ? (
            <p className='eo-empty'>{t('没有待办')}</p>
          ) : (
            pending.map((r) => (
              <div key={r.id} className='eo-row'>
                <div className='eo-row-main'>
                  <strong>{r.member_name}</strong>
                  <span>{r.reason || '—'}</span>
                </div>
                <b>{renderQuota(r.amount)}</b>
                <Button
                  size='small'
                  onClick={() => navigate('/enterprise/console/approvals')}
                >
                  {t('处理')}
                </Button>
              </div>
            ))
          )}
        </section>

        <section className='eo-card'>
          <h2>{t('成员消费')}</h2>
          {spenders.length === 0 ? (
            <p className='eo-empty'>{t('没有记录')}</p>
          ) : (
            spenders.map((s) => (
              <div key={s.member_id} className='eo-spend'>
                <div className='eo-spend-line'>
                  <span>{s.name}</span>
                  <b>{renderQuota(s.consumed)}</b>
                </div>
                <div className='eo-track'>
                  <i style={{ width: `${(s.consumed / maxSpent) * 100}%` }} />
                </div>
              </div>
            ))
          )}
        </section>
      </div>
    </div>
  );
}
