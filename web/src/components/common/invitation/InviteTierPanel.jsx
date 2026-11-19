import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  Input,
  Modal,
  Select,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, renderQuota, showError, showSuccess } from '../../../helpers';
import InviteEarlyPayoutModal from '../../topup/modals/InviteEarlyPayoutModal';

export const tierNames = [
  '普通',
  '晋级 A',
  '晋级 B',
  '宣传专员',
  '认证成员',
  '宣传部长',
];
export const inviteStatusName = (value) =>
  ({
    pending: '待审核',
    approved: '已通过',
    rejected: '已拒绝',
    settled: '已发放',
    skipped: '已跳过',
    missing: '未提交',
    1: '待出账',
    2: '已出账',
    3: '已发放',
    4: '已冻结',
  })[value] || value;
export async function inviteRequest(url, body) {
  const response =
    body === undefined ? await API.get(url) : await API.post(url, body);
  if (!response.data?.success)
    throw new Error(response.data?.message || 'Request failed');
  return response.data.data;
}
export default function InviteTierPanel({
  userId,
  admin = false,
  compact = false,
}) {
  const { t } = useTranslation();
  const [status, setStatus] = useState(null),
    [error, setError] = useState(''),
    [open, setOpen] = useState(false),
    [early, setEarly] = useState(false);
  const [departments, setDepartments] = useState([]),
    [applications, setApplications] = useState([]),
    [rebinds, setRebinds] = useState([]);
  const [tier, setTier] = useState(3),
    [department, setDepartment] = useState(),
    [name, setName] = useState(''),
    [document, setDocument] = useState(''),
    [newInviter, setNewInviter] = useState(''),
    [busy, setBusy] = useState(false);
  const [listPage, setListPage] = useState(1),
    [listTotal, setListTotal] = useState(0);
  const generation = useRef(0);
  const load = async () => {
    const seq = ++generation.current;
    try {
      const result = await inviteRequest(
        admin
          ? `/api/admin/invite/promoters/${userId}`
          : '/api/invite/tier/status',
      );
      if (seq === generation.current) {
        setStatus(result);
        setError('');
      }
    } catch (e) {
      if (seq === generation.current) setError(e.message);
    }
  };
  useEffect(() => {
    setStatus(null);
    load();
    return () => {
      generation.current++;
    };
  }, [userId, admin]);
  useEffect(() => {
    if (!open || admin) return;
    let active = true;
    Promise.all(
      [
        `/api/invite/departments?page_size=20&p=${listPage}`,
        `/api/invite/department/apply?page_size=20&p=${listPage}`,
        `/api/invite/rebind?page_size=20&p=${listPage}`,
      ].map((url) => inviteRequest(url)),
    )
      .then(([d, a, r]) => {
        if (active) {
          setListTotal(Math.max(d?.total || 0, a?.total || 0, r?.total || 0));
          setDepartments(d?.items || []);
          setApplications(a?.items || []);
          setRebinds(r?.items || []);
        }
      })
      .catch((e) => {
        if (active) showError(e.message);
      });
    return () => {
      active = false;
    };
  }, [open, admin, listPage]);
  useEffect(() => {
    if (admin || !status?.enabled) return;
    try {
      let device = localStorage.getItem('invite_device_key');
      if (!device) {
        device = crypto.randomUUID();
        localStorage.setItem('invite_device_key', device);
      }
      inviteRequest('/api/invite/device', { device }).catch(() => {});
    } catch {
      /* Storage can be disabled; server identity and payment checks remain active. */
    }
  }, [admin, status?.enabled]);
  const mutate = async (path, body) => {
    if (busy) return;
    setBusy(true);
    try {
      await inviteRequest('/api/invite/' + path, body);
      showSuccess(t('操作成功'));
      await load();
      setOpen(false);
      setName('');
      setDocument('');
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };
  if (error)
    return (
      <Space>
        <Typography.Text type='danger'>{error}</Typography.Text>
        <Button onClick={load}>{t('重试')}</Button>
      </Space>
    );
  if (!status || (!status.enabled && !admin)) return null;
  return (
    <div className='my-3 space-y-3'>
      <Space wrap>
        <Tag color='green'>{t(tierNames[status.tier])}</Tag>
        <Typography.Text>
          {t('一级比例')} {Number(status.first_rate * 100).toFixed(1)}%
        </Typography.Text>
        <Tag>
          {t('本月达标')} {status.qualified_count}
        </Tag>
        {status.frozen && <Tag color='red'>{t('已冻结')}</Tag>}
        {status.department_id > 0 && (
          <Tag>
            {t('部门')} #{status.department_id}
          </Tag>
        )}
      </Space>
      {!compact && (
        <>
          <div>
            {t('距离下一档')}：{status.next_tier_remaining} · {t('最低提现')}
            {'：'}
            {new Intl.NumberFormat(undefined, {
              style: 'currency',
              currency: 'CNY',
            }).format(status.minimum_withdrawal_cents / 100)}
          </div>
          <div className='text-sm text-gray-500'>
            {t(
              '普通与晋级档按月结算，专员及以上按周结算，支付后至少观察七天。',
            )}
          </div>
          <div className='text-sm'>{t('划转与提现不同，划转沿用原门槛。')}</div>
          {status.debt_amount > 0 && (
            <div>
              {t('待抵扣欠款')}：{renderQuota(status.debt_amount)}
            </div>
          )}
        </>
      )}
      {!admin && (
        <Space wrap>
          <Button onClick={() => setOpen(true)}>{t('推广者申请与实名')}</Button>
          {status.tier >= 3 && (
            <Button onClick={() => setEarly(true)}>{t('申请提前发放')}</Button>
          )}
        </Space>
      )}
      <InviteEarlyPayoutModal
        visible={early}
        onCancel={() => setEarly(false)}
        onSuccess={load}
      />
      <Modal
        title={t('推广者申请与实名')}
        visible={open}
        onCancel={() => {
          setOpen(false);
          setDocument('');
        }}
        footer={null}
        width='min(680px, calc(100vw - 24px))'
      >
        <div className='space-y-4'>
          <Space>
            <Button
              disabled={listPage <= 1}
              onClick={() => setListPage((p) => p - 1)}
            >
              {t('上一页')}
            </Button>
            <span>{listPage}</span>
            <Button
              disabled={listPage * 20 >= listTotal}
              onClick={() => setListPage((p) => p + 1)}
            >
              {t('下一页')}
            </Button>
          </Space>
          <Space wrap>
            <Select
              value={tier}
              onChange={setTier}
              optionList={[3, 4, 5].map((value) => ({
                value,
                label: t(tierNames[value]),
              }))}
            />
            {tier === 4 && (
              <Select
                placeholder={t('选择部门')}
                value={department}
                onChange={setDepartment}
                optionList={departments.map((d) => ({
                  value: d.id,
                  label: d.name || `#${d.id}`,
                }))}
              />
            )}
            <Button
              loading={busy}
              disabled={tier === 4 && (!department || status.tier < 2)}
              onClick={() =>
                mutate('department/apply', {
                  tier,
                  department_id: department || 0,
                })
              }
            >
              {t('提交申请')}
            </Button>
          </Space>
          {departments.length === 0 && tier === 4 && <div>{t('暂无部门')}</div>}
          <div className='text-sm'>
            {t('加入部门永久切断上游，主动退出次月生效。')}
          </div>
          {status.department_id > 0 && (
            <Button
              type='danger'
              loading={busy}
              onClick={() =>
                Modal.confirm({
                  title: t('确认退出部门'),
                  content: t('加入部门永久切断上游，主动退出次月生效。'),
                  onOk: () => mutate('department/leave', {}),
                })
              }
            >
              {t('退出部门')}
            </Button>
          )}
          <div>{t('申请记录')}</div>
          {applications.length === 0 ? (
            <div>{t('暂无数据')}</div>
          ) : (
            applications.map((a) => (
              <div key={a.id}>
                {t(tierNames[a.tier])} · {t(inviteStatusName(a.status))} ·{' '}
                {a.remark}
              </div>
            ))
          )}
          <div>
            {t('实名认证')}：{t(inviteStatusName(status.identity_status))}
          </div>
          {status.require_identity && (
            <Typography.Text type='warning'>
              {t('实名审核通过前暂停发放，不影响已记录收益。')}
            </Typography.Text>
          )}
          {status.identity_status !== 'approved' && (
            <>
              <Input
                placeholder={t('姓名')}
                value={name}
                maxLength={64}
                onChange={setName}
              />
              <Input
                mode='password'
                placeholder={t('证件号码')}
                value={document}
                maxLength={64}
                onChange={setDocument}
              />
              <Button
                loading={busy}
                disabled={!name || !document}
                onClick={() => mutate('identity', { name, document })}
              >
                {t('提交实名')}
              </Button>
            </>
          )}
          <div>{t('邀请关系改绑')}</div>
          <Input
            placeholder={t('新上级用户 ID')}
            value={newInviter}
            onChange={setNewInviter}
          />
          <Button
            loading={busy}
            disabled={!/^[1-9]\d*$/.test(newInviter)}
            onClick={() =>
              mutate('rebind', { new_inviter_id: Number(newInviter) })
            }
          >
            {t('申请改绑')}
          </Button>
          <div className='text-sm'>
            {t('注册三十天内仅可申请一次，需新上级确认，历史账单不迁移。')}
          </div>
          {rebinds
            .filter(
              (r) =>
                r.status === 'pending' && r.new_inviter_id === status.user_id,
            )
            .map((r) => (
              <Space key={r.id}>
                <span>
                  #{r.id} · {t(inviteStatusName(r.status))}
                </span>
                <Button
                  loading={busy}
                  onClick={() => mutate(`rebind/${r.id}/confirm`, {})}
                >
                  {t('确认接收')}
                </Button>
              </Space>
            ))}
        </div>
      </Modal>
    </div>
  );
}
