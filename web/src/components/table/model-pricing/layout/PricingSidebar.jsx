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
import PricingGroups from '../filter/PricingGroups';

const PricingSidebar = (props) => {
  // Group availability depends on vendor, not on a transient model search.
  const models = props.models.filter(
    (model) =>
      props.filterVendor === 'all' ||
      (props.filterVendor === 'unknown'
        ? !model.vendor_name
        : model.vendor_name === props.filterVendor),
  );
  return (
    <div className='pricing-group-sidebar'>
      <PricingGroups
        {...props}
        models={models}
        setFilterGroup={props.handleGroupClick}
        hideUnavailable
        vertical
      />
    </div>
  );
};

export default PricingSidebar;
