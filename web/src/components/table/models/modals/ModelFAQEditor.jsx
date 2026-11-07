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

import React from 'react';
import { TranslationEditor } from './ModelDescriptionEditor';
import { Button, Input, TextArea, withField } from '@douyinfe/semi-ui';
import { ArrowUp, ArrowDown, Plus, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

function ModelFAQEditor({ value, onChange }) {
  const { t } = useTranslation();
  const items = Array.isArray(value) ? value : [];
  const update = (index, patch) =>
    onChange(
      items.map((item, i) => (i === index ? { ...item, ...patch } : item)),
    );
  const move = (index, offset) => {
    const next = [...items];
    [next[index], next[index + offset]] = [next[index + offset], next[index]];
    onChange(next);
  };
  return (
    <div className='model-faq-editor flex flex-col gap-3'>
      <p className='text-xs text-[var(--semi-color-text-2)]'>
        {t('显示在模型详情底部；未配置时隐藏，不受自动同步影响。')}{' '}
        {t('最多 20 条；问题限 200 字，答案限 2000 字，支持换行。')}
      </p>
      {items.map((item, index) => (
        <div
          key={index}
          className='model-faq-editor-item border border-[var(--semi-color-border)] rounded-xl p-3 flex flex-col gap-2 min-w-0'
        >
          <div className='flex items-center justify-between gap-2'>
            <span className='text-sm font-medium'>FAQ {index + 1}</span>
            <div className='flex gap-1'>
              <Button
                htmlType='button'
                size='small'
                aria-label={`${t('上移')} FAQ ${index + 1}`}
                icon={<ArrowUp size={14} />}
                disabled={index === 0}
                onClick={() => move(index, -1)}
              />
              <Button
                htmlType='button'
                size='small'
                aria-label={`${t('下移')} FAQ ${index + 1}`}
                icon={<ArrowDown size={14} />}
                disabled={index === items.length - 1}
                onClick={() => move(index, 1)}
              />
              <Button
                htmlType='button'
                size='small'
                type='danger'
                aria-label={`${t('删除')} FAQ ${index + 1}`}
                icon={<Trash2 size={14} />}
                onClick={() => onChange(items.filter((_, i) => i !== index))}
              />
            </div>
          </div>
          <label htmlFor={`model-faq-question-${index}`} className='text-sm'>
            {t('问题标题')}
          </label>
          <Input
            id={`model-faq-question-${index}`}
            value={item.question}
            maxLength={200}
            onChange={(question) =>
              update(index, {
                question,
                question_automatic: {},
                question_translations: {},
              })
            }
          />
          <label htmlFor={`model-faq-answer-${index}`} className='text-sm'>
            {t('回答内容')}
          </label>
          <TextArea
            id={`model-faq-answer-${index}`}
            value={item.answer}
            maxLength={2000}
            rows={4}
            onChange={(answer) =>
              update(index, {
                answer,
                answer_automatic: {},
                answer_translations: {},
              })
            }
          />
          <details>
            <summary className='cursor-pointer text-sm'>
              {t('自动翻译')}
            </summary>
            <div className='flex flex-col gap-3 pt-3'>
              <label className='text-sm'>{t('问题标题')}</label>
              <TranslationEditor
                label={t('问题标题')}
                value={item.question_overrides}
                automatic={item.question_automatic}
                maxLength={800}
                onChange={(question_overrides) =>
                  update(index, { question_overrides })
                }
              />
              <label className='text-sm'>{t('回答内容')}</label>
              <TranslationEditor
                label={t('回答内容')}
                value={item.answer_overrides}
                automatic={item.answer_automatic}
                maxLength={8000}
                onChange={(answer_overrides) =>
                  update(index, { answer_overrides })
                }
              />
            </div>
          </details>
        </div>
      ))}
      <Button
        htmlType='button'
        icon={<Plus size={16} />}
        disabled={items.length >= 20}
        onClick={() => onChange([...items, { question: '', answer: '' }])}
      >
        {t('添加常见问题')}
      </Button>
    </div>
  );
}

export default withField(ModelFAQEditor);
