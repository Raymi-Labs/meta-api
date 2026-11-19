import React, { useEffect, useState } from 'react';
import {
  Button,
  Card,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Tabs,
  TabPane,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showSuccess,
  renderQuota,
  isRoot,
} from '../../../helpers';
import CardTable from '../../../components/common/ui/CardTable';
import InviteTierLedgerTabs from '../../../components/common/invitation/InviteTierLedgerTabs';
import {
  inviteRequest,
  inviteStatusName,
  tierNames,
} from '../../../components/common/invitation/InviteTierPanel';

function ReviewList({ resource }) {
  const { t } = useTranslation();
  const [data, setData] = useState({ items: [], total: 0 }),
    [page, setPage] = useState(1),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [rev, setRev] = useState(0),
    [filter, setFilter] = useState('');
  useEffect(() => {
    let active = true;
    inviteRequest(
      `/api/admin/invite/${resource}?p=${page}&page_size=10&status=${filter}`,
    )
      .then((d) => {
        if (active) {
          setData(d);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, [resource, page, rev, filter]);
  const action = (row, approve) => {
    let remark = '';
    Modal.confirm({
      title: t('确认操作'),
      content: (
        <Input
          placeholder={t('审核备注')}
          onChange={(v) => {
            remark = v;
          }}
          maxLength={255}
        />
      ),
      onOk: async () => {
        setBusy(true);
        try {
          const id = resource === 'identities' ? row.user_id : row.id;
          const operation = resource === 'actions' ? 'reverse' : 'review';
          await inviteRequest(
            `/api/admin/invite/${resource}/${id}/${operation}`,
            { approve, remark },
          );
          setRev((v) => v + 1);
          showSuccess(t('操作成功'));
        } catch (e) {
          showError(e.message);
          throw e;
        } finally {
          setBusy(false);
        }
      },
    });
  };
  const viewIdentity = (row) => {
    let reason = '';
    Modal.confirm({
      title: t('查看实名原文'),
      content: (
        <Input
          placeholder={t('查看原因')}
          onChange={(v) => {
            reason = v;
          }}
          maxLength={255}
        />
      ),
      onOk: async () => {
        if (reason.trim().length < 4) throw new Error(t('查看原因'));
        const value = await inviteRequest(
          `/api/admin/invite/identities/${row.user_id}/plaintext?reason=${encodeURIComponent(reason)}`,
        );
        Modal.info({
          title: t('实名认证'),
          content: (
            <div>
              <div>{value.name}</div>
              <div>{value.document}</div>
            </div>
          ),
        });
      },
    });
  };
  const columns = [
    { title: 'ID', dataIndex: 'id' },
    {
      title: t('用户 ID'),
      dataIndex: resource === 'departments' ? 'leader_user_id' : 'user_id',
    },
    { title: t('名称'), dataIndex: 'name' },
    { title: t('关联用户'), dataIndex: 'related_user_id' },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (v) => t(inviteStatusName(v) || '—'),
    },
    {
      title: t('档位'),
      dataIndex: 'tier',
      render: (v) => (v === undefined ? '—' : t(tierNames[v])),
    },
    { title: t('周期'), dataIndex: 'week_key' },
    { title: t('姓名'), dataIndex: 'masked_name' },
    { title: t('证件号码'), dataIndex: 'masked_document' },
    { title: t('类型'), dataIndex: 'kind' },
    { title: t('备注'), dataIndex: 'remark' },
    {
      title: t('操作'),
      render: (_, row) =>
        resource === 'actions' ? (
          <Button
            disabled={busy || row.reversed}
            onClick={() => action(row, false)}
          >
            {t('撤销')}
          </Button>
        ) : (
          ['applications', 'early-payout', 'identities'].includes(resource) && (
            <Space>
              {resource === 'identities' && isRoot() && (
                <Button onClick={() => viewIdentity(row)}>
                  {t('查看实名原文')}
                </Button>
              )}
              <Button
                disabled={busy || row.status === 'approved'}
                onClick={() => action(row, true)}
              >
                {t('通过')}
              </Button>
              <Button
                disabled={busy || row.status === 'rejected'}
                onClick={() => action(row, false)}
              >
                {t('拒绝')}
              </Button>
            </Space>
          )
        ),
    },
  ];
  return (
    <div className='space-y-3'>
      {error && <Typography.Text type='danger'>{error}</Typography.Text>}
      {['applications', 'early-payout', 'identities'].includes(resource) && (
        <Select
          value={filter}
          onChange={(v) => {
            setFilter(v);
            setPage(1);
          }}
          optionList={['', 'pending', 'approved', 'rejected'].map((value) => ({
            value,
            label: value ? t(inviteStatusName(value)) : t('全部状态'),
          }))}
        />
      )}
      <Button onClick={() => setRev((v) => v + 1)}>{t('刷新')}</Button>
      <CardTable
        empty={<div className='p-6 text-center'>{t('暂无数据')}</div>}
        rowKey='id'
        columns={columns}
        dataSource={data.items || []}
        scroll={{ x: 1000 }}
        pagination={{
          currentPage: page,
          pageSize: 10,
          total: data.total || 0,
          onPageChange: setPage,
        }}
        hidePagination={false}
      />
    </div>
  );
}
export default function SettingsInviteTier() {
  const { t } = useTranslation();
  const [settings, setSettings] = useState(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  const [user, setUser] = useState(''),
    [tier, setTier] = useState(0),
    [amount, setAmount] = useState(0),
    [expires, setExpires] = useState(0),
    [remark, setRemark] = useState(''),
    [statement, setStatement] = useState('');
  const load = () =>
    inviteRequest('/api/admin/invite/tier/settings')
      .then((s) => {
        setSettings(s);
        setError('');
      })
      .catch((e) => setError(e.message));
  useEffect(() => {
    load();
  }, []);
  const invalidCombination =
    settings &&
    (settings.first_level_rates
      .slice(0, 4)
      .some((first, x) =>
        settings.second_level_matrix.some(
          (row, y) =>
            first +
              row[x] +
              (x === 3 ? settings.qualified_bonus_rate : 0) +
              (y === 4 ? settings.leader_slot_rates[x] : 0) >
            settings.pool_ratio + 1e-9,
        ),
      ) ||
      settings.first_level_rates[4] + settings.department_commission_rate >
        settings.pool_ratio + 1e-9 ||
      settings.first_level_rates[5] + settings.qualified_bonus_rate >
        settings.pool_ratio + 1e-9);
  const save = async () => {
    if (invalidCombination) return;
    setBusy(true);
    try {
      const { data } = await API.put(
        '/api/admin/invite/tier/settings',
        settings,
      );
      if (!data?.success) throw new Error(data?.message);
      setSettings(data.data);
      showSuccess(t('保存成功'));
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };
  const change = (key, value) => setSettings((s) => ({ ...s, [key]: value }));
  const arrayChange = (key, index, value) =>
    change(
      key,
      settings[key].map((v, i) => (i === index ? value : v)),
    );
  const act = (action, frozen = true) =>
    Modal.confirm({
      title: t('确认操作'),
      content: t('此操作影响后续推广收益，请核对用户和原因。'),
      onOk: async () => {
        setBusy(true);
        try {
          await inviteRequest(`/api/admin/invite/promoters/${user}/${action}`, {
            tier,
            frozen,
            amount,
            expires_at: expires,
            remark,
          });
          showSuccess(t('操作成功'));
        } catch (e) {
          showError(e.message);
          throw e;
        } finally {
          setBusy(false);
        }
      },
    });
  return (
    <Card title={t('邀请档位与结算')} className='mt-4'>
      {error && (
        <Space>
          <Typography.Text type='danger'>{error}</Typography.Text>
          <Button onClick={load}>{t('重试')}</Button>
        </Space>
      )}
      <Tabs>
        <TabPane tab={t('档位配置')} itemKey='settings'>
          {settings && (
            <div className='space-y-4'>
              <Space wrap>
                <span>{t('启用档位返佣')}</span>
                <Switch
                  checked={settings.enabled}
                  onChange={(v) => change('enabled', v)}
                />
                <span>{t('发放要求实名')}</span>
                <Switch
                  checked={settings.require_identity_for_payout}
                  onChange={(v) => change('require_identity_for_payout', v)}
                />
              </Space>
              <Typography.Paragraph>
                {t(
                  '历史邀请不补发，历史下级不计入晋级人数。关闭开关不取消已入池账单。',
                )}
              </Typography.Paragraph>
              <Space wrap>
                {[
                  ['pool_ratio', '返利池比例'],
                  ['qualified_bonus_rate', '达标奖金比例'],
                  ['department_commission_rate', '部门分成比例'],
                  ['qualified_threshold_money', '奖金门槛（元）'],
                  ['second_level_monthly_cap_money', '二级月基数（元）'],
                  ['commissioner_retention', '专员保级人数'],
                ].map(([key, label]) => (
                  <label key={key}>
                    {t(label)}
                    <InputNumber
                      min={0}
                      max={
                        key.includes('rate') || key === 'pool_ratio'
                          ? 1
                          : undefined
                      }
                      value={settings[key]}
                      onChange={(v) => change(key, Number(v))}
                    />
                  </label>
                ))}
              </Space>
              <div style={{ overflowX: 'auto' }}>
                <table className='w-full text-sm'>
                  <thead>
                    <tr>
                      {[
                        '档位',
                        '一级比例',
                        '注册奖励（闪）',
                        '最低提现（分）',
                      ].map((k) => (
                        <th key={k}>{t(k)}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {tierNames.map((label, i) => (
                      <tr key={label}>
                        <td>{t(label)}</td>
                        {[
                          'first_level_rates',
                          'register_rewards',
                          'min_withdrawal_amounts',
                        ].map((key) => (
                          <td key={key}>
                            <InputNumber
                              min={0}
                              max={key === 'first_level_rates' ? 1 : undefined}
                              value={settings[key][i]}
                              onChange={(v) => arrayChange(key, i, Number(v))}
                            />
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div>
                {t(
                  '二级矩阵：行是收款身份，列是一级档位。比例合计不得超过返利池。',
                )}
              </div>
              <div style={{ overflowX: 'auto' }}>
                <table className='w-full text-sm'>
                  <thead>
                    <tr>
                      <th>{t('收款身份')}</th>
                      {tierNames.slice(0, 4).map((v) => (
                        <th key={v}>{t(v)}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {tierNames.map((label, i) => (
                      <tr key={label}>
                        <td>{t(label)}</td>
                        {settings.second_level_matrix[i].map((v, j) => (
                          <td key={j}>
                            <InputNumber
                              min={0}
                              max={1}
                              value={v}
                              onChange={(next) =>
                                change(
                                  'second_level_matrix',
                                  settings.second_level_matrix.map((row, x) =>
                                    row.map((cell, y) =>
                                      x === i && y === j ? Number(next) : cell,
                                    ),
                                  ),
                                )
                              }
                            />
                          </td>
                        ))}
                      </tr>
                    ))}
                    <tr>
                      <td>{t('成员的部长位')}</td>
                      {settings.leader_slot_rates.map((v, i) => (
                        <td key={i}>
                          <InputNumber
                            min={0}
                            max={1}
                            value={v}
                            onChange={(next) =>
                              arrayChange('leader_slot_rates', i, Number(next))
                            }
                          />
                        </td>
                      ))}
                    </tr>
                  </tbody>
                </table>
              </div>
              <Space>
                {settings.promotion_thresholds.map((v, i) => (
                  <label key={i}>
                    {t(i === 0 ? 'A 档人数' : 'B 档人数')}
                    <InputNumber
                      min={1}
                      precision={0}
                      value={v}
                      onChange={(n) =>
                        arrayChange('promotion_thresholds', i, Number(n))
                      }
                    />
                  </label>
                ))}
              </Space>
              {invalidCombination && (
                <Typography.Text type='danger'>
                  {t('组合比例超过返利池，请检查一级、二级、奖金与部长位。')}
                </Typography.Text>
              )}
              <Button
                theme='solid'
                disabled={!!invalidCombination}
                loading={busy}
                onClick={save}
              >
                {t('保存')}
              </Button>
            </div>
          )}
        </TabPane>
        <TabPane tab={t('周期账单')} itemKey='statements'>
          <InviteTierLedgerTabs admin />
          <Space wrap className='mt-3'>
            <Input
              placeholder={t('账单 ID')}
              value={statement}
              onChange={setStatement}
            />
            {[true, false].map((frozen) => (
              <Button
                key={String(frozen)}
                disabled={!/^[1-9]\d*$/.test(statement)}
                onClick={async () => {
                  try {
                    await inviteRequest(
                      `/api/admin/invite/statements/${statement}/freeze`,
                      { frozen },
                    );
                    showSuccess(t('操作成功'));
                  } catch (e) {
                    showError(e.message);
                  }
                }}
              >
                {t(frozen ? '冻结账单' : '解冻账单')}
              </Button>
            ))}
          </Space>
        </TabPane>
        {[
          'applications',
          'early-payout',
          'identities',
          'departments',
          'actions',
          'signals',
        ].map((resource, i) => (
          <TabPane
            key={resource}
            itemKey={resource}
            tab={t(
              [
                '晋级与部门申请',
                '提前发放审核',
                '实名审核',
                '部门',
                '处置记录',
                '风险线索',
              ][i],
            )}
          >
            <ReviewList resource={resource} />
          </TabPane>
        ))}
        <TabPane tab={t('推广者处置')} itemKey='manage'>
          <div className='space-y-3'>
            <Input placeholder={t('用户 ID')} value={user} onChange={setUser} />
            <Input
              placeholder={t('操作原因')}
              value={remark}
              maxLength={255}
              onChange={setRemark}
            />
            <Space wrap>
              <Select
                value={tier}
                onChange={setTier}
                optionList={tierNames.map((label, value) => ({
                  label: t(label),
                  value,
                }))}
              />
              <label>
                {t('覆盖截止时间戳（零为长期）')}
                <InputNumber
                  min={0}
                  precision={0}
                  value={expires}
                  onChange={(v) => setExpires(Number(v))}
                />
              </label>
              <label>
                {t('扣回额度')}
                <InputNumber
                  min={0}
                  precision={0}
                  value={amount}
                  onChange={(v) => setAmount(Number(v))}
                />
              </label>
              <span>{renderQuota(amount)}</span>
            </Space>
            <Space wrap>
              {[
                ['tier', '覆盖档位'],
                ['freeze', '冻结推广者'],
                ['unfreeze', '解冻推广者'],
                ['clear', '清退'],
                ['clawback', '扣回收益'],
              ].map(([action, label]) => (
                <Button
                  key={action}
                  disabled={busy || !/^[1-9]\d*$/.test(user) || !remark.trim()}
                  onClick={() =>
                    act(
                      action === 'unfreeze' ? 'freeze' : action,
                      action !== 'unfreeze',
                    )
                  }
                >
                  {t(label)}
                </Button>
              ))}
            </Space>
          </div>
        </TabPane>
      </Tabs>
    </Card>
  );
}
