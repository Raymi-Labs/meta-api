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
import { Skeleton } from '@douyinfe/semi-ui';

export default function PricingCardSkeleton({ skeletonCount = 6 }) {
  return (
    <div className='pricing-catalog-grid' aria-hidden='true'>
      {Array.from({ length: skeletonCount }, (_, index) => (
        <div className='pricing-model-card' key={index}>
          <Skeleton.Title
            active
            style={{ width: '65%', height: 24, marginBottom: 24 }}
          />
          <Skeleton.Paragraph active rows={3} />
          <Skeleton.Title
            active
            style={{ width: '100%', height: 70, marginTop: 24 }}
          />
        </div>
      ))}
    </div>
  );
}
