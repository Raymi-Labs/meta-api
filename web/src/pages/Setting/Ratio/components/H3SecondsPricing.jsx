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

import React, { useState } from 'react';
import { Button, InputNumber, Typography } from '@douyinfe/semi-ui';
import {
  buildH3SecondsExpression,
  h3VideoProfiles,
} from '../../../../helpers/h3VideoPricing';

export default function H3SecondsPricing({ model, onApply, t }) {
  const [rates, setRates] = useState({});
  const profile = h3VideoProfiles[model];
  if (!profile) return null;
  const expr = buildH3SecondsExpression(model, rates);
  return (
    <section
      className='mb-4 p-3 rounded-lg border'
      aria-label={t('视频按秒模板')}
    >
      <Typography.Text strong>{t('视频按秒模板')} · H3</Typography.Text>
      <p className='my-2 text-sm' style={{ color: 'var(--semi-color-text-2)' }}>
        {t(
          '仅文生视频，按请求输出秒数计费；填写 USD/秒，分组和时间倍率另算。生成模板后仍需应用更改，不提交视频。',
        )}
      </p>
      <div className='flex flex-wrap gap-3 mb-3'>
        {profile.resolutions.map((resolution) => (
          <label key={resolution}>
            <div className='mb-1'>
              {resolution} · {profile.min}–{profile.max}s
            </div>
            <InputNumber
              aria-label={resolution + ' USD/s'}
              value={rates[resolution] ?? ''}
              min={0}
              max={1000000}
              placeholder='USD/s'
              onChange={(value) =>
                setRates((previous) => ({ ...previous, [resolution]: value }))
              }
            />
          </label>
        ))}
      </div>
      <Button disabled={!expr} onClick={() => onApply(expr)}>
        {t('生成秒价表达式')}
      </Button>
    </section>
  );
}
