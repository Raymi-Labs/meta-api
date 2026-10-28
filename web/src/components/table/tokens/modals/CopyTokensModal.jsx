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

import React, { useEffect, useRef, useState } from 'react';
import { Modal, Button, Space } from '@douyinfe/semi-ui';
import { showError } from '../../../../helpers';
import { collectTokenKeys } from '../tokenCopy';

const CopyTokensModal = ({
  visible,
  onCancel,
  selectedKeys,
  getTokenFullKey,
  copyText,
  t,
}) => {
  const [busy, setBusy] = useState(false);
  const session = useRef(0);
  const inFlight = useRef(false);
  useEffect(() => {
    session.current++;
    inFlight.current = false;
    setBusy(false);
    return () => {
      session.current++;
    };
  }, [visible, selectedKeys]);
  const close = () => {
    session.current++;
    onCancel();
  };
  const run = async (withName) => {
    if (inFlight.current) return;
    const current = session.current;
    inFlight.current = true;
    setBusy(true);
    try {
      const content = await collectTokenKeys(
        [...selectedKeys],
        getTokenFullKey,
        withName,
        () => current === session.current,
      );
      if (content === null || current !== session.current) return;
      const copied = await copyText(content);
      if (current === session.current && copied !== false) onCancel();
    } catch {
      if (current === session.current) showError(t('获取令牌失败'));
    } finally {
      if (current === session.current) {
        inFlight.current = false;
        setBusy(false);
      }
    }
  };
  return (
    <Modal
      title={t('复制令牌')}
      icon={null}
      visible={visible}
      onCancel={close}
      footer={
        <Space>
          <Button
            type='tertiary'
            disabled={busy}
            loading={busy}
            onClick={() => run(true)}
          >
            {t('名称+密钥')}
          </Button>
          <Button disabled={busy} loading={busy} onClick={() => run(false)}>
            {t('仅密钥')}
          </Button>
        </Space>
      }
    >
      {t('请选择你的复制方式')}
    </Modal>
  );
};
export default CopyTokensModal;
