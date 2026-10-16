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

import {
  STORAGE_KEYS,
  DEFAULT_CONFIG,
} from '../../constants/playground.constants';

export const getPlaygroundOwner = () => {
  try {
    const id = JSON.parse(localStorage.getItem('user') || 'null')?.id;
    return Number.isSafeInteger(id) && id > 0 ? String(id) : null;
  } catch {
    return null;
  }
};

// A delayed callback must never write an old account's data into the new account.
const accountStorage = (owner) => {
  const active = () => owner !== null && owner === getPlaygroundOwner();
  const key = (name) => `${name}:user:${owner}`;
  return {
    getItem: (name) => (active() ? localStorage.getItem(key(name)) : null),
    setItem: (name, value) => {
      if (active()) localStorage.setItem(key(name), value);
    },
    removeItem: (name) => {
      if (active()) localStorage.removeItem(key(name));
    },
  };
};

const clearLegacyStorage = () => {
  localStorage.removeItem(STORAGE_KEYS.CONFIG);
  localStorage.removeItem(STORAGE_KEYS.MESSAGES);
};

/**
 * 保存配置到 localStorage
 * @param {Object} config - 要保存的配置对象
 */
export const saveConfig = (config, owner = getPlaygroundOwner()) => {
  try {
    const configToSave = {
      ...config,
      timestamp: new Date().toISOString(),
    };
    accountStorage(owner).setItem(
      STORAGE_KEYS.CONFIG,
      JSON.stringify(configToSave),
    );
  } catch (error) {
    console.error('保存配置失败:', error);
  }
};

/**
 * 保存消息到 localStorage
 * @param {Array} messages - 要保存的消息数组
 */
export const saveMessages = (messages, owner = getPlaygroundOwner()) => {
  try {
    const messagesToSave = {
      messages,
      timestamp: new Date().toISOString(),
    };
    accountStorage(owner).setItem(
      STORAGE_KEYS.MESSAGES,
      JSON.stringify(messagesToSave),
    );
  } catch (error) {
    console.error('保存消息失败:', error);
  }
};

/**
 * 从 localStorage 加载配置
 * @returns {Object} 配置对象，如果不存在则返回默认配置
 */
export const loadConfig = (owner = getPlaygroundOwner()) => {
  try {
    clearLegacyStorage();
    const savedConfig = accountStorage(owner).getItem(STORAGE_KEYS.CONFIG);
    if (savedConfig) {
      const parsedConfig = JSON.parse(savedConfig);

      const mergedConfig = {
        inputs: {
          ...DEFAULT_CONFIG.inputs,
          ...parsedConfig.inputs,
        },
        parameterEnabled: {
          ...DEFAULT_CONFIG.parameterEnabled,
          ...parsedConfig.parameterEnabled,
        },
        showDebugPanel:
          parsedConfig.showDebugPanel || DEFAULT_CONFIG.showDebugPanel,
        customRequestMode:
          parsedConfig.customRequestMode || DEFAULT_CONFIG.customRequestMode,
        customRequestBody:
          parsedConfig.customRequestBody || DEFAULT_CONFIG.customRequestBody,
      };

      return mergedConfig;
    }
  } catch (error) {
    console.error('加载配置失败:', error);
  }

  return DEFAULT_CONFIG;
};

/**
 * 从 localStorage 加载消息
 * @returns {Array} 消息数组，如果不存在则返回 null
 */
export const loadMessages = (owner = getPlaygroundOwner()) => {
  try {
    const savedMessages = accountStorage(owner).getItem(STORAGE_KEYS.MESSAGES);
    if (savedMessages) {
      const parsedMessages = JSON.parse(savedMessages);
      return parsedMessages.messages || null;
    }
  } catch (error) {
    console.error('加载消息失败:', error);
  }

  return null;
};

/**
 * 清除保存的配置
 */
export const clearConfig = (owner = getPlaygroundOwner()) => {
  try {
    accountStorage(owner).removeItem(STORAGE_KEYS.CONFIG);
    accountStorage(owner).removeItem(STORAGE_KEYS.MESSAGES); // 同时清除消息
  } catch (error) {
    console.error('清除配置失败:', error);
  }
};

/**
 * 清除保存的消息
 */
export const clearMessages = (owner = getPlaygroundOwner()) => {
  try {
    accountStorage(owner).removeItem(STORAGE_KEYS.MESSAGES);
  } catch (error) {
    console.error('清除消息失败:', error);
  }
};

/**
 * 检查是否有保存的配置
 * @returns {boolean} 是否存在保存的配置
 */
export const hasStoredConfig = (owner = getPlaygroundOwner()) => {
  try {
    return accountStorage(owner).getItem(STORAGE_KEYS.CONFIG) !== null;
  } catch (error) {
    console.error('检查配置失败:', error);
    return false;
  }
};

/**
 * 获取配置的最后保存时间
 * @returns {string|null} 最后保存时间的 ISO 字符串
 */
export const getConfigTimestamp = (owner = getPlaygroundOwner()) => {
  try {
    const savedConfig = accountStorage(owner).getItem(STORAGE_KEYS.CONFIG);
    if (savedConfig) {
      const parsedConfig = JSON.parse(savedConfig);
      return parsedConfig.timestamp || null;
    }
  } catch (error) {
    console.error('获取配置时间戳失败:', error);
  }
  return null;
};

/**
 * 导出配置为 JSON 文件（包含消息）
 * @param {Object} config - 要导出的配置
 * @param {Array} messages - 要导出的消息
 */
export const exportConfig = (config, messages = null) => {
  try {
    const configToExport = {
      ...config,
      messages: messages || loadMessages(), // 包含消息数据
      exportTime: new Date().toISOString(),
      version: '1.0',
    };

    const dataStr = JSON.stringify(configToExport, null, 2);
    const dataBlob = new Blob([dataStr], { type: 'application/json' });

    const link = document.createElement('a');
    link.href = URL.createObjectURL(dataBlob);
    link.download = `playground-config-${new Date().toISOString().split('T')[0]}.json`;
    link.click();

    URL.revokeObjectURL(link.href);
  } catch (error) {
    console.error('导出配置失败:', error);
  }
};

/**
 * 从文件导入配置（包含消息）
 * @param {File} file - 包含配置的 JSON 文件
 * @returns {Promise<Object>} 导入的配置对象
 */
export const importConfig = (file) => {
  const owner = getPlaygroundOwner();
  return new Promise((resolve, reject) => {
    try {
      const reader = new FileReader();
      reader.onload = (e) => {
        try {
          if (!owner || owner !== getPlaygroundOwner())
            throw new Error('登录状态已变化，请刷新页面');
          const importedConfig = JSON.parse(e.target.result);

          if (importedConfig.inputs && importedConfig.parameterEnabled) {
            resolve(importedConfig);
          } else {
            reject(new Error('配置文件格式无效'));
          }
        } catch (parseError) {
          reject(new Error('解析配置文件失败: ' + parseError.message));
        }
      };
      reader.onerror = () => reject(new Error('读取文件失败'));
      reader.readAsText(file);
    } catch (error) {
      reject(new Error('导入配置失败: ' + error.message));
    }
  });
};
