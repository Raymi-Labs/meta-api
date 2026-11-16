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

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Input, Modal, Spin } from '@douyinfe/semi-ui';
import { enterpriseRequest } from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { showError, showSuccess } from '../../helpers/toast';

export default function ArchiveModal({ enterprise, onClose, onDone }) {
  const { t } = useTranslation();
  const [preview, setPreview] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState('');
  const [reason, setReason] = useState('');
  const sequence = useRef(0);
  const submitting = useRef(false);
  const base = `/api/enterprise/admin/enterprises/${enterprise.id}`;
  const load = useCallback(async () => {
    const id = ++sequence.current;
    setLoading(true);
    setError('');
    setPreview(null);
    try {
      const data = await enterpriseRequest('get', `${base}/archive-preview`);
      if (id === sequence.current) setPreview(data);
    } catch (e) {
      if (id === sequence.current) setError(e.message);
    } finally {
      if (id === sequence.current) setLoading(false);
    }
  }, [base]);
  useEffect(() => {
    load();
    return () => {
      ++sequence.current;
    };
  }, [load]);
  const blockers = {
    archived: t('企业已归档'),
    not_frozen: t('请先冻结企业'),
    unbalanced: t('账本不平衡，请先核对流水'),
    remaining_balance: t('净余额须结清，请先处理退款或调账'),
    open_reservations: t('仍有预留或待对账请求'),
    pending_tasks: t('仍有未完成结算的视频或音乐任务'),
    pending_midjourney: t('仍有未完成结算的 Midjourney 任务'),
  };
  const valid =
    !loading &&
    !error &&
    preview?.can_archive &&
    name === preview.enterprise.name &&
    reason.trim();
  const submit = async () => {
    if (!valid || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    try {
      await enterpriseRequest('post', `${base}/archive`, {
        name,
        reason: reason.trim(),
      });
      showSuccess(t('已归档'));
      onDone();
      onClose();
    } catch (e) {
      showError(e.message);
      await load();
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  };
  return (
    <Modal
      className='ec-modal ea-archive-modal'
      title={t('归档 {{name}}', {
        name: preview?.enterprise?.name || enterprise.name,
      })}
      visible
      okText={t('归档企业')}
      okButtonProps={{ type: 'danger', disabled: !valid || busy }}
      confirmLoading={busy}
      onOk={submit}
      onCancel={() => !busy && onClose()}
      maskClosable={!busy}
      closeOnEsc={!busy}
      closable={!busy}
      cancelButtonProps={{ disabled: busy }}
      style={{ maxWidth: 'calc(100vw - 24px)' }}
      bodyStyle={{ maxHeight: 'calc(100dvh - 180px)', overflowY: 'auto' }}
    >
      <p className='ea-warn'>
        {t('归档后无法恢复。成员退出、企业令牌停用；账单和操作记录保留。')}
      </p>
      {loading && <Spin aria-label={t('加载中')} />}
      {error && (
        <div role='alert' className='ea-load-error'>
          {error}
          <Button onClick={load}>{t('重试')}</Button>
        </div>
      )}
      {preview && (
        <>
          <dl className='ea-archive-stats'>
            <div>
              <dt>{t('净余额')}</dt>
              <dd>{renderQuota(preview.remaining, 8)}</dd>
            </div>
            <div>
              <dt>{t('原始额度')}</dt>
              <dd>{preview.remaining}</dd>
            </div>
            <div>
              <dt>{t('成员')}</dt>
              <dd>{preview.members}</dd>
            </div>
            <div>
              <dt>{t('令牌')}</dt>
              <dd>{preview.tokens}</dd>
            </div>
            <div>
              <dt>{t('待对账')}</dt>
              <dd>{preview.open_reservations}</dd>
            </div>
            <div>
              <dt>{t('异步任务')}</dt>
              <dd>{preview.pending_tasks + preview.pending_midjourney}</dd>
            </div>
          </dl>
          {preview.blockers.length > 0 && (
            <ul className='ea-archive-blockers' role='alert'>
              {preview.blockers.map((code) => (
                <li key={code}>{blockers[code] || t('操作失败')}</li>
              ))}
            </ul>
          )}
          <Button onClick={load} disabled={busy}>
            {t('重新检查')}
          </Button>
          {preview.can_archive && (
            <div className='ea-archive-confirm'>
              <label className='ec-field' htmlFor='archive-name'>
                {t('输入完整企业名称')}
                <Input
                  id='archive-name'
                  value={name}
                  onChange={setName}
                  disabled={busy}
                  maxLength={128}
                  autoComplete='off'
                />
              </label>
              <label className='ec-field' htmlFor='archive-reason'>
                {t('归档原因')}
                <Input
                  id='archive-reason'
                  value={reason}
                  onChange={setReason}
                  disabled={busy}
                  maxLength={200}
                />
              </label>
            </div>
          )}
        </>
      )}
    </Modal>
  );
}
