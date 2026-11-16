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

import './enterpriseApplication.css';
import React, { useEffect, useState } from 'react';
import { Button, Input, Modal, Select } from '@douyinfe/semi-ui';
import { enterpriseRequest } from '../../helpers/enterprise';
import {
  CONTACT_TYPES,
  APPLICATION_KINDS,
  APPLICATION_STATUS,
} from '../../helpers/enterpriseApplications';

export default function ApplicationModal({ kind, t, onClose, onDone }) {
  const [name, setName] = useState('');
  const [contactType, setContactType] = useState('wechat');
  const [contact, setContact] = useState('');
  const [latest, setLatest] = useState(null);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setLoadFailed(false);
    setError('');
    enterpriseRequest('get', '/api/enterprise/applications', undefined, {
      kind,
      page_size: 1,
    })
      .then((data) => {
        if (active) setLatest(data.items?.[0] || null);
      })
      .catch((e) => {
        if (active) {
          setError(e.message);
          setLoadFailed(true);
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [kind, retry]);
  const pending = latest?.status === 'pending';
  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      const app = await enterpriseRequest(
        'post',
        '/api/enterprise/applications',
        {
          kind,
          name: name.trim(),
          contact_type: contactType,
          contact: contact.trim(),
        },
      );
      setLatest(app);
      onDone();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      className='enterprise-application-modal'
      width='min(520px, calc(100vw - 32px))'
      visible
      title={t(APPLICATION_KINDS[kind])}
      onCancel={onClose}
      onOk={submit}
      okText={t('提交申请')}
      confirmLoading={busy}
      cancelButtonProps={{ disabled: busy }}
      closable={!busy}
      maskClosable={!busy}
      okButtonProps={{
        disabled:
          loading ||
          pending ||
          loadFailed ||
          !contact.trim() ||
          (kind === 'opening' && !name.trim()),
      }}
    >
      {loading ? (
        <p role='status'>{t('加载中...')}</p>
      ) : (
        <>
          {latest && (
            <p role='status'>
              {t(APPLICATION_STATUS[latest.status])}
              {latest.reason && ` · ${latest.reason}`}
            </p>
          )}
          {pending ? (
            <p>{t('申请已提交，请等待管理员处理。')}</p>
          ) : (
            <>
              <p>
                {t(
                  kind === 'opening'
                    ? '审核通过后即可进入企业工作台。'
                    : '管理员会通过您填写的联系方式与您联系。',
                )}
              </p>
              {kind === 'opening' && (
                <label className='enterprise-application-field'>
                  {t('企业名称')}
                  <Input
                    aria-label={t('企业名称')}
                    maxLength={64}
                    value={name}
                    onChange={setName}
                    disabled={busy}
                  />
                </label>
              )}
              <label className='enterprise-application-field'>
                {t('联系方式类型')}
                <Select
                  aria-label={t('联系方式类型')}
                  value={contactType}
                  onChange={setContactType}
                  disabled={busy}
                  optionList={Object.entries(CONTACT_TYPES).map(
                    ([value, label]) => ({ value, label: t(label) }),
                  )}
                  style={{ width: '100%' }}
                />
              </label>
              <label className='enterprise-application-field'>
                {t('联系方式')}
                <Input
                  aria-label={t('联系方式')}
                  maxLength={200}
                  value={contact}
                  onChange={setContact}
                  disabled={busy}
                />
              </label>
            </>
          )}
        </>
      )}
      {error && (
        <div role='alert'>
          <p>{error}</p>
          <Button disabled={busy} onClick={() => setRetry(retry + 1)}>
            {t('重试')}
          </Button>
        </div>
      )}
    </Modal>
  );
}
