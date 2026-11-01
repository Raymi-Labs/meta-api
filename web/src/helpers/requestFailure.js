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

// Only public troubleshooting fields enter the copyable report, even for admins.
export const getRequestFailureDetails = (record, t) => {
  if (record?.type !== 5) return [];
  let other = {};
  try {
    other = JSON.parse(record.other || '{}') || {};
  } catch {
    // Legacy logs may not have structured diagnostics.
  }
  const details = [
    {
      key: t('失败原因'),
      value: t(other.failure_reason || '请求处理失败'),
    },
  ];
  if (other.request_failure !== true) {
    details.push({
      key: t('响应说明'),
      value: t('历史错误记录可能来自重试，不能据此判断最终调用结果。'),
    });
  }
  if (other.http_status) {
    details.push({ key: t('HTTP 状态码'), value: String(other.http_status) });
  }
  if (other.status_code) {
    details.push({ key: t('失败状态码'), value: String(other.status_code) });
  }
  if (other.http_status === 200) {
    details.push({
      key: t('响应说明'),
      value: t('HTTP 200 仅表示响应已开始，流内错误仍代表调用失败。'),
    });
  }
  if (other.error_code) {
    details.push({ key: t('错误码'), value: String(other.error_code) });
  }
  if (other.request_path) {
    details.push({ key: t('请求路径'), value: other.request_path });
  }
  if (Number.isFinite(other.latency_ms)) {
    details.push({ key: t('总耗时'), value: `${other.latency_ms} ms` });
  }
  if (other.retry_after_seconds > 0) {
    details.push({
      key: 'Retry-After',
      value: `${other.retry_after_seconds} s`,
    });
  }
  details.push({
    key: t('处理建议'),
    value: t(
      other.failure_hint || '请稍后重试；仍失败时复制排查信息并联系管理员。',
    ),
  });
  details.push({
    key: t('费用说明'),
    value: t('费用请以相同 Request ID 下的消费或退款记录为准。'),
  });
  return details;
};

export const getRequestLogUserLabel = (record) =>
  record.username || (record.user_id > 0 ? `ID: ${record.user_id}` : '-');

export const buildRequestFailureReport = (record, t, isAdmin = false) => {
  const base = [
    ['Request ID', record.request_id || '-'],
    [t('时间'), new Date(record.created_at * 1000).toISOString()],
    [t('模型名称'), record.model_name || '-'],
  ];
  if (isAdmin) {
    base.push(
      [t('用户'), getRequestLogUserLabel(record)],
      [`${t('用户')} ID`, record.user_id || '-'],
      [t('令牌名称'), record.token_name || '-'],
      [`${t('令牌')} ID`, record.token_id || '-'],
    );
  }
  return [
    ...base,
    ...getRequestFailureDetails(record, t).map(({ key, value }) => [
      key,
      value,
    ]),
  ]
    .map(([key, value]) => `${key}: ${value}`)
    .join('\n');
};
