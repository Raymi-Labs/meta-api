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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useState,
  useRef,
} from 'react';
import { useOutletContext } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  DatePicker,
  Input,
  Select,
  Table,
  Tag,
} from '@douyinfe/semi-ui';
import { Download } from 'lucide-react';
import { API } from '../../helpers/apiCore';
import {
  enterpriseRequest,
  formatEnterpriseTime,
} from '../../helpers/enterprise';
import { readLogExportError } from '../../helpers/usageLogExport';
import { renderQuota } from '../../helpers/render';
import { showError } from '../../helpers/toast';
import './enterpriseMembers.css';
import './enterpriseUsage.css';

const KIND = {
  request: '请求',
  violation_fee: '违规费',
  task_adjust: '任务调整',
  manual: '人工结算',
};
const STATUS = {
  reserved: '进行中',
  settled: '已结算',
  refunded: '已退回',
  review: '待对账',
};
const STATUS_COLOR = {
  reserved: 'blue',
  settled: 'green',
  refunded: 'grey',
  review: 'orange',
};
const DAY = 86400;

const weekAgo = () => {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  d.setDate(d.getDate() - 6);
  return d;
};

function Breakdown({ title, rows }) {
  const max = Math.max(1, ...rows.map((r) => r.net_consumed));
  return (
    <section className='eu-card'>
      <h2>{title}</h2>
      {rows.slice(0, 6).map((r) => (
        <div key={r.name} className='eu-line'>
          <div className='eu-line-head'>
            <span>{r.name}</span>
            <b>{renderQuota(r.net_consumed)}</b>
          </div>
          <div className='eu-track'>
            <i
              style={{ width: `${Math.max(0, (r.net_consumed / max) * 100)}%` }}
            />
          </div>
        </div>
      ))}
    </section>
  );
}

export default function EnterpriseUsage() {
  const { t } = useTranslation();
  const { enterpriseId, enterprise } = useOutletContext();
  const [range, setRange] = useState([weekAgo(), new Date()]);
  const [memberId, setMemberId] = useState('');
  const [model, setModel] = useState('');
  const [modelDraft, setModelDraft] = useState('');
  const [kind, setKind] = useState('');
  const [status, setStatus] = useState('all');
  const [members, setMembers] = useState([]);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [summary, setSummary] = useState(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);

  const params = useMemo(() => {
    const p = {};
    if (range?.[0]) p.start_timestamp = Math.floor(range[0].getTime() / 1000);
    if (range?.[1]) p.end_timestamp = Math.floor(range[1].getTime() / 1000);
    if (memberId) p.member_id = memberId;
    if (model) p.model_name = model;
    if (kind) p.kind = kind;
    if (status) p.status = status;
    return p;
  }, [range, memberId, model, kind, status]);

  const base = `/api/enterprises/${enterpriseId}`;

  useEffect(() => {
    enterpriseRequest('get', `${base}/members`, undefined, {
      p: 1,
      page_size: 100,
      status: 'all',
    })
      .then((data) => setMembers(data?.items || []))
      .catch((e) => showError(e.message));
  }, [base]);

  const loadSeq = useRef(0);
  const load = useCallback(async () => {
    const sequence = ++loadSeq.current;
    setLoading(true);
    try {
      const [list, sum] = await Promise.all([
        enterpriseRequest('get', `${base}/usage`, undefined, {
          ...params,
          p: page,
          page_size: pageSize,
        }),
        enterpriseRequest('get', `${base}/usage/summary`, undefined, params),
      ]);
      if (sequence !== loadSeq.current) return;
      setItems(list?.items || []);
      setTotal(list?.total || 0);
      setSummary(sum);
    } catch (e) {
      if (sequence === loadSeq.current) showError(e.message);
    } finally {
      if (sequence === loadSeq.current) setLoading(false);
    }
  }, [base, params, page, pageSize]);

  useEffect(() => {
    load();
    return () => {
      loadSeq.current++;
    };
  }, [load]);

  const reset = (setter) => (v) => {
    setPage(1);
    setter(v);
  };

  const exportCsv = async () => {
    const start = params.start_timestamp;
    const end = params.end_timestamp;
    if (!start || !end || end < start || end - start > 31 * DAY) {
      showError(t('请选择有效时间范围，单次导出最多 31 天'));
      return;
    }
    setExporting(true);
    try {
      const res = await API.get(`${base}/usage/export`, {
        params,
        responseType: 'blob',
        timeout: 60000,
        skipErrorHandler: true,
      });
      if (!String(res.headers['content-type']).includes('text/csv'))
        throw { response: res };
      const url = URL.createObjectURL(res.data);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'enterprise-usage.csv';
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      showError(
        await readLogExportError(e, t('导出失败，请缩小筛选范围后重试')),
      );
    } finally {
      setExporting(false);
    }
  };

  const columns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => (
        <span className='em-num'>
          {formatEnterpriseTime(v, enterprise.timezone)}
        </span>
      ),
    },
    { title: t('成员'), dataIndex: 'member_name' },
    {
      title: t('模型'),
      dataIndex: 'model_name',
      render: (v, r) => (
        <div className='em-who'>
          <strong>{v || '—'}</strong>
          {r.token_name && <span>{r.token_name}</span>}
        </div>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'kind',
      render: (v) => t(KIND[v] || v),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (v) => (
        <Tag color={STATUS_COLOR[v] || 'grey'}>{t(STATUS[v] || v)}</Tag>
      ),
    },
    {
      title: t('额度'),
      dataIndex: 'quota',
      align: 'right',
      render: (v) => (
        <span className={`em-num${v < 0 ? ' neg' : ''}`}>{renderQuota(v)}</span>
      ),
    },
  ];

  const stats = summary
    ? [
        { label: t('净消费'), value: renderQuota(summary.net_consumed) },
        { label: t('请求数'), value: String(summary.requests) },
        { label: t('在途'), value: renderQuota(summary.pending) },
      ]
    : [];

  return (
    <div className='em'>
      <div className='em-head'>
        <h1>{t('用量')}</h1>
        <Button
          icon={<Download size={16} aria-hidden='true' />}
          loading={exporting}
          onClick={exportCsv}
        >
          {t('导出 CSV')}
        </Button>
      </div>

      <div className='em-tools eu-filters'>
        <DatePicker
          type='dateTimeRange'
          aria-label={t('时间')}
          value={range}
          onChange={reset(setRange)}
        />
        <Select
          aria-label={t('成员')}
          value={memberId}
          onChange={reset(setMemberId)}
          optionList={[{ value: '', label: t('全部') }].concat(
            members.map((m) => ({
              value: m.id,
              label: m.display_name || m.username,
            })),
          )}
        />
        <Input
          showClear
          aria-label={t('模型')}
          placeholder={t('模型')}
          value={modelDraft}
          onChange={setModelDraft}
          onEnterPress={() => reset(setModel)(modelDraft.trim())}
          onClear={() => {
            setModelDraft('');
            reset(setModel)('');
          }}
        />
        <Select
          aria-label={t('类型')}
          value={kind}
          onChange={reset(setKind)}
          optionList={[{ value: '', label: t('全部类型') }].concat(
            Object.keys(KIND).map((k) => ({ value: k, label: t(KIND[k]) })),
          )}
        />
        <Select
          aria-label={t('状态')}
          value={status}
          onChange={reset(setStatus)}
          optionList={[
            { value: 'all', label: t('全部状态') },
            ...Object.keys(STATUS).map((s) => ({
              value: s,
              label: t(STATUS[s]),
            })),
          ]}
        />
      </div>

      {summary && (
        <div className='eu-card eu-stats'>
          {stats.map((s) => (
            <div key={s.label} className='eu-stat'>
              <span>{s.label}</span>
              <strong>{s.value}</strong>
            </div>
          ))}
        </div>
      )}

      {summary &&
        (summary.by_member?.length > 0 || summary.by_model?.length > 0) && (
          <div className='eu-cols'>
            <Breakdown title={t('按成员')} rows={summary.by_member || []} />
            <Breakdown title={t('按模型')} rows={summary.by_model || []} />
          </div>
        )}

      <div className='em-card'>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={items}
          loading={loading}
          empty={t('没有记录')}
          pagination={{
            currentPage: page,
            pageSize,
            total,
            pageSizeOpts: [10, 20, 50, 100],
            showSizeChanger: true,
            onPageChange: setPage,
            onPageSizeChange: (size) => {
              setPage(1);
              setPageSize(size);
            },
          }}
        />
      </div>
    </div>
  );
}
