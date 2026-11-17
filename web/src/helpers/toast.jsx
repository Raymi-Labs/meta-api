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

import { clearStoredUser } from './storage';
import { Toast } from '@douyinfe/semi-ui';
import { toastConstants } from '../constants/toast.constants';
import React from 'react';
import { MOBILE_BREAKPOINT } from '../hooks/common/useIsMobile';
import i18n from '../i18n/i18n';
import RichContent from '../components/common/RichContent';

const HTMLToastContent = ({ htmlContent }) => {
  return <RichContent content={htmlContent} mode='html' />;
};
export default HTMLToastContent;

let showErrorOptions = { autoClose: toastConstants.ERROR_TIMEOUT };
let showWarningOptions = { autoClose: toastConstants.WARNING_TIMEOUT };
let showSuccessOptions = { autoClose: toastConstants.SUCCESS_TIMEOUT };
let showInfoOptions = { autoClose: toastConstants.INFO_TIMEOUT };
let showNoticeOptions = { autoClose: false };

const isMobileScreen = window.matchMedia(
  `(max-width: ${MOBILE_BREAKPOINT - 1}px)`,
).matches;
if (isMobileScreen) {
  showErrorOptions.position = 'top-center';
  showSuccessOptions.position = 'top-center';
  showInfoOptions.position = 'top-center';
  showNoticeOptions.position = 'top-center';
}

export function showError(error) {
  console.error(error);
  if (error.message) {
    if (error.name === 'AxiosError') {
      switch (error.response.status) {
        case 401:
          clearStoredUser();
          window.location.href = '/login?expired=true';
          break;
        case 429:
          Toast.error(i18n.t('错误：请求次数过多，请稍后再试！'));
          break;
        case 500:
          Toast.error(i18n.t('错误：服务器内部错误，请联系管理员！'));
          break;
        case 405:
          Toast.info(i18n.t('本站仅作演示之用，无服务端！'));
          break;
        default:
          Toast.error(i18n.t('错误：{{message}}', { message: error.message }));
      }
      return;
    }
    Toast.error(i18n.t('错误：{{message}}', { message: error.message }));
  } else {
    Toast.error(i18n.t('错误：{{message}}', { message: error }));
  }
}

export function showWarning(message) {
  Toast.warning(message);
}

export function showSuccess(message) {
  Toast.success(message);
}

export function showInfo(message) {
  Toast.info(message);
}

export function showNotice(message, isHTML = false) {
  if (isHTML) {
    Toast.info({
      content: <HTMLToastContent htmlContent={message} />,
      duration: 0,
      ...showNoticeOptions,
    });
  } else {
    Toast.info(message);
  }
}
