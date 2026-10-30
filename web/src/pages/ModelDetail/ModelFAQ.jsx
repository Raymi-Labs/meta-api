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
import { ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function ModelFAQ({ items }) {
  const { t } = useTranslation();
  if (!Array.isArray(items) || items.length === 0) return null;
  return (
    <section
      id='faq'
      className='model-detail-section model-detail-faq'
      aria-labelledby='model-faq-title'
    >
      <h2 id='model-faq-title'>{t('常见问题')}</h2>
      <div className='model-detail-faq-list'>
        {items.map((item, index) => (
          <details key={index}>
            <summary>
              <span>{item.question}</span>
              <ChevronDown size={18} aria-hidden='true' />
            </summary>
            <p>{item.answer}</p>
          </details>
        ))}
      </div>
    </section>
  );
}
