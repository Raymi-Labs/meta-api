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

import { clearEnterpriseOperationDrafts } from './enterpriseOperation';
import { TABLE_COMPACT_MODES_KEY } from '../constants/common.constant';

const DEFAULT_SYSTEM_NAME = '元衡 API';
const DEFAULT_LOGO = '/yuanheng-logo.png';
const LEGACY_SYSTEM_NAMES = new Set(['New API', '山海衡 AI']);
const LEGACY_LOGO_PATHS = new Set(['/logo.png', '/logo.svg']);

function isLegacyLogo(logo) {
  if (LEGACY_LOGO_PATHS.has(logo)) return true;
  const currentOrigin =
    typeof window !== 'undefined' ? window.location.origin : '';
  return (
    (currentOrigin && LEGACY_LOGO_PATHS.has(logo.replace(currentOrigin, ''))) ||
    logo === 'https://cn.meta-api.vip/logo.png' ||
    logo === 'https://cn.meta-api.vip/logo.svg'
  );
}

export function clearStoredUser() {
  localStorage.removeItem('user');
  try {
    clearEnterpriseOperationDrafts(sessionStorage);
  } catch {
    /* Storage may be disabled. */
  }
}

export function getStoredUser() {
  const raw = localStorage.getItem('user');
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

export function isAdmin() {
  const user = getStoredUser();
  return Boolean(user && user.role >= 10);
}

export function isRoot() {
  const user = getStoredUser();
  return Boolean(user && user.role >= 100);
}

export function getSystemName() {
  const systemName = localStorage.getItem('system_name')?.trim();
  if (!systemName || LEGACY_SYSTEM_NAMES.has(systemName)) {
    return DEFAULT_SYSTEM_NAME;
  }
  return systemName;
}

export function getLogo() {
  const logo = localStorage.getItem('logo')?.trim();
  if (!logo || isLegacyLogo(logo)) {
    return DEFAULT_LOGO;
  }
  return logo;
}

export function getUserIdFromLocalStorage() {
  const user = getStoredUser();
  return user ? user.id : -1;
}

export function getFooterHTML() {
  return localStorage.getItem('footer_html');
}

function readTableCompactModes() {
  try {
    const json = localStorage.getItem(TABLE_COMPACT_MODES_KEY);
    return json ? JSON.parse(json) : {};
  } catch {
    return {};
  }
}

function writeTableCompactModes(modes) {
  try {
    localStorage.setItem(TABLE_COMPACT_MODES_KEY, JSON.stringify(modes));
  } catch {
    // ignore
  }
}

export function getTableCompactMode(tableKey = 'global') {
  const modes = readTableCompactModes();
  return !!modes[tableKey];
}

export function setTableCompactMode(compact, tableKey = 'global') {
  const modes = readTableCompactModes();
  modes[tableKey] = compact;
  writeTableCompactModes(modes);
}
