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

import '../Enterprise/enterpriseApplication.css';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Button, Input, Modal, Select, Table, Tag } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  enterpriseRequest,
  formatEnterpriseDate,
} from '../../helpers/enterprise';
import {
  CONTACT_TYPES,
  APPLICATION_KINDS,
  APPLICATION_STATUS,
} from '../../helpers/enterpriseApplications';
import { showError, showSuccess } from '../../helpers/toast';

export default function Applications({
  groups,
  groupsLoading,
  groupsError,
  loadGroups,
  onDone,
}) {
  const { t } = useTranslation();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState('pending');
  const [kind, setKind] = useState('');
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [review, setReview] = useState(null);
  const [group, setGroup] = useState();
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const sequence = useRef(0);
  const load = useCallback(async () => {
    const current = ++sequence.current;
    setLoading(true);
    setFailed(false);
    setItems([]);
    try {
      const data = await enterpriseRequest(
        'get',
        '/api/enterprise/admin/applications',
        undefined,
        { p: page, page_size: 20, status, kind },
      );
      if (current === sequence.current) {
        setItems(data.items || []);
        setTotal(data.total || 0);
      }
    } catch (e) {
      if (current === sequence.current) {
        setFailed(true);
        showError(e.message);
      }
    } finally {
      if (current === sequence.current) setLoading(false);
    }
  }, [page, status, kind]);
  useEffect(() => {
    load();
    return () => {
      ++sequence.current;
    };
  }, [load]);
  const open = (item, action) => {
    setReview({ item, action });
    setReason('');
    setGroup(undefined);
    if (action === 'approve') loadGroups();
  };
  const submit = async () => {
    setBusy(true);
    try {
      await enterpriseRequest(
        'post',
        `/api/enterprise/admin/applications/${review.item.id}/review`,
        {
          action: review.action,
          billing_group: group,
          reason: reason.trim(),
        },
      );
      showSuccess(t('已保存'));
      setReview(null);
      load();
      onDone();
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };
  const columns = [
    {
      title: t('申请类型'),
      dataIndex: 'kind',
      render: (v) => t(APPLICATION_KINDS[v]),
    },
    { title: t('企业名称'), dataIndex: 'name', render: (v) => v || '—' },
    { title: t('用户 ID'), dataIndex: 'user_id' },
    {
      title: t('联系方式'),
      dataIndex: 'contact',
      render: (v, row) => (
        <div style={{ overflowWrap: 'anywhere', maxWidth: 260 }}>
          <strong>{t(CONTACT_TYPES[row.contact_type])}</strong>
          <div style={{ userSelect: 'text' }}>{v}</div>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (v) => (
        <Tag
          color={
            v === 'pending' ? 'orange' : v === 'rejected' ? 'red' : 'green'
          }
        >
          {t(APPLICATION_STATUS[v])}
        </Tag>
      ),
    },
    {
      title: t('申请时间'),
      dataIndex: 'created_at',
      render: formatEnterpriseDate,
    },
    { title: t('备注'), dataIndex: 'reason' },
    {
      title: t('操作'),
      render: (_, row) =>
        row.status === 'pending' ? (
          <div className='em-actions'>
            <Button
              size='small'
              onClick={() =>
                open(row, row.kind === 'opening' ? 'approve' : 'contact')
              }
            >
              {t(row.kind === 'opening' ? '审核并开通' : '标记已联系')}
            </Button>
            <Button
              size='small'
              type='danger'
              onClick={() => open(row, 'reject')}
            >
              {t('拒绝')}
            </Button>
          </div>
        ) : (
          <span>
            {row.reviewer_id
              ? `#${row.reviewer_id} · ${formatEnterpriseDate(row.reviewed_at)}`
              : '—'}
          </span>
        ),
    },
  ];
  return (
    <div className='ea-pane'>
      <div className='em-tools' style={{ flexWrap: 'wrap', marginBottom: 16 }}>
        <Select
          aria-label={t('申请类型')}
          value={kind}
          onChange={(v) => {
            setKind(v);
            setPage(1);
          }}
          optionList={[
            { value: '', label: t('全部') },
            ...Object.entries(APPLICATION_KINDS).map(([value, label]) => ({
              value,
              label: t(label),
            })),
          ]}
        />
        <Select
          aria-label={t('状态')}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          optionList={[
            { value: '', label: t('全部') },
            ...Object.entries(APPLICATION_STATUS).map(([value, label]) => ({
              value,
              label: t(label),
            })),
          ]}
        />
        <Button onClick={load}>{t(failed ? '重试' : '刷新')}</Button>
      </div>
      <div style={{ overflowX: 'auto' }}>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={items}
          loading={loading}
          empty={failed ? t('操作失败，请稍后重试') : t('暂无数据')}
          pagination={{
            currentPage: page,
            pageSize: 20,
            total,
            onPageChange: setPage,
          }}
        />
      </div>
      {review && (
        <Modal
          className='enterprise-application-modal'
          width='min(520px, calc(100vw - 32px))'
          visible
          title={t(
            review.action === 'approve'
              ? '审核并开通'
              : review.action === 'contact'
                ? '标记已联系'
                : '拒绝',
          )}
          onCancel={() => setReview(null)}
          onOk={submit}
          confirmLoading={busy}
          closable={!busy}
          maskClosable={!busy}
          cancelButtonProps={{ disabled: busy }}
          okButtonProps={{
            disabled:
              review.action === 'approve'
                ? groupsLoading ||
                  groupsError ||
                  !groups.some((g) => g.name === group)
                : review.action === 'reject' && !reason.trim(),
          }}
        >
          <p>
            {review.item.name || t('定制需求')} · #{review.item.user_id}
          </p>
          <p style={{ overflowWrap: 'anywhere' }}>
            {t(CONTACT_TYPES[review.item.contact_type])} · {review.item.contact}
          </p>
          {review.action === 'approve' && (
            <label className='ec-field'>
              {t('计费分组')}
              <Select
                aria-label={t('计费分组')}
                value={group}
                onChange={setGroup}
                loading={groupsLoading}
                disabled={groupsLoading || groupsError || busy}
                style={{ width: '100%' }}
                optionList={groups.map((g) => ({
                  value: g.name,
                  label: `${g.name} · ${g.ratio}×`,
                }))}
              />
              {groupsError && <Button onClick={loadGroups}>{t('重试')}</Button>}
            </label>
          )}
          <label className='ec-field'>
            {t('备注')}
            <Input
              aria-label={t('备注')}
              maxLength={200}
              value={reason}
              onChange={setReason}
              disabled={busy}
              placeholder={
                review.action === 'reject' ? t('请填写拒绝原因') : undefined
              }
            />
          </label>
        </Modal>
      )}
    </div>
  );
}
