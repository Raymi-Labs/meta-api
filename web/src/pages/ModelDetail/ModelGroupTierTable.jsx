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
import ModelGroupPrice from './ModelGroupPrice';

export default function ModelGroupTierTable({ rows, model, data }) {
  const columns = [
    ...new Map(
      rows.flatMap((row) => row.items.map((item) => [item.key, item.label])),
    ).entries(),
  ];
  return (
    <div
      className='model-official-table-wrap model-group-tier-table'
      tabIndex={0}
      role='region'
      aria-label={data.t('分档价格表')}
    >
      <table>
        <thead>
          <tr>
            <th scope='col'>{data.t('档位')}</th>
            {columns.map(([key, label]) => (
              <th scope='col' key={key}>
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={index}>
              <th scope='row'>
                {row.label}
                {row.condition && <small>{row.condition}</small>}
                {(row.comparisonNote || row.comparisonUnavailable) && (
                  <small className='model-group-comparison-note'>
                    {data.t(row.comparisonNote || row.comparisonUnavailable)}
                  </small>
                )}
              </th>
              {columns.map(([key]) => {
                const item = row.items.find((value) => value.key === key);
                return (
                  <td key={key}>
                    {item ? (
                      <ModelGroupPrice
                        item={item}
                        pricing={row.data}
                        model={model}
                        officialTier={row.officialTier}
                        data={data}
                        hideLabel
                      />
                    ) : (
                      '—'
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
