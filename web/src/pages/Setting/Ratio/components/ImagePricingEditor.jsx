import React, { useEffect, useRef, useState } from 'react';
import { InputNumber, Typography } from '@douyinfe/semi-ui';
import {
  emptyImagePrices,
  buildImagePriceExpression,
  parseImagePriceExpression,
  IMAGE_PRICE_REFERENCES,
} from '../../../../helpers/imagePricing';

export function ImagePriceReference({ name, t }) {
  const reference = IMAGE_PRICE_REFERENCES[name];
  if (!reference) return null;
  return (
    <section
      className='mb-4 p-3 rounded-lg border'
      aria-label={t('图片官方参考价')}
    >
      <Typography.Text strong>
        {t('图片官方参考价')} · CNY/{t('张')} · {t('北京')} · 2026-10-31
      </Typography.Text>
      <p className='my-2 text-xs text-gray-500'>
        {t('参考价仅供对照，不会自动写入本站售价；其他地域请查看官方文档。')}
      </p>
      <table className='w-full text-sm'>
        <thead>
          <tr>
            <th className='text-left'>{t('档位')}</th>
            <th>{t('输入图片')}</th>
            <th>{t('输出图片')}</th>
          </tr>
        </thead>
        <tbody>
          {reference.rows.map((r) => (
            <tr key={r.label}>
              <td>{r.label}</td>
              <td className='text-center'>
                {r.input == null ? t('不单独计费') : `¥${r.input}`}
              </td>
              <td className='text-center'>¥{r.output}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <a
        href={reference.url}
        target='_blank'
        rel='noreferrer'
        className='text-blue-500 text-sm'
      >
        {t('官方定价文档')}
      </a>
    </section>
  );
}

export default function ImagePricingEditor({ expr, onChange, t }) {
  const [rows, setRows] = useState(
    () => parseImagePriceExpression(expr) || emptyImagePrices(),
  );
  const emitted = useRef(expr);
  useEffect(() => {
    if (expr !== emitted.current) {
      emitted.current = expr;
      setRows(parseImagePriceExpression(expr) || emptyImagePrices());
    }
  }, [expr]);
  const update = (i, key, value) => {
    const next = rows.map((r, j) => (i === j ? { ...r, [key]: value } : r));
    setRows(next);
    const nextExpr = buildImagePriceExpression(next) || '';
    emitted.current = nextExpr;
    onChange(nextExpr);
  };
  return (
    <section aria-label={t('图片输入 / 输出计费')} className='mb-4'>
      <Typography.Text strong>
        {t('图片输入 / 输出计费')} · USD/{t('张')}
      </Typography.Text>
      <p className='my-2 text-sm'>
        {t(
          '费用 = 输入图片数 × 输入单价 + 输出图片数 × 输出单价。未填写不能保存，免费请填写 0。',
        )}
      </p>
      {rows.map((r, i) => (
        <div
          key={r.resolution}
          className='grid gap-3 items-center my-3'
          style={{
            gridTemplateColumns: '2.5rem minmax(0, 1fr) minmax(0, 1fr)',
            maxWidth: 620,
          }}
        >
          <span className='w-10 font-medium'>{r.resolution}</span>
          {['input', 'output'].map((key) => (
            <label key={key} style={{ minWidth: 0 }}>
              <div className='mb-1'>
                {t(key === 'input' ? '输入图片单价' : '输出图片单价')}
              </div>
              <InputNumber
                aria-label={`${r.resolution}-${key}`}
                style={{ width: '100%' }}
                min={0}
                max={1000000}
                value={r[key]}
                placeholder='USD/image'
                onChange={(value) => update(i, key, value)}
              />
            </label>
          ))}
        </div>
      ))}
      <p className='text-xs text-gray-500'>
        {t(
          '适用于阿里云渠道的 Qwen Image 3.0。档位按输出像素面积划分：不超过 2250000 为 1K，其余为 2K；自动尺寸先预留较高费用，成功后按实际用量结算。',
        )}
      </p>
    </section>
  );
}
