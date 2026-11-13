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

import React, {
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import { Button, Modal, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../../context/Status';
import { enterpriseRequest } from '../../helpers/enterprise';
import { isRoot } from '../../helpers/storage';
import { showError } from '../../helpers/toast';
import { refreshEnterpriseSelf } from '../../hooks/enterprise/useEnterpriseSelf';

export default function EnterpriseFeatureGate({ children }) {
  const { t } = useTranslation();
  const [, dispatch] = useContext(StatusContext);
  const [status, setStatus] = useState(null);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [confirmClose, setConfirmClose] = useState(false);
  const sequence = useRef(0);
  const submitting = useRef(false);
  const root = isRoot();

  const publish = useCallback(
    (value) => {
      setStatus(value);
      dispatch({ type: 'set', payload: value });
      try {
        localStorage.setItem('status', JSON.stringify(value));
      } catch {
        /* private browsing */
      }
    },
    [dispatch],
  );

  const load = useCallback(async () => {
    const id = ++sequence.current;
    setError(false);
    try {
      const value = await enterpriseRequest('get', '/api/status');
      if (typeof value?.enterprise_enabled !== 'boolean') throw new Error();
      if (id === sequence.current) publish(value);
    } catch {
      if (id === sequence.current) setError(true);
    }
  }, [publish]);

  useEffect(() => {
    load();
    return () => {
      sequence.current++;
    };
  }, [load]);

  const setEnabled = async (enabled) => {
    if (!root || submitting.current || !status) return;
    submitting.current = true;
    setBusy(true);
    const id = ++sequence.current;
    try {
      await enterpriseRequest('put', '/api/option/', {
        key: 'EnterpriseEnabled',
        value: String(enabled),
      });
      if (id !== sequence.current) return;
      publish({ ...status, enterprise_enabled: enabled });
      setConfirmClose(false);
      if (enabled) refreshEnterpriseSelf();
    } catch (e) {
      if (id === sequence.current) showError(e.message);
    } finally {
      submitting.current = false;
      if (id === sequence.current) setBusy(false);
    }
  };

  if (error)
    return (
      <div className='ea' role='alert'>
        <p>{t('企业信息读取失败，请重试')}</p>
        <Button onClick={load}>{t('重试')}</Button>
      </div>
    );
  if (!status)
    return (
      <div className='ea'>
        <Spin />
      </div>
    );
  if (!status.enterprise_enabled)
    return (
      <div className='ea'>
        <div className='em-head'>
          <h1>{t('企业管理')}</h1>
        </div>
        <div className='ea-disabled'>
          <p>{t('企业功能未开启')}</p>
          {root && (
            <Button
              theme='solid'
              type='primary'
              loading={busy}
              onClick={() => setEnabled(true)}
            >
              {t('启用企业功能')}
            </Button>
          )}
        </div>
      </div>
    );
  return (
    <>
      {children(
        root ? (
          <Button loading={busy} onClick={() => setConfirmClose(true)}>
            {t('关闭企业功能')}
          </Button>
        ) : null,
      )}
      <Modal
        visible={confirmClose}
        title={t('关闭企业功能？')}
        okText={t('确认')}
        cancelText={t('取消')}
        okButtonProps={{ type: 'danger' }}
        confirmLoading={busy}
        maskClosable={!busy}
        closable={!busy}
        onCancel={() => {
          if (!busy) setConfirmClose(false);
        }}
        onOk={() => setEnabled(false)}
      >
        {t(
          '关闭后，企业令牌无法发起新请求；已有账单与在途结算保留，个人调用不受影响。',
        )}
      </Modal>
    </>
  );
}
