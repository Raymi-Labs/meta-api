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

import { buildAuthPathWithNext } from './authRedirect';
import { withDocsLanguage } from './docsLink';

export const DEFAULT_HEADER_NAV_MODULES = {
  home: true,
  console: true,
  pricing: true,
  docs: true,
  about: true,
  usage: false,
};

export const getTopNavigationLinks = (t, docsLink, headerNavModules) => {
  docsLink = withDocsLanguage(docsLink);
  const modules = headerNavModules || DEFAULT_HEADER_NAV_MODULES;
  const pricingEnabled =
    typeof modules.pricing === 'object'
      ? modules.pricing?.enabled
      : modules.pricing;

  return [
    pricingEnabled && {
      text: t('模型广场'),
      itemKey: 'pricing',
      to: '/pricing',
      requireAuth: Boolean(modules.pricing?.requireAuth),
    },
    modules.console && {
      text: t('模型体验'),
      itemKey: 'playground',
      to: '/console/playground',
      requireAuth: true,
    },
    {
      text: t('企业定制'),
      itemKey: 'enterprise',
      to: '/enterprise',
      prefetchApp: false,
    },
    modules.docs &&
      docsLink && {
        text: t('文档'),
        itemKey: 'docs',
        isExternal: !docsLink.startsWith('/'),
        externalLink: docsLink,
        to: docsLink,
      },
    modules.console && {
      text: t('控制台'),
      itemKey: 'console',
      to: '/console',
      requireAuth: true,
    },
  ].filter(Boolean);
};

export const getTopNavigationTarget = (
  link,
  user,
  pricingRequireAuth = false,
) =>
  !user &&
  (link.requireAuth || (link.itemKey === 'pricing' && pricingRequireAuth))
    ? buildAuthPathWithNext('/login', link.to)
    : link.to;
