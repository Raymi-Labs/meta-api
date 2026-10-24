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

export const THEME_STORAGE_KEY = 'theme-mode';
export const SYSTEM_THEME_QUERY = '(prefers-color-scheme: dark)';

export const normalizeTheme = (value) =>
  value === 'dark' || value === 'light' ? value : 'auto';

export const getStoredTheme = () => {
  try {
    return normalizeTheme(window.localStorage.getItem(THEME_STORAGE_KEY));
  } catch {
    return 'auto';
  }
};

export const storeTheme = (theme) => {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // Theme switching remains usable when browser storage is disabled or full.
  }
};

export const getSystemTheme = () => {
  try {
    return window.matchMedia(SYSTEM_THEME_QUERY).matches ? 'dark' : 'light';
  } catch {
    return 'light';
  }
};

export const applyTheme = (theme) => {
  if (typeof document === 'undefined') return;
  const dark = theme === 'dark';
  document.documentElement.classList.toggle('dark', dark);
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
  if (dark) document.body?.setAttribute('theme-mode', 'dark');
  else document.body?.removeAttribute('theme-mode');
};

export const subscribeSystemTheme = (onChange) => {
  let mediaQuery;
  try {
    mediaQuery = window.matchMedia(SYSTEM_THEME_QUERY);
  } catch {
    return undefined;
  }
  const handleChange = (event) => onChange(event.matches ? 'dark' : 'light');
  // Read again after subscribing so a system change during mount is not lost.
  if (mediaQuery.addEventListener) {
    mediaQuery.addEventListener('change', handleChange);
    handleChange(mediaQuery);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }
  if (mediaQuery.addListener) {
    mediaQuery.addListener(handleChange);
    handleChange(mediaQuery);
    return () => mediaQuery.removeListener(handleChange);
  }
  return undefined;
};
