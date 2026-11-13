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

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input, Modal, Select } from '@douyinfe/semi-ui';
import { enterpriseRequest } from '../../helpers/enterprise';
import { showError, showSuccess } from '../../helpers/toast';

const DEFAULT_ZONE = 'Asia/Shanghai';
const zoneList = () => {
  const all =
    typeof Intl.supportedValuesOf === 'function'
      ? Intl.supportedValuesOf('timeZone')
      : [];
  return Array.from(new Set([DEFAULT_ZONE, 'UTC', ...all]));
};

// 开通企业与修改企业共用：修改时只能改名称和计费用户组。
export default function EnterpriseFormModal({
  enterprise,
  groups,
  onClose,
  onDone,
}) {
  const { t } = useTranslation();
  const editing = !!enterprise?.id;
  const [name, setName] = useState('');
  const [owner, setOwner] = useState('');
  const [group, setGroup] = useState();
  const [timezone, setTimezone] = useState(DEFAULT_ZONE);
  const [busy, setBusy] = useState(false);
  const zones = useMemo(zoneList, []);

  useEffect(() => {
    setName(enterprise?.name || '');
    setOwner('');
    setGroup(enterprise?.billing_group || undefined);
    setTimezone(DEFAULT_ZONE);
  }, [enterprise]);

  if (!enterprise) return null;

  const submit = async () => {
    if (!name.trim()) return showError(t('请填写名称'));
    if (!editing && !owner.trim()) return showError(t('请填写所有者账号'));
    if (!group) return showError(t('请选择计费用户组'));
    setBusy(true);
    try {
      if (editing) {
        await enterpriseRequest(
          'put',
          `/api/enterprise/admin/enterprises/${enterprise.id}`,
          { name: name.trim(), billing_group: group },
        );
      } else {
        await enterpriseRequest('post', '/api/enterprise/admin/enterprises', {
          name: name.trim(),
          owner: owner.trim(),
          billing_group: group,
          timezone,
        });
      }
      showSuccess(editing ? t('已保存') : t('已开通'));
      onDone();
      onClose();
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      className='ec-modal'
      title={editing ? t('编辑企业') : t('开通企业')}
      visible
      okText={editing ? t('保存') : t('开通')}
      confirmLoading={busy}
      onOk={submit}
      onCancel={onClose}
    >
      <label className='ec-field'>
        {t('企业名称')}
        <Input maxLength={64} value={name} onChange={setName} />
      </label>
      {!editing && (
        <label className='ec-field'>
          {t('所有者账号')}
          <Input
            placeholder={t('用户名、邮箱或用户 ID')}
            value={owner}
            onChange={setOwner}
          />
        </label>
      )}
      <label className='ec-field'>
        {t('计费用户组')}
        <Select
          filter
          value={group}
          optionList={groups.map((g) => ({ value: g, label: g }))}
          onChange={setGroup}
          style={{ width: '100%' }}
        />
      </label>
      {!editing && (
        <label className='ec-field'>
          {t('时区')}
          <Select
            filter
            value={timezone}
            optionList={zones.map((z) => ({ value: z, label: z }))}
            onChange={setTimezone}
            style={{ width: '100%' }}
          />
        </label>
      )}
    </Modal>
  );
}
