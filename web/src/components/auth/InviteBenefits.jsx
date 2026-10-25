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
import './InviteBenefits.css';
import { Link } from 'react-router-dom';
import { ArrowRight, ArrowUpRight, Check, Sparkles } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { INVITE_PROMOTION } from '../../helpers/invitePromotion';

export default function InviteBenefits() {
  const { t } = useTranslation();
  return (
    <section className='invite-benefits' aria-label={t('元衡优势')}>
      <div className='invite-benefits__backdrop' aria-hidden='true'>
        <span className='invite-benefits__ring' />
        <span className='invite-benefits__crystal invite-benefits__crystal--one' />
        <span className='invite-benefits__crystal invite-benefits__crystal--two' />
        <span className='invite-benefits__watermark'>YUANHENG</span>
      </div>
      <div className='invite-benefits__brand'>
        <span className='invite-benefits__wordmark'>{t('元衡 AI')}</span>
        <span className='invite-benefits__eyebrow'>AI, ALL IN ONE.</span>
      </div>
      <div className='invite-benefits__stage'>
        <div className='invite-benefits__content'>
          <p className='invite-benefits__kicker'>
            <Sparkles size={14} aria-hidden='true' />
            {t('用 AI，不必东奔西找')}
          </p>
          <h1>
            {t(INVITE_PROMOTION.title)}
            <br />
            <span>{t(INVITE_PROMOTION.subtitle)}</span>
          </h1>
          <p className='invite-benefits__description'>
            {t('接模型，元衡一站就好。')}
          </p>
          <div className='invite-benefits__models'>
            {INVITE_PROMOTION.models.map((model) => (
              <div key={model.name} className='invite-benefits__model'>
                <span
                  className='invite-benefits__model-mark'
                  aria-hidden='true'
                >
                  {model.name === 'CC Max' ? 'C' : 'G'}
                </span>
                <div>
                  <h2>{model.name}</h2>
                  <p>{t(model.description)}</p>
                </div>
                <span className='invite-benefits__model-badge'>
                  {t(model.badge)}
                </span>
              </div>
            ))}
          </div>
          <div className='invite-benefits__actions'>
            <a href='#invite-register-form' className='invite-benefits__cta'>
              {t('立即注册，开始体验')}
              <ArrowRight size={18} aria-hidden='true' />
            </a>
            <Link
              to='/pricing'
              target='_blank'
              rel='noopener noreferrer'
              className='invite-benefits__prices'
            >
              {t('先看模型与价格')}
              <ArrowUpRight size={15} aria-hidden='true' />
            </Link>
          </div>
        </div>
        <div className='invite-benefits__character' aria-hidden='true'>
          {['upper', 'lower'].map((part) => (
            <img
              key={part}
              className={`invite-benefits__figure-${part}`}
              src='/images/yuanheng-invite-character-front.webp'
              alt=''
              width='480'
              height='1024'
              decoding='async'
              fetchPriority='high'
              draggable='false'
            />
          ))}
        </div>
      </div>
      <div className='invite-benefits__footer'>
        {INVITE_PROMOTION.benefits.map((benefit) => (
          <span key={benefit}>
            <Check size={13} aria-hidden='true' />
            {t(benefit)}
          </span>
        ))}
      </div>
    </section>
  );
}
