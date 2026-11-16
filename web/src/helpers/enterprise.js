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

// 企业后台的时间统一按企业时区展示，和账单、导出的 CSV 保持一致。
export const formatEnterpriseTime = (timestamp, timezone) => {
  if (!timestamp || timestamp <= 0) return '—';
  const date = new Date(timestamp * 1000);
  try {
    return date.toLocaleString(undefined, {
      timeZone: timezone || undefined,
      hour12: false,
    });
  } catch {
    return date.toLocaleString();
  }
};

export const ENTERPRISE_AUDIT_ACTION = {
  'budget.allocate': '分配额度',
  'budget.reclaim': '回收额度',
  'enterprise.create': '创建企业',
  'enterprise.settings': '修改设置',
  'enterprise.status': '状态变更',
  'enterprise.transfer': '转移所有权',
  'enterprise.update': '修改设置',
  'invitation.create': '邀请成员',
  'invitation.revoke': '撤销邀请',
  'member.disable': '停用成员',
  'member.leave': '成员退出',
  'member.join': '成员加入',
  'member.models': '成员模型',
  'member.role': '调整角色',
  'request.approved': '批准申请',
  'request.rejected': '拒绝申请',
  'reservation.resolve': '人工结算',
  'token.revoke': '撤销令牌',
  'wallet.credit': '入账',
  'wallet.adjust': '调账',
  'enterprise.archive': '归档企业',
};

export const ENTERPRISE_LEDGER_KIND = {
  credit: '入账',
  adjust: '调账',
  allocate: '分配',
  reclaim: '回收',
  invite_grant: '邀请',
  request_grant: '申请',
  member_sweep: '额度归并',
};
