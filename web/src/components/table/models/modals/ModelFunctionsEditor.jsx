import React from 'react';
import { Select, withField } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  MODEL_FUNCTION_LABELS,
  automaticModelFunctions,
  getModelFunctions,
  getCatalogTags,
  catalogTagLabel,
} from '../../../../helpers/modelCatalogFacets';

export function ModelFunctionsPreview({ model }) {
  const { t } = useTranslation();
  const functions = getModelFunctions(model),
    tags = getCatalogTags(model);
  return (
    <div
      className='text-sm text-[var(--semi-color-text-2)] mt-3'
      aria-label={t('广场展示预览')}
    >
      <div>
        {t('功能')}：
        {functions.map((k) => t(MODEL_FUNCTION_LABELS[k])).join(' · ') ||
          t('未分类')}
      </div>
      <div>
        {t('标签')}：
        {tags.map((k) => catalogTagLabel(k, t, [model])).join(' · ') || '—'}
      </div>
    </div>
  );
}
export function ModelFunctionsField({ value = {}, onChange, model = {} }) {
  const { t } = useTranslation();
  const automatic = automaticModelFunctions(model);
  return (
    <div className='grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-3'>
      {Object.entries(MODEL_FUNCTION_LABELS).map(([key, label]) => (
        <div
          key={key}
          className='flex items-center justify-between gap-2 min-w-0'
        >
          <span className='text-sm' id={`function-label-${key}`}>
            {t(label)}
          </span>
          <Select
            aria-labelledby={`function-label-${key}`}
            value={
              value[key] === true ? 'yes' : value[key] === false ? 'no' : 'auto'
            }
            style={{ width: 140, flexShrink: 0 }}
            optionList={[
              {
                value: 'auto',
                label: `${t('自动')} · ${t(automatic.includes(key) ? '包含' : '排除')}`,
              },
              { value: 'yes', label: t('包含') },
              { value: 'no', label: t('排除') },
            ]}
            onChange={(selected) => {
              const next = { ...value };
              if (selected === 'auto') delete next[key];
              else next[key] = selected === 'yes';
              onChange(next);
            }}
          />
        </div>
      ))}
    </div>
  );
}
export default withField(ModelFunctionsField);
