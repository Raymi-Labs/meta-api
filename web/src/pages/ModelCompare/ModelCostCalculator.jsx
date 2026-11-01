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
import { Link } from 'react-router-dom';
import { estimateModelMonthlyCost } from '../../helpers/modelCompare';
import { buildModelDetailPath } from '../../helpers/modelDetailNavigation';

export default function ModelCostCalculator({
  models,
  prices,
  data,
  navigationState,
  t,
}) {
  const [workload, setWorkload] = useState({
    requests: 1000,
    input: 2000,
    output: 500,
  });
  return (
    <section className='model-detail-section'>
      <h2>{t('费用估算')}</h2>
      <p className='model-detail-caption'>
        {t(
          '按当前分组价格和每天用量估算 30 天文本费用，不含缓存、图片、工具费用及未来价格变化。',
        )}
      </p>
      <div className='model-cost-inputs'>
        {[
          ['requests', '每日请求数'],
          ['input', '每次输入 Token'],
          ['output', '每次输出 Token'],
        ].map(([key, label]) => (
          <label key={key}>
            <span>{t(label)}</span>
            <input
              aria-label={t(label)}
              type='number'
              min='0'
              max='1000000000'
              step='1'
              value={workload[key]}
              onChange={(event) => {
                const raw = event.target.value;
                setWorkload((previous) => ({
                  ...previous,
                  [key]:
                    raw === '' ? '' : Math.min(1e9, Math.max(0, Number(raw))),
                }));
              }}
            />
          </label>
        ))}
      </div>
      <div className='model-cost-results'>
        {models.map((model, index) => {
          const cost = estimateModelMonthlyCost(
            model,
            prices[index]?.data,
            workload,
            data.tokenUnit,
          );
          return (
            <article key={model.model_name}>
              <h3>{model.display_name || model.model_name}</h3>
              {cost === null ? (
                <p>
                  {t('暂无可用估算')} ·{' '}
                  <Link
                    to={buildModelDetailPath(model.model_name)}
                    state={navigationState}
                  >
                    {t('查看详情')}
                  </Link>
                </p>
              ) : (
                <strong>
                  {data.displayPrice.currencySymbol}
                  {data.displayPrice.toAmount(cost).toLocaleString(undefined, {
                    minimumFractionDigits: 2,
                    maximumFractionDigits: 2,
                  })}{' '}
                  <small>/ {t('月')}</small>
                </strong>
              )}
              {prices[index]?.data.isDynamicPricing && (
                <small>{t('动态计费')}</small>
              )}
            </article>
          );
        })}
      </div>
    </section>
  );
}
