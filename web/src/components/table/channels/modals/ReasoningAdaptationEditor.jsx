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
import { Select, TextArea, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

export default function ReasoningAdaptationEditor({
  mode,
  rules,
  onModeChange,
  onRulesChange,
}) {
  const { t } = useTranslation();
  return (
    <div
      className='my-4 flex flex-col gap-2'
      data-testid='reasoning-adaptation-editor'
    >
      <label id='reasoning-adaptation-label' htmlFor='reasoning-adaptation-mode'>
        {t('思考参数适配')}
      </label>
      <Select
        id='reasoning-adaptation-mode'
        aria-labelledby='reasoning-adaptation-label'
        value={mode}
        onChange={onModeChange}
        optionList={[
          { value: 'inherit', label: t('继承全局映射') },
          { value: 'auto', label: t('按厂商自动适配') },
          { value: 'zhipu-glm53-v1', label: t('智谱 GLM-5.3 Chat 模板') },
        ]}
        style={{ width: '100%' }}
      />
      <Typography.Text type='tertiary'>
        {t(
          '仅对已登记模型的 OpenAI/Azure Chat 转发生效；透传和其他协议保持原行为。',
        )}
      </Typography.Text>
      {mode !== 'inherit' && (
        <Typography.Text type='tertiary'>
          GLM-5.3: low → low · medium → high · high → high · max/xhigh → max
        </Typography.Text>
      )}
      <label htmlFor='reasoning-adaptation-rules'>
        {t('渠道专属规则（可选）')}
      </label>
      <TextArea
        id='reasoning-adaptation-rules'
        aria-label={t('渠道专属规则（可选）')}
        value={rules}
        onChange={onRulesChange}
        autosize={{ minRows: 2, maxRows: 12 }}
        placeholder='[{"model":"your-model","protocol":"chat","mapping":{"low":"low","medium":"high","high":"high","max":"max"}}]'
      />
      <Typography.Text type='tertiary'>
        {t('精确匹配上游模型，优先于模板；参数覆盖最后执行。')}
      </Typography.Text>
    </div>
  );
}
