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
import { Download, Smartphone } from 'lucide-react';
import { androidDownload } from './androidDownload';
import { formatDesktopDownloadSize } from './desktopDownload';

export default function AndroidDownloadCard({ t }) {
  return (
    <section
      className='desktop-download-panel android-download-panel'
      id='android'
      aria-labelledby='android-download-title'
    >
      <div className='android-download-content'>
        <div className='android-download-heading'>
          <span className='desktop-download-platform-icon'>
            <Smartphone size={26} />
          </span>
          <div>
            <span className='android-download-badge'>{t('公开测试版')}</span>
            <h2 id='android-download-title'>{t('元衡 Android')}</h2>
          </div>
        </div>
        <p className='android-download-description'>
          {t('在手机上聊天、生图、查看额度和管理令牌，支持兑换码与官网充值。')}
        </p>
        <div className='android-download-meta'>
          <span>v{androidDownload.version}</span>
          <span>Android 10+ · ARM64</span>
          <span>{formatDesktopDownloadSize(androidDownload.size)}</span>
        </div>
      </div>
      <div className='android-download-actions'>
        <a
          className='android-download-primary'
          href={androidDownload.url}
          download={androidDownload.filename}
        >
          <Download size={19} />
          {t('下载安卓 APK')}
        </a>
        <a href={androidDownload.backupURL} download={androidDownload.filename}>
          {t('GitHub 备用下载')}
        </a>
      </div>
      <div className='android-download-help'>
        <p>{t('下载后打开 APK，按系统提示允许当前浏览器安装应用。')}</p>
        <p>{t('安卓测试版暂不支持应用内更新，新版本需重新下载安装。')}</p>
        <details>
          <summary>{t('查看安装包校验信息')}</summary>
          <div className='android-download-checksum'>
            <span>SHA-256</span>
            <code>{androidDownload.sha256}</code>
            <a href={androidDownload.checksumURL} download>
              {t('下载校验文件')}
            </a>
          </div>
        </details>
      </div>
    </section>
  );
}
