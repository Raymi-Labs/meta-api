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
  Input,
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
  Select,
  Table,
  Tooltip,
} from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import {
  ENTERPRISE_ROLE_LABEL,
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
import './enterpriseSettings.css';

// 操作记录里的动作名，没有对应文案的动作直接显示原始名称。
const ACTION = {
  'budget.allocate': '分配额度',
  'budget.reclaim': '回收额度',
  'enterprise.create': '创建企业',
  'enterprise.settings': '修改设置',
  'enterprise.status': '状态变更',
  'enterprise.transfer': '转移所有权',
  'enterprise.update': '修改设置',
  'invitation.create': '邀请成员',
  'invitation.revoke': '撤销邀请',
  'member.disable': '停用成员',
  'member.leave': '成员退出',
  'member.join': '成员加入',
  'member.models': '成员模型',
  'member.role': '调整角色',
  'request.approved': '批准申请',
  'request.rejected': '拒绝申请',
  'reservation.resolve': '人工结算',
  'token.revoke': '撤销令牌',
  'wallet.credit': '入账',
  'wallet.adjust': '调账',
};
const ACTOR = { platform: '平台', system: '系统' };

const parse = (text) => {
  try {
    return (text && JSON.parse(text)) || {};
  } catch {
    return {};
  }
};

// 每条记录最关键的一个值：金额、邮箱、令牌名或新角色。
const auditDetail = (r, t) => {
  const before = parse(r.before);
  const after = parse(r.after);
  switch (r.action) {
    case 'budget.allocate':
    case 'budget.reclaim':
    case 'wallet.credit':
      return renderQuota(after.amount);
    case 'wallet.adjust':
      return renderQuota(after.delta);
    case 'request.approved':
      return renderQuota(after.approved_amount);
    case 'invitation.create':
      return after.email;
    case 'token.revoke':
      return before.name;
    case 'member.role':
    case 'member.join':
      return after.role
        ? t(ENTERPRISE_ROLE_LABEL[after.role] || after.role)
        : '';
    default:
      return '';
  }
};

const zoneList = (current) => {
  const all =
    typeof Intl.supportedValuesOf === 'function'
      ? Intl.supportedValuesOf('timeZone')
      : [];
  return Array.from(new Set(['UTC', current, ...all].filter(Boolean)));
};

export default function EnterpriseSettings() {
  const { t } = useTranslation();
  const { enterpriseId, role, refreshSelf } = useOutletContext();
  const owner = role === 'owner';
  const manager = owner || role === 'admin';
  const base = `/api/enterprises/${enterpriseId}`;
  const [form, setForm] = useState(null);
  const [options, setOptions] = useState([]);
  const [busy, setBusy] = useState(false);
  const [logs, setLogs] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(false);
  const patch = (part) => setForm((f) => ({ ...f, ...part }));

  const loadFormSeq = useRef(0);
  const loadForm = useCallback(async () => {
    const sequence = ++loadFormSeq.current;
    try {
      const [overview, models] = await Promise.all([
        enterpriseRequest('get', `${base}/overview`),
        enterpriseRequest('get', `${base}/model-options`),
      ]);
      if (sequence !== loadFormSeq.current) return;
      const e = overview.enterprise;
      setOptions(models || []);
      setForm({
        name: e.name,
        timezone: e.timezone,
        model_mode: e.model_mode || 'inherit',
        models: e.models || [],
        threshold: quotaToDisplayAmount(e.low_balance_threshold),
      });
    } catch (err) {
      if (sequence === loadFormSeq.current) showError(err.message);
    }
  }, [base]);

  const loadLogsSeq = useRef(0);
  const loadLogs = useCallback(async () => {
    const sequence = ++loadLogsSeq.current;
    setLoading(true);
    try {
      const data = await enterpriseRequest('get', `${base}/audit`, undefined, {
        p: page,
        page_size: pageSize,
      });
      if (sequence !== loadLogsSeq.current) return;
      setLogs(data?.items || []);
      setTotal(data?.total || 0);
    } catch (err) {
      if (sequence === loadLogsSeq.current) showError(err.message);
    } finally {
      if (sequence === loadLogsSeq.current) setLoading(false);
    }
  }, [base, page, pageSize]);

  useEffect(() => {
    loadForm();
    return () => {
      loadFormSeq.current++;
    };
  }, [loadForm]);
  useEffect(() => {
    loadLogs();
    return () => {
      loadLogsSeq.current++;
    };
  }, [loadLogs]);

  const zones = useMemo(() => zoneList(form?.timezone), [form?.timezone]);

  const save = async (confirmedEmpty = false) => {
    if (!form.name.trim()) {
      showError(t('请填写名称'));
      return;
    }
    if (
      form.model_mode === 'custom' &&
      form.models.length === 0 &&
      confirmedEmpty !== true &&
      owner
    ) {
      Modal.confirm({
        className: 'ec-modal',
        title: t('禁止全部模型？'),
        content: t('保存后将无法使用任何模型，已有账单保留。'),
        okText: t('确认'),
        cancelText: t('取消'),
        onOk: () => save(true),
      });
      return;
    }
    setBusy(true);
    try {
      await enterpriseRequest('put', `${base}/settings`, {
        name: form.name.trim(),
        timezone: form.timezone,
        model_mode: form.model_mode,
        models: form.model_mode === 'custom' ? form.models : [],
        low_balance_threshold: displayAmountToQuota(form.threshold),
      });
      showSuccess(t('已保存'));
      refreshSelf();
      loadLogs();
    } catch (err) {
      showError(err.message);
    } finally {
      setBusy(false);
    }
  };

  const columns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => (
        <span className='em-num'>
          {formatEnterpriseTime(v, form?.timezone)}
        </span>
      ),
    },
    {
      title: t('操作人'),
      dataIndex: 'actor_name',
      render: (v, r) => v || t(ACTOR[r.actor_role] || '—'),
    },
    {
      title: t('操作'),
      dataIndex: 'action',
      render: (v) => t(ACTION[v] || v),
    },
    {
      title: t('详情'),
      dataIndex: 'after',
      render: (_, r) =>
        [auditDetail(r, t), r.reason].filter(Boolean).join(' · ') || '—',
    },
  ];

  return (
    <div className='em'>
      <div className='em-head'>
        <h1>{t('设置')}</h1>
        {manager && form && (
          <Button
            theme='solid'
            type='primary'
            loading={busy}
            onClick={() => save()}
          >
            {t('保存')}
          </Button>
        )}
      </div>

      {form && (
        <section className='es-card'>
          <h2>{t('企业资料')}</h2>
          <div className='es-grid'>
            <label className='ec-field'>
              {t('企业名称')}
              <Input
                maxLength={64}
                disabled={!manager}
                value={form.name}
                onChange={(v) => patch({ name: v })}
              />
            </label>
            <label className='ec-field'>
              {t('时区')}
              <Select
                filter
                disabled={!manager}
                value={form.timezone}
                optionList={zones.map((z) => ({ value: z, label: z }))}
                onChange={(v) => patch({ timezone: v })}
                style={{ width: '100%' }}
              />
            </label>
            <label className='ec-field'>
              <span className='es-label'>
                {t('余额提醒')}
                <Tooltip
                  content={t(
                    '未分配余额低于该金额时，邮件提醒所有者和管理员；填 0 不提醒。',
                  )}
                >
                  <Info size={14} aria-hidden='true' />
                </Tooltip>
              </span>
              <InputNumber
                min={0}
                precision={2}
                disabled={!manager}
                value={form.threshold}
                onChange={(v) => patch({ threshold: v })}
                style={{ width: '100%' }}
              />
            </label>
          </div>
          <div className='ec-field'>
            {t('可用模型')}
            <RadioGroup
              value={form.model_mode}
              onChange={(e) => patch({ model_mode: e.target.value })}
            >
              <Radio value='inherit' disabled={!owner}>
                {t('全部模型')}
              </Radio>
              <Radio value='custom' disabled={!owner}>
                {t('指定模型')}
              </Radio>
            </RadioGroup>
            {form.model_mode === 'custom' && (
              <Select
                multiple
                filter
                disabled={!owner}
                placeholder={t('选择模型')}
                value={form.models}
                optionList={options.map((m) => ({ value: m, label: m }))}
                onChange={(v) => patch({ models: v })}
                style={{ width: '100%' }}
              />
            )}
          </div>
        </section>
      )}

      <section className='es-card es-flush'>
        <h2>{t('操作记录')}</h2>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={logs}
          loading={loading}
          empty={t('没有记录')}
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
      </section>
    </div>
  );
}
