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
import { Info } from 'lucide-react';

export default function ModelSectionHelp({ label, children }) {
  const [visible, setVisible] = useState(false);
  const hideTimer = useRef();
  const show = () => {
    clearTimeout(hideTimer.current);
    setVisible(true);
  };
  const hide = () => {
    clearTimeout(hideTimer.current);
    setVisible(false);
  };
  const scheduleHide = () => {
    clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => setVisible(false), 150);
  };
  useEffect(() => () => clearTimeout(hideTimer.current), []);
  return (
    <Popover
      trigger='custom'
      visible={visible}
      position='bottomLeft'
      showArrow
      closeOnEsc
      guardFocus={false}
      returnFocusOnClose={false}
      onEscKeyDown={hide}
      onClickOutSide={hide}
      content={
        <div
          className='model-section-help-content'
          onMouseEnter={show}
          onMouseLeave={scheduleHide}
          onFocus={show}
          onBlur={scheduleHide}
        >
          {children}
        </div>
      }
    >
      <button
        type='button'
        className='model-section-help'
        aria-label={label}
        aria-expanded={visible}
        aria-haspopup='dialog'
        onMouseEnter={show}
        onMouseLeave={scheduleHide}
        onFocus={show}
        onBlur={scheduleHide}
        onClick={show}
      >
        <Info size={16} aria-hidden='true' />
      </button>
    </Popover>
  );
}
