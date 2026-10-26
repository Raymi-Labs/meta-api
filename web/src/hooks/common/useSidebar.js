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

import { useState, useEffect, useMemo, useContext, useRef } from 'react';
import { StatusContext } from '../../context/Status';
import { API } from '../../helpers/apiCore';

// 创建一个全局事件系统来同步所有useSidebar实例
const sidebarEventTarget = new EventTarget();
const SIDEBAR_REFRESH_EVENT = 'sidebar-refresh';

export const DEFAULT_ADMIN_CONFIG = {
  chat: {
    enabled: true,
    playground: true,
    imagePlayground: true,
    chat: true,
  },
  console: {
    enabled: true,
    detail: true,
    token: true,
    log: true,
    midjourney: true,
    task: true,
  },
  personal: {
    enabled: true,
    topup: true,
    pulse: true,
    personal: true,
  },
  admin: {
    enabled: true,
    upstreamAccounts: true,
    channel: true,
    models: true,
    deployment: true,
    redemption: true,
    user: true,
    subscription: true,
    pulseOps: true,
    desktopFeedback: true,
    setting: true,
  },
};

const deepClone = (value) => JSON.parse(JSON.stringify(value));

export const migrateSidebarConfig = (savedConfig) => {
  if (!savedConfig || typeof savedConfig !== 'object') return savedConfig;

  const migrated = deepClone(savedConfig);
  const admin = migrated.admin;
  if (admin && typeof admin === 'object') {
    if (
      !Object.prototype.hasOwnProperty.call(admin, 'upstreamAccounts') &&
      Object.prototype.hasOwnProperty.call(admin, 'profitBoard')
    ) {
      admin.upstreamAccounts = admin.profitBoard;
    }
    delete admin.profitBoard;
  }
  return migrated;
};

export const mergeAdminConfig = (savedConfig) => {
  const merged = deepClone(DEFAULT_ADMIN_CONFIG);
  const migratedConfig = migrateSidebarConfig(savedConfig);
  if (!migratedConfig || typeof migratedConfig !== 'object') return merged;

  for (const [sectionKey, sectionConfig] of Object.entries(migratedConfig)) {
    if (!sectionConfig || typeof sectionConfig !== 'object') continue;

    if (!merged[sectionKey]) {
      merged[sectionKey] = { ...sectionConfig };
      continue;
    }

    merged[sectionKey] = { ...merged[sectionKey], ...sectionConfig };
  }

  return merged;
};

export const useSidebar = () => {
  const [statusState] = useContext(StatusContext);
  const [userConfig, setUserConfig] = useState(null);
  // 默认 false：finalConfig 已基于默认 adminConfig 提供兜底，菜单可立即渲染，
  // 不需要骨架屏阻塞首屏。loadUserConfig 异步刷新 userConfig 后会自动 patch。
  const [loading, setLoading] = useState(false);
  const instanceIdRef = useRef(null);
  const hasLoadedOnceRef = useRef(false);

  if (!instanceIdRef.current) {
    const randomPart = Math.random().toString(16).slice(2);
    instanceIdRef.current = `sidebar-${Date.now()}-${randomPart}`;
  }

  // 获取管理员配置
  const adminConfig = useMemo(() => {
    if (statusState?.status?.SidebarModulesAdmin) {
      try {
        const config = JSON.parse(statusState.status.SidebarModulesAdmin);
        return mergeAdminConfig(config);
      } catch (error) {
        return mergeAdminConfig(null);
      }
    }
    return mergeAdminConfig(null);
  }, [statusState?.status?.SidebarModulesAdmin]);

  // 加载用户配置的通用方法
  const loadUserConfig = async ({ withLoading } = {}) => {
    // 默认不显示骨架屏：finalConfig 已有基于 adminConfig 的兜底，
    // user/self 在后台异步刷新即可。仅当显式传 withLoading:true 时（如手动刷新）才展示。
    const shouldShowLoader = withLoading === true;

    try {
      if (shouldShowLoader) {
        setLoading(true);
      }

      const res = await API.get('/api/user/self');
      if (res.data.success && res.data.data.sidebar_modules) {
        let config;
        // 检查sidebar_modules是字符串还是对象
        if (typeof res.data.data.sidebar_modules === 'string') {
          config = JSON.parse(res.data.data.sidebar_modules);
        } else {
          config = res.data.data.sidebar_modules;
        }
        setUserConfig(migrateSidebarConfig(config));
      } else {
        // 当用户没有配置时，生成一个基于管理员配置的默认用户配置
        // 这样可以确保权限控制正确生效
        const defaultUserConfig = {};
        Object.keys(adminConfig).forEach((sectionKey) => {
          if (adminConfig[sectionKey]?.enabled) {
            defaultUserConfig[sectionKey] = { enabled: true };
            // 为每个管理员允许的模块设置默认值为true
            Object.keys(adminConfig[sectionKey]).forEach((moduleKey) => {
              if (
                moduleKey !== 'enabled' &&
                adminConfig[sectionKey][moduleKey]
              ) {
                defaultUserConfig[sectionKey][moduleKey] = true;
              }
            });
          }
        });
        setUserConfig(defaultUserConfig);
      }
    } catch (error) {
      // 出错时也生成默认配置，而不是设置为空对象
      const defaultUserConfig = {};
      Object.keys(adminConfig).forEach((sectionKey) => {
        if (adminConfig[sectionKey]?.enabled) {
          defaultUserConfig[sectionKey] = { enabled: true };
          Object.keys(adminConfig[sectionKey]).forEach((moduleKey) => {
            if (moduleKey !== 'enabled' && adminConfig[sectionKey][moduleKey]) {
              defaultUserConfig[sectionKey][moduleKey] = true;
            }
          });
        }
      });
      setUserConfig(defaultUserConfig);
    } finally {
      if (shouldShowLoader) {
        setLoading(false);
      }
      hasLoadedOnceRef.current = true;
    }
  };

  // 刷新用户配置的方法（供外部调用）
  const refreshUserConfig = async () => {
    if (Object.keys(adminConfig).length > 0) {
      await loadUserConfig({ withLoading: false });
    }

    // 触发全局刷新事件，通知所有useSidebar实例更新
    sidebarEventTarget.dispatchEvent(
      new CustomEvent(SIDEBAR_REFRESH_EVENT, {
        detail: { sourceId: instanceIdRef.current, skipLoader: true },
      }),
    );
  };

  // 加载用户配置
  useEffect(() => {
    // mount 时立刻触发：adminConfig 永远有默认兜底（mergeAdminConfig(null) 返回
    // DEFAULT_ADMIN_CONFIG），不必等 /api/status 返回，避免侧边栏被串行链路阻塞。
    loadUserConfig();
    // 仅 mount 触发一次；adminConfig 因 statusState 更新引起的二次执行无意义。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 监听全局刷新事件
  useEffect(() => {
    const handleRefresh = (event) => {
      if (event?.detail?.sourceId === instanceIdRef.current) {
        return;
      }

      if (Object.keys(adminConfig).length > 0) {
        loadUserConfig({
          withLoading: event?.detail?.skipLoader ? false : undefined,
        });
      }
    };

    sidebarEventTarget.addEventListener(SIDEBAR_REFRESH_EVENT, handleRefresh);

    return () => {
      sidebarEventTarget.removeEventListener(
        SIDEBAR_REFRESH_EVENT,
        handleRefresh,
      );
    };
  }, [adminConfig]);

  // 计算最终的显示配置
  const finalConfig = useMemo(() => {
    const result = {};

    // 确保adminConfig已加载
    if (!adminConfig || Object.keys(adminConfig).length === 0) {
      return result;
    }

    // userConfig 未加载时不再 return 空对象；下面的遍历逻辑会把 userSection 视为
    // undefined → 默认放行（与 useSection ? ... : true 的兜底分支一致），
    // 这样侧边栏可以基于 adminConfig 立即渲染出默认菜单，避免首屏白屏。
    // 待 user/self 返回后 userConfig 更新会触发 finalConfig 重新计算并 patch 差异。

    // 遍历所有区域
    Object.keys(adminConfig).forEach((sectionKey) => {
      const adminSection = adminConfig[sectionKey];
      // userConfig 可能为 null（首次 mount 时 user/self 尚未返回），用可选链兜底。
      const userSection = userConfig?.[sectionKey];

      // 如果管理员禁用了整个区域，则该区域不显示
      if (!adminSection?.enabled) {
        result[sectionKey] = { enabled: false };
        return;
      }

      // 区域级别：用户可以选择隐藏管理员允许的区域
      // 当userSection存在时检查enabled状态，否则默认为true
      const sectionEnabled = userSection ? userSection.enabled !== false : true;
      result[sectionKey] = { enabled: sectionEnabled };

      // 功能级别：只有管理员和用户都允许的功能才显示
      Object.keys(adminSection).forEach((moduleKey) => {
        if (moduleKey === 'enabled') return;

        const adminAllowed = adminSection[moduleKey];
        // 当userSection存在时检查模块状态，否则默认为true
        const userAllowed = userSection
          ? userSection[moduleKey] !== false
          : true;

        result[sectionKey][moduleKey] =
          adminAllowed && userAllowed && sectionEnabled;
      });
    });

    return result;
  }, [adminConfig, userConfig]);

  // 检查特定功能是否应该显示
  const isModuleVisible = (sectionKey, moduleKey = null) => {
    if (moduleKey) {
      return finalConfig[sectionKey]?.[moduleKey] === true;
    } else {
      return finalConfig[sectionKey]?.enabled === true;
    }
  };

  // 检查区域是否有任何可见的功能
  const hasSectionVisibleModules = (sectionKey) => {
    const section = finalConfig[sectionKey];
    if (!section?.enabled) return false;

    return Object.keys(section).some(
      (key) => key !== 'enabled' && section[key] === true,
    );
  };

  // 获取区域的可见功能列表
  const getVisibleModules = (sectionKey) => {
    const section = finalConfig[sectionKey];
    if (!section?.enabled) return [];

    return Object.keys(section).filter(
      (key) => key !== 'enabled' && section[key] === true,
    );
  };

  return {
    loading,
    adminConfig,
    userConfig,
    finalConfig,
    isModuleVisible,
    hasSectionVisibleModules,
    getVisibleModules,
    refreshUserConfig,
  };
};
