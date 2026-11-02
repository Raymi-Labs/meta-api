import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../helpers';
import { filterOpenRouterActivity, openRouterUSD } from '../openRouterFinance';

const { Text } = Typography;
const endpoint = '/api/upstream_accounts';

export default function OpenRouterAccountDashboard({
  accounts,
  reloadAccounts,
  active,
  t,
}) {
  const options = useMemo(
    () => accounts.filter((a) => a.account_type === 'openrouter'),
    [accounts],
  );
  const [selected, setSelected] = useState(0);
  const account = options.find((a) => a.id === selected);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  const [draft, setDraft] = useState(null);
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');
  const [date, setDate] = useState('');
  const [page, setPage] = useState(1);
  const sequence = useRef(0);
  const configIdentity = account
    ? `${account.id}:${account.enabled}:${account.access_token_masked}`
    : '';

  useEffect(() => {
    if (!options.some((a) => a.id === selected))
      setSelected(options[0]?.id || 0);
  }, [options, selected]);

  useEffect(() => {
    const request = ++sequence.current;
    const controller = new AbortController();
    setData(null);
    setError('');
    setLoading(false);
    setDate('');
    setSearch('');
    setPage(1);
    if (!active || !selected || !account?.enabled)
      return () => controller.abort();
    setLoading(true);
    API.get(`${endpoint}/${selected}/openrouter/finance`, {
      signal: controller.signal,
    })
      .then(({ data: response }) => {
        if (sequence.current !== request) return;
        if (!response.success) throw new Error(response.message);
        setData(response.data);
      })
      .catch((e) => {
        if (!controller.signal.aborted && sequence.current === request)
          setError(e.message);
      })
      .finally(() => {
        if (sequence.current === request) setLoading(false);
      });
    return () => {
      ++sequence.current;
      controller.abort();
    };
  }, [selected, configIdentity, active, refresh]);

  const rows = useMemo(
    () => filterOpenRouterActivity(data?.activity, search, date),
    [data, search, date],
  );
  const dates = [...new Set((data?.activity || []).map((r) => r.date))]
    .sort()
    .reverse();
  const spend = rows.reduce((sum, r) => sum + r.usage, 0);
  const credits = data?.credits;
  const save = async () => {
    if (!draft.name.trim() || (!draft.id && !draft.access_token.trim())) return;
    setSaving(true);
    try {
      const payload = {
        ...draft,
        account_type: 'openrouter',
        base_url: 'https://openrouter.ai',
        resource_display_mode: 'wallet',
      };
      const { data: response } = await API[draft.id ? 'put' : 'post'](
        draft.id ? `${endpoint}/${draft.id}` : endpoint,
        payload,
      );
      if (!response.success) throw new Error(response.message);
      await reloadAccounts();
      setSelected(response.data.id);
      setDraft(null);
      setRefresh((n) => n + 1);
      showSuccess(t('保存成功'));
    } catch (e) {
      showError(e.message);
    } finally {
      setSaving(false);
    }
  };
  const remove = () =>
    Modal.confirm({
      title: t('确认删除'),
      okText: t('确认删除'),
      cancelText: t('取消'),
      content: t('确定要删除账户「{{name}}」吗？删除后无法恢复。', {
        name: account.name,
      }),
      onOk: async () => {
        const { data: response } = await API.delete(
          `${endpoint}/${account.id}`,
        );
        if (!response.success) {
          showError(response.message);
          return Promise.reject(new Error(response.message));
        }
        await reloadAccounts();
      },
    });
  const columns = [
    { title: t('日期') + ' (UTC)', dataIndex: 'date', width: 120 },
    { title: t('模型'), dataIndex: 'model', width: 230 },
    { title: t('供应商'), dataIndex: 'provider_name', width: 150 },
    { title: t('请求数'), dataIndex: 'requests', width: 100 },
    { title: t('输入 Token'), dataIndex: 'prompt_tokens', width: 120 },
    { title: t('输出 Token'), dataIndex: 'completion_tokens', width: 120 },
    {
      title: t('消费金额') + ' (USD)',
      dataIndex: 'usage',
      width: 140,
      render: openRouterUSD,
    },
    {
      title: t('BYOK 外部成本') + ' (USD)',
      dataIndex: 'byok_usage_inference',
      width: 170,
      render: openRouterUSD,
    },
  ];

  return (
    <div className='mt-3 space-y-4'>
      <Card>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <Space wrap>
            <Select
              aria-label={t('OpenRouter 账户')}
              placeholder={t('选择账户')}
              value={selected || undefined}
              style={{ minWidth: 220 }}
              optionList={options.map((a) => ({ label: a.name, value: a.id }))}
              onChange={setSelected}
            />
            <Button
              onClick={() =>
                setDraft({ name: '', access_token: '', enabled: true })
              }
            >
              {t('新增 OpenRouter 账户')}
            </Button>
            {account && (
              <>
                <Button
                  onClick={() =>
                    setDraft({
                      id: account.id,
                      name: account.name,
                      remark: account.remark || '',
                      enabled: account.enabled,
                      access_token: '',
                    })
                  }
                >
                  {t('编辑')}
                </Button>
                <Button type='danger' onClick={remove}>
                  {t('删除')}
                </Button>
              </>
            )}
          </Space>
          <Space wrap>
            <Button
              loading={loading}
              disabled={!account?.enabled}
              onClick={() => setRefresh((n) => n + 1)}
            >
              {t('刷新')}
            </Button>
            <a
              href='https://openrouter.ai/settings/credits'
              target='_blank'
              rel='noopener noreferrer'
            >
              {t('OpenRouter 官方账单')}
            </a>
          </Space>
        </div>
        <Text type='tertiary' className='mt-3 block'>
          {t(
            'OpenRouter 金额以 USD 展示；累计购入额度不含支付手续费，逐笔充值订单请查看官方账单。',
          )}
        </Text>
      </Card>
      {!account ? (
        <Empty
          description={t(
            '添加 OpenRouter Management Key 后查看充值、余额和消费明细',
          )}
        />
      ) : !account.enabled ? (
        <Banner type='warning' description={t('已停用')} />
      ) : (
        <>
          {error && <Banner type='danger' description={error} />}
          <Spin spinning={loading}>
            {data?.credits_error && (
              <Banner type='danger' description={data.credits_error} />
            )}
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
              {[
                [t('累计购入额度'), credits?.total_credits],
                [
                  t('钱包余额'),
                  credits ? credits.total_credits - credits.total_usage : null,
                ],
                [t('累计消费'), credits?.total_usage],
                [t('筛选范围消费'), data?.activity ? spend : null],
              ].map(([label, value]) => (
                <Card key={label}>
                  <Text type='tertiary'>{label} (USD)</Text>
                  <div className='mt-2 text-2xl font-semibold tabular-nums'>
                    {openRouterUSD(value)}
                  </div>
                </Card>
              ))}
            </div>
            <Card className='mt-4' title={t('OpenRouter 消费明细')}>
              <Text type='tertiary'>
                {t(
                  '最近 30 个完整 UTC 日，按日期、模型及供应商汇总，不含今日实时用量。BYOK 外部成本不计入 OpenRouter 消费合计。',
                )}
              </Text>
              <div className='my-4 flex flex-wrap gap-3'>
                <Input
                  aria-label={t('搜索模型或供应商')}
                  placeholder={t('搜索模型或供应商')}
                  value={search}
                  showClear
                  onChange={(v) => {
                    setSearch(v);
                    setPage(1);
                  }}
                />
                <Select
                  aria-label={t('日期')}
                  value={date}
                  style={{ minWidth: 160 }}
                  optionList={[
                    { label: t('全部日期'), value: '' },
                    ...dates.map((v) => ({ label: v, value: v })),
                  ]}
                  onChange={(v) => {
                    setDate(v);
                    setPage(1);
                  }}
                />
              </div>
              {data?.activity_error ? (
                <Banner type='danger' description={data.activity_error} />
              ) : (
                <Table
                  columns={columns}
                  dataSource={rows
                    .slice((page - 1) * 20, page * 20)
                    .map((r, i) => ({ ...r, key: (page - 1) * 20 + i }))}
                  loading={loading}
                  scroll={{ x: 1150 }}
                  pagination={{
                    currentPage: page,
                    pageSize: 20,
                    total: rows.length,
                    onPageChange: setPage,
                  }}
                  empty={error ? t('加载失败') : t('暂无数据')}
                />
              )}
              {data?.fetched_at && (
                <Text type='tertiary' size='small'>
                  {t('更新时间')}：
                  {new Date(data.fetched_at * 1000).toLocaleString()}
                </Text>
              )}
            </Card>
          </Spin>
        </>
      )}
      <Modal
        title={t('OpenRouter 账户')}
        okText={t('保存')}
        cancelText={t('取消')}
        visible={!!draft}
        onCancel={() => !saving && setDraft(null)}
        onOk={save}
        confirmLoading={saving}
        okButtonProps={{
          disabled:
            !draft?.name?.trim() ||
            (!draft?.id && !draft?.access_token?.trim()),
        }}
      >
        {draft && (
          <div className='space-y-4'>
            <Text type='tertiary'>
              {t(
                '请使用 OpenRouter Management Key；密钥加密保存，仅用于后台只读查询。',
              )}
            </Text>
            <Input
              aria-label={t('账户名称')}
              placeholder={t('账户名称')}
              value={draft.name}
              onChange={(name) => setDraft((d) => ({ ...d, name }))}
            />
            <Input
              aria-label='Management Key'
              mode='password'
              autoComplete='new-password'
              placeholder={
                draft.id ? t('留空则保留当前密钥') : 'Management Key'
              }
              value={draft.access_token}
              onChange={(access_token) =>
                setDraft((d) => ({ ...d, access_token }))
              }
            />
            <div>
              <Switch
                checked={draft.enabled}
                onChange={(enabled) => setDraft((d) => ({ ...d, enabled }))}
              />{' '}
              <Text>{t('启用')}</Text>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}
