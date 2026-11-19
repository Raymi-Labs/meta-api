import React, { useState } from 'react';
import { Modal, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
export default function InviteEarlyPayoutModal({
  visible,
  onCancel,
  onSuccess,
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const { data } = await API.post('/api/invite/early-payout', {});
      if (!data?.success) throw new Error(data?.message);
      showSuccess(t('申请已提交'));
      onCancel();
      onSuccess?.();
    } catch (e) {
      showError(e.message || t('操作失败'));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      visible={visible}
      title={t('申请提前发放')}
      confirmLoading={busy}
      onCancel={onCancel}
      onOk={submit}
    >
      <Typography.Paragraph>
        {t('每周最多申请一次，仅发放已出账且过观察期的收益，需管理员审核。')}
      </Typography.Paragraph>
    </Modal>
  );
}
