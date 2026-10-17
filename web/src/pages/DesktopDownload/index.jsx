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

import React, { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Apple,
  ArrowLeft,
  CheckCircle2,
  Download,
  Laptop,
  RefreshCw,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import { usePublicTranslation } from '../../helpers/publicLocale';
import {
  detectDesktopOS,
  formatDesktopDownloadSize,
  normalizeDesktopDownloadCatalog,
  orderDesktopDownloadPackages,
} from './desktopDownload';
import './DesktopDownload.css';

const packagePresentation = (item, t) => {
  if (item.id === 'macos-arm64') {
    return {
      Icon: Apple,
      title: t('macOS · Apple 芯片'),
      description: t('适用于 Apple 芯片的 Mac'),
      action: t('下载 DMG'),
    };
  }
  if (item.id === 'macos-x64') {
    return {
      Icon: Laptop,
      title: t('macOS · Intel 芯片'),
      description: t('适用于 Intel 芯片的 Mac'),
      action: t('下载 DMG'),
    };
  }
  return {
    Icon: Laptop,
    title: t('Windows · 64 位'),
    description: t('适用于 64 位 Windows 电脑'),
    action: t('下载安装程序'),
  };
};

const formatReleaseDate = (value, language) => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return new Intl.DateTimeFormat(language.startsWith('zh') ? 'zh-CN' : 'en', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date);
};

const DesktopDownload = () => {
  const { t, language } = usePublicTranslation();
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState({ status: 'loading', catalog: null });
  const detectedOS = useMemo(
    () => detectDesktopOS(typeof navigator === 'undefined' ? {} : navigator),
    [],
  );

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: 'loading', catalog: null });
    const loadCatalog = async () => {
      try {
        const response = await fetch('/desktop/update/downloads.json', {
          cache: 'no-cache',
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
          signal: controller.signal,
        });
        if (!response.ok) {
          setState({
            status: response.status === 404 ? 'unavailable' : 'error',
            catalog: null,
          });
          return;
        }
        const payload = await response.json();
        const catalog = normalizeDesktopDownloadCatalog(
          payload,
          window.location.origin,
        );
        setState({ status: 'ready', catalog });
      } catch (error) {
        if (error?.name !== 'AbortError') {
          setState({ status: 'error', catalog: null });
        }
      }
    };
    loadCatalog();
    return () => controller.abort();
  }, [attempt]);

  const packages = useMemo(
    () =>
      state.catalog
        ? orderDesktopDownloadPackages(state.catalog.packages, detectedOS)
        : [],
    [detectedOS, state.catalog],
  );
  const releaseDate = formatReleaseDate(state.catalog?.pubDate, language);

  return (
    <main className='desktop-download-page'>
      <div className='desktop-download-glow desktop-download-glow-one' />
      <div className='desktop-download-glow desktop-download-glow-two' />
      <section
        className='desktop-download-shell'
        aria-labelledby='download-title'
      >
        <Link className='desktop-download-back' to='/'>
          <ArrowLeft size={17} />
          {t('返回首页')}
        </Link>

        <div className='desktop-download-hero'>
          <div className='desktop-download-kicker'>
            <Sparkles size={16} />
            YuanHeng Desktop
          </div>
          <h1 id='download-title'>{t('把你的 AI 工具，放进一个控制中心')}</h1>
          <p>
            {t(
              '下载安装 YuanHeng Desktop，统一管理终端、客户端、模型和分组，后续版本可在应用内直接更新。',
            )}
          </p>
          {state.catalog && (
            <div
              className='desktop-download-release'
              aria-label={t('当前发布版本')}
            >
              <span>v{state.catalog.version}</span>
              {releaseDate && <small>{releaseDate}</small>}
              <em>{t('最新稳定版')}</em>
            </div>
          )}
        </div>

        <section className='desktop-download-panel' aria-live='polite'>
          <div className='desktop-download-panel-heading'>
            <div>
              <span>{t('选择安装包')}</span>
              <h2>{t('选择与你的电脑匹配的版本')}</h2>
            </div>
            {detectedOS === 'macos' && (
              <p>{t('检测到 macOS，请根据芯片类型选择。')}</p>
            )}
            {detectedOS === 'windows' && (
              <p>{t('检测到 Windows，已优先显示对应版本。')}</p>
            )}
          </div>

          {state.status === 'loading' && (
            <div
              className='desktop-download-grid'
              aria-label={t('正在获取最新版本')}
            >
              {[0, 1, 2].map((item) => (
                <div className='desktop-download-card is-loading' key={item}>
                  <span />
                  <strong />
                  <small />
                  <i />
                </div>
              ))}
            </div>
          )}

          {state.status === 'ready' && (
            <div className='desktop-download-grid'>
              {packages.map((item) => {
                const presentation = packagePresentation(item, t);
                const Icon = presentation.Icon;
                const recommended =
                  detectedOS === 'windows' && item.id === 'windows-x64';
                return (
                  <article
                    className={`desktop-download-card${recommended ? ' is-recommended' : ''}`}
                    key={item.id}
                  >
                    <div className='desktop-download-card-top'>
                      <span className='desktop-download-platform-icon'>
                        <Icon size={24} />
                      </span>
                      {recommended && (
                        <span className='desktop-download-recommended'>
                          <CheckCircle2 size={14} />
                          {t('推荐版本')}
                        </span>
                      )}
                    </div>
                    <h3>{presentation.title}</h3>
                    <p>{presentation.description}</p>
                    <div className='desktop-download-file-meta'>
                      <span>{formatDesktopDownloadSize(item.size)}</span>
                      <span>{item.format.toUpperCase()}</span>
                    </div>
                    <code title={item.filename}>{item.filename}</code>
                    <a
                      className='desktop-download-action'
                      href={item.url}
                      download={item.filename}
                      aria-label={`${presentation.title} · ${presentation.action}`}
                    >
                      <Download size={18} />
                      {presentation.action}
                    </a>
                    {item.backups.length > 0 && (
                      <div className='desktop-download-backups'>
                        <span>{t('备用线路')}</span>
                        {item.backups.map((source) => (
                          <a
                            key={source.label}
                            href={source.url}
                            download={item.filename}
                            aria-label={`${presentation.title} · ${source.label}`}
                          >
                            {source.label}
                          </a>
                        ))}
                      </div>
                    )}
                  </article>
                );
              })}
            </div>
          )}

          {(state.status === 'unavailable' || state.status === 'error') && (
            <div className='desktop-download-empty'>
              <span className='desktop-download-empty-icon'>
                <RefreshCw size={25} />
              </span>
              <h3>
                {state.status === 'unavailable'
                  ? t('客户端暂未开放下载')
                  : t('暂时无法获取下载信息')}
              </h3>
              <p>
                {state.status === 'unavailable'
                  ? t('当前还没有可公开下载的稳定版本，请稍后再来。')
                  : t('网络或更新服务暂时不可用，请稍后重试。')}
              </p>
              <button
                type='button'
                onClick={() => setAttempt((value) => value + 1)}
              >
                <RefreshCw size={17} />
                {t('重新加载')}
              </button>
            </div>
          )}
        </section>

        <div className='desktop-download-trust'>
          <span>
            <ShieldCheck size={19} />
            {t('安装包由元衡平台直接托管')}
          </span>
          <span>
            <CheckCircle2 size={19} />
            {t('安装后支持应用内自动更新')}
          </span>
        </div>
      </section>
    </main>
  );
};

export default DesktopDownload;
