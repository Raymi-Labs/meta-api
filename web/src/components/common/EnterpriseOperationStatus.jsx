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
import './enterpriseFinancialModal.css';
import { useTranslation } from 'react-i18next';

export default function EnterpriseOperationStatus({ operation }) {
  const { t } = useTranslation();
  const data = operation.pending?.data;
  if (!data && !operation.error) return null;
  return (
    <div role='status' className='ec-field'>
      {data && (
        <div>
          {t('原始额度')}: {String(data.delta ?? data.amount ?? 0)}
          {data.reason ? ` · ${data.reason}` : ''}
        </div>
      )}
      {operation.error && <p role='alert'>{operation.error}</p>}
    </div>
  );
}
