import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  TagInput,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { API } from '../../helpers/apiCore';
import { showError, showSuccess } from '../../helpers/toast';
import { formToPlan, planToForm } from './format';

const { Text } = Typography;

const Field = ({ label, extra, children }) => (
  <div style={{ marginBottom: 16 }}>
    <Text strong>{label}</Text>
    <div style={{ marginTop: 6 }}>{children}</div>
    {extra && (
      <Text type='tertiary' size='small'>
        {extra}
      </Text>
    )}
  </div>
);

export default function RoutePlansTab() {
  const { t } = useTranslation();
  const [plans, setPlans] = useState([]);
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [editing, setEditing] = useState(null);
  const [saving, setSaving] = useState(false);
  const generation = useRef(0);

  const load = useCallback(async () => {
    const current = ++generation.current;
    setLoading(true);
    setError('');
    try {
      const [planResponse, groupResponse] = await Promise.all([
        API.get('/api/route_plan/'),
        API.get('/api/group/'),
      ]);
      if (current !== generation.current) return;
      if (!planResponse.data?.success)
        throw new Error(planResponse.data?.message);
      setPlans(planResponse.data.data || []);
      setGroups(groupResponse.data?.success ? groupResponse.data.data : []);
    } catch (failure) {
      if (current !== generation.current) return;
      setError(
        failure?.response?.data?.message || failure?.message || t('加载失败'),
      );
    } finally {
      if (current === generation.current) setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const save = async () => {
    let payload;
    try {
      payload = formToPlan(editing);
    } catch {
      showError(t('模型单独定价必须是 JSON 对象，值为非负数'));
      return;
    }
    if (!payload.name) {
      showError(t('请填写方案名称'));
      return;
    }
    if ((editing.groups || []).length === 0) {
      showError(t('请至少选择一个线路分组'));
      return;
    }
    setSaving(true);
    try {
      const response = payload.id
        ? await API.put('/api/route_plan/', payload)
        : await API.post('/api/route_plan/', payload);
      if (!response.data?.success) throw new Error(response.data?.message);
      showSuccess(t('保存成功'));
      setEditing(null);
      load();
    } catch (failure) {
      showError(failure?.response?.data?.message || failure?.message);
    } finally {
      setSaving(false);
    }
  };

  const remove = async (id) => {
    try {
      const response = await API.delete(`/api/route_plan/${id}`);
      if (!response.data?.success) throw new Error(response.data?.message);
      showSuccess(t('删除成功'));
      load();
    } catch (failure) {
      showError(failure?.response?.data?.message || failure?.message);
    }
  };

  const parseList = (raw) => {
    try {
      return JSON.parse(raw || '[]');
    } catch {
      return [];
    }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: t('方案'),
      render: (_, row) => (
        <Space vertical align='start' spacing={2}>
          <Text strong>{row.name}</Text>
          {row.description && (
            <Text type='tertiary' size='small'>
              {row.description}
            </Text>
          )}
        </Space>
      ),
    },
    {
      title: t('状态'),
      render: (_, row) =>
        row.status === 1 ? (
          <Tag color='green'>{t('已启用')}</Tag>
        ) : (
          <Tag color='grey'>{t('已停用')}</Tag>
        ),
    },
    {
      title: t('线路分组（顺序）'),
      render: (_, row) => (
        <Space wrap>
          {parseList(row.groups).map((group) => (
            <Tag key={group}>{group}</Tag>
          ))}
        </Space>
      ),
    },
    {
      title: t('默认售价倍率'),
      dataIndex: 'sale_ratio',
    },
    {
      title: t('单独定价模型'),
      render: (_, row) => {
        try {
          return Object.keys(JSON.parse(row.model_ratios || '{}')).length;
        } catch {
          return '-';
        }
      },
    },
    {
      title: t('授权（用户 / 用户组）'),
      render: (_, row) =>
        `${parseList(row.user_ids).length} / ${parseList(row.user_groups).length}`,
    },
    { title: t('令牌数'), dataIndex: 'token_count' },
    { title: t('版本'), dataIndex: 'version' },
    {
      title: t('操作'),
      render: (_, row) => (
        <Space>
          <Button size='small' onClick={() => setEditing(planToForm(row))}>
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('确定删除该方案？')}
            onConfirm={() => remove(row.id)}
          >
            <Button size='small' type='danger' disabled={row.token_count > 0}>
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const groupOptions = groups.map((group) => ({ value: group, label: group }));
  const update = (patch) => setEditing((current) => ({ ...current, ...patch }));

  return (
    <Space vertical align='start' style={{ width: '100%' }} spacing='medium'>
      <Banner
        type='info'
        description={t(
          '方案把多个线路分组打包成对客户的报价：始终按方案价格计费，不受实际分组、用户组专属倍率和时段倍率影响。仅授权的用户或用户组可选用。',
        )}
      />
      {error && <Banner type='danger' description={error} />}
      <Space>
        <Button theme='solid' onClick={() => setEditing(planToForm(null))}>
          {t('新建方案')}
        </Button>
        <Button onClick={load} loading={loading}>
          {t('刷新')}
        </Button>
      </Space>
      <Table
        size='small'
        rowKey='id'
        loading={loading}
        columns={columns}
        dataSource={plans}
        pagination={false}
        scroll={{ x: 'max-content' }}
        style={{ width: '100%' }}
        empty={t('暂无路由方案')}
      />
      <Modal
        title={editing?.id ? t('编辑路由方案') : t('新建方案')}
        visible={!!editing}
        onCancel={() => setEditing(null)}
        onOk={save}
        okButtonProps={{ loading: saving }}
        width={640}
      >
        {editing && (
          <div>
            <Field label={t('方案名称')}>
              <Input
                value={editing.name}
                maxLength={64}
                onChange={(name) => update({ name })}
                placeholder={t('例如：企业稳定优先')}
              />
            </Field>
            <Field label={t('说明（客户可见）')}>
              <TextArea
                value={editing.description}
                autosize={{ minRows: 2, maxRows: 4 }}
                onChange={(description) => update({ description })}
              />
            </Field>
            <Field label={t('启用')}>
              <Switch
                checked={editing.status === 1}
                onChange={(checked) => update({ status: checked ? 1 : 2 })}
              />
            </Field>
            <Field
              label={t('线路分组')}
              extra={t(
                '按顺序回退；开启方案智能路由后跨分组按健康和成本选择。',
              )}
            >
              <Select
                multiple
                filter
                value={editing.groups}
                optionList={groupOptions}
                onChange={(value) => update({ groups: value })}
                style={{ width: '100%' }}
              />
            </Field>
            <Field
              label={t('默认售价倍率')}
              extra={t('最终单价 = 模型基础价 × 该倍率')}
            >
              <InputNumber
                min={0}
                max={1000}
                step={0.01}
                value={editing.sale_ratio}
                onChange={(sale_ratio) => update({ sale_ratio })}
                style={{ width: 200 }}
              />
            </Field>
            <Field
              label={t('模型单独定价')}
              extra={t('例如 {"gpt-5": 1.2}，未列出的模型用默认倍率')}
            >
              <TextArea
                value={editing.model_ratios}
                autosize={{ minRows: 3, maxRows: 8 }}
                onChange={(model_ratios) => update({ model_ratios })}
                style={{ fontFamily: 'monospace' }}
              />
            </Field>
            <Field label={t('授权用户 ID')}>
              <TagInput
                value={editing.user_ids}
                onChange={(user_ids) => update({ user_ids })}
                placeholder={t('输入用户 ID 后回车')}
                separator={[',', ' ']}
                style={{ width: '100%' }}
              />
            </Field>
            <Field label={t('授权用户组')}>
              <Select
                multiple
                filter
                allowCreate
                value={editing.user_groups}
                optionList={groupOptions}
                onChange={(value) => update({ user_groups: value })}
                style={{ width: '100%' }}
              />
            </Field>
            {editing.id > 0 && (
              <Text type='tertiary' size='small'>
                {t('保存后版本加一，30 秒内对新请求生效。')}
              </Text>
            )}
          </div>
        )}
      </Modal>
    </Space>
  );
}
