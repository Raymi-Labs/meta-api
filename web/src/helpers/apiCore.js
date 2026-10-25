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

import axios from 'axios';
import { getUserIdFromLocalStorage } from './storage';
import { beginGlobalLoading, endGlobalLoading } from './globalLoading';
import { getSiteLanguageSnapshot } from './siteLanguage';

const GLOBAL_LOADING_TRACKED_KEY = '__globalLoadingTracked';
const REQUEST_LANGUAGE_KEY = '__siteLanguage';

const createAPIHeaders = () => ({
  'New-API-User': getUserIdFromLocalStorage(),
  'Cache-Control': 'no-store',
});

const shouldTrackGlobalLoading = (config = {}) =>
  config.skipGlobalLoading !== true;

const markGlobalLoadingStarted = (config = {}) => {
  if (!shouldTrackGlobalLoading(config)) {
    return config;
  }
  config[GLOBAL_LOADING_TRACKED_KEY] = true;
  beginGlobalLoading();
  return config;
};

const markGlobalLoadingFinished = (config) => {
  if (!config?.[GLOBAL_LOADING_TRACKED_KEY]) {
    return;
  }
  delete config[GLOBAL_LOADING_TRACKED_KEY];
  endGlobalLoading();
};

function patchAPIInstance(instance) {
  const originalGet = instance.get.bind(instance);
  const inFlightGetRequests = new Map();

  const genKey = (url, config = {}) => {
    const params = config.params ? JSON.stringify(config.params) : '{}';
    return `${url}?${params}|${config[REQUEST_LANGUAGE_KEY]}`;
  };

  instance.get = (url, config = {}) => {
    config = {
      ...config,
      [REQUEST_LANGUAGE_KEY]: getSiteLanguageSnapshot().language,
    };
    if (config?.disableDuplicate) {
      return originalGet(url, config);
    }

    const key = genKey(url, config);
    if (inFlightGetRequests.has(key)) {
      return inFlightGetRequests.get(key);
    }

    const reqPromise = originalGet(url, config).finally(() => {
      inFlightGetRequests.delete(key);
    });

    inFlightGetRequests.set(key, reqPromise);
    return reqPromise;
  };
}

function attachAPIInterceptors(instance) {
  instance.interceptors.request.use(
    (config) => {
      config.headers = config.headers || {};
      config.headers['Accept-Language'] =
        config[REQUEST_LANGUAGE_KEY] || getSiteLanguageSnapshot().language;
      return markGlobalLoadingStarted(config);
    },
    (error) => {
      markGlobalLoadingFinished(error?.config);
      return Promise.reject(error);
    },
  );

  instance.interceptors.response.use(
    (response) => {
      markGlobalLoadingFinished(response.config);
      return response;
    },
    (error) => {
      markGlobalLoadingFinished(error?.config);
      if (error.config && error.config.skipErrorHandler) {
        return Promise.reject(error);
      }
      import('./toast')
        .then(({ showError }) => showError(error))
        .catch(() => console.error(error));
      return Promise.reject(error);
    },
  );
}

function createAPIInstance() {
  const instance = axios.create({
    baseURL: import.meta.env.VITE_REACT_APP_SERVER_URL
      ? import.meta.env.VITE_REACT_APP_SERVER_URL
      : '',
    headers: createAPIHeaders(),
  });

  patchAPIInstance(instance);
  attachAPIInterceptors(instance);
  return instance;
}

export let API = createAPIInstance();

export function updateAPI() {
  API = createAPIInstance();
}
