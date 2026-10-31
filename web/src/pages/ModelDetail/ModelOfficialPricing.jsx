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

import React, { useEffect, useRef, useState } from 'react';
import { Popover } from '@douyinfe/semi-ui';
import { ArrowUpRight, Info } from 'lucide-react';
import {
  officialPriceColumns,
  OFFICIAL_PRICE_UNITS,
  formatOfficialPrice,
  officialPricingSource,
} from '../../helpers/modelOfficialPricing';

export default function ModelOfficialPricing({ pricing, provenance, t }) {
  const [helpVisible, setHelpVisible] = useState(false);
  const hideTimer = useRef();
  const showHelp = () => {
    clearTimeout(hideTimer.current);
    setHelpVisible(true);
  };
  const hideHelp = () => {
    clearTimeout(hideTimer.current);
    setHelpVisible(false);
  };
  const scheduleHide = () => {
    clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => setHelpVisible(false), 150);
  };
  useEffect(() => () => clearTimeout(hideTimer.current), []);
  const tiers = Array.isArray(pricing?.tiers) ? pricing.tiers : [];
  const columns = officialPriceColumns(tiers);
  const source = officialPricingSource(pricing?.source_url);
  return (
    <section
      id='official-pricing'
      className='model-detail-section model-official-pricing'
    >
      <div className='model-detail-section-heading'>
        <h2 className='model-official-title'>
          {t('官方参考价')}
          <Popover
            trigger='custom'
            visible={helpVisible}
            position='bottomLeft'
            showArrow
            closeOnEsc
            guardFocus={false}
            returnFocusOnClose={false}
            onEscKeyDown={hideHelp}
            onClickOutSide={hideHelp}
            content={
              <div
                className='model-official-help-content'
                onMouseEnter={showHelp}
                onMouseLeave={scheduleHide}
              >
                <p>{t('官方参考价仅供对照，实际费用以本站分组价格为准。')}</p>
                {tiers.length > 0 &&
                  provenance?.kind === 'official' &&
                  provenance?.synced_at > 0 && (
                    <p>
                      {t('官方价格自动更新于 {{time}}', {
                        time: new Date(
                          provenance.synced_at * 1000,
                        ).toLocaleString(),
                      })}
                    </p>
                  )}
                {pricing?.notes && (
                  <p className='model-official-notes'>{pricing.notes}</p>
                )}
              </div>
            }
          >
            <button
              type='button'
              className='model-official-help'
              aria-label={`${t('官方参考价')} · ${t('费用说明')}`}
              aria-expanded={helpVisible}
              aria-haspopup='dialog'
              onMouseEnter={showHelp}
              onMouseLeave={scheduleHide}
              onFocus={showHelp}
              onBlur={scheduleHide}
              onClick={showHelp}
            >
              <Info size={16} aria-hidden='true' />
            </button>
          </Popover>
        </h2>
        {source && (
          <a href={source} target='_blank' rel='noopener noreferrer'>
            {t('价格来源')}
            <ArrowUpRight size={15} />
          </a>
        )}
      </div>
      {tiers.length && columns.length ? (
        <div
          className='model-official-table-wrap'
          tabIndex={0}
          role='region'
          aria-label={t('官方参考价')}
        >
          <table>
            <thead>
              <tr>
                <th>{t('档位')}</th>
                {columns.map(([key, label]) => (
                  <th key={key}>{t(label)}</th>
                ))}
                <th>{t('单位')}</th>
              </tr>
            </thead>
            <tbody>
              {tiers.map((tier, index) => (
                <tr key={index}>
                  <th scope='row'>{tier.label}</th>
                  {columns.map(([key]) => (
                    <td key={key}>
                      {formatOfficialPrice(tier[key], pricing.currency)}
                    </td>
                  ))}
                  <td>
                    {pricing.currency} /{' '}
                    {OFFICIAL_PRICE_UNITS[tier.unit] || tier.unit}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className='model-official-empty'>{t('暂无官方参考价')}</p>
      )}
    </section>
  );
}
