import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Card,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { API } from '../../helpers/apiCore';

const timeText = (value) =>
  value ? new Date(value * 1000).toLocaleString() : '-';

export const feedbackQuery = (page, filters) => {
  const params = new URLSearchParams({ p: String(page), page_size: '20' });
  for (const [key, value] of Object.entries(filters)) {
    if (String(value).trim()) params.set(key, String(value).trim());
  }
  return `/api/desktop-feedback/?${params}`;
};

export default function DesktopFeedback() {
  const { t } = useTranslation();
  const [filters, setFilters] = useState({
    status: '',
    platform: '',
    appVersion: '',
    userId: '',
  });
  const [appliedFilters, setAppliedFilters] = useState(filters);
  const [page, setPage] = useState(1);
  const [rows, setRows] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selectedId, setSelectedId] = useState(null);
  const [detail, setDetail] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');
  const [nextStatus, setNextStatus] = useState('pending');
  const [saving, setSaving] = useState(false);
  const listGeneration = useRef(0);
  const detailGeneration = useRef(0);
  const statuses = [
    { value: 'pending', label: t('待处理'), color: 'amber' },
    { value: 'in_progress', label: t('处理中'), color: 'blue' },
    { value: 'resolved', label: t('已解决'), color: 'green' },
  ];
  const platforms = ['macos', 'windows', 'linux', 'unknown'];
  const statusTag = (value) => {
    const item = statuses.find((item) => item.value === value);
    return <Tag color={item?.color}>{item?.label || value}</Tag>;
  };

  const load = useCallback(async () => {
    const generation = ++listGeneration.current;
    setLoading(true);
    setError('');
    try {
      const response = await API.get(feedbackQuery(page, appliedFilters));
      if (generation !== listGeneration.current) return;
      if (!response.data?.success)
        throw new Error(response.data?.message || t('加载失败'));
      setRows(response.data.data.items);
      setTotal(response.data.data.total);
    } catch (failure) {
      if (generation !== listGeneration.current) return;
      setRows([]);
      setTotal(0);
      setError(
        failure?.response?.data?.message || failure.message || t('加载失败'),
      );
    } finally {
      if (generation === listGeneration.current) setLoading(false);
    }
  }, [page, appliedFilters, t]);

  useEffect(() => {
    void load();
    return () => {
      listGeneration.current += 1;
    };
  }, [load]);

  const openDetail = async (id) => {
    const generation = ++detailGeneration.current;
    setSelectedId(id);
    setDetail(null);
    setDetailError('');
    setDetailLoading(true);
    try {
      const response = await API.get(`/api/desktop-feedback/${id}`);
      if (generation !== detailGeneration.current) return;
      if (!response.data?.success)
        throw new Error(response.data?.message || t('加载失败'));
      setDetail(response.data.data);
      setNextStatus(response.data.data.status);
    } catch (failure) {
      if (generation === detailGeneration.current)
        setDetailError(
          failure?.response?.data?.message || failure.message || t('加载失败'),
        );
    } finally {
      if (generation === detailGeneration.current) setDetailLoading(false);
    }
  };

  const closeDetail = () => {
    if (saving) return;
    detailGeneration.current += 1;
    setSelectedId(null);
    setDetail(null);
  };

  const saveStatus = async () => {
    if (!detail || saving) return;
    setSaving(true);
    setDetailError('');
    try {
      const response = await API.patch(`/api/desktop-feedback/${detail.id}`, {
        status: nextStatus,
      });
      if (!response.data?.success)
        throw new Error(response.data?.message || t('保存失败'));
      setDetail(response.data.data);
      setNextStatus(response.data.data.status);
      void load();
    } catch (failure) {
      setDetailError(
        failure?.response?.data?.message || failure.message || t('保存失败'),
      );
    } finally {
      setSaving(false);
    }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    {
      title: t('创建时间'),
      dataIndex: 'createdAt',
      render: timeText,
      width: 180,
    },
    { title: t('用户 ID'), dataIndex: 'userId', width: 90 },
    { title: t('版本'), dataIndex: 'appVersion', width: 100 },
    { title: t('操作系统'), dataIndex: 'platform', width: 110 },
    {
      title: t('问题描述'),
      dataIndex: 'description',
      render: (value) => (
        <div
          style={{
            maxWidth: 360,
            overflowWrap: 'anywhere',
            whiteSpace: 'pre-wrap',
          }}
        >
          {value.length > 160 ? `${value.slice(0, 160)}…` : value}
        </div>
      ),
    },
    { title: t('状态'), dataIndex: 'status', width: 100, render: statusTag },
    {
      title: t('操作'),
      width: 100,
      render: (_, row) => (
        <Button onClick={() => void openDetail(row.id)}>{t('查看详情')}</Button>
      ),
    },
  ];

  return (
    <div className='p-4 md:p-6'>
      <Card
        title={t('客户端反馈')}
        headerExtraContent={
          <Button loading={loading} onClick={() => void load()}>
            {t('刷新')}
          </Button>
        }
      >
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setPage(1);
            setAppliedFilters({ ...filters });
          }}
        >
          <Space wrap className='mb-4'>
            <Select
              aria-label={t('状态')}
              value={filters.status}
              onChange={(status) => setFilters({ ...filters, status })}
              optionList={[
                { value: '', label: `${t('状态')} · ${t('全部')}` },
                ...statuses,
              ]}
              style={{ width: 170 }}
            />
            <Select
              aria-label={t('操作系统')}
              value={filters.platform}
              onChange={(platform) => setFilters({ ...filters, platform })}
              optionList={[
                { value: '', label: `${t('操作系统')} · ${t('全部')}` },
                ...platforms.map((value) => ({ value, label: value })),
              ]}
              style={{ width: 170 }}
            />
            <Input
              aria-label={t('版本')}
              placeholder={t('版本')}
              value={filters.appVersion}
              maxLength={32}
              onChange={(appVersion) => setFilters({ ...filters, appVersion })}
              style={{ width: 140 }}
            />
            <Input
              aria-label={t('用户 ID')}
              placeholder={t('用户 ID')}
              value={filters.userId}
              onChange={(userId) => setFilters({ ...filters, userId })}
              style={{ width: 140 }}
            />
            <Button htmlType='submit'>{t('查询')}</Button>
          </Space>
        </form>
        {error && (
          <Banner
            type='danger'
            description={error}
            className='mb-4'
            closeIcon={null}
          />
        )}
        <Table
          rowKey='id'
          columns={columns}
          dataSource={rows}
          loading={loading}
          empty={t('暂无数据')}
          scroll={{ x: 1000 }}
          pagination={{
            currentPage: page,
            pageSize: 20,
            total,
            showSizeChanger: false,
            onPageChange: setPage,
          }}
        />
      </Card>
      <Modal
        title={`${t('反馈详情')}${selectedId ? ` #${selectedId}` : ''}`}
        visible={selectedId !== null}
        onCancel={closeDetail}
        width='min(760px, calc(100vw - 32px))'
        bodyStyle={{ maxHeight: '65vh', overflowY: 'auto' }}
        footer={null}
        maskClosable={!saving}
        closable={!saving}
      >
        {detailLoading && <Spin />}
        {detailError && (
          <Banner
            type='danger'
            description={detailError}
            closeIcon={null}
            className='mb-4'
          />
        )}
        {!detail && !detailLoading && selectedId !== null && (
          <Button onClick={() => void openDetail(selectedId)}>
            {t('重试')}
          </Button>
        )}
        {detail && (
          <div className='space-y-4'>
            <Typography.Text type='tertiary'>
              {t('用户 ID')}: {detail.userId} · {t('版本')}: {detail.appVersion}{' '}
              · {detail.platform}
            </Typography.Text>
            <div>
              {t('创建时间')}: {timeText(detail.createdAt)}
              <br />
              {t('更新时间')}: {timeText(detail.updatedAt)}
              <br />
              {t('页面')}: {detail.context.view || '-'} · {t('工具')}:{' '}
              {detail.context.tool || '-'}
            </div>
            <div>
              <Typography.Title heading={6}>{t('问题描述')}</Typography.Title>
              <pre className='whitespace-pre-wrap break-words rounded-lg bg-black/5 p-3 dark:bg-white/5'>
                {detail.description}
              </pre>
            </div>
            <div>
              <Typography.Title heading={6}>{t('错误信息')}</Typography.Title>
              {detail.errors.length ? (
                detail.errors.map((item, index) => (
                  <div key={index} className='mb-3 rounded-lg border p-3'>
                    <div className='text-xs opacity-70'>
                      {item.occurredAt} · {t('来源')}: {item.source}
                    </div>
                    <pre className='mt-2 whitespace-pre-wrap break-words'>
                      {item.message}
                    </pre>
                  </div>
                ))
              ) : (
                <div>{t('暂无数据')}</div>
              )}
            </div>
            <Space>
              <Select
                aria-label={t('状态')}
                value={nextStatus}
                onChange={setNextStatus}
                disabled={saving}
                optionList={statuses}
                style={{ width: 150 }}
              />
              <Button
                theme='solid'
                loading={saving}
                disabled={saving || nextStatus === detail.status}
                onClick={() => void saveStatus()}
              >
                {t('保存')}
              </Button>
            </Space>
          </div>
        )}
      </Modal>
    </div>
  );
}
