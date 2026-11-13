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

import { API } from './apiCore';

// 企业接口统一封装：成功返回 data，失败抛出带后端文案的 Error，由调用方决定怎么提示。
export async function enterpriseRequest(method, url, data, params) {
  const res = await API.request({
    method,
    url,
    data,
    params,
    skipErrorHandler: true,
    skipGlobalLoading: true,
  });
  if (!res.data?.success) {
    throw new Error(res.data?.message || '操作失败');
  }
  return res.data.data;
}

export const ENTERPRISE_ROLE_LABEL = {
  owner: '所有者',
  admin: '管理员',
  member: '成员',
};

export const ENTERPRISE_REQUEST_STATUS = {
  pending: '待审批',
  approved: '已批准',
  rejected: '已拒绝',
  cancelled: '已撤销',
  expired: '已过期',
};

export const ENTERPRISE_REQUEST_STATUS_COLOR = {
  pending: 'orange',
  approved: 'green',
  rejected: 'red',
  cancelled: 'grey',
  expired: 'grey',
};

export const formatEnterpriseDate = (timestamp) =>
  timestamp ? new Date(timestamp * 1000).toLocaleDateString() : '—';
