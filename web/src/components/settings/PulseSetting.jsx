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

import React, { useEffect, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Checkbox,
  Collapse,
  Input,
  Modal,
  Select,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import {
  buildPulseSettingsUpdate,
  pulseQuotaToUnits,
} from '../../helpers/pulseSettings';

const { Text, Title } = Typography;

const PulseSetting = () => {
  const { t, i18n } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [snapshot, setSnapshot] = useState(null);
  const [config, setConfig] = useState({});
  const [secrets, setSecrets] = useState({});
  const [clearSecrets, setClearSecrets] = useState([]);
  const [generating, setGenerating] = useState(null);
  const [generatedSecret, setGeneratedSecret] = useState(null);

  const secretFields = [
    {
      key: 'PulseUserBFFHMACSecret',
      label: t('Pulse 用户侧密钥'),
      counterpart: 'PULSE_USER_BFF_HMAC_SECRET',
    },
    {
      key: 'PulseAdminHMACSecret',
      label: t('Pulse 运营侧密钥'),
      counterpart: 'PULSE_ADMIN_HMAC_SECRET',
    },
    {
      key: 'PulseServiceHMACSecret',
      label: t('Pulse 自动发奖密钥'),
      counterpart: 'PULSE_SERVICE_HMAC_SECRET',
    },
    {
      key: 'PulseRollbackHMACSecret',
      label: t('Pulse 奖励撤销密钥'),
      counterpart: 'PULSE_ROLLBACK_HMAC_SECRET',
    },
    {
      key: 'PulseForumSSOSecret',
      label: t('社区 SSO 密钥'),
      counterpart: 'sso_hmac_secret',
      forum: true,
    },
  ];
  const previousFields = [
    {
      key: 'PulseServiceHMACSecretPrevious',
      label: t('上一组自动发奖密钥'),
    },
    {
      key: 'PulseRollbackHMACSecretPrevious',
      label: t('上一组奖励撤销密钥'),
    },
    { key: 'PulseForumSSOSecretPrevious', label: t('上一组社区 SSO 密钥') },
  ];
  const quotaFields = [
    { key: 'PulseBenefitMaxGrantQuota', label: t('单笔奖励上限') },
    { key: 'PulseBenefitUserDailyQuota', label: t('每用户每日奖励上限') },
    { key: 'PulseBenefitDailyQuota', label: t('全站每日奖励上限') },
  ];

  const applySnapshot = (data) => {
    setSnapshot(data);
    setConfig(data.config);
    setSecrets({});
    setClearSecrets([]);
    setGeneratedSecret(null);
  };

  const load = async () => {
    setLoading(true);
    try {
      const response = await API.get('/api/option/pulse', {
        skipErrorHandler: true,
      });
      if (!response.data.success) throw new Error(response.data.message);
      applySnapshot(response.data.data);
    } catch (error) {
      showError(
        error?.response?.data?.message || error.message || t('加载失败'),
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const setValue = (key, value) => {
    setConfig((previous) => ({ ...previous, [key]: value }));
  };

  const save = async () => {
    const payload = buildPulseSettingsUpdate(
      config,
      snapshot.config,
      secrets,
      clearSecrets,
    );
    if (
      Object.keys(payload.config).length === 0 &&
      payload.clear_secrets.length === 0
    ) {
      showSuccess(t('没有需要保存的更改'));
      return;
    }
    setSaving(true);
    try {
      const response = await API.put('/api/option/pulse', payload, {
        skipErrorHandler: true,
      });
      if (!response.data.success) throw new Error(response.data.message);
      applySnapshot(response.data.data);
      showSuccess(t('Pulse 设置已保存并生效'));
    } catch (error) {
      showError(
        error?.response?.data?.message || error.message || t('保存失败'),
      );
    } finally {
      setSaving(false);
    }
  };

  const generate = async (field) => {
    setGenerating(field.key);
    try {
      const response = await API.get('/api/pulse/ops/secret/generate', {
        skipErrorHandler: true,
      });
      if (!response.data.success) throw new Error(response.data.message);
      setGeneratedSecret({ ...field, value: response.data.data });
    } catch (error) {
      showError(
        error?.response?.data?.message || error.message || t('生成密钥失败'),
      );
    } finally {
      setGenerating(null);
    }
  };

  const renderSecret = (field, previous = false) => (
    <div key={field.key} className='space-y-2'>
      <div className='flex items-center gap-2'>
        <label htmlFor={field.key}>{field.label}</label>
        <Tag color={snapshot.secrets[field.key] ? 'green' : 'grey'}>
          {snapshot.secrets[field.key] ? t('已配置') : t('未配置')}
        </Tag>
      </div>
      <Input
        id={field.key}
        type='password'
        autoComplete='new-password'
        value={secrets[field.key] || ''}
        disabled={saving || clearSecrets.includes(field.key)}
        placeholder={t('留空保留现有密钥')}
        onChange={(value) =>
          setSecrets((values) => ({ ...values, [field.key]: value }))
        }
      />
      {field.counterpart && (
        <Text type='tertiary' size='small'>
          {field.forum
            ? t('对应社区插件的 {{name}}', { name: field.counterpart })
            : t('对应 Pulse 的 {{name}}', { name: field.counterpart })}
        </Text>
      )}
      <div className='flex flex-wrap items-center gap-3'>
        {!previous && (
          <Button
            size='small'
            theme='light'
            loading={generating === field.key}
            disabled={saving || generating !== null}
            onClick={() => generate(field)}
          >
            {t('为此用途生成新密钥')}
          </Button>
        )}
        {snapshot.secrets[field.key] && (
          <Checkbox
            disabled={saving}
            checked={clearSecrets.includes(field.key)}
            onChange={(event) =>
              setClearSecrets((keys) =>
                event.target.checked
                  ? [...keys, field.key]
                  : keys.filter((key) => key !== field.key),
              )
            }
          >
            {t('保存时清除此密钥')}
          </Checkbox>
        )}
      </div>
    </div>
  );

  return (
    <Card className='!mt-3' title={t('配置 Meta Pulse 对接')}>
      {loading ? (
        <div className='flex justify-center p-8'>
          <Spin />
        </div>
      ) : !snapshot ? (
        <Button onClick={load}>{t('重新加载')}</Button>
      ) : (
        <div className='space-y-5'>
          <Banner
            type='info'
            closeIcon={null}
            description={t(
              '保存后在当前实例即时生效，无需重建容器；其他实例将在配置同步后生效（默认约 60 秒）。未修改的配置保持不变，密钥不回显，留空保留。',
            )}
          />
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='space-y-2'>
              <label htmlFor='PulseInternalURL'>{t('Pulse 内网地址')}</label>
              <Input
                id='PulseInternalURL'
                value={config.PulseInternalURL}
                placeholder='http://pulse-api:8088'
                disabled={saving}
                onChange={(value) => setValue('PulseInternalURL', value)}
              />
            </div>
            <div className='space-y-2'>
              <label htmlFor='PulseEnv'>{t('Pulse 运行环境')}</label>
              <Select
                id='PulseEnv'
                className='w-full'
                value={config.PulseEnv}
                disabled={saving}
                onChange={(value) => setValue('PulseEnv', value)}
                optionList={[
                  { value: 'production', label: t('生产环境（推荐）') },
                  { value: 'development', label: t('开发环境') },
                  { value: 'test', label: t('测试环境') },
                ]}
              />
            </div>
          </div>
          <Title heading={6}>{t('密钥与社区绑定')}</Title>
          <Text type='tertiary'>
            {t(
              '不同用途必须使用不同密钥，并与 Pulse 或社区插件的对应字段保持一致。只在缺少密钥或主动轮换时生成。',
            )}
          </Text>
          <div className='grid gap-5 md:grid-cols-2'>
            {secretFields.map((field) => renderSecret(field))}
            <div className='space-y-2'>
              <label htmlFor='PulseForumSSOCallbackURL'>
                {t('社区 SSO 回调地址')}
              </label>
              <Input
                id='PulseForumSSOCallbackURL'
                value={config.PulseForumSSOCallbackURL}
                placeholder='https://metar.uk/api/user-center/login/callback'
                disabled={saving}
                onChange={(value) =>
                  setValue('PulseForumSSOCallbackURL', value)
                }
              />
            </div>
          </div>
          <Collapse>
            <Collapse.Panel
              header={t('高级：密钥轮换兼容')}
              itemKey='previous-secrets'
            >
              <Text type='tertiary'>
                {t(
                  '轮换期间可填写上一组密钥以兼容尚未同步的服务。同步完成后勾选清除并保存，结束旧密钥兼容。',
                )}
              </Text>
              <div className='mt-4 grid gap-5 md:grid-cols-2'>
                {previousFields.map((field) => renderSecret(field, true))}
              </div>
            </Collapse.Panel>
          </Collapse>

          <Title heading={6}>{t('自动发奖与限额')}</Title>
          <Text type='tertiary'>
            {t(
              '限额填写内部 quota，不是人民币。每 1 API 额度单位 = {{quota}} quota；每日上限按北京时间自然日累计，撤销奖励不恢复当日额度。',
              { quota: snapshot.quota_per_unit },
            )}
          </Text>
          <div className='grid gap-4 md:grid-cols-3'>
            {quotaFields.map((field) => {
              const units = pulseQuotaToUnits(
                config[field.key],
                snapshot.quota_per_unit,
              );
              return (
                <div key={field.key} className='space-y-2'>
                  <label htmlFor={field.key}>{field.label}</label>
                  <Input
                    id={field.key}
                    inputMode='numeric'
                    value={config[field.key]}
                    disabled={saving}
                    suffix='quota'
                    placeholder={t('请输入正整数')}
                    onChange={(value) => setValue(field.key, value)}
                  />
                  <Text type='tertiary' size='small'>
                    {units === null
                      ? t('未设置有效限额')
                      : t('约 {{units}} API 额度单位', {
                          units: units.toLocaleString(i18n.resolvedLanguage, {
                            maximumFractionDigits: 10,
                          }),
                        })}
                  </Text>
                </div>
              );
            })}
          </div>
          {!snapshot.redis_ready && (
            <Banner
              type='warning'
              closeIcon={null}
              description={t(
                '尚未配置 Redis 连接，生产环境自动发奖需要 Redis 提供防重放保护。',
              )}
            />
          )}
          <div className='space-y-2'>
            <div className='flex items-center gap-3'>
              <Switch
                aria-label={t('启用自动发奖')}
                checked={config.PulseBenefitEnabled === 'true'}
                disabled={saving}
                onChange={(value) =>
                  setValue('PulseBenefitEnabled', String(value))
                }
              />
              <Text>{t('启用自动发奖')}</Text>
            </div>
            <Text type='tertiary' size='small'>
              {t(
                '先完成密钥、限额和奖池配置，再启用此开关。Pulse 侧还需关闭影子模式并启用动作。',
              )}
            </Text>
          </div>
          <Button
            theme='solid'
            loading={saving}
            disabled={generating !== null}
            onClick={save}
          >
            {t('保存 Pulse 设置')}
          </Button>
        </div>
      )}
      <Modal
        title={t('为 {{name}} 生成的新密钥', {
          name: generatedSecret?.label || '',
        })}
        visible={generatedSecret !== null}
        onCancel={() => setGeneratedSecret(null)}
        okText={t('填入此字段')}
        cancelText={t('取消')}
        onOk={() => {
          if (!generatedSecret) return;
          setSecrets((values) => ({
            ...values,
            [generatedSecret.key]: generatedSecret.value,
          }));
          setClearSecrets((keys) =>
            keys.filter((key) => key !== generatedSecret.key),
          );
          setGeneratedSecret(null);
        }}
      >
        <Banner
          type='warning'
          closeIcon={null}
          description={t(
            '请先复制并同步到对应服务，再填入此字段并保存。此密钥仅用于当前用途，保存后不会再次显示。',
          )}
        />
        <Typography.Paragraph
          className='!mt-4 break-all font-mono'
          copyable={{ content: generatedSecret?.value || '' }}
        >
          {generatedSecret?.value}
        </Typography.Paragraph>
      </Modal>
    </Card>
  );
};

export default PulseSetting;
