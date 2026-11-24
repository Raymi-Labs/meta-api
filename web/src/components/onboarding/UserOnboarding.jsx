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
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { Button, Typography } from '@douyinfe/semi-ui';
import { ArrowLeft, ArrowRight, MousePointer2, X } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import './UserOnboarding.css';

const { Text, Title } = Typography;

const TOUR_VERSION = 1;
const TOUR_STORAGE_PREFIX = `newapi:user-onboarding:regular:v${TOUR_VERSION}`;
const RESTART_EVENT = 'newapi:user-onboarding:restart';

function isElementVisible(element) {
  if (!element) return false;
  const rect = element.getBoundingClientRect();
  const style = window.getComputedStyle(element);
  return (
    rect.width > 0 &&
    rect.height > 0 &&
    style.display !== 'none' &&
    style.visibility !== 'hidden' &&
    Number(style.opacity || 1) !== 0
  );
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

function readTourState(storageKey) {
  try {
    return localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}

function writeTourState(storageKey, status) {
  try {
    localStorage.setItem(
      storageKey,
      JSON.stringify({
        version: TOUR_VERSION,
        status,
        updatedAt: new Date().toISOString(),
      }),
    );
  } catch {
    // 浏览器禁用 localStorage 时只在本次会话隐藏。
  }
}

function removeTourState(storageKey) {
  try {
    localStorage.removeItem(storageKey);
  } catch {
    // ignore
  }
}

const getStorageKey = (userId) =>
  userId ? `${TOUR_STORAGE_PREFIX}:user:${userId}` : TOUR_STORAGE_PREFIX;

const buildSteps = (t, docsLink) => [
  {
    id: 'docs',
    badge: t('文档'),
    title: t('配置教程先看这里'),
    body: docsLink
      ? t(
          '顶部「文档」入口会跳到配置教程。配置 Cherry Studio、Open WebUI、ChatBox 等客户端时，先从这里开始。',
        )
      : t(
          '这里用于放配置教程入口。若顶部暂时没有「文档」，可以先访问 /docs 或联系管理员配置文档链接。',
        ),
    selectors: [
      '[data-onboarding="nav-docs"]',
      '[data-onboarding="app-header"]',
    ],
  },
  {
    id: 'console',
    badge: t('控制台'),
    title: t('日常操作都在控制台完成'),
    body: t(
      '令牌、测速、聊天测试、钱包充值和使用日志都在控制台里完成。后续步骤会带你走一遍常用路径。',
    ),
    path: '/console',
    selectors: [
      '[data-onboarding="console-sidebar"]',
      '[data-onboarding="nav-console"]',
      '[data-onboarding="console-dashboard-panel"]',
    ],
  },
  {
    id: 'token',
    badge: t('令牌管理'),
    title: t('先创建并管理 API 令牌'),
    body: t(
      '客户端调用接口需要令牌。你可以在这里新建令牌、复制密钥，也可以设置额度、过期时间和可用模型。',
    ),
    path: '/console/token',
    selectors: [
      '[data-onboarding="create-token-button"]',
      '[data-onboarding="tokens-actions"]',
      '[data-onboarding="sidebar-token"]',
      '[data-onboarding="token-management-panel"]',
    ],
  },
  {
    id: 'test',
    badge: t('测速'),
    title: t('用测速确认令牌和模型是否可用'),
    body: t(
      '已有令牌后，可以点令牌行右侧的「测试」，选择模型检测是否能正常调用。测试会发起真实请求，可能产生少量消耗。',
    ),
    path: '/console/token',
    selectors: [
      '[data-onboarding="token-test-action"]',
      '[data-onboarding="tokens-actions"]',
      '[data-onboarding="token-management-panel"]',
    ],
  },
  {
    id: 'chat',
    badge: t('聊天'),
    title: t('聊天下拉可快速进入配置'),
    body: t(
      '令牌行里的「聊天」按钮和右侧下拉可以快速选择已配置的聊天客户端，用来验证真实对话效果。',
    ),
    path: '/console/token',
    selectors: [
      '[data-onboarding="token-chat-action"]',
      '[data-onboarding="token-chat-dropdown"]',
      '[data-onboarding="tokens-actions"]',
      '[data-onboarding="sidebar-chat-section"]',
    ],
  },
  {
    id: 'wallet',
    badge: t('钱包管理'),
    title: t('额度不足时到钱包充值'),
    body: t(
      '这里可以查看余额、充值额度、兑换充值码，也能查看充值记录。建议先跑通调用，再按需充值。',
    ),
    path: '/console/topup',
    selectors: [
      '[data-onboarding="wallet-management-panel"]',
      '[data-onboarding="sidebar-topup"]',
      '[data-onboarding="sidebar-personal-section"]',
    ],
  },
  {
    id: 'logs',
    badge: t('使用日志'),
    title: t('调用后到使用日志查结果'),
    body: t(
      '每次调用后，可以在这里查看模型、消耗、状态和错误信息。排查失败请求时，优先看这里。',
    ),
    path: '/console/log',
    selectors: [
      '[data-onboarding="usage-log-panel"]',
      '[data-onboarding="sidebar-log"]',
      '[data-onboarding="sidebar-console-section"]',
    ],
  },
];

const UserOnboarding = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const [userState] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [visible, setVisible] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const [targetRect, setTargetRect] = useState(null);
  const [dismissedInSession, setDismissedInSession] = useState(false);

  const user = userState?.user;
  const isRegularUser = Boolean(user?.id && Number(user.role || 0) < 10);
  const storageKey = useMemo(() => getStorageKey(user?.id), [user?.id]);
  const docsLink = statusState?.status?.docs_link || '';
  const steps = useMemo(() => buildSteps(t, docsLink), [t, docsLink]);
  const activeStep = steps[activeIndex] || steps[0];

  const resolveTarget = useCallback(() => {
    if (typeof window === 'undefined' || !activeStep?.selectors) return null;
    for (const selector of activeStep.selectors) {
      const element = document.querySelector(selector);
      if (isElementVisible(element)) return element;
    }
    return null;
  }, [activeStep]);

  const refreshTarget = useCallback(() => {
    const element = resolveTarget();
    setTargetRect(element ? element.getBoundingClientRect() : null);
    return element;
  }, [resolveTarget]);

  const closeTour = useCallback(
    (status) => {
      writeTourState(storageKey, status);
      setDismissedInSession(true);
      setVisible(false);
      setTargetRect(null);
    },
    [storageKey],
  );

  const restartTour = useCallback(() => {
    if (!isRegularUser) return;
    removeTourState(storageKey);
    setDismissedInSession(false);
    setActiveIndex(0);
    setVisible(true);
  }, [isRegularUser, storageKey]);

  useEffect(() => {
    const handleRestart = () => restartTour();
    window.addEventListener(RESTART_EVENT, handleRestart);
    return () => window.removeEventListener(RESTART_EVENT, handleRestart);
  }, [restartTour]);

  useEffect(() => {
    if (!isRegularUser || visible) return;
    if (dismissedInSession) return;
    if (!location.pathname.startsWith('/console')) return;
    if (readTourState(storageKey)) return;

    if (location.pathname.replace(/\/+$/, '') === '/console') {
      writeTourState(storageKey, 'dashboard');
      setDismissedInSession(true);
      return;
    }

    const timer = window.setTimeout(() => {
      setActiveIndex(0);
      setVisible(true);
    }, 500);

    return () => window.clearTimeout(timer);
  }, [
    dismissedInSession,
    isRegularUser,
    location.pathname,
    storageKey,
    visible,
  ]);

  useEffect(() => {
    if (!visible || !activeStep?.path) return;
    if (location.pathname !== activeStep.path) {
      navigate(activeStep.path);
    }
  }, [activeStep, location.pathname, navigate, visible]);

  useEffect(() => {
    if (!visible) return;

    const scrollTimer = window.setTimeout(() => {
      const element = refreshTarget();
      element?.scrollIntoView?.({
        block: 'center',
        inline: 'nearest',
        behavior: 'smooth',
      });
    }, 120);

    const timers = [0, 180, 420, 900].map((delay) =>
      window.setTimeout(refreshTarget, delay),
    );

    window.addEventListener('resize', refreshTarget);
    window.addEventListener('scroll', refreshTarget, true);

    return () => {
      window.clearTimeout(scrollTimer);
      timers.forEach((timer) => window.clearTimeout(timer));
      window.removeEventListener('resize', refreshTarget);
      window.removeEventListener('scroll', refreshTarget, true);
    };
  }, [activeIndex, location.pathname, refreshTarget, visible]);

  if (!visible || !isRegularUser || !activeStep) return null;

  const progressText = t('第 {{current}} / {{total}} 步', {
    current: activeIndex + 1,
    total: steps.length,
  });
  const isLastStep = activeIndex === steps.length - 1;
  const viewportWidth = window.innerWidth || 1024;
  const viewportHeight = window.innerHeight || 768;
  const cardWidth = Math.min(360, viewportWidth - 32);

  const cardStyle = (() => {
    if (!targetRect || viewportWidth < 768) {
      return {
        left: 16,
        right: 16,
        bottom: 18,
        width: 'auto',
      };
    }

    let left = targetRect.right + 22;
    if (left + cardWidth > viewportWidth - 16) {
      left = targetRect.left - cardWidth - 22;
    }
    left = clamp(left, 16, viewportWidth - cardWidth - 16);

    const top = clamp(
      targetRect.top + targetRect.height / 2 - 130,
      82,
      viewportHeight - 300,
    );

    return {
      left,
      top,
      width: cardWidth,
    };
  })();

  const spotlightStyle = targetRect
    ? {
        left: targetRect.left - 8,
        top: targetRect.top - 8,
        width: targetRect.width + 16,
        height: targetRect.height + 16,
      }
    : null;

  const cursorStyle = targetRect
    ? {
        left: clamp(
          targetRect.left + targetRect.width / 2 + 8,
          16,
          viewportWidth - 48,
        ),
        top: clamp(
          targetRect.top + targetRect.height / 2 + 8,
          80,
          viewportHeight - 48,
        ),
      }
    : null;

  return (
    <div className='newapi-onboarding-layer' aria-live='polite'>
      {spotlightStyle ? (
        <div className='newapi-onboarding-spotlight' style={spotlightStyle} />
      ) : (
        <div className='newapi-onboarding-backdrop' />
      )}

      {cursorStyle && (
        <div className='newapi-onboarding-cursor' style={cursorStyle}>
          <MousePointer2 size={28} fill='currentColor' />
        </div>
      )}

      <section
        className='newapi-onboarding-card'
        style={cardStyle}
        role='dialog'
        aria-modal='false'
        aria-label={activeStep.title}
      >
        <div className='newapi-onboarding-card-head'>
          <div>
            <Text className='newapi-onboarding-eyebrow'>
              {activeStep.badge}
            </Text>
            <Title heading={5} className='!my-1'>
              {activeStep.title}
            </Title>
          </div>
          <Button
            theme='borderless'
            type='tertiary'
            icon={<X size={16} />}
            aria-label={t('关闭新手引导')}
            onClick={() => closeTour('skipped')}
          />
        </div>

        <Text className='newapi-onboarding-body'>{activeStep.body}</Text>

        <div className='newapi-onboarding-progress'>
          <span>{progressText}</span>
          <span>{Math.round(((activeIndex + 1) / steps.length) * 100)}%</span>
        </div>
        <div className='newapi-onboarding-progress-track'>
          <div
            className='newapi-onboarding-progress-bar'
            style={{ width: `${((activeIndex + 1) / steps.length) * 100}%` }}
          />
        </div>

        <div className='newapi-onboarding-actions'>
          <Button
            theme='borderless'
            type='tertiary'
            onClick={() => closeTour('skipped')}
          >
            {t('跳过')}
          </Button>
          <div className='newapi-onboarding-actions-right'>
            <Button
              theme='outline'
              type='tertiary'
              disabled={activeIndex === 0}
              icon={<ArrowLeft size={15} />}
              onClick={() => setActiveIndex((index) => Math.max(index - 1, 0))}
            >
              {t('上一步')}
            </Button>
            <Button
              theme='solid'
              type='primary'
              icon={!isLastStep ? <ArrowRight size={15} /> : undefined}
              iconPosition='right'
              onClick={() => {
                if (isLastStep) {
                  closeTour('completed');
                  return;
                }
                setActiveIndex((index) =>
                  Math.min(index + 1, steps.length - 1),
                );
              }}
            >
              {isLastStep ? t('完成') : t('下一步')}
            </Button>
          </div>
        </div>
      </section>
    </div>
  );
};

export default UserOnboarding;
