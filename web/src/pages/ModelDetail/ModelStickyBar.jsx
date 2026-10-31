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

import React, { useLayoutEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Copy, ArrowUpRight } from 'lucide-react';
import { getLobeHubIcon } from '../../helpers';

export default function ModelStickyBar({
  model,
  pageRef,
  copyText,
  comparePath,
  playgroundPath,
  t,
}) {
  const [visible, setVisible] = useState(false);
  useLayoutEffect(() => {
    const title = pageRef.current?.querySelector('.model-detail-title-row');
    if (!title) return;
    let frame = 0;
    const update = () => {
      frame = 0;
      setVisible(title.getBoundingClientRect().bottom <= 64);
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    window.addEventListener('scroll', schedule, {
      passive: true,
      capture: true,
    });
    window.addEventListener('resize', schedule);
    const observer = new ResizeObserver(schedule);
    observer.observe(title);
    update();
    return () => {
      window.removeEventListener('scroll', schedule, true);
      window.removeEventListener('resize', schedule);
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [pageRef, model.model_name]);
  const icon = model.icon || model.vendor_icon;
  return (
    <div
      className='model-sticky-bar'
      hidden={!visible}
      role='region'
      aria-label={t('当前模型')}
    >
      <div className='model-sticky-inner'>
        <div className='model-sticky-identity'>
          <span className='model-sticky-icon' aria-hidden='true'>
            {icon
              ? getLobeHubIcon(icon, 26, model.vendor_icon)
              : model.model_name.slice(0, 2).toUpperCase()}
          </span>
          <strong title={model.display_name || model.model_name}>
            {model.display_name || model.model_name}
          </strong>
          <button
            type='button'
            className='model-sticky-copy'
            title={t('复制模型名称')}
            aria-label={t('复制模型名称')}
            onClick={() => copyText(model.model_name)}
          >
            <Copy size={16} aria-hidden='true' />
          </button>
        </div>
        <div className='model-sticky-actions'>
          <Link to={comparePath} className='model-action-secondary'>
            {t('对比')}
          </Link>
          <Link
            to={playgroundPath}
            className='model-action-primary'
            aria-label={t('在模型体验中测试')}
          >
            <span className='model-action-full'>{t('在模型体验中测试')}</span>
            <span className='model-action-short'>{t('测试')}</span>
            <ArrowUpRight size={16} aria-hidden='true' />
          </Link>
        </div>
      </div>
    </div>
  );
}
