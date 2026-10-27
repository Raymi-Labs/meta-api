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
import React, { useId } from 'react';
import { Select, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { readTokenRouting } from './tokenRouting';

export default function TokenRoutingEditor({
  value,
  groups,
  models,
  modelGroups,
  onChange,
  disabled,
}) {
  const { t } = useTranslation();
  const labelId = useId();
  const config = readTokenRouting(value);
  const options = groups
    .filter(({ value }) => value !== 'auto')
    .map((group) => ({
      value: group.value,
      label: `${group.value}${Number.isFinite(group.ratio) ? ` · ${group.ratio}x` : ''}`,
    }));
  const update = (next) => onChange(JSON.stringify(next));
  return (
    <div className='flex flex-col gap-3' data-testid='token-routing-editor'>
      <Typography.Text strong id={labelId}>
        {t('可用分组（可多选，按选择顺序路由）')}
      </Typography.Text>
      <Select
        aria-labelledby={labelId}
        multiple
        filter
        optionList={options}
        value={config.groups}
        disabled={disabled}
        style={{ width: '100%' }}
        placeholder={t('请选择令牌分组')}
        onChange={(selected) =>
          update({
            groups: selected,
            model_groups: Object.fromEntries(
              Object.entries(config.model_groups).map(([name, assigned]) => [
                name,
                assigned.filter((group) => selected.includes(group)),
              ]),
            ),
          })
        }
      />
      <Typography.Text type='tertiary'>
        {t(
          '仅在所选分组内自动路由和重试，按实际使用分组计费；所有模型共用此 Key 的额度。',
        )}
      </Typography.Text>
      {(models || []).map((name) => (
        <div key={name} className='flex flex-col gap-1'>
          <Typography.Text id={`${labelId}-${name}`}>
            {name} {t('可用分组')}
          </Typography.Text>
          <Select
            aria-labelledby={`${labelId}-${name}`}
            multiple
            filter
            showClear
            placeholder={t('继承此 Key 的全部可用分组')}
            value={config.model_groups[name] ?? []}
            optionList={config.groups
              .filter(
                (group) =>
                  !modelGroups?.[name] || modelGroups[name].includes(group),
              )
              .map((group) => ({
                value: group,
                label: group,
              }))}
            style={{ width: '100%' }}
            disabled={disabled}
            onChange={(selected) => {
              const mapped = { ...config.model_groups };
              if (selected.length) mapped[name] = selected;
              else delete mapped[name];
              update({ ...config, model_groups: mapped });
            }}
          />
          {config.model_groups[name]?.length === 0 && (
            <Typography.Text type='danger'>
              {t('原分组已移除，请重新指定分组或清除规则以继承。')}
            </Typography.Text>
          )}
        </div>
      ))}
    </div>
  );
}
