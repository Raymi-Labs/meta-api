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
import { Download } from 'lucide-react';
import { Link, useLocation } from 'react-router-dom';
import './HeaderDownloadLink.css';

export default function HeaderDownloadLink({ t, href }) {
  const { pathname } = useLocation();
  const label = t('下载客户端');
  const props = {
    className: 'header-download-link',
    'aria-label': label,
    'aria-current':
      !href && /^\/download\/?$/.test(pathname) ? 'page' : undefined,
    title: label,
  };
  const content = (
    <>
      <Download size={16} aria-hidden='true' />
      <span className='header-download-label'>{label}</span>
    </>
  );
  return href ? (
    <a {...props} href={href}>
      {content}
    </a>
  ) : (
    <Link {...props} to='/download'>
      {content}
    </Link>
  );
}
