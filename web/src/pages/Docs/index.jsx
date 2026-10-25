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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { Button, Input, Typography } from '@douyinfe/semi-ui';
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  Copy,
  Search,
  Menu,
  X,
} from 'lucide-react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import MarkdownRenderer from '../../components/common/markdown/MarkdownRenderer';
import { useContentLanguage } from '../../helpers/contentLanguageReact';
import { DOCS } from './catalog';
import {
  docPath,
  extractHeadings,
  getActiveDoc,
  MAIN_ORIGIN,
  NAV_GROUPS,
  prepareMarkdown,
  searchDocs,
  standaloneMarkdown,
} from './docsNavigation';
import { renderRuntimeDocs } from './runtimeDocs';
import useRuntimeDocs from './useRuntimeDocs';
import './index.css';

const { Text } = Typography;

export default function DocsPage({ standalone = false, brand, controls }) {
  const { t, i18n } = useTranslation();
  const bodyLanguage = useContentLanguage(
    i18n.resolvedLanguage || i18n.language,
  );
  const location = useLocation();
  const navigate = useNavigate();
  const activeDoc = getActiveDoc(location.pathname, standalone);
  const runtime = useRuntimeDocs({ publicOnly: standalone });
  const [query, setQuery] = useState('');
  const [mobileOpen, setMobileOpen] = useState(false);
  const [copyStatus, setCopyStatus] = useState('');
  const searchRef = useRef(null);
  const navigationRef = useRef(null);
  const language = bodyLanguage.language;
  const translateMetadata = useCallback(
    (text) => t(bodyLanguage.translate(text)),
    [bodyLanguage.translate, t],
  );
  const content = useMemo(() => {
    if (!activeDoc) return '';
    const rendered = prepareMarkdown(
      renderRuntimeDocs(bodyLanguage.markdown(activeDoc.content), {
        ...runtime,
        translate: bodyLanguage.markdown,
        origin: standalone ? MAIN_ORIGIN : window.location.origin,
      }),
    );
    return standalone ? standaloneMarkdown(rendered, language) : rendered;
  }, [
    activeDoc,
    bodyLanguage.markdown,
    runtime.serverAddress,
    runtime.pricing,
    runtime.loading,
    standalone,
    language,
  ]);
  const headings = useMemo(() => extractHeadings(content), [content]);
  const searchableDocs = useMemo(
    () =>
      DOCS.map((doc) => ({
        ...doc,
        title: translateMetadata(doc.title),
        description: translateMetadata(doc.description),
        content: bodyLanguage.markdown(doc.content),
      })),
    [translateMetadata, bodyLanguage.markdown],
  );
  const results = useMemo(
    () => searchDocs(query, searchableDocs),
    [query, searchableDocs],
  );
  const index = DOCS.findIndex((doc) => doc.key === activeDoc?.key);
  const go = (path) => {
    navigate(docPath(path, standalone));
    setQuery('');
    setMobileOpen(false);
  };

  useEffect(() => {
    setMobileOpen(false);
    setCopyStatus('');
    if (!location.hash) window.scrollTo({ top: 0 });
  }, [location.pathname]);
  useEffect(() => {
    if (!location.hash) return;
    let id;
    try {
      id = decodeURIComponent(location.hash.slice(1));
    } catch {
      return;
    }
    requestAnimationFrame(() => document.getElementById(id)?.scrollIntoView());
  }, [location.hash, content]);
  useEffect(() => {
    const onKey = (event) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setMobileOpen(true);
        requestAnimationFrame(() => searchRef.current?.focus());
      }
      if (event.key === 'Escape') {
        setQuery('');
        setMobileOpen(false);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    navigationRef.current?.scrollTo({ top: 0 });
  }, [query]);

  const copyPage = async () => {
    try {
      await navigator.clipboard.writeText(content);
      setCopyStatus('复制成功');
    } catch {
      setCopyStatus('复制失败');
    }
  };

  return (
    <div
      className={`nan-docs-page${standalone ? ' nan-docs-standalone' : ''}`}
      id='docs-top'
    >
      <div className='nan-docs-mobile-bar'>
        {brand || <strong>{t('平台使用文档')}</strong>}
        {controls}
        <Button
          aria-label={t(mobileOpen ? '关闭' : '选择文档')}
          aria-expanded={mobileOpen}
          aria-controls='docs-sidebar'
          onClick={() => setMobileOpen(!mobileOpen)}
          icon={mobileOpen ? <X size={18} /> : <Menu size={18} />}
        />
      </div>
      <aside
        className={`nan-docs-sidebar${mobileOpen ? ' is-open' : ''}`}
        id='docs-sidebar'
      >
        <div className='nan-docs-brand'>
          {brand || (
            <>
              <BookOpen size={22} />
              <strong>{t('平台使用文档')}</strong>
            </>
          )}
        </div>
        <div className='nan-docs-search'>
          <Input
            ref={searchRef}
            aria-label={t('搜索')}
            placeholder={`${t('搜索')}…`}
            prefix={<Search size={16} />}
            suffix={<kbd>⌘ K</kbd>}
            value={query}
            onChange={setQuery}
            showClear
          />
        </div>
        <nav
          ref={navigationRef}
          className='nan-docs-navigation'
          aria-label={t('选择文档')}
        >
          {query.trim() ? (
            <div className='nan-docs-results' aria-live='polite'>
              {results.length ? (
                results.map((doc) => (
                  <button key={doc.key} onClick={() => go(doc.path)}>
                    <strong>{doc.title}</strong>
                    <span>{doc.description}</span>
                  </button>
                ))
              ) : (
                <p>{t('暂无数据')}</p>
              )}
            </div>
          ) : (
            NAV_GROUPS.map((group) => (
              <div className='nan-docs-nav-group' key={group}>
                <span className='nan-docs-nav-group-title'>
                  {translateMetadata(group)}
                </span>
                {DOCS.filter((doc) => doc.group === group).map((doc) => (
                  <Link
                    key={doc.key}
                    to={docPath(doc.path, standalone)}
                    aria-current={
                      activeDoc?.key === doc.key ? 'page' : undefined
                    }
                    className='nan-docs-nav-link'
                  >
                    {translateMetadata(doc.title)}
                  </Link>
                ))}
              </div>
            ))
          )}
        </nav>
        <div className='nan-docs-sidebar-footer'>
          <a
            href='https://docs.newapi.pro/zh/docs'
            target='_blank'
            rel='noreferrer'
          >
            {t('上游文档')} ↗
          </a>
        </div>
      </aside>
      {mobileOpen && (
        <button
          className='nan-docs-backdrop'
          aria-label={t('关闭')}
          onClick={() => setMobileOpen(false)}
        />
      )}
      <div className='nan-docs-workspace'>
        <header className='nan-docs-header'>
          <nav className='nan-docs-tabs' aria-label={t('文档')}>
            <Link to={docPath('/docs', standalone)}>{t('文档')}</Link>
            <Link to={docPath('/docs/reference/overview', standalone)}>
              {t('API 参考')}
            </Link>
            <Link to={docPath('/docs/integrations/overview', standalone)}>
              {translateMetadata('集成指南')}
            </Link>
            <Link to={docPath('/docs/troubleshooting', standalone)}>
              {t('常见报错排查')}
            </Link>
            <a
              className='nan-docs-console'
              href={
                standalone
                  ? `${MAIN_ORIGIN}/console?lang=${language}`
                  : '/console'
              }
            >
              {t('控制台')} ↗
            </a>
          </nav>
          {controls}
        </header>
        <div className='nan-docs-layout'>
          <main className='nan-docs-content'>
            {!activeDoc ? (
              <>
                <h1>{t('文档不存在')}</h1>
                <Link to={docPath('/docs', standalone)}>
                  {t('平台使用文档')}
                </Link>
              </>
            ) : (
              <>
                <header className='nan-docs-card-header'>
                  <span className='nan-docs-eyebrow'>
                    {translateMetadata(activeDoc.group)}
                  </span>
                  <div className='nan-docs-title-row'>
                    <h1>{translateMetadata(activeDoc.title)}</h1>
                    <Button
                      theme='borderless'
                      icon={<Copy size={16} />}
                      onClick={copyPage}
                    >
                      {t('复制')}
                    </Button>
                  </div>
                  <p>{translateMetadata(activeDoc.description)}</p>
                  <span className='sr-only' role='status'>
                    {copyStatus && t(copyStatus)}
                  </span>
                </header>
                <div className='nan-docs-sync' role='status'>
                  <Text type={runtime.failed ? 'warning' : 'tertiary'}>
                    {runtime.loading
                      ? t('正在更新文档配置…')
                      : runtime.failed
                        ? standalone
                          ? `${t('加载失败')} · ${MAIN_ORIGIN}`
                          : t(
                              '部分配置加载失败，请重试；地址暂用当前站点，模型以模型广场为准。',
                            )
                        : t('地址、分组和请求样例随站点配置自动更新')}
                  </Text>
                  <Button
                    theme='borderless'
                    size='small'
                    loading={runtime.loading}
                    onClick={runtime.refresh}
                  >
                    {t('刷新')}
                  </Button>
                </div>
                {(bodyLanguage.pending || bodyLanguage.failed) && (
                  <div className='nan-docs-sync' role='status'>
                    <Text>
                      {t(
                        bodyLanguage.failed
                          ? '正文翻译加载失败，请重试。'
                          : '正在加载正文翻译…',
                      )}
                    </Text>
                    {bodyLanguage.failed && (
                      <Button onClick={bodyLanguage.retry}>{t('重试')}</Button>
                    )}
                  </div>
                )}
                <article
                  className='nan-docs-card'
                  lang={bodyLanguage.ready ? language : 'zh-CN'}
                >
                  <MarkdownRenderer content={content} fontSize={15} />
                </article>
                <nav className='nan-docs-pagination' aria-label={t('选择文档')}>
                  {index > 0 ? (
                    <Link to={docPath(DOCS[index - 1].path, standalone)}>
                      <ArrowLeft size={18} />
                      {translateMetadata(DOCS[index - 1].title)}
                    </Link>
                  ) : (
                    <span />
                  )}
                  {index < DOCS.length - 1 && (
                    <Link to={docPath(DOCS[index + 1].path, standalone)}>
                      {translateMetadata(DOCS[index + 1].title)}
                      <ArrowRight size={18} />
                    </Link>
                  )}
                </nav>
              </>
            )}
            {standalone && (
              <footer className='nan-docs-attribution'>
                元衡 API ·{' '}
                <a
                  href='https://github.com/QuantumNous/new-api'
                  target='_blank'
                  rel='noreferrer'
                >
                  New API · QuantumNous
                </a>
              </footer>
            )}
          </main>
          <aside className='nan-docs-toc'>
            <strong>{t('本页目录')}</strong>
            <nav aria-label={t('本页目录')}>
              {headings.map((heading, i) => (
                <a
                  key={`${heading.id}-${i}`}
                  href={`#${heading.id}`}
                  className={heading.level === 3 ? 'nan-docs-toc-link-sub' : ''}
                >
                  {heading.title}
                </a>
              ))}
            </nav>
            <a href='#docs-top'>{t('返回顶部')}</a>
          </aside>
        </div>
      </div>
    </div>
  );
}
