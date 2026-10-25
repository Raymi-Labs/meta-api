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

import React, { useEffect, useState } from 'react';
import { API, showError } from '../../../helpers';
import { Empty, Card, Spin, Typography, Button } from '@douyinfe/semi-ui';
const { Title } = Typography;
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import { useTranslation } from 'react-i18next';
import MarkdownRenderer from '../markdown/MarkdownRenderer';
import RichContent from '../RichContent';
import { useContentLanguage } from '../../../helpers/contentLanguageReact';
import {
  translateConfiguredDocument,
  canonicalContent,
} from '../../../helpers/contentLanguage';
import contentOriginals from '../../../helpers/contentLanguageSources.json';
import {
  isHttpUrl,
  isLikelyStandaloneHtml,
  sanitizeHtmlContent,
} from '../../../helpers/richContent';

// 检查是否为 URL
const isUrl = (content) => {
  return isHttpUrl(content);
};

// 检查是否为 HTML 内容
const isHtmlContent = (content) => {
  if (!content || typeof content !== 'string') return false;

  return isLikelyStandaloneHtml(content);
};

// 安全地渲染HTML内容
const sanitizeHtml = (html) => {
  // 创建一个临时元素来解析HTML
  const tempDiv = document.createElement('div');
  tempDiv.innerHTML = sanitizeHtmlContent(html);

  // 提取样式
  const styleNodes = Array.from(tempDiv.querySelectorAll('style'));
  const styles = styleNodes.map((style) => style.textContent || '').join('\n');
  styleNodes.forEach((style) => style.remove());

  // 提取body内容，如果没有body标签则使用全部内容
  const bodyContent = tempDiv.querySelector('body');
  const content = bodyContent ? bodyContent.innerHTML : tempDiv.innerHTML;

  return { content, styles };
};

/**
 * 通用文档渲染组件
 * @param {string} apiEndpoint - API 接口地址
 * @param {string} title - 文档标题
 * @param {string} cacheKey - 本地存储缓存键
 * @param {string} emptyMessage - 空内容时的提示消息
 */
const DocumentRenderer = ({ apiEndpoint, title, cacheKey, emptyMessage }) => {
  const { t, i18n } = useTranslation();
  const bodyLanguage = useContentLanguage(
    i18n.resolvedLanguage || i18n.language,
  );
  const [showOriginal, setShowOriginal] = useState(false);
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [htmlStyles, setHtmlStyles] = useState('');
  const [processedHtmlContent, setProcessedHtmlContent] = useState('');

  const translation = translateConfiguredDocument(
    content,
    cacheKey,
    bodyLanguage.dictionary,
    contentOriginals,
    bodyLanguage.language,
  );
  const renderedContent = showOriginal ? content : translation.content;
  const knownOriginal =
    canonicalContent(contentOriginals[cacheKey]) === canonicalContent(content);
  const contentLang =
    translation.translated && !showOriginal
      ? bodyLanguage.language
      : knownOriginal
        ? 'zh-CN'
        : undefined;

  useEffect(() => {
    setShowOriginal(false);
  }, [content, bodyLanguage.language]);
  useEffect(() => {
    processContent(renderedContent);
  }, [renderedContent]);

  const languageNotice = bodyLanguage.language !== 'zh-CN' && (
    <div
      className='mb-6 text-sm text-gray-500 dark:text-gray-400'
      role='status'
    >
      {bodyLanguage.pending ? (
        t('正在加载正文翻译…')
      ) : bodyLanguage.failed ? (
        <>
          {t('正文翻译加载失败，请重试。')}{' '}
          <Button size='small' onClick={bodyLanguage.retry}>
            {t('重试')}
          </Button>
        </>
      ) : translation.original ? (
        t('当前内容暂无此语言译文，显示原文。')
      ) : (
        <>
          <p>{t('译文供阅读参考，解释以原文为准。')}</p>
          <Button
            size='small'
            theme='borderless'
            onClick={() => setShowOriginal((current) => !current)}
          >
            {t(showOriginal ? '查看译文' : '查看原文')}
          </Button>
        </>
      )}
    </div>
  );

  const loadContent = async () => {
    // 先从缓存中获取
    let cachedContent = '';
    try {
      cachedContent = localStorage.getItem(cacheKey) || '';
    } catch {
      /* Continue without storage. */
    }
    if (cachedContent) {
      setContent(cachedContent);
      setLoading(false);
    }

    try {
      const res = await API.get(apiEndpoint);
      const { success, message, data } = res.data;
      if (success) {
        const current = typeof data === 'string' ? data : '';
        setContent(current);
        try {
          localStorage.setItem(cacheKey, current);
        } catch {
          /* The current server response still wins. */
        }
      } else {
        if (!cachedContent) {
          showError(message || emptyMessage);
          setContent('');
        }
      }
    } catch (error) {
      if (!cachedContent) {
        showError(emptyMessage);
        setContent('');
      }
    } finally {
      setLoading(false);
    }
  };

  const processContent = (rawContent) => {
    if (isHtmlContent(rawContent)) {
      const { content: htmlContent, styles } = sanitizeHtml(rawContent);
      setProcessedHtmlContent(htmlContent);
      setHtmlStyles(styles);
    } else {
      setProcessedHtmlContent('');
      setHtmlStyles('');
    }
  };

  useEffect(() => {
    loadContent();
  }, []);

  // 处理HTML样式注入
  useEffect(() => {
    const styleId = `document-renderer-styles-${cacheKey}`;

    if (htmlStyles) {
      let styleEl = document.getElementById(styleId);
      if (!styleEl) {
        styleEl = document.createElement('style');
        styleEl.id = styleId;
        styleEl.type = 'text/css';
        document.head.appendChild(styleEl);
      }
      styleEl.innerHTML = htmlStyles;
    } else {
      const el = document.getElementById(styleId);
      if (el) el.remove();
    }

    return () => {
      const el = document.getElementById(styleId);
      if (el) el.remove();
    };
  }, [htmlStyles, cacheKey]);

  // 显示加载状态
  if (loading) {
    return (
      <div className='flex justify-center items-center min-h-screen'>
        <Spin size='large' />
      </div>
    );
  }

  // 如果没有内容，显示空状态
  if (!content || content.trim() === '') {
    return (
      <div className='flex justify-center items-center min-h-screen bg-gray-50'>
        <Empty
          title={t('管理员尚未设置此文档内容')}
          image={
            <IllustrationConstruction style={{ width: 150, height: 150 }} />
          }
          darkModeImage={
            <IllustrationConstructionDark style={{ width: 150, height: 150 }} />
          }
          className='p-8'
        />
      </div>
    );
  }

  // 如果是 URL，显示链接卡片
  if (isUrl(content)) {
    return (
      <div className='flex justify-center items-center min-h-screen bg-gray-50 p-4'>
        <Card className='max-w-md w-full'>
          <div className='text-center'>
            <Title heading={4} className='mb-4'>
              {title}
            </Title>
            <p className='text-gray-600 mb-4'>
              {t('管理员设置了外部链接，点击下方按钮访问')}
            </p>
            <a
              href={content.trim()}
              target='_blank'
              rel='noopener noreferrer'
              title={content.trim()}
              aria-label={`${t('访问文档')}: ${content.trim()}`}
              className='inline-block px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors'
            >
              {t('访问文档')}
            </a>
          </div>
        </Card>
      </div>
    );
  }

  // 如果是 HTML 内容，直接渲染
  if (isHtmlContent(content)) {
    return (
      <div className='min-h-screen bg-gray-50'>
        <div className='max-w-4xl mx-auto py-12 px-4 sm:px-6 lg:px-8'>
          <div className='bg-white rounded-lg shadow-sm p-8'>
            <Title heading={2} className='text-center mb-8'>
              {title}
            </Title>
            {languageNotice}
            <div lang={contentLang}>
              <RichContent
                className='prose prose-lg max-w-none'
                content={processedHtmlContent}
                mode='html'
              />
            </div>
          </div>
        </div>
      </div>
    );
  }

  // 其他内容统一使用 Markdown 渲染器
  return (
    <div className='min-h-screen bg-gray-50'>
      <div className='max-w-4xl mx-auto py-12 px-4 sm:px-6 lg:px-8'>
        <div className='bg-white rounded-lg shadow-sm p-8'>
          <Title heading={2} className='text-center mb-8'>
            {title}
          </Title>
          {languageNotice}
          <div className='prose prose-lg max-w-none' lang={contentLang}>
            <MarkdownRenderer content={renderedContent} />
          </div>
        </div>
      </div>
    </div>
  );
};

export default DocumentRenderer;
