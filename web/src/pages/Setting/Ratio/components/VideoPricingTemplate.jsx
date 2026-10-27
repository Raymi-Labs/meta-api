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
import { Button, Input, InputNumber, Typography } from '@douyinfe/semi-ui';
import { buildVideoPricingTemplate } from '../../../../helpers/videoPricingTemplate';
export default function VideoPricingTemplate({ onApply, t }) {
  const [mode, setMode] = useState('seconds');
  const [rows, setRows] = useState([
    { resolution: '', min: '', max: '', rate: '' },
  ]);
  const update = (i, key, value) =>
    setRows((old) => old.map((r, j) => (i === j ? { ...r, [key]: value } : r)));
  const expr = buildVideoPricingTemplate(rows, mode);
  return (
    <section
      className='mb-4 p-3 rounded-lg border'
      aria-label={t('视频按秒模板')}
    >
      <Typography.Text strong>{t('视频按秒模板')}</Typography.Text>
      <p className='my-2 text-sm'>
        {t(
          '此模板仅配置本站售价，不代表模型支持这些参数。按实际出站时长和档位收费；输入素材等额外成本请使用完整表达式。',
        )}
      </p>
      <div className='flex gap-2'>
        <Button
          aria-pressed={mode === 'seconds'}
          theme={mode === 'seconds' ? 'solid' : 'outline'}
          onClick={() => setMode('seconds')}
        >
          USD/s
        </Button>
        <Button
          aria-label='video-fixed-mode'
          aria-pressed={mode === 'fixed'}
          theme={mode === 'fixed' ? 'solid' : 'outline'}
          onClick={() => setMode('fixed')}
        >
          {t('固定价格')}
        </Button>
      </div>
      {rows.map((r, i) => (
        <div
          key={i}
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'end',
            gap: 8,
            margin: '12px 0',
          }}
        >
          <label>
            <div>{t('分辨率')} / mode</div>
            <Input
              aria-label={'video-resolution-' + i}
              placeholder={t('分辨率') + ' / mode'}
              value={r.resolution}
              style={{ width: 150 }}
              onChange={(v) => update(i, 'resolution', v)}
            />
          </label>
          <label>
            <div>
              {t('秒')}
              {mode === 'seconds' ? ' ≥' : ''}
            </div>
            <InputNumber
              aria-label={'video-min-' + i}
              placeholder={mode === 'fixed' ? 'seconds' : 'min seconds'}
              min={1}
              max={3600}
              value={r.min}
              onChange={(v) => update(i, 'min', v)}
            />
          </label>
          {mode === 'seconds' && (
            <label>
              <div>{t('秒')} ≤</div>
              <InputNumber
                aria-label={'video-max-' + i}
                placeholder='max seconds'
                min={1}
                max={3600}
                value={r.max}
                onChange={(v) => update(i, 'max', v)}
              />
            </label>
          )}
          <label>
            <div>
              {t('价格')} · {mode === 'seconds' ? 'USD/s' : 'USD/task'}
            </div>
            <InputNumber
              aria-label={'video-rate-' + i}
              placeholder={mode === 'seconds' ? 'USD/s' : 'USD/task'}
              min={0}
              value={r.rate}
              onChange={(v) => update(i, 'rate', v)}
            />
          </label>
          <Button
            onClick={() => setRows((old) => old.filter((_, j) => j !== i))}
          >
            {t('删除')}
          </Button>
        </div>
      ))}
      <div className='flex gap-2'>
        <Button
          onClick={() =>
            setRows((old) => [
              ...old,
              { resolution: '', min: '', max: '', rate: '' },
            ])
          }
        >
          {t('添加')}
        </Button>
        <Button disabled={!expr} onClick={() => onApply(expr)}>
          {t('生成秒价表达式')}
        </Button>
      </div>
    </section>
  );
}
