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
  Tabs,
  TabPane,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../helpers';
import OpenRouterCashDashboard from './OpenRouterCashDashboard';
import OpenRouterConsumption from './OpenRouterConsumption';

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
  const [view, setView] = useState('balance');
  const account = options.find((a) => a.id === selected);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  const [draft, setDraft] = useState(null);
  const [saving, setSaving] = useState(false);
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
    if (!active || view !== 'balance' || !selected || !account?.enabled)
      return () => controller.abort();
    setLoading(true);
    API.get(`${endpoint}/${selected}/openrouter/finance`, {
      signal: controller.signal,
      skipErrorHandler: true,
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
  }, [selected, configIdentity, active, refresh, view]);

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
        {view === 'balance' && (
          <Text type='tertiary' className='mt-3 block'>
            {t(
              'OpenRouter 金额以 USD 展示；累计购入额度不含支付手续费，逐笔充值订单请查看官方账单。',
            )}
          </Text>
        )}
      </Card>
      {account && (
        <Tabs
          activeKey={view}
          onChange={setView}
          contentStyle={{ display: 'none' }}
        >
          {[
            ['balance', '模型消耗看板'],
            ['profit', '现金收益'],
            ['flow', '现金流'],
            ['review', '待核对'],
          ].map(([key, label]) => (
            <TabPane itemKey={key} tab={t(label)} key={key} />
          ))}
        </Tabs>
      )}
      {!account ? (
        <Empty
          description={t(
            '添加 OpenRouter Management Key 后查看充值、余额和消费明细',
          )}
        />
      ) : view !== 'balance' ? (
        <>
          {!account.enabled && (
            <Banner type='warning' description={t('已停用')} />
          )}
          <OpenRouterCashDashboard
            key={account.id}
            account={account}
            active={active}
            view={view}
            onViewChange={setView}
            refresh={refresh}
            t={t}
          />
        </>
      ) : !account.enabled ? (
        <Banner type='warning' description={t('已停用')} />
      ) : (
        <>
          {error && <Banner type='danger' description={error} />}
          <Spin spinning={loading}>
            <OpenRouterConsumption data={data} loading={loading} t={t} />
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
            {draft.id ? (
              <div>
                <Switch
                  aria-label={t('启用')}
                  checked={draft.enabled}
                  onChange={(enabled) => setDraft((d) => ({ ...d, enabled }))}
                />{' '}
                <Text>{t('启用')}</Text>
              </div>
            ) : null}
          </div>
        )}
      </Modal>
    </div>
  );
}
