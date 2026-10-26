import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { getPublicShellPage } from '../src/helpers/publicRoutes.js';
import {
  detectDesktopOS,
  formatDesktopDownloadSize,
  normalizeDesktopDownloadCatalog,
  orderDesktopDownloadPackages,
} from '../src/pages/DesktopDownload/desktopDownload.js';

const catalog = normalizeDesktopDownloadCatalog(
  {
    version: '1.2.3',
    notes: 'release',
    pub_date: '2026-08-01T00:00:00Z',
    packages: [
      {
        id: 'windows-x64',
        filename: 'YuanHeng_1.2.3_x64-setup.exe',
        size: 20 * 1024 * 1024,
        url: '/desktop/update/releases/1.2.3/YuanHeng_1.2.3_x64-setup.exe',
      },
      {
        id: 'future-linux',
        filename: 'YuanHeng.AppImage',
        size: 1,
        url: 'https://updates.example.com/YuanHeng.AppImage',
      },
      {
        id: 'macos-arm64',
        filename: 'YuanHeng_1.2.3_aarch64.dmg',
        size: 22 * 1024 * 1024,
        url: 'https://updates.example.com/YuanHeng_1.2.3_aarch64.dmg',
      },
    ],
  },
  'https://yuanheng.example/download',
);
assert.equal(catalog.version, '1.2.3');
assert.deepEqual(
  catalog.packages.map((item) => item.id),
  ['macos-arm64', 'windows-x64'],
);
assert.equal(
  catalog.packages[1].url,
  'https://yuanheng.example/desktop/update/releases/1.2.3/YuanHeng_1.2.3_x64-setup.exe',
);

const officialPackage = (id, suffix) => ({
  id,
  filename: `YuanHeng.Desktop_1.2.3_${suffix}`,
  size: 23000000,
  url: `https://meta-api.vip/desktop/update/releases/1.2.3/YuanHeng.Desktop_1.2.3_${suffix}`,
});
for (const [id, suffix] of [
  ['macos-arm64', 'aarch64.dmg'],
  ['macos-x64', 'x64.dmg'],
  ['windows-x64', 'x64-setup.exe'],
]) {
  const item = officialPackage(id, suffix);
  const normalize = (item, version = '1.2.3') =>
    normalizeDesktopDownloadCatalog({ version, packages: [item] }).packages[0];
  const result = normalize(item);
  assert.equal(result.url, `${item.url}?origin=1`);
  assert.deepEqual(result.backups, [
    {
      label: 'R2',
      url: item.url.replace('meta-api.vip', 'download.meta-api.vip'),
    },
    {
      label: 'GitHub',
      url: `https://github.com/nanashiwang/yuanheng-switch/releases/download/v1.2.3/${item.filename}`,
    },
  ]);
  assert.deepEqual(normalize({ ...item, url: result.url }), result);
  for (const url of [
    `${item.url}?token=private`,
    `${item.url}?origin=1&token=private`,
    `${item.url}#fragment`,
    item.url.replace('https:', 'http:'),
    item.url.replace('meta-api.vip', 'meta-api.vip.evil.test'),
    item.url.replace('meta-api.vip', 'meta-api.vip:444'),
    item.url.replace('meta-api.vip', 'custom.example.com'),
    item.url.replace('/1.2.3/', '/1.2.2/'),
  ]) {
    const custom = normalize({ ...item, url });
    assert.equal(custom.url, url);
    assert.deepEqual(custom.backups, []);
  }
  assert.deepEqual(normalize(item, '1.2.4').backups, []);
  assert.deepEqual(normalize(item, '1.2.3-beta.1').backups, []);
  assert.deepEqual(
    normalize({ ...item, filename: `Custom_${suffix}` }).backups,
    [],
  );
}
assert.ok(catalog.packages.every((item) => item.backups.length === 0));

assert.equal(
  detectDesktopOS({
    userAgent: 'Mozilla/5.0 (Windows NT 10.0)',
    platform: 'Win32',
  }),
  'windows',
);
assert.equal(
  detectDesktopOS({
    userAgent: 'Mozilla/5.0 (Macintosh)',
    platform: 'MacIntel',
  }),
  'macos',
);
assert.equal(
  detectDesktopOS({
    userAgent: 'Mozilla/5.0 (iPad)',
    platform: 'MacIntel',
    maxTouchPoints: 5,
  }),
  'other',
);
assert.equal(
  detectDesktopOS({ userAgent: 'Mozilla/5.0 (X11; Linux x86_64)' }),
  'other',
);

assert.deepEqual(
  orderDesktopDownloadPackages(catalog.packages, 'windows').map(
    (item) => item.id,
  ),
  ['windows-x64', 'macos-arm64'],
);
assert.equal(formatDesktopDownloadSize(22 * 1024 * 1024), '22.0 MB');
assert.equal(formatDesktopDownloadSize(512), '1 KB');

for (const invalid of [
  {
    version: '1.2.3',
    packages: [
      {
        id: 'windows-x64',
        filename: '../bad.exe',
        size: 1,
        url: 'https://example.com/bad.exe',
      },
    ],
  },
  {
    version: '1.2.3',
    packages: [
      {
        id: 'windows-x64',
        filename: 'safe_x64-setup.exe',
        size: 1,
        url: 'javascript:alert(1)',
      },
    ],
  },
  {
    version: '1.2.3',
    packages: [
      {
        id: 'windows-x64',
        filename: 'one_x64-setup.exe',
        size: 1,
        url: 'https://example.com/one_x64-setup.exe',
      },
      {
        id: 'windows-x64',
        filename: 'two_x64-setup.exe',
        size: 1,
        url: 'https://example.com/two_x64-setup.exe',
      },
    ],
  },
  {
    version: '1.2.3',
    packages: [
      {
        id: 'windows-x64',
        filename: 'wrong.dmg',
        size: 1,
        url: 'https://example.com/wrong.dmg',
      },
    ],
  },
  {
    version: '1.2.3',
    packages: [
      {
        id: 'windows-x64',
        filename: 'safe_x64-setup.exe',
        size: 1,
      },
    ],
  },
  {
    version: '1.2.3',
    packages: [
      {
        id: 'windows-x64',
        filename: 'safe_x64-setup.exe',
        size: 1,
        url: 'https://user:secret@example.com/safe_x64-setup.exe',
      },
    ],
  },
]) {
  assert.throws(() => normalizeDesktopDownloadCatalog(invalid));
}

// Download stays in the public shell without requiring console authentication.
for (const pathname of ['/download', '/download/']) {
  assert.equal(getPublicShellPage(pathname), 'download');
}

Promise.all([
  readFile(
    new URL('../src/pages/Home/EnterpriseHome.jsx', import.meta.url),
    'utf8',
  ),
  readFile(
    new URL('../src/components/layout/PublicHomeShell.jsx', import.meta.url),
    'utf8',
  ),
  readFile(new URL('../src/index.jsx', import.meta.url), 'utf8'),
  readFile(
    new URL('../src/components/layout/HeaderDownloadLink.jsx', import.meta.url),
    'utf8',
  ),
  readFile(
    new URL('../src/components/layout/PublicFooter.jsx', import.meta.url),
    'utf8',
  ),
])
  .then(
    ([
      homeSource,
      shellSource,
      entrySource,
      downloadLinkSource,
      footerSource,
    ]) => {
      assert.match(homeSource, /to='\/download'/);
      assert.match(homeSource, /t\('下载客户端'\)/);
      assert.match(shellSource, /DesktopDownload/);
      assert.match(
        entrySource,
        /if \(getPublicShellPage\(location\.pathname\)\) \{\s*return <PublicHomeShell \/>;/,
      );
      assert.match(downloadLinkSource, /to='\/download'/);
      assert.match(downloadLinkSource, /t\('下载客户端'\)/);
      // Unlike console links, the download entry must not trigger app-shell prefetch.
      assert.doesNotMatch(
        downloadLinkSource,
        /prefetchAppShell|onMouseEnter=|onTouchStart=/,
      );
      assert.match(
        footerSource,
        /text: '下载客户端', to: '\/download', prefetchApp: false/,
      );

      console.log('desktop download helpers: ok');
    },
  )
  .catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
