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

export const desktopPackageDefinitions = [
  {
    id: 'macos-arm64',
    os: 'macos',
    arch: 'arm64',
    format: 'dmg',
    filenameSuffixes: ['_aarch64.dmg', '_arm64.dmg'],
  },
  {
    id: 'macos-x64',
    os: 'macos',
    arch: 'x86_64',
    format: 'dmg',
    filenameSuffixes: ['_x64.dmg', '_x86_64.dmg'],
  },
  {
    id: 'windows-x64',
    os: 'windows',
    arch: 'x86_64',
    format: 'exe',
    filenameSuffixes: ['_x64-setup.exe', '_x86_64-setup.exe'],
  },
];

const packageDefinitionsByID = new Map(
  desktopPackageDefinitions.map((item) => [item.id, item]),
);

const desktopVersionPattern =
  /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

// Preserve the server-owned URL unless it is our exact public stable asset.
// Never copy private query parameters or guess mirrors for custom deployments.
const officialDownloadLinks = (url, filename, version, definition) => {
  const unchanged = { url: url.href, backups: [] };
  if (!/^[0-9]+\.[0-9]+\.[0-9]+$/.test(version)) return unchanged;
  const expectedFilename = `YuanHeng.Desktop_${version}${definition.filenameSuffixes[0]}`;
  if (filename !== expectedFilename) return unchanged;
  const path = `/desktop/update/releases/${version}/${filename}`;
  const mirror = `https://meta-api.vip${path}`;
  if (url.href !== mirror && url.href !== `${mirror}?origin=1`)
    return unchanged;
  return {
    url: `${mirror}?origin=1`,
    backups: [
      { label: 'R2', url: `https://download.meta-api.vip${path}` },
      {
        label: 'GitHub',
        url: `https://github.com/nanashiwang/yuanheng-switch/releases/download/v${version}/${filename}`,
      },
    ],
  };
};

export const normalizeDesktopDownloadCatalog = (
  value,
  baseURL = 'https://localhost/',
) => {
  if (!value || typeof value !== 'object') {
    throw new Error('invalid desktop download catalog');
  }
  const version = typeof value.version === 'string' ? value.version.trim() : '';
  if (version.length > 128 || !desktopVersionPattern.test(version)) {
    throw new Error('invalid desktop download version');
  }
  if (!Array.isArray(value.packages)) {
    throw new Error('invalid desktop download packages');
  }

  const seen = new Set();
  const packages = [];
  for (const item of value.packages) {
    if (!item || typeof item !== 'object') {
      throw new Error('invalid desktop download package');
    }
    const definition = packageDefinitionsByID.get(item.id);
    if (!definition) continue;
    if (seen.has(definition.id)) {
      throw new Error('duplicate desktop download package');
    }
    const filename =
      typeof item.filename === 'string' ? item.filename.trim() : '';
    if (
      !filename ||
      filename.length > 200 ||
      filename === '.' ||
      filename === '..' ||
      filename.includes('/') ||
      filename.includes('\\')
    ) {
      throw new Error('invalid desktop download filename');
    }
    if (
      !definition.filenameSuffixes.some((suffix) =>
        filename.toLowerCase().endsWith(suffix),
      )
    ) {
      throw new Error('mismatched desktop download filename');
    }
    if (!Number.isSafeInteger(item.size) || item.size <= 0) {
      throw new Error('invalid desktop download size');
    }
    const rawURL = typeof item.url === 'string' ? item.url.trim() : '';
    if (!rawURL || rawURL.length > 2048) {
      throw new Error('invalid desktop download url');
    }
    let parsedURL;
    try {
      parsedURL = new URL(rawURL, baseURL);
    } catch {
      throw new Error('invalid desktop download url');
    }
    if (
      (parsedURL.protocol !== 'https:' && parsedURL.protocol !== 'http:') ||
      parsedURL.username ||
      parsedURL.password
    ) {
      throw new Error('unsafe desktop download url');
    }
    seen.add(definition.id);
    packages.push({
      ...definition,
      filename,
      size: item.size,
      ...officialDownloadLinks(parsedURL, filename, version, definition),
    });
  }

  packages.sort(
    (left, right) =>
      desktopPackageDefinitions.findIndex((item) => item.id === left.id) -
      desktopPackageDefinitions.findIndex((item) => item.id === right.id),
  );
  if (packages.length === 0) {
    throw new Error('desktop download catalog is empty');
  }
  return {
    version,
    notes: typeof value.notes === 'string' ? value.notes.trim() : '',
    pubDate: typeof value.pub_date === 'string' ? value.pub_date.trim() : '',
    packages,
  };
};

export const detectDesktopOS = (navigatorLike = {}) => {
  const userAgent = String(navigatorLike.userAgent || '').toLowerCase();
  const platform = String(
    navigatorLike.userAgentData?.platform || navigatorLike.platform || '',
  ).toLowerCase();
  const touchPoints = Number(navigatorLike.maxTouchPoints || 0);
  const isIOS =
    /iphone|ipad|ipod/.test(userAgent) ||
    (platform === 'macintel' && touchPoints > 1);
  if (isIOS) return 'other';
  if (platform.includes('win') || userAgent.includes('windows'))
    return 'windows';
  if (platform.includes('mac') || userAgent.includes('macintosh'))
    return 'macos';
  return 'other';
};

export const orderDesktopDownloadPackages = (packages, detectedOS) => {
  const osRank =
    detectedOS === 'windows'
      ? { windows: 0, macos: 1 }
      : { macos: 0, windows: 1 };
  const definitionRank = new Map(
    desktopPackageDefinitions.map((item, index) => [item.id, index]),
  );
  return [...packages].sort((left, right) => {
    const byOS = (osRank[left.os] ?? 9) - (osRank[right.os] ?? 9);
    return (
      byOS ||
      (definitionRank.get(left.id) ?? 99) - (definitionRank.get(right.id) ?? 99)
    );
  });
};

export const formatDesktopDownloadSize = (bytes) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '';
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};
