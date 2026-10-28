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
import React, { useEffect, useMemo, useState, useContext, useRef } from 'react';
import {
  Avatar,
  Badge,
  Button,
  Card,
  Checkbox,
  Collapsible,
  DatePicker,
  Empty,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  TextArea,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { IconFilter, IconSearch } from '@douyinfe/semi-icons';
import { Coins } from 'lucide-react';
import {
  API,
  renderQuota,
  renderQuotaWithAmount,
  stringToColor,
  timestamp2string,
} from '../../../helpers';
import { isAdmin } from '../../../helpers/utils';
import { getQuotaPerUnit } from '../../../helpers/quota';
import {
  openInvoicePdf,
  renderInvoicePdf,
  prepareInvoiceAttachmentFiles,
} from '../../../helpers/invoicePdf';
import {
  PAYMENT_STATUS_CONFIG,
  resolveOrderType,
  formatPaymentRecordMoney as formatMoney,
} from '../../../helpers/paymentRecordView';
import { UserContext } from '../../../context/User';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import PaymentRiskCaseDetailModal from './PaymentRiskCaseDetailModal';
import { EPAY_CRYPTO_METHODS } from '../../../helpers/epayMethods';
import i18n from '../../../i18n/i18n';
import { createInvoiceTemplateTranslator } from '../../../helpers/invoiceTemplateLanguage';

const invoiceText = createInvoiceTemplateTranslator((source) => i18n.t(source));

const { Text } = Typography;

const STATUS_CONFIG = PAYMENT_STATUS_CONFIG;

const PAYMENT_METHOD_MAP = {
  stripe: 'Stripe',
  creem: 'Creem',
  alipay: '支付宝',
  wxpay: '微信',
  wallet: '钱包',
  bank_transfer: '银行转账',
  ...Object.fromEntries(
    EPAY_CRYPTO_METHODS.map(({ type, name }) => [type, name]),
  ),
};

const RISK_STATUS_CONFIG = {
  open: { color: 'red', label: '待处理' },
  confirmed: { color: 'green', label: '已确认' },
  reversed: { color: 'orange', label: '已回退' },
  voided: { color: 'grey', label: '已作废' },
};

const RISK_REASON_MAP = {
  manual_review: '人工标记',
  order_not_found: '订单不存在',
  order_status_invalid: '订单状态异常',
  payment_method_mismatch: '支付方式不匹配',
  amount_mismatch: '支付金额不匹配',
  unsupported_order_type: '订单类型不支持',
};

const RECORD_TYPE_MAP = {
  topup: '在线充值',
  subscription: '订阅套餐',
  sellable_token_purchase: '钱包购买',
  manual_transfer: '转账',
};

const EMPTY_FILTERS = {
  keyword: '',
  username: '',
  status: '',
  paymentMethod: '',
};

const EMPTY_RISK_FILTERS = {
  keyword: '',
  username: '',
  status: 'open',
  recordType: '',
  reason: '',
};

const EMPTY_WITHDRAWAL_FILTERS = {
  username: '',
  status: '',
};

const STATUS_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '待支付', value: 'pending' },
  { label: '成功', value: 'success' },
  { label: '已过期', value: 'expired' },
  { label: '已取消', value: 'cancelled' },
];

const PAYMENT_OPTIONS = [
  { label: '全部支付方式', value: '' },
  { label: '钱包', value: 'wallet' },
  { label: '微信', value: 'wxpay' },
  { label: '支付宝', value: 'alipay' },
  ...EPAY_CRYPTO_METHODS.map(({ name, type }) => ({
    label: name,
    value: type,
  })),
  { label: 'Stripe', value: 'stripe' },
  { label: 'Creem', value: 'creem' },
];

const DASHBOARD_PRESET_OPTIONS = [
  { key: 'today', label: '今天' },
  { key: 'week', label: '近7天' },
  { key: 'month', label: '近30天' },
];

const DASHBOARD_RANK_LIMIT_OPTIONS = [
  { label: 'Top 10', value: 10 },
  { label: 'Top 20', value: 20 },
  { label: 'Top 50', value: 50 },
];

const RISK_STATUS_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '待处理', value: 'open' },
  { label: '已确认', value: 'confirmed' },
  { label: '已回退', value: 'reversed' },
  { label: '已作废', value: 'voided' },
];

const WITHDRAWAL_STATUS_CONFIG = {
  pending: { color: 'orange', label: '待审核' },
  approved: { color: 'green', label: '已通过' },
  rejected: { color: 'red', label: '已驳回' },
};

const WITHDRAWAL_STATUS_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '待审核', value: 'pending' },
  { label: '已通过', value: 'approved' },
  { label: '已驳回', value: 'rejected' },
];

const INVOICE_STATUS_CONFIG = {
  pending: { color: 'orange', label: '申请中' },
  invoiced: { color: 'green', label: '已开票' },
  rejected: { color: 'red', label: '已驳回' },
};

const INVOICE_SEND_STATUS_CONFIG = {
  pending: { color: 'orange', label: '待发送' },
  sent: { color: 'green', label: '已发送' },
  failed: { color: 'red', label: '发送失败' },
};

const INVOICE_STATUS_OPTIONS = [
  { label: '全部状态', value: '' },
  { label: '待审核', value: 'pending' },
  { label: '已开票', value: 'invoiced' },
  { label: '已驳回', value: 'rejected' },
];

const INVOICE_MANUAL_PAYEE_NAME = '上海曜算智能科技有限公司';
const MAX_MANUAL_INVOICE_ROWS = 50;

const EMPTY_INVOICE_FILTERS = {
  username: '',
  status: '',
};

let invoiceRowSequence = 0;

function createInvoiceRowKey(prefix) {
  invoiceRowSequence += 1;
  return `${prefix}-${Date.now()}-${invoiceRowSequence}`;
}

function createManualInvoiceTransaction() {
  return {
    key: createInvoiceRowKey('transfer'),
    tradeNo: '',
    payerName: '',
    payeeName: INVOICE_MANUAL_PAYEE_NAME,
    transferBankName: '',
    money: '',
    paidAt: new Date(),
    remark: '',
  };
}

function createEmptyInvoiceForm() {
  return {
    sourceType: 'system_order',
    invoiceType: 'normal',
    titleType: 'company',
    title: '',
    taxNumber: '',
    registeredAddress: '',
    registeredPhone: '',
    bankName: '',
    bankAccount: '',
    email: '',
    phone: '',
    remark: '',
    needDetailBill: true,
    needServiceConfirmation: false,
    manualTransactions: [createManualInvoiceTransaction()],
  };
}

const RISK_RECORD_TYPE_OPTIONS = [
  { label: '全部订单类型', value: '' },
  { label: '充值订单', value: 'topup' },
  { label: '订阅订单', value: 'subscription' },
];

const RISK_REASON_OPTIONS = [
  { label: '全部原因', value: '' },
  { label: '人工标记', value: 'manual_review' },
  { label: '订单不存在', value: 'order_not_found' },
  { label: '订单状态异常', value: 'order_status_invalid' },
  { label: '支付方式不匹配', value: 'payment_method_mismatch' },
  { label: '支付金额不匹配', value: 'amount_mismatch' },
];

function formatMoneyAmounts(amounts, fallbackMoney, fallbackCurrency = 'CNY') {
  if (amounts && typeof amounts === 'object') {
    const entries = Object.entries(amounts)
      .map(([currency, value]) => [currency, Number(value)])
      .filter(([, value]) => Number.isFinite(value) && Math.abs(value) > 1e-9)
      .sort(([left], [right]) => {
        const order = { CNY: 0, USD: 1, EUR: 2, GBP: 3 };
        return (order[left] ?? 99) - (order[right] ?? 99);
      });
    if (entries.length > 0) {
      return entries
        .map(([currency, value]) => formatMoney(value, currency))
        .join(' + ');
    }
    return '-';
  }
  return formatMoney(fallbackMoney, fallbackCurrency);
}

function formatAmountCents(cents) {
  return `¥${(Number(cents || 0) / 100).toFixed(2)}`;
}

function getInvoiceOrderKey(record) {
  if (!record?.id) {
    return '';
  }
  return `${resolveOrderType(record)}-${record.id}`;
}

function getInvoiceItemKey(item) {
  if (item?.id) {
    return String(item.id);
  }
  if (
    item?.source_type === 'manual_transfer' ||
    item?.order_type === 'manual_transfer'
  ) {
    return `manual_transfer-${item?.trade_no || item?.complete_time || '-'}`;
  }
  if (!item?.order_id) {
    return '';
  }
  return `${item.order_type}-${item.order_id}`;
}

function getInvoiceOrderTypeLabel(orderType) {
  return RECORD_TYPE_MAP[orderType]
    ? i18n.t(RECORD_TYPE_MAP[orderType])
    : orderType || '-';
}

function getInvoiceTypeLabel(invoiceType) {
  return invoiceType === 'special' ? invoiceText`专票` : invoiceText`普票`;
}

function buildInvoiceTitleCopyText(record) {
  return [
    invoiceText`发票类型：${getInvoiceTypeLabel(record?.invoice_type)}`,
    invoiceText`单位名称：${record?.title || '-'}`,
    invoiceText`税号：${record?.tax_number || '-'}`,
    invoiceText`申请金额：${formatMoney(record?.total_money)}`,
  ].join('\n');
}

function getInvoicePaymentLabel(paymentMethod) {
  return PAYMENT_METHOD_MAP[paymentMethod]
    ? i18n.t(PAYMENT_METHOD_MAP[paymentMethod])
    : paymentMethod || '-';
}

function getInvoicePaymentLabelForItem(item) {
  const label = getInvoicePaymentLabel(item?.payment_method);
  if (item?.payment_method === 'bank_transfer' && item?.transfer_bank_name) {
    return `${label}（${item.transfer_bank_name}）`;
  }
  return label;
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function displayValue(value) {
  return value === undefined || value === null || value === '' ? '-' : value;
}

function formatInvoiceTime(value) {
  return value
    ? new Intl.DateTimeFormat(i18n.resolvedLanguage || i18n.language, {
        timeZone: 'Asia/Shanghai',
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hourCycle: 'h23',
      }).format(new Date(value * 1000))
    : '-';
}

function formatInvoiceCode(item) {
  if (item?.trade_no) {
    return item.trade_no;
  }
  if (
    item?.source_type === 'manual_transfer' ||
    item?.order_type === 'manual_transfer' ||
    !item?.order_id
  ) {
    return '-';
  }
  return `${item?.order_type || '-'}-${item.order_id}`;
}

function formatInvoiceBusinessId(item) {
  if (
    item?.source_type === 'manual_transfer' ||
    item?.order_type === 'manual_transfer' ||
    !item?.order_id
  ) {
    return '-';
  }
  return item.order_id;
}

function isManualInvoice(invoice) {
  if (invoice?.source_type === 'manual_transfer') {
    return true;
  }
  return (invoice?.items || []).some(
    (item) =>
      item?.source_type === 'manual_transfer' ||
      item?.order_type === 'manual_transfer',
  );
}

function toUnixTimestamp(value) {
  if (!value) return 0;
  const timestamp = value instanceof Date ? value.getTime() : Date.parse(value);
  if (!Number.isFinite(timestamp)) return 0;
  return Math.floor(timestamp / 1000);
}

function hasAtMostTwoMoneyDecimals(value) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) return false;
  return Math.abs(amount * 100 - Math.round(amount * 100)) < 1e-7;
}

function formatInvoiceUser(invoice) {
  if (invoice?.username) {
    const displayName =
      invoice.display_name && invoice.display_name !== invoice.username
        ? ` / ${invoice.display_name}`
        : '';
    return invoiceText`${invoice.username}${displayName}（用户 ID：${invoice.user_id || '-'}）`;
  }
  return invoice?.user_id ? invoiceText`用户 ID：${invoice.user_id}` : '-';
}

function formatInvoiceReviewer(invoice) {
  if (invoice?.reviewer_username) {
    const displayName =
      invoice.reviewer_display_name &&
      invoice.reviewer_display_name !== invoice.reviewer_username
        ? ` / ${invoice.reviewer_display_name}`
        : '';
    return `${invoice.reviewer_username}${displayName}（ID: ${invoice.reviewer_user_id || '-'}）`;
  }
  return invoice?.reviewer_user_id ? `ID: ${invoice.reviewer_user_id}` : '-';
}

function formatInvoicePrintDate(timestamp) {
  const date = timestamp ? new Date(timestamp * 1000) : new Date();
  if (Number.isNaN(date.getTime())) {
    return '-';
  }
  return new Intl.DateTimeFormat(i18n.resolvedLanguage || i18n.language, {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  }).format(date);
}

function formatInvoiceDateCompact(timestamp) {
  const date = timestamp ? new Date(timestamp * 1000) : new Date();
  if (Number.isNaN(date.getTime())) {
    return '00000000';
  }
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat('en-CA', {
      timeZone: 'Asia/Shanghai',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
      .formatToParts(date)
      .map(({ type, value }) => [type, value]),
  );
  return `${parts.year}${parts.month}${parts.day}`;
}

function formatInvoicePrintMoney(value) {
  const amount = Number(value || 0);
  if (!Number.isFinite(amount)) {
    return '-';
  }
  return `¥${amount.toLocaleString(i18n.resolvedLanguage || i18n.language, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function formatInvoiceServiceAmount(invoice) {
  const totalMoney = Number(invoice?.total_money || 0);
  if (Number.isFinite(totalMoney) && totalMoney > 0) {
    return invoiceText`人民币 ${formatInvoicePrintMoney(totalMoney)}`;
  }
  return invoiceText`以账户实际订单金额及发票开具金额为准`;
}

function formatInvoiceServiceQuota(invoice) {
  const totalQuota = Number(invoice?.total_quota || 0);
  if (Number.isFinite(totalQuota) && totalQuota > 0) {
    return invoiceText`${totalQuota.toLocaleString('zh-CN')} 额度单位`;
  }
  return invoiceText`以账户实际可用额度及调用扣减记录为准`;
}

function formatInvoiceProductQuantity(item) {
  const quantity = Number(item?.quantity || 0);
  if (!Number.isFinite(quantity) || quantity <= 0) {
    return '-';
  }
  const text = Number.isInteger(quantity)
    ? String(quantity)
    : quantity.toFixed(6).replace(/0+$/, '').replace(/\.$/, '');
  return `${text}${item?.unit ? ` ${item.unit}` : ''}`;
}

function formatInvoiceProductServicePeriod(item) {
  const start = Number(item?.service_start_at || 0);
  const end = Number(item?.service_end_at || 0);
  if (!start || !end) {
    return '-';
  }
  return invoiceText`${formatInvoicePrintDate(start)} 至 ${formatInvoicePrintDate(end)}`;
}

function formatInvoicePaymentAmount(item) {
  if (Number(item?.money || 0) > 0) {
    return formatInvoicePrintMoney(item.money);
  }
  return formatInvoicePrintMoney(0);
}

function buildInvoicePrintSummary(invoice) {
  const items = invoice?.items || [];
  const summary = {
    count: items.length,
    money: Number(invoice?.total_money || 0),
    quota: Number(invoice?.total_quota || 0),
    startTime: 0,
    endTime: 0,
    byType: {},
  };

  items.forEach((item) => {
    const type = item?.order_type || 'unknown';
    const bucket = summary.byType[type] || { count: 0, money: 0, quota: 0 };
    bucket.count += 1;
    bucket.money += Number(item?.money || 0);
    bucket.quota += Number(item?.amount || 0);
    summary.byType[type] = bucket;

    const time = Number(item?.complete_time || item?.create_time || 0);
    if (time > 0) {
      summary.startTime = summary.startTime
        ? Math.min(summary.startTime, time)
        : time;
      summary.endTime = Math.max(summary.endTime, time);
    }
  });

  return summary;
}

function formatInvoiceTypeSummary(summary) {
  return Object.entries(summary.byType)
    .filter(([, stats]) => stats.count > 0)
    .map(([type, stats]) => {
      const parts = [
        invoiceText`${getInvoiceOrderTypeLabel(type)} ${stats.count} 笔`,
      ];
      if (Number(stats.money || 0) > 0) {
        parts.push(
          invoiceText`金额合计 ${formatInvoicePrintMoney(stats.money)}`,
        );
      }
      return parts.join('，');
    })
    .join('；');
}

function buildInvoicePrintHtml(invoice, stampUrl = '', options = {}) {
  const items = invoice?.items || [];
  const summary = buildInvoicePrintSummary(invoice);
  const cell = (value) => escapeHtml(displayValue(value));
  const manual = isManualInvoice(invoice);
  const official = Boolean(options.official ?? invoice?.status === 'invoiced');
  const documentTitle = official
    ? invoiceText`曜算平台交易明细证明`
    : invoiceText`曜算平台交易明细账单（待审核）`;
  const intro = manual
    ? official
      ? invoiceText`兹证明：用户 ${cell(formatInvoiceUser(invoice))} 提交的转账资料已经平台审核，相关交易明细如下：`
      : invoiceText`用户 ${cell(formatInvoiceUser(invoice))} 提交了转账资料，以下内容仅为待审核申请信息预览：`
    : invoiceText`用户 ${cell(formatInvoiceUser(invoice))} 于曜算平台存在相关交易记录，申请范围内的交易明细如下：`;
  const sourceNotes = manual
    ? official
      ? invoiceText`<p>本文件所列人工转账信息依据申请人提交的银行转账资料及平台审核记录生成。</p>
      <p>人工转账记录不属于平台支付渠道自动回传订单，不会据此自动增加或扣减用户钱包余额。</p>`
      : invoiceText`<p>本文件中的银行转账信息由申请人自行填报，当前尚未完成平台核验，不构成到账、开票或服务交付证明。</p>
      <p>人工转账记录不会据此自动增加或扣减用户钱包余额。</p>`
    : invoiceText`<p>本文件所列订单明细依据用户申请时选择的订单及平台系统快照生成。</p>`;
  const totalParts = [
    invoiceText`支付金额合计人民币 ${formatInvoicePrintMoney(summary.money)}`,
  ];
  const typeSummary = formatInvoiceTypeSummary(summary);
  const timeRange = summary.startTime
    ? invoiceText`${formatInvoiceTime(summary.startTime)} 至 ${formatInvoiceTime(summary.endTime)}`
    : '-';
  const orderRows =
    items.length > 0
      ? items
          .map((item, index) => {
            const payer = displayValue(item?.payer_name);
            const payee = displayValue(item?.payee_name);
            const parties =
              payer !== '-' && payee !== '-'
                ? `${payer} → ${payee}`
                : payer !== '-'
                  ? payer
                  : payee;
            return `
              <tr>
                <td class="center">${index + 1}</td>
                <td class="center">${cell(getInvoiceOrderTypeLabel(item?.order_type))}</td>
                <td class="center">${cell(formatInvoiceBusinessId(item))}</td>
                <td>${cell(formatInvoiceCode(item))}</td>
                <td class="center">${cell(parties)}</td>
                <td class="center">${cell(item?.product_name)}</td>
                <td class="center">${cell(getInvoicePaymentLabelForItem(item))}</td>
                <td class="money">${cell(formatInvoicePaymentAmount(item))}</td>
                <td class="center">${cell(formatInvoiceTime(item?.complete_time || item?.create_time))}</td>
              </tr>
            `;
          })
          .join('')
      : invoiceText`<tr><td colspan="9" class="empty">暂无交易明细</td></tr>`;
  const watermark = official
    ? ''
    : invoiceText`<div class="draft-watermark">待审核 · 非正式文件</div>`;
  const signSection = official
    ? invoiceText`<section class="sign">
      <div class="sign-box">
        <p>上海曜算智能科技有限公司</p>
        <p>盖章：<span class="stamp-hint">见右下角公章</span></p>
        <p>出具日期：${cell(formatInvoicePrintDate(invoice?.reviewed_at || Math.floor(Date.now() / 1000)))}</p>
        ${stampUrl ? invoiceText`<img class="sign-seal-image" src="${cell(stampUrl)}" alt="公司公章" />` : ''}
      </div>
    </section>`
    : invoiceText`<section class="pending-note">当前状态：待审核。审核通过前不得作为正式交易证明使用。</section>`;
  return invoiceText`<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>${cell(documentTitle)} #${cell(invoice?.id)}</title>
  <style>
    @page { size: A4 landscape; margin: 14mm; }
    * { box-sizing: border-box; print-color-adjust: exact; -webkit-print-color-adjust: exact; }
    body { margin: 0; color: #243244; background: #fff; font: 14px/1.55 "Noto Serif CJK SC", "Songti SC", "SimSun", serif; }
    .certificate { position: relative; min-height: 176mm; padding: 4mm 18mm 6mm 2mm; overflow: hidden; }
    h1 { margin: 0; text-align: center; font: 700 25px/1.25 "Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", sans-serif; letter-spacing: 1px; color: #23364a; }
    .title-line { height: 2px; margin: 14px 0 8px; background: #2d5f86; }
    .intro { margin: 0 26px 16px; text-indent: 2em; font-size: 14px; }
    h2 { margin: 0 0 8px; font: 700 16px/1.2 "Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", sans-serif; color: #1f3447; }
    table { width: calc(100% - 40px); border-collapse: collapse; table-layout: fixed; margin-left: 20px; }
    th { background: #22364b; color: #fff; font-weight: 700; }
    th, td { border: 1px solid #cfd8e3; padding: 6px 7px; vertical-align: middle; word-break: break-all; }
    thead { display: table-header-group; }
    tr { break-inside: avoid; page-break-inside: avoid; }
    tbody tr:nth-child(even) { background: #f7f9fb; }
    .center { text-align: center; }
    .money { text-align: right; white-space: nowrap; }
    .summary { margin: 16px 20px 10px; padding: 9px 12px; border: 1px solid #b9c7d7; background: #f7f9fc; font-size: 14px; }
    .summary div { margin: 2px 0; }
    .notes { margin: 10px 20px 0; }
    .notes p { margin: 5px 0; text-indent: 2em; }
    .sign { display: flex; justify-content: flex-end; margin: 20px 20px 0; font-size: 14px; break-inside: avoid; page-break-inside: avoid; }
    .sign-box { position: relative; min-width: 250px; min-height: 118px; break-inside: avoid; }
    .sign-box p { margin: 8px 0; }
    .stamp-hint { color: #526579; font-size: 13px; }
    .sign-seal-image { position: absolute; width: 110px; height: 110px; right: 0; bottom: 0; object-fit: contain; opacity: 0.94; }
    .pending-note { margin: 24px 20px 0; padding: 10px 12px; border: 1px solid #f59e0b; color: #9a5b00; background: #fff8e6; text-align: center; font-weight: 700; }
    .draft-watermark { position: fixed; z-index: 10; left: 50%; top: 48%; transform: translate(-50%, -50%) rotate(-24deg); color: rgba(185, 28, 28, 0.16); font: 800 54px/1 "PingFang SC", sans-serif; letter-spacing: 5px; white-space: nowrap; pointer-events: none; }
    .empty { padding: 18px; text-align: center; color: #697586; }
    @media screen { body { padding: 18px; background: #eef2f7; } .certificate { max-width: 1120px; margin: 0 auto; padding: 28px 76px 72px 32px; background: #fff; box-shadow: 0 10px 34px rgba(15, 23, 42, 0.12); } }
  </style>
</head>
<body>
  <main class="certificate">
    ${watermark}
    <h1>${cell(documentTitle)}</h1>
    <div class="title-line"></div>
    <p class="intro">${intro}</p>

    <h2>交易明细表</h2>
    <table>
      <thead>
        <tr>
          <th style="width: 4%;">序号</th>
          <th style="width: 8%;">交易类型</th>
          <th style="width: 9%;">业务编号</th>
          <th>订单编码/交易号</th>
          <th style="width: 12%;">付款方/收款方</th>
          <th style="width: 10%;">商品/套餐</th>
          <th style="width: 10%;">支付渠道</th>
          <th style="width: 10%;">支付金额</th>
          <th style="width: 16%;">支付时间</th>
        </tr>
      </thead>
      <tbody>${orderRows}</tbody>
    </table>

    <section class="summary">
      <div>合计：共 ${summary.count} 笔交易，${cell(totalParts.join('，'))}。</div>
      <div>其中：${cell(typeSummary || '-')}。</div>
      <div>交易时间范围：${cell(timeRange)}。</div>
    </section>

    <section class="notes">
      <h2>说明</h2>
      ${sourceNotes}
      <p>${official ? invoiceText`本证明` : invoiceText`本预览`}不得擅自修改、涂改、拆分或用于与申请目的不一致的其他用途。</p>
      <p>本文件中所列时间均为北京时间（UTC+08:00）。</p>
      ${official ? invoiceText`<p>本证明经上海曜算智能科技有限公司加盖公章后生效。</p>` : ''}
    </section>

    ${signSection}
  </main>
</body>
</html>`;
}

function buildInvoiceServiceConfirmationHtml(
  invoice,
  stampUrl = '',
  options = {},
) {
  const cell = (value) => escapeHtml(displayValue(value));
  const official = Boolean(options.official ?? invoice?.status === 'invoiced');
  const documentNo = `YS-AIAPI-${formatInvoiceDateCompact(invoice?.created_at)}-${String(
    invoice?.id || 0,
  ).padStart(4, '0')}`;
  const issueDate = formatInvoicePrintDate(
    official
      ? invoice?.reviewed_at || Math.floor(Date.now() / 1000)
      : invoice?.created_at || Math.floor(Date.now() / 1000),
  );
  const clientName = invoice?.title || formatInvoiceUser(invoice);
  const productItems =
    Array.isArray(invoice?.product_items) && invoice.product_items.length > 0
      ? invoice.product_items
      : [
          {
            product_name: invoiceText`AI API 调用额度`,
            specification: invoiceText`以对应交易及账户实际记录为准`,
            unit: invoiceText`项`,
            quantity: 1,
            unit_price: Number(invoice?.total_money || 0),
            money: Number(invoice?.total_money || 0),
            quota: Number(invoice?.total_quota || 0),
            remark: '',
          },
        ];
  const productRows = productItems
    .map((item, index) => {
      const details = [
        Number(item?.quota || 0) > 0
          ? invoiceText`额度：${formatInvoiceServiceQuota({ total_quota: item.quota })}`
          : '',
        formatInvoiceProductServicePeriod(item) !== '-'
          ? invoiceText`服务周期：${formatInvoiceProductServicePeriod(item)}`
          : '',
        item?.remark ? invoiceText`备注：${item.remark}` : '',
      ].filter(Boolean);
      return `<tr>
        <td class="center">${index + 1}</td>
        <td>${cell(item?.product_name)}</td>
        <td>${cell(item?.specification)}</td>
        <td class="center">${cell(formatInvoiceProductQuantity(item))}</td>
        <td class="money">${cell(formatInvoicePrintMoney(item?.unit_price))}</td>
        <td class="money">${cell(formatInvoicePrintMoney(item?.money))}</td>
        <td>${cell(details.join('；'))}</td>
      </tr>`;
    })
    .join('');
  const documentTitle = official
    ? invoiceText`AI API 技术服务产品明细清单`
    : invoiceText`AI API 技术服务产品明细清单（待审核）`;
  const watermark = official
    ? ''
    : invoiceText`<div class="draft-watermark">待审核 · 非正式文件</div>`;
  const signSection = official
    ? invoiceText`<section class="sign">
      <div class="sign-box">
        <p class="provider-name">上海曜算智能科技有限公司</p>
        <p>盖章：<span class="stamp-hint">见右下角公章</span></p>
        <p>出具日期：${cell(issueDate)}</p>
        ${stampUrl ? invoiceText`<img class="provider-seal" src="${cell(stampUrl)}" alt="公司公章" />` : ''}
      </div>
    </section>`
    : invoiceText`<section class="pending-note">当前状态：待审核。产品内容及金额须经管理员核验后方可作为正式文件使用。</section>`;
  return invoiceText`<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>${cell(documentTitle)} #${cell(invoice?.id)}</title>
  <style>
    @page { size: A4 portrait; margin: 11mm 12mm; }
    * { box-sizing: border-box; print-color-adjust: exact; -webkit-print-color-adjust: exact; }
    body { margin: 0; color: #243244; background: #fff; font: 12px/1.38 "Noto Serif CJK SC", "Songti SC", "SimSun", serif; }
    .certificate { position: relative; min-height: 273mm; max-width: 186mm; margin: 0 auto; padding: 0 0 4mm; overflow: hidden; }
    .topline { display: flex; justify-content: space-between; align-items: center; border-bottom: 2px solid #2d5f86; padding-bottom: 4px; color: #2d5f86; font: 600 11px/1.2 "PingFang SC", sans-serif; }
    h1 { margin: 8px 0 5px; text-align: center; color: #23364a; font: 700 22px/1.2 "Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", sans-serif; letter-spacing: 0.8px; }
    .lead { margin: 0 4px 7px; text-align: center; color: #526579; font-size: 11px; }
    h2 { margin: 7px 0 4px; color: #1f3447; font: 700 14px/1.2 "Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", sans-serif; page-break-after: avoid; }
    table { width: 100%; border-collapse: collapse; table-layout: fixed; }
    thead { display: table-header-group; }
    tr { break-inside: avoid; page-break-inside: avoid; }
    .meta-table th { width: 19mm; background: #eef3f8; color: #23364a; }
    .meta-table td { background: #fff; }
    th { background: #22364b; color: #fff; font-weight: 700; text-align: center; }
    th, td { border: 1px solid #c8d4e2; padding: 6px 7px; vertical-align: middle; word-break: break-word; line-height: 1.34; }
    th { line-height: 1.2; }
    .center { text-align: center; }
    .money { text-align: right; white-space: nowrap; }
    .notes { margin: 0; padding: 8px 10px; border: 1px solid #c8d4e2; background: #fafcff; page-break-inside: avoid; }
    .notes p { margin: 4px 0; text-indent: 2em; }
    .sign { display: flex; justify-content: flex-end; margin-top: 40px; page-break-inside: avoid; }
    .sign-box { position: relative; min-width: 62mm; }
    .sign-box p { margin: 7px 0; }
    .provider-name { font-weight: 700; }
    .stamp-hint { color: #526579; }
    .provider-seal { position: absolute; z-index: 2; right: 2mm; bottom: -4mm; width: 32mm; height: 32mm; object-fit: contain; opacity: 0.9; transform: rotate(-6deg); pointer-events: none; }
    .pending-note { margin-top: 20px; padding: 10px 12px; border: 1px solid #f59e0b; color: #9a5b00; background: #fff8e6; text-align: center; font-weight: 700; }
    .draft-watermark { position: fixed; z-index: 10; left: 50%; top: 48%; transform: translate(-50%, -50%) rotate(-24deg); color: rgba(185, 28, 28, 0.15); font: 800 42px/1 "PingFang SC", sans-serif; letter-spacing: 4px; white-space: nowrap; pointer-events: none; }
    @media screen { body { padding: 18px; background: #eef2f7; } .certificate { padding: 12mm; background: #fff; box-shadow: 0 10px 34px rgba(15, 23, 42, 0.12); } }
  </style>
</head>
<body>
  <main class="certificate" data-page-orientation="portrait">
    ${watermark}
    <div class="topline">
      <span>上海曜算智能科技有限公司</span>
      <span>文件编号：${cell(documentNo)}</span>
    </div>
    <h1>${cell(documentTitle)}</h1>
    <p class="lead">${official ? invoiceText`本文件依据对应交易及产品快照生成，盖章后作为服务交付与费用确认依据。` : invoiceText`本文件依据用户提交内容自动生成，当前仅供核对，尚未完成平台审核。`}</p>
    <table class="meta-table">
      <tbody>
        <tr><th>文件编号</th><td>${cell(documentNo)}</td><th>${official ? invoiceText`签发日期` : invoiceText`申请日期`}</th><td>${cell(issueDate)}</td></tr>
        <tr><th>服务提供方</th><td>上海曜算智能科技有限公司</td><th>客户名称</th><td>${cell(clientName)}</td></tr>
        <tr><th>凭证来源</th><td>${cell(isManualInvoice(invoice) ? invoiceText`申请人填报的转账资料` : invoiceText`平台订单快照`)}</td><th>申请金额</th><td>${cell(formatInvoicePrintMoney(invoice?.total_money))}</td></tr>
      </tbody>
    </table>

    <h2>一、产品及费用明细</h2>
    <table>
      <thead><tr><th style="width: 10mm;">序号</th><th style="width: 30mm;">产品名称</th><th style="width: 34mm;">规格说明</th><th style="width: 22mm;">数量</th><th style="width: 26mm;">单价</th><th style="width: 26mm;">金额</th><th>额度 / 服务周期 / 备注</th></tr></thead>
      <tbody>${productRows}</tbody>
    </table>

    <h2>二、交付与核验说明</h2>
    <section class="notes">
      <p>产品名称、规格、数量、单价、金额及额度均以本清单快照为准；如与发票或双方书面约定不一致，应在审核前修正。</p>
      <p>转账资料由申请人填报，不会自动生成充值订单或改变钱包余额，须由管理员核对到账信息后处理。</p>
      <p>API 服务的实际可用范围、计量方式及使用限制仍以平台规则和双方书面约定为准。</p>
    </section>

    ${signSection}
  </main>
</body>
</html>`;
}

function pickInvoiceStatus(current, next) {
  const priority = { invoiced: 3, pending: 2, rejected: 1 };
  if (!current) return next || '';
  if (!next) return current;
  return (priority[next] || 0) > (priority[current] || 0) ? next : current;
}

function maskAlipayAccount(account) {
  const text = String(account || '');
  if (text.length <= 4) return text ? '****' : '-';
  return `${text.slice(0, 2)}****${text.slice(-2)}`;
}

function buildTableEmpty(t, description) {
  return (
    <Empty
      image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
      darkModeImage={
        <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
      }
      description={t(description)}
      style={{ padding: 30 }}
    />
  );
}

function createDashboardDateRange(preset) {
  const now = new Date();
  const start = new Date(now);
  start.setMilliseconds(0);

  switch (preset) {
    case 'month':
      start.setDate(start.getDate() - 29);
      start.setHours(0, 0, 0, 0);
      return [start, now];
    case 'week':
      start.setDate(start.getDate() - 6);
      start.setHours(0, 0, 0, 0);
      return [start, now];
    case 'today':
    default:
      start.setHours(0, 0, 0, 0);
      return [start, now];
  }
}

function toTimestampSeconds(value) {
  if (!value) {
    return 0;
  }
  const time = new Date(value).getTime();
  if (!Number.isFinite(time) || time <= 0) {
    return 0;
  }
  return Math.floor(time / 1000);
}

function formatCount(value) {
  const count = Number(value || 0);
  if (!Number.isFinite(count)) {
    return '0';
  }
  return count.toLocaleString();
}

function normalizeDashboardStats(stats) {
  return {
    totals: stats?.totals || { money: 0, order_count: 0 },
    statuses: stats?.statuses || {},
    payment_methods: stats?.payment_methods || {},
  };
}

const TopupHistoryModal = ({
  visible,
  onCancel,
  t,
  initialTab = 'records',
}) => {
  const [userState] = useContext(UserContext);
  const [loading, setLoading] = useState(false);
  const [topups, setTopups] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [filters, setFilters] = useState(EMPTY_FILTERS);
  const [appliedFilters, setAppliedFilters] = useState(EMPTY_FILTERS);
  const [showFilters, setShowFilters] = useState(false);
  const [activeTab, setActiveTab] = useState('records');

  const [dashboardLoading, setDashboardLoading] = useState(false);
  const [dashboardStats, setDashboardStats] = useState(null);
  const [dashboardRankings, setDashboardRankings] = useState([]);
  const [dashboardPreset, setDashboardPreset] = useState('today');
  const [dashboardDateRange, setDashboardDateRange] = useState(() =>
    createDashboardDateRange('today'),
  );
  const [dashboardRankLimit, setDashboardRankLimit] = useState(10);

  const [riskLoading, setRiskLoading] = useState(false);
  const [riskCases, setRiskCases] = useState([]);
  const [riskTotal, setRiskTotal] = useState(0);
  const [riskPage, setRiskPage] = useState(1);
  const [riskPageSize, setRiskPageSize] = useState(10);
  const [riskFilters, setRiskFilters] = useState(EMPTY_RISK_FILTERS);
  const [riskAppliedFilters, setRiskAppliedFilters] =
    useState(EMPTY_RISK_FILTERS);
  const [riskDetailVisible, setRiskDetailVisible] = useState(false);
  const [selectedRiskCaseId, setSelectedRiskCaseId] = useState(0);
  const [selectedRiskCaseSeed, setSelectedRiskCaseSeed] = useState(null);

  const [withdrawalLoading, setWithdrawalLoading] = useState(false);
  const [withdrawals, setWithdrawals] = useState([]);
  const [withdrawalTotal, setWithdrawalTotal] = useState(0);
  const [withdrawalPage, setWithdrawalPage] = useState(1);
  const [withdrawalPageSize, setWithdrawalPageSize] = useState(10);
  const [withdrawalFilters, setWithdrawalFilters] = useState(
    EMPTY_WITHDRAWAL_FILTERS,
  );
  const [withdrawalAppliedFilters, setWithdrawalAppliedFilters] = useState(
    EMPTY_WITHDRAWAL_FILTERS,
  );
  const [reviewState, setReviewState] = useState({
    visible: false,
    action: null,
    record: null,
  });
  const [reviewRemark, setReviewRemark] = useState('');
  const [reviewSubmitting, setReviewSubmitting] = useState(false);

  const [invoiceLoading, setInvoiceLoading] = useState(false);
  const [invoices, setInvoices] = useState([]);
  const [invoiceTotal, setInvoiceTotal] = useState(0);
  const [invoicePage, setInvoicePage] = useState(1);
  const [invoicePageSize, setInvoicePageSize] = useState(10);
  const [invoiceFilters, setInvoiceFilters] = useState(EMPTY_INVOICE_FILTERS);
  const [invoiceAppliedFilters, setInvoiceAppliedFilters] = useState(
    EMPTY_INVOICE_FILTERS,
  );
  const [invoiceStatusMap, setInvoiceStatusMap] = useState({});
  const [invoiceApplyVisible, setInvoiceApplyVisible] = useState(false);
  const [eligibleInvoiceOrders, setEligibleInvoiceOrders] = useState([]);
  const [eligibleInvoiceLoading, setEligibleInvoiceLoading] = useState(false);
  const [selectedInvoiceOrderKeys, setSelectedInvoiceOrderKeys] = useState([]);
  const [invoiceForm, setInvoiceForm] = useState(createEmptyInvoiceForm);
  const [invoiceSubmitting, setInvoiceSubmitting] = useState(false);
  const [invoiceReviewState, setInvoiceReviewState] = useState({
    visible: false,
    action: null,
    record: null,
  });
  const [invoiceReviewForm, setInvoiceReviewForm] = useState({
    invoiceUrl: '',
    invoiceSentTo: '',
    sendEmail: true,
    sendDetailBill: true,
    sendServiceConfirmation: false,
    adminRemark: '',
  });
  const [invoiceReviewFile, setInvoiceReviewFile] = useState(null);
  const [invoiceReviewDetailBillFile, setInvoiceReviewDetailBillFile] =
    useState(null);
  const [
    invoiceReviewServiceConfirmationFile,
    setInvoiceReviewServiceConfirmationFile,
  ] = useState(null);
  const [invoiceReviewSubmitting, setInvoiceReviewSubmitting] = useState(false);
  const invoiceReviewBusy = useRef(false);
  const invoiceEmailBusy = useRef(false);
  const [invoiceEmailState, setInvoiceEmailState] = useState({
    visible: false,
    record: null,
    invoiceSentTo: '',
    sendDetailBill: true,
    sendServiceConfirmation: false,
    detailBillFile: null,
    serviceConfirmationFile: null,
    submitting: false,
  });
  const [invoiceDetailVisible, setInvoiceDetailVisible] = useState(false);
  const [invoiceDetailLoading, setInvoiceDetailLoading] = useState(false);
  const [invoiceDetail, setInvoiceDetail] = useState(null);

  const isMobile = useIsMobile();
  const userIsAdmin = useMemo(() => isAdmin(), []);

  const loadTopups = async (currentPage, currentPageSize, currentFilters) => {
    setLoading(true);
    try {
      const base = userIsAdmin
        ? '/api/user/payment-records'
        : '/api/user/payment-records/self';
      const searchParams = new URLSearchParams({
        p: String(currentPage),
        page_size: String(currentPageSize),
      });

      if (currentFilters.keyword) {
        searchParams.set('keyword', currentFilters.keyword.trim());
      }
      if (currentFilters.status) {
        searchParams.set('status', currentFilters.status);
      }
      if (currentFilters.paymentMethod) {
        searchParams.set('payment_method', currentFilters.paymentMethod);
      }
      if (userIsAdmin && currentFilters.username) {
        searchParams.set('username', currentFilters.username.trim());
      }

      const res = await API.get(`${base}?${searchParams.toString()}`);
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '加载支付记录失败') });
        return;
      }

      setTopups(data?.items || []);
      setTotal(data?.total || 0);
    } catch (error) {
      Toast.error({ content: t('加载支付记录失败') });
    } finally {
      setLoading(false);
    }
  };

  const loadRiskCases = async (
    currentPage,
    currentPageSize,
    currentFilters,
  ) => {
    if (!userIsAdmin) {
      return;
    }
    setRiskLoading(true);
    try {
      const searchParams = new URLSearchParams({
        p: String(currentPage),
        page_size: String(currentPageSize),
      });

      if (currentFilters.keyword) {
        searchParams.set('keyword', currentFilters.keyword.trim());
      }
      if (currentFilters.username) {
        searchParams.set('username', currentFilters.username.trim());
      }
      if (currentFilters.status) {
        searchParams.set('status', currentFilters.status);
      }
      if (currentFilters.recordType) {
        searchParams.set('record_type', currentFilters.recordType);
      }
      if (currentFilters.reason) {
        searchParams.set('reason', currentFilters.reason);
      }

      const res = await API.get(
        `/api/user/payment-risk-cases?${searchParams.toString()}`,
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '加载异常单失败') });
        return;
      }

      setRiskCases(data?.items || []);
      setRiskTotal(data?.total || 0);
    } catch (error) {
      Toast.error({ content: t('加载异常单失败') });
    } finally {
      setRiskLoading(false);
    }
  };

  const loadWithdrawals = async (
    currentPage,
    currentPageSize,
    currentFilters,
    viewMode = 'self',
  ) => {
    setWithdrawalLoading(true);
    try {
      const isAdminView = viewMode === 'admin' && userIsAdmin;
      const base = isAdminView
        ? '/api/user/aff-withdrawals'
        : '/api/user/aff-withdrawals/self';
      const searchParams = new URLSearchParams({
        p: String(currentPage),
        page_size: String(currentPageSize),
      });
      if (currentFilters.status) {
        searchParams.set('status', currentFilters.status);
      }
      if (isAdminView && currentFilters.username) {
        searchParams.set('username', currentFilters.username.trim());
      }

      const res = await API.get(`${base}?${searchParams.toString()}`);
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '加载提现记录失败') });
        return;
      }
      setWithdrawals(data?.items || []);
      setWithdrawalTotal(data?.total || 0);
    } catch (error) {
      Toast.error({ content: t('加载提现记录失败') });
    } finally {
      setWithdrawalLoading(false);
    }
  };

  const loadInvoices = async (
    currentPage,
    currentPageSize,
    currentFilters,
    viewMode = 'self',
  ) => {
    setInvoiceLoading(true);
    try {
      const isAdminView = viewMode === 'admin' && userIsAdmin;
      const base = isAdminView
        ? '/api/user/invoices'
        : '/api/user/invoices/self';
      const searchParams = new URLSearchParams({
        p: String(currentPage),
        page_size: String(currentPageSize),
      });
      if (currentFilters.status) {
        searchParams.set('status', currentFilters.status);
      }
      if (isAdminView && currentFilters.username) {
        searchParams.set('username', currentFilters.username.trim());
      }

      const res = await API.get(`${base}?${searchParams.toString()}`);
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '加载发票申请失败') });
        return;
      }
      setInvoices(data?.items || []);
      setInvoiceTotal(data?.total || 0);
    } catch (error) {
      Toast.error({ content: t('加载发票申请失败') });
    } finally {
      setInvoiceLoading(false);
    }
  };

  const loadInvoiceStatusMap = async () => {
    try {
      const base = userIsAdmin
        ? '/api/user/invoices'
        : '/api/user/invoices/self';
      const res = await API.get(`${base}?p=1&page_size=100`);
      const { success, data } = res.data || {};
      if (!success) {
        return;
      }
      const nextMap = {};
      (data?.items || []).forEach((request) => {
        (request?.items || []).forEach((item) => {
          const key = getInvoiceItemKey(item);
          if (!key) return;
          nextMap[key] = pickInvoiceStatus(nextMap[key], request.status);
        });
      });
      setInvoiceStatusMap(nextMap);
    } catch (error) {
      // 状态展示失败不阻塞支付记录。
    }
  };

  const loadEligibleInvoiceOrders = async (prefillRecord = null) => {
    setEligibleInvoiceLoading(true);
    try {
      const res = await API.get(
        '/api/user/invoices/eligible-orders?p=1&page_size=100',
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '加载可开票订单失败') });
        return;
      }
      const items = data?.items || [];
      setEligibleInvoiceOrders(items);
      if (prefillRecord) {
        const key = getInvoiceOrderKey(prefillRecord);
        setSelectedInvoiceOrderKeys(
          items.some((item) => getInvoiceOrderKey(item) === key) ? [key] : [],
        );
      }
    } catch (error) {
      Toast.error({ content: t('加载可开票订单失败') });
    } finally {
      setEligibleInvoiceLoading(false);
    }
  };

  const loadDashboard = async (
    dateRange = dashboardDateRange,
    limit = dashboardRankLimit,
  ) => {
    if (!userIsAdmin) {
      return;
    }

    setDashboardLoading(true);
    try {
      const [start, end] = dateRange || [];
      const params = {
        start_timestamp: toTimestampSeconds(start),
        end_timestamp: toTimestampSeconds(end),
      };

      const [statsRes, rankingsRes] = await Promise.all([
        API.get('/api/user/payment-records/stats', { params }),
        API.get('/api/user/payment-records/rankings', {
          params: {
            ...params,
            limit,
          },
        }),
      ]);

      const statsPayload = statsRes?.data || {};
      const rankingsPayload = rankingsRes?.data || {};

      if (!statsPayload.success) {
        Toast.error({ content: t(statsPayload.message || '加载对账统计失败') });
        return;
      }
      if (!rankingsPayload.success) {
        Toast.error({
          content: t(rankingsPayload.message || '加载充值榜单失败'),
        });
        return;
      }

      setDashboardStats(normalizeDashboardStats(statsPayload.data));
      setDashboardRankings(rankingsPayload.data?.items || []);
    } catch (error) {
      Toast.error({ content: t('加载对账看板失败') });
    } finally {
      setDashboardLoading(false);
    }
  };

  const refreshRecords = async () => {
    await loadTopups(page, pageSize, appliedFilters);
  };

  const refreshDashboard = async () => {
    await loadDashboard(dashboardDateRange, dashboardRankLimit);
  };

  const refreshRiskCases = async () => {
    if (!userIsAdmin) {
      return;
    }
    await loadRiskCases(riskPage, riskPageSize, riskAppliedFilters);
  };

  const refreshWithdrawals = async () => {
    const mode = activeTab === 'withdrawals' ? 'admin' : 'self';
    await loadWithdrawals(
      withdrawalPage,
      withdrawalPageSize,
      withdrawalAppliedFilters,
      mode,
    );
  };

  const refreshInvoices = async () => {
    const mode = activeTab === 'invoices' && userIsAdmin ? 'admin' : 'self';
    await loadInvoices(
      invoicePage,
      invoicePageSize,
      invoiceAppliedFilters,
      mode,
    );
    await loadInvoiceStatusMap();
  };

  useEffect(() => {
    if (visible && initialTab) {
      const safeTab =
        !userIsAdmin && initialTab === 'withdrawals'
          ? 'my-withdrawals'
          : initialTab;
      setActiveTab(safeTab);
    }
  }, [visible, initialTab, userIsAdmin]);

  useEffect(() => {
    if (!visible) {
      return;
    }
    if (activeTab === 'records') {
      loadTopups(page, pageSize, appliedFilters);
      loadInvoiceStatusMap();
    }
  }, [visible, activeTab, page, pageSize, appliedFilters, userIsAdmin]);

  useEffect(() => {
    if (!visible || !userIsAdmin) {
      return;
    }
    if (activeTab === 'dashboard') {
      loadDashboard(dashboardDateRange, dashboardRankLimit);
    }
  }, [visible, activeTab, userIsAdmin, dashboardDateRange, dashboardRankLimit]);

  useEffect(() => {
    if (!visible || !userIsAdmin) {
      return;
    }
    if (activeTab === 'risk') {
      loadRiskCases(riskPage, riskPageSize, riskAppliedFilters);
    }
  }, [
    visible,
    activeTab,
    riskPage,
    riskPageSize,
    riskAppliedFilters,
    userIsAdmin,
  ]);

  useEffect(() => {
    if (!visible) {
      return;
    }
    if (activeTab !== 'withdrawals' && activeTab !== 'my-withdrawals') {
      return;
    }
    const mode = activeTab === 'withdrawals' ? 'admin' : 'self';
    loadWithdrawals(
      withdrawalPage,
      withdrawalPageSize,
      withdrawalAppliedFilters,
      mode,
    );
  }, [
    visible,
    activeTab,
    withdrawalPage,
    withdrawalPageSize,
    withdrawalAppliedFilters,
    userIsAdmin,
  ]);

  useEffect(() => {
    if (!visible || activeTab !== 'invoices') {
      return;
    }
    const mode = userIsAdmin ? 'admin' : 'self';
    loadInvoices(invoicePage, invoicePageSize, invoiceAppliedFilters, mode);
  }, [
    visible,
    activeTab,
    invoicePage,
    invoicePageSize,
    invoiceAppliedFilters,
    userIsAdmin,
  ]);

  const handleFilterChange = (key, value) => {
    setFilters((prev) => ({
      ...prev,
      [key]: value || '',
    }));
  };

  const applyFilters = (nextFilters = filters) => {
    setPage(1);
    setAppliedFilters({
      ...nextFilters,
      keyword: nextFilters.keyword.trim(),
      username: nextFilters.username.trim(),
    });
  };

  const resetFilters = () => {
    setPage(1);
    setFilters(EMPTY_FILTERS);
    setAppliedFilters(EMPTY_FILTERS);
    setShowFilters(false);
  };

  const handleRiskFilterChange = (key, value) => {
    setRiskFilters((prev) => ({
      ...prev,
      [key]: value || '',
    }));
  };

  const applyRiskFilters = (nextFilters = riskFilters) => {
    setRiskPage(1);
    setRiskAppliedFilters({
      ...nextFilters,
      keyword: nextFilters.keyword.trim(),
      username: nextFilters.username.trim(),
    });
  };

  const resetRiskFilters = () => {
    setRiskPage(1);
    setRiskFilters(EMPTY_RISK_FILTERS);
    setRiskAppliedFilters(EMPTY_RISK_FILTERS);
  };

  const handleWithdrawalFilterChange = (key, value) => {
    setWithdrawalFilters((prev) => ({
      ...prev,
      [key]: value || '',
    }));
  };

  const applyWithdrawalFilters = (nextFilters = withdrawalFilters) => {
    setWithdrawalPage(1);
    setWithdrawalAppliedFilters({
      ...nextFilters,
      username: nextFilters.username.trim(),
    });
  };

  const resetWithdrawalFilters = () => {
    setWithdrawalPage(1);
    setWithdrawalFilters(EMPTY_WITHDRAWAL_FILTERS);
    setWithdrawalAppliedFilters(EMPTY_WITHDRAWAL_FILTERS);
  };

  const handleInvoiceFilterChange = (key, value) => {
    setInvoiceFilters((prev) => ({
      ...prev,
      [key]: value || '',
    }));
  };

  const applyInvoiceFilters = (nextFilters = invoiceFilters) => {
    setInvoicePage(1);
    setInvoiceAppliedFilters({
      ...nextFilters,
      username: nextFilters.username.trim(),
    });
  };

  const resetInvoiceFilters = () => {
    setInvoicePage(1);
    setInvoiceFilters(EMPTY_INVOICE_FILTERS);
    setInvoiceAppliedFilters(EMPTY_INVOICE_FILTERS);
  };

  const activeFilterTags = useMemo(() => {
    const tags = [];
    if (appliedFilters.username) {
      tags.push({
        key: 'username',
        label: `ID/用户名: ${appliedFilters.username}`,
      });
    }
    if (appliedFilters.status) {
      const found = STATUS_OPTIONS.find(
        (option) => option.value === appliedFilters.status,
      );
      tags.push({
        key: 'status',
        label: `状态: ${found ? found.label : appliedFilters.status}`,
      });
    }
    if (appliedFilters.paymentMethod) {
      const found = PAYMENT_OPTIONS.find(
        (option) => option.value === appliedFilters.paymentMethod,
      );
      tags.push({
        key: 'paymentMethod',
        label: `支付方式: ${found ? found.label : appliedFilters.paymentMethod}`,
      });
    }
    return tags;
  }, [appliedFilters]);

  const removeFilterTag = (key) => {
    const nextFilters = { ...filters, [key]: '' };
    setFilters(nextFilters);
    applyFilters(nextFilters);
  };

  const filterByUsername = (username) => {
    if (!username) {
      return;
    }
    if (activeTab === 'risk') {
      const nextFilters = { ...riskFilters, username };
      setRiskFilters(nextFilters);
      applyRiskFilters(nextFilters);
      return;
    }
    if (activeTab === 'withdrawals') {
      const nextFilters = { ...withdrawalFilters, username };
      setWithdrawalFilters(nextFilters);
      applyWithdrawalFilters(nextFilters);
      return;
    }
    if (activeTab === 'invoices') {
      const nextFilters = { ...invoiceFilters, username };
      setInvoiceFilters(nextFilters);
      applyInvoiceFilters(nextFilters);
      return;
    }
    const nextFilters = { ...filters, username };
    setFilters(nextFilters);
    applyFilters(nextFilters);
  };

  const openRecordTabForUsername = (username) => {
    if (!username) {
      return;
    }
    const nextFilters = { ...filters, username };
    setFilters(nextFilters);
    applyFilters(nextFilters);
    setActiveTab('records');
  };

  const handleDashboardPresetChange = (preset) => {
    setDashboardPreset(preset);
    if (preset === 'custom') {
      return;
    }
    setDashboardDateRange(createDashboardDateRange(preset));
  };

  const handleDashboardDateRangeChange = (value) => {
    setDashboardPreset('custom');
    setDashboardDateRange(value || []);
  };

  const handleAdminComplete = async (tradeNo) => {
    try {
      const res = await API.post('/api/user/topup/complete', {
        trade_no: tradeNo,
      });
      const { success, message } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '补单失败') });
        return;
      }
      Toast.success({ content: t('补单成功') });
      await refreshRecords();
    } catch (error) {
      Toast.error({ content: t('补单失败') });
    }
  };

  const confirmAdminComplete = (tradeNo) => {
    Modal.confirm({
      title: t('确认补单'),
      content: t('是否将该订单标记为成功并为用户入账？'),
      onOk: () => handleAdminComplete(tradeNo),
    });
  };

  const openRiskCaseDetail = (riskCaseId, riskCase) => {
    setSelectedRiskCaseId(Number(riskCaseId || 0));
    setSelectedRiskCaseSeed(riskCase || null);
    setRiskDetailVisible(true);
  };

  const resolveRiskRecordType = (record) => {
    const orderType = resolveOrderType(record);
    if (orderType === 'subscription') {
      return 'subscription';
    }
    if (record?.record_type === 'topup') {
      return 'topup';
    }
    return '';
  };

  const canCreateRiskCase = (record) => {
    if (!userIsAdmin || record?.risk_case_id) {
      return false;
    }
    return resolveRiskRecordType(record) !== '';
  };

  const handleCreateRiskCase = async (record) => {
    const recordType = resolveRiskRecordType(record);
    if (!recordType || !record?.trade_no) {
      Toast.error({ content: t('当前记录不支持标记异常') });
      return;
    }
    try {
      const res = await API.post('/api/user/payment-risk-cases', {
        record_type: recordType,
        trade_no: record.trade_no,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '标记异常失败') });
        return;
      }
      Toast.success({ content: t('已加入异常审核队列') });
      await Promise.all([refreshRecords(), refreshRiskCases()]);
      openRiskCaseDetail(data?.risk_case?.id, data?.risk_case || null);
    } catch (error) {
      Toast.error({ content: t('标记异常失败') });
    }
  };

  const confirmCreateRiskCase = (record) => {
    Modal.confirm({
      title: t('标记异常'),
      content: t('确认将这笔订单加入人工审核队列吗？'),
      onOk: () => handleCreateRiskCase(record),
    });
  };

  const handleRiskCaseResolved = async (updatedRiskCase) => {
    if (updatedRiskCase?.id) {
      setSelectedRiskCaseSeed(updatedRiskCase);
    }
    await Promise.all([refreshRecords(), refreshRiskCases()]);
  };

  const handleReviewWithdrawal = async (record, action, remark = '') => {
    const id = Number(record?.id || 0);
    if (!id) return false;
    try {
      const res = await API.post(`/api/user/aff-withdrawals/${id}/${action}`, {
        admin_remark: remark,
      });
      const { success, message } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '审核提现失败') });
        return false;
      }
      Toast.success({
        content: t(action === 'approve' ? '提现已通过' : '提现已驳回'),
      });
      await refreshWithdrawals();
      return true;
    } catch (error) {
      const msg =
        error?.response?.data?.message || error?.message || '审核提现失败';
      Toast.error({ content: t(msg) });
      return false;
    }
  };

  const confirmApproveWithdrawal = (record) => {
    setReviewRemark('');
    setReviewState({ visible: true, action: 'approve', record });
  };

  const confirmRejectWithdrawal = (record) => {
    setReviewRemark('');
    setReviewState({ visible: true, action: 'reject', record });
  };

  const closeReviewModal = () => {
    setReviewState({ visible: false, action: null, record: null });
    setReviewRemark('');
  };

  const submitReviewModal = async () => {
    const { record, action } = reviewState;
    if (!record || !action) return;
    setReviewSubmitting(true);
    try {
      const ok = await handleReviewWithdrawal(
        record,
        action,
        reviewRemark.trim(),
      );
      if (ok) closeReviewModal();
    } finally {
      setReviewSubmitting(false);
    }
  };

  const openInvoiceApplyModal = async (record = null) => {
    setInvoiceForm(createEmptyInvoiceForm());
    setSelectedInvoiceOrderKeys(record ? [getInvoiceOrderKey(record)] : []);
    setInvoiceApplyVisible(true);
    await loadEligibleInvoiceOrders(record);
  };

  const closeInvoiceApplyModal = () => {
    setInvoiceApplyVisible(false);
    setSelectedInvoiceOrderKeys([]);
    setEligibleInvoiceOrders([]);
    setInvoiceForm(createEmptyInvoiceForm());
  };

  const handleInvoiceFormChange = (key, value) => {
    setInvoiceForm((prev) => {
      const next = {
        ...prev,
        [key]: value,
      };
      if (key === 'invoiceType' && value === 'special') {
        next.titleType = 'company';
      }
      return next;
    });
  };

  const handleInvoiceSourceChange = (value) => {
    if (value === 'manual_transfer') {
      setSelectedInvoiceOrderKeys([]);
    }
    setInvoiceForm((prev) => ({
      ...prev,
      sourceType: value,
      needDetailBill: value === 'manual_transfer' ? true : prev.needDetailBill,
      needServiceConfirmation:
        value === 'manual_transfer' ? true : prev.needServiceConfirmation,
    }));
  };

  const updateManualInvoiceTransaction = (key, field, value) => {
    setInvoiceForm((prev) => ({
      ...prev,
      manualTransactions: prev.manualTransactions.map((item) =>
        item.key === key ? { ...item, [field]: value } : item,
      ),
    }));
  };

  const addManualInvoiceTransaction = () => {
    setInvoiceForm((prev) => ({
      ...prev,
      manualTransactions: [
        ...prev.manualTransactions,
        createManualInvoiceTransaction(),
      ],
    }));
  };

  const removeManualInvoiceTransaction = (key) => {
    setInvoiceForm((prev) => ({
      ...prev,
      manualTransactions:
        prev.manualTransactions.length > 1
          ? prev.manualTransactions.filter((item) => item.key !== key)
          : prev.manualTransactions,
    }));
  };

  const selectedInvoiceOrders = useMemo(() => {
    const selected = new Set(selectedInvoiceOrderKeys);
    return eligibleInvoiceOrders.filter((item) =>
      selected.has(getInvoiceOrderKey(item)),
    );
  }, [eligibleInvoiceOrders, selectedInvoiceOrderKeys]);

  const selectedInvoiceSummary = useMemo(() => {
    return selectedInvoiceOrders.reduce(
      (summary, item) => ({
        money: summary.money + Number(item?.money || 0),
        quota: summary.quota + Number(item?.amount || 0),
      }),
      { money: 0, quota: 0 },
    );
  }, [selectedInvoiceOrders]);

  const manualInvoiceSummary = useMemo(
    () =>
      invoiceForm.manualTransactions.reduce(
        (summary, item) => ({
          money: summary.money + Number(item?.money || 0),
          quota: summary.quota,
        }),
        { money: 0, quota: 0 },
      ),
    [invoiceForm.manualTransactions],
  );

  const invoiceApplySummary =
    invoiceForm.sourceType === 'manual_transfer'
      ? {
          money: manualInvoiceSummary.money,
          quota: 0,
        }
      : selectedInvoiceSummary;

  // 发票申请手续费：开票金额(money)为人民币，按充值汇率 price 换回美元再 × 每元额度，
  // 与后端口径一致：feeQuota = money × rate × QuotaPerUnit / price，截断取整。
  const invoiceServiceFeeRate = useMemo(() => {
    try {
      const status = JSON.parse(localStorage.getItem('status') || '{}');
      const rate = Number(status?.invoice_service_fee_rate || 0);
      return Number.isFinite(rate) && rate > 0 ? rate : 0;
    } catch {
      return 0;
    }
  }, [visible]);
  const invoicePrice = useMemo(() => {
    try {
      const status = JSON.parse(localStorage.getItem('status') || '{}');
      const p = Number(status?.price || 0);
      return Number.isFinite(p) && p > 0 ? p : 0;
    } catch {
      return 0;
    }
  }, [visible]);
  const estimatedInvoiceFeeQuota = useMemo(() => {
    if (
      invoiceServiceFeeRate <= 0 ||
      invoicePrice <= 0 ||
      invoiceApplySummary.money <= 0
    )
      return 0;
    return Math.trunc(
      (invoiceApplySummary.money * invoiceServiceFeeRate * getQuotaPerUnit()) /
        invoicePrice,
    );
  }, [invoiceServiceFeeRate, invoicePrice, invoiceApplySummary.money]);
  const userWalletQuota = Number(userState?.user?.quota || 0);
  const invoiceFeeInsufficient =
    estimatedInvoiceFeeQuota > 0 && userWalletQuota < estimatedInvoiceFeeQuota;

  const submitInvoiceRequest = async () => {
    const form = {
      invoiceType: invoiceForm.invoiceType || 'normal',
      titleType: invoiceForm.titleType,
      title: invoiceForm.title.trim(),
      taxNumber: invoiceForm.taxNumber.trim(),
      registeredAddress: invoiceForm.registeredAddress.trim(),
      registeredPhone: invoiceForm.registeredPhone.trim(),
      bankName: invoiceForm.bankName.trim(),
      bankAccount: invoiceForm.bankAccount.trim(),
      email: invoiceForm.email.trim(),
      phone: invoiceForm.phone.trim(),
      remark: invoiceForm.remark.trim(),
    };
    const isManual = invoiceForm.sourceType === 'manual_transfer';
    if (!isManual && selectedInvoiceOrders.length === 0) {
      Toast.error({ content: t('请选择需要开票的订单') });
      return;
    }
    const manualTransactions = invoiceForm.manualTransactions.map((item) => ({
      trade_no: item.tradeNo.trim(),
      payer_name: item.payerName.trim(),
      payee_name: item.payeeName.trim(),
      transfer_bank_name: item.transferBankName.trim(),
      money: Number(item.money),
      paid_at: toUnixTimestamp(item.paidAt),
      remark: item.remark.trim(),
    }));
    if (isManual) {
      const invalidTransfer = manualTransactions.find(
        (item) =>
          !item.trade_no ||
          !item.payer_name ||
          !item.payee_name ||
          !item.transfer_bank_name ||
          !Number.isFinite(item.money) ||
          item.money <= 0 ||
          !hasAtMostTwoMoneyDecimals(item.money) ||
          !item.paid_at ||
          item.paid_at > Math.floor(Date.now() / 1000),
      );
      if (invalidTransfer) {
        Toast.error({ content: t('请完整填写银行转账信息') });
        return;
      }
    }
    if (!form.title) {
      Toast.error({
        content: t(
          form.invoiceType === 'special'
            ? '单位名称不能为空'
            : '发票抬头不能为空',
        ),
      });
      return;
    }
    if (form.invoiceType === 'special' && !form.taxNumber) {
      Toast.error({ content: t('专票需要填写税号') });
      return;
    }
    if (form.titleType === 'company' && !form.taxNumber) {
      Toast.error({ content: t('企业抬头需要填写税号') });
      return;
    }
    if (!form.email) {
      Toast.error({ content: t('接收邮箱不能为空') });
      return;
    }
    if (invoiceFeeInsufficient) {
      Toast.error({ content: t('余额不足，无法支付发票手续费') });
      return;
    }

    const doSubmit = async () => {
      setInvoiceSubmitting(true);
      try {
        const res = await API.post('/api/user/invoices', {
          invoice_type: form.invoiceType,
          title_type: form.titleType,
          title: form.title,
          tax_number: form.taxNumber,
          registered_address: form.registeredAddress,
          registered_phone: form.registeredPhone,
          bank_name: form.bankName,
          bank_account: form.bankAccount,
          email: form.email,
          phone: form.phone,
          remark: form.remark,
          need_detail_bill: Boolean(invoiceForm.needDetailBill),
          need_service_confirmation: Boolean(
            invoiceForm.needServiceConfirmation,
          ),
          orders: isManual
            ? []
            : selectedInvoiceOrders.map((item) => ({
                order_type: resolveOrderType(item),
                id: item.id,
              })),
          manual_transactions: isManual ? manualTransactions : [],
          product_items: [],
        });
        const { success, message, data } = res.data || {};
        if (!success) {
          Toast.error({ content: t(message || '提交发票申请失败') });
          return;
        }
        Toast.success({ content: t('发票申请已提交') });
        closeInvoiceApplyModal();
        if (data?.id) {
          setInvoiceDetail(data);
          setInvoiceDetailVisible(true);
        }
        await Promise.all([refreshInvoices(), refreshRecords()]);
      } catch (error) {
        Toast.error({
          content: t(
            error?.response?.data?.message ||
              error?.message ||
              '提交发票申请失败',
          ),
        });
      } finally {
        setInvoiceSubmitting(false);
      }
    };

    // 有手续费时先二次确认，明确告知将从钱包额度扣除多少，用户点确定后才真正提交扣费
    if (estimatedInvoiceFeeQuota > 0) {
      Modal.confirm({
        title: t('确认提交发票申请'),
        content: (
          <div>
            {t('本次申请将从你的钱包额度扣除手续费')}{' '}
            <Text strong type='warning'>
              {renderQuota(estimatedInvoiceFeeQuota)}
            </Text>
            （{t('开票金额')} {formatMoney(invoiceApplySummary.money)} ×{' '}
            {(invoiceServiceFeeRate * 100).toFixed(2)}%）。
            <br />
            {t('若申请被驳回，手续费将原额退还。')}
          </div>
        ),
        okText: t('确认并提交'),
        cancelText: t('取消'),
        onOk: doSubmit,
      });
      return;
    }
    doSubmit();
  };

  const openInvoiceReviewModal = (record, action) => {
    if (invoiceReviewBusy.current) return;
    setInvoiceReviewState({ visible: true, action, record });
    setInvoiceReviewForm({
      invoiceUrl: record?.invoice_url || '',
      invoiceSentTo: record?.invoice_sent_to || record?.email || '',
      sendEmail: true,
      sendDetailBill: record?.need_detail_bill !== false,
      sendServiceConfirmation: Boolean(record?.need_service_confirmation),
      adminRemark: '',
    });
    setInvoiceReviewFile(null);
    setInvoiceReviewDetailBillFile(null);
    setInvoiceReviewServiceConfirmationFile(null);
  };

  const closeInvoiceReviewModal = () => {
    if (invoiceReviewBusy.current) return;
    setInvoiceReviewState({ visible: false, action: null, record: null });
    setInvoiceReviewForm({
      invoiceUrl: '',
      invoiceSentTo: '',
      sendEmail: true,
      sendDetailBill: true,
      sendServiceConfirmation: false,
      adminRemark: '',
    });
    setInvoiceReviewFile(null);
    setInvoiceReviewDetailBillFile(null);
    setInvoiceReviewServiceConfirmationFile(null);
  };

  const loadInvoiceStampDataUrl = async ({ required = false } = {}) => {
    const stampUrl = `${window.location.origin}/invoice-stamp.png`;
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 15000);
    try {
      const response = await fetch(stampUrl, {
        cache: 'force-cache',
        signal: controller.signal,
      });
      if (!response.ok) throw new Error('stamp download failed');
      const blob = await response.blob();
      const signature = new Uint8Array(await blob.slice(0, 8).arrayBuffer());
      if (
        signature.join(',') !== '137,80,78,71,13,10,26,10' ||
        blob.size > 1024 * 1024
      ) {
        throw new Error('invalid stamp');
      }
      const dataUrl = await new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsDataURL(new Blob([blob], { type: 'image/png' }));
      });
      const image = new Image();
      image.src = dataUrl;
      await image.decode();
      return dataUrl;
    } catch (error) {
      if (required) {
        throw new Error(t('印章加载失败，请重试或手动上传 PDF'));
      }
      return stampUrl;
    } finally {
      window.clearTimeout(timeout);
    }
  };

  const prepareInvoiceFiles = (record, selected, files, saved = {}) =>
    prepareInvoiceAttachmentFiles({
      selected,
      files,
      saved,
      loadDetail: () => loadInvoiceDetailData(record),
      loadStamp: () => loadInvoiceStampDataUrl({ required: true }),
      buildHtml: (kind, detail, stamp) =>
        kind === 'detail_bill'
          ? buildInvoicePrintHtml(detail, stamp, { official: true })
          : buildInvoiceServiceConfirmationHtml(detail, stamp, {
              official: true,
            }),
      renderPdf: (kind, html) =>
        renderInvoicePdf(
          API,
          record.id,
          kind,
          html,
          t('自动生成 PDF 失败，请重试或手动上传 PDF'),
        ),
    });

  const submitInvoiceReview = async () => {
    const { record, action } = invoiceReviewState;
    const id = Number(record?.id || 0);
    if (!id || !action || invoiceReviewBusy.current) return;
    if (action === 'approve' && !invoiceReviewFile) {
      Toast.error({ content: t('请上传发票 PDF') });
      return;
    }
    if (
      action === 'approve' &&
      invoiceReviewForm.sendEmail &&
      !invoiceReviewForm.invoiceSentTo.trim()
    ) {
      Toast.error({ content: t('发票接收邮箱不能为空') });
      return;
    }
    invoiceReviewBusy.current = true;
    setInvoiceReviewSubmitting(true);
    try {
      let payload;
      if (action === 'approve') {
        const files = await prepareInvoiceFiles(
          record,
          {
            detail_bill:
              invoiceReviewForm.sendEmail && invoiceReviewForm.sendDetailBill,
            service_confirmation:
              invoiceReviewForm.sendEmail &&
              invoiceReviewForm.sendServiceConfirmation,
          },
          {
            detail_bill: invoiceReviewDetailBillFile,
            service_confirmation: invoiceReviewServiceConfirmationFile,
          },
        );
        payload = new FormData();
        payload.append('invoice_url', invoiceReviewForm.invoiceUrl.trim());
        payload.append(
          'invoice_sent_to',
          invoiceReviewForm.invoiceSentTo.trim(),
        );
        payload.append('send_email', String(invoiceReviewForm.sendEmail));
        payload.append(
          'send_detail_bill',
          String(
            invoiceReviewForm.sendEmail && invoiceReviewForm.sendDetailBill,
          ),
        );
        payload.append(
          'send_service_confirmation',
          String(
            invoiceReviewForm.sendEmail &&
              invoiceReviewForm.sendServiceConfirmation,
          ),
        );
        payload.append('admin_remark', invoiceReviewForm.adminRemark.trim());
        payload.append('invoice_file', invoiceReviewFile);
        if (invoiceReviewForm.sendEmail && invoiceReviewForm.sendDetailBill) {
          payload.append('detail_bill_file', files.detail_bill);
        }
        if (
          invoiceReviewForm.sendEmail &&
          invoiceReviewForm.sendServiceConfirmation
        ) {
          payload.append(
            'service_confirmation_file',
            files.service_confirmation,
          );
        }
      } else {
        payload = {
          admin_remark: invoiceReviewForm.adminRemark.trim(),
        };
      }
      const res = await API.post(`/api/user/invoices/${id}/${action}`, payload);
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '审核发票失败') });
        return;
      }
      if (
        action === 'approve' &&
        data?.invoice_send_status === 'failed' &&
        data?.invoice_send_error
      ) {
        Toast.warning({
          content: t(`发票已通过，但邮件发送失败：${data.invoice_send_error}`),
        });
      } else {
        Toast.success({
          content: t(action === 'approve' ? '发票已通过' : '发票已驳回'),
        });
      }
      await refreshInvoices();
      invoiceReviewBusy.current = false;
      closeInvoiceReviewModal();
    } catch (error) {
      Toast.error({ content: t(error?.message || '审核发票失败') });
    } finally {
      invoiceReviewBusy.current = false;
      setInvoiceReviewSubmitting(false);
    }
  };

  const loadInvoiceDetailData = async (record) => {
    const id = Number(record?.id || 0);
    if (!id) {
      throw new Error('参数错误');
    }
    const endpoint = userIsAdmin
      ? `/api/user/invoices/${id}`
      : `/api/user/invoices/self/${id}`;
    const res = await API.get(endpoint);
    const { success, message, data } = res.data || {};
    if (!success) {
      throw new Error(message || '加载发票详情失败');
    }
    if (!data || Number(data.id) !== id) {
      throw new Error('加载发票详情失败');
    }
    return data;
  };

  const openResendInvoiceEmailModal = (record) => {
    if (invoiceEmailBusy.current) return;
    setInvoiceEmailState({
      visible: true,
      record,
      invoiceSentTo: record?.invoice_sent_to || record?.email || '',
      sendDetailBill:
        Boolean(record?.detail_bill_file_name) ||
        record?.need_detail_bill !== false,
      sendServiceConfirmation: Boolean(
        record?.service_confirmation_file_name ||
          record?.need_service_confirmation,
      ),
      detailBillFile: null,
      serviceConfirmationFile: null,
      submitting: false,
    });
  };

  const closeResendInvoiceEmailModal = () => {
    if (invoiceEmailBusy.current) return;
    setInvoiceEmailState({
      visible: false,
      record: null,
      invoiceSentTo: '',
      sendDetailBill: true,
      sendServiceConfirmation: false,
      detailBillFile: null,
      serviceConfirmationFile: null,
      submitting: false,
    });
  };

  const submitResendInvoiceEmail = async () => {
    const id = Number(invoiceEmailState.record?.id || 0);
    if (!id || invoiceEmailBusy.current) return;
    if (!invoiceEmailState.invoiceSentTo.trim()) {
      Toast.error({ content: t('发票接收邮箱不能为空') });
      return;
    }
    invoiceEmailBusy.current = true;
    setInvoiceEmailState((prev) => ({ ...prev, submitting: true }));
    try {
      const files = await prepareInvoiceFiles(
        invoiceEmailState.record,
        {
          detail_bill: invoiceEmailState.sendDetailBill,
          service_confirmation: invoiceEmailState.sendServiceConfirmation,
        },
        {
          detail_bill: invoiceEmailState.detailBillFile,
          service_confirmation: invoiceEmailState.serviceConfirmationFile,
        },
        {
          detail_bill: invoiceEmailState.record?.detail_bill_file_name,
          service_confirmation:
            invoiceEmailState.record?.service_confirmation_file_name,
        },
      );
      const payload = new FormData();
      payload.append('invoice_sent_to', invoiceEmailState.invoiceSentTo.trim());
      payload.append(
        'send_detail_bill',
        String(invoiceEmailState.sendDetailBill),
      );
      payload.append(
        'send_service_confirmation',
        String(invoiceEmailState.sendServiceConfirmation),
      );
      if (files.detail_bill) {
        payload.append('detail_bill_file', files.detail_bill);
      }
      if (files.service_confirmation) {
        payload.append('service_confirmation_file', files.service_confirmation);
      }
      const res = await API.post(
        `/api/user/invoices/${id}/resend-email`,
        payload,
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        Toast.error({ content: t(message || '重发邮件失败') });
        return;
      }
      if (data?.invoice_send_status === 'failed') {
        Toast.warning({
          content: t(`邮件发送失败：${data?.invoice_send_error || '-'}`),
        });
      } else {
        Toast.success({ content: t('邮件已发送') });
      }
      await refreshInvoices();
      invoiceEmailBusy.current = false;
      closeResendInvoiceEmailModal();
    } catch (error) {
      Toast.error({ content: t(error?.message || '重发邮件失败') });
    } finally {
      invoiceEmailBusy.current = false;
      setInvoiceEmailState((prev) => ({ ...prev, submitting: false }));
    }
  };

  const closeInvoiceDetail = () => {
    setInvoiceDetailVisible(false);
    setInvoiceDetail(null);
  };

  const openInvoiceDetailBillPdfWindow = async (
    detail,
    { targetWindow = null, autoPrint = false } = {},
  ) => {
    const pdfWindow =
      targetWindow || window.open('', '_blank', 'width=960,height=720');
    if (!pdfWindow) {
      return false;
    }
    pdfWindow.opener = null;

    const official = userIsAdmin || detail?.status === 'invoiced';
    const stampUrl = official ? await loadInvoiceStampDataUrl() : '';
    const html = buildInvoicePrintHtml(detail, stampUrl, { official });
    // 浏览器原生渲染（所见即所得，与页面预览完全一致）；需要 PDF 时走浏览器“打印→另存为 PDF”。
    const doc = autoPrint
      ? html.replace(
          '</body>',
          '<script>window.addEventListener("load",function(){setTimeout(function(){try{window.print();}catch(e){}},300);});<\/script></body>',
        )
      : html;
    pdfWindow.document.open();
    pdfWindow.document.write(doc);
    pdfWindow.document.close();
    pdfWindow.focus();
    return true;
  };

  const openInvoiceServiceConfirmationPdfWindow = async (
    detail,
    { targetWindow = null, autoPrint = false } = {},
  ) => {
    const pdfWindow =
      targetWindow || window.open('', '_blank', 'width=960,height=720');
    if (!pdfWindow) {
      return false;
    }
    pdfWindow.opener = null;

    const official = userIsAdmin || detail?.status === 'invoiced';
    const stampUrl = official ? await loadInvoiceStampDataUrl() : '';
    const html = buildInvoiceServiceConfirmationHtml(detail, stampUrl, {
      official,
    });
    // 浏览器原生渲染（所见即所得，与页面预览完全一致）；需要 PDF 时走浏览器“打印→另存为 PDF”。
    const doc = autoPrint
      ? html.replace(
          '</body>',
          '<script>window.addEventListener("load",function(){setTimeout(function(){try{window.print();}catch(e){}},300);});<\/script></body>',
        )
      : html;
    pdfWindow.document.open();
    pdfWindow.document.write(doc);
    pdfWindow.document.close();
    pdfWindow.focus();
    return true;
  };

  const printInvoiceDetail = async () => {
    if (!invoiceDetail) {
      return;
    }
    try {
      if (
        !(await openInvoiceDetailBillPdfWindow(invoiceDetail, {
          autoPrint: true,
        }))
      ) {
        Toast.error({ content: t('浏览器阻止了打印窗口') });
      }
    } catch (error) {
      Toast.error({ content: t('生成明细账单失败') });
    }
  };

  const printInvoiceServiceConfirmation = async () => {
    if (!invoiceDetail) {
      return;
    }
    try {
      if (
        !(await openInvoiceServiceConfirmationPdfWindow(invoiceDetail, {
          autoPrint: true,
        }))
      ) {
        Toast.error({ content: t('浏览器阻止了产品明细清单窗口') });
      }
    } catch (error) {
      Toast.error({ content: t('生成产品明细清单失败') });
    }
  };

  const viewInvoiceDetailBill = async (record) => {
    const targetWindow = window.open('', '_blank', 'width=960,height=720');
    if (!targetWindow) {
      Toast.error({ content: t('浏览器阻止了明细账单窗口') });
      return;
    }
    targetWindow.opener = null;
    targetWindow.document.open();
    targetWindow.document.write(
      invoiceText`<!doctype html><meta charset="utf-8"><title>明细账单</title><body style="font:14px sans-serif;padding:24px;">正在加载明细账单...</body>`,
    );
    targetWindow.document.close();
    try {
      const detail = await loadInvoiceDetailData(record);
      await openInvoiceDetailBillPdfWindow(detail, { targetWindow });
    } catch (error) {
      targetWindow.document.open();
      targetWindow.document.write(
        invoiceText`<!doctype html><meta charset="utf-8"><title>明细账单</title><body style="font:14px sans-serif;padding:24px;color:#b91c1c;">${escapeHtml(
          error?.message || invoiceText`加载明细账单失败`,
        )}</body>`,
      );
      targetWindow.document.close();
    }
  };

  const viewInvoiceServiceConfirmation = async (record) => {
    const targetWindow = window.open('', '_blank', 'width=960,height=720');
    if (!targetWindow) {
      Toast.error({ content: t('浏览器阻止了产品明细清单窗口') });
      return;
    }
    targetWindow.opener = null;
    targetWindow.document.open();
    targetWindow.document.write(
      invoiceText`<!doctype html><meta charset="utf-8"><title>产品明细清单</title><body style="font:14px sans-serif;padding:24px;">正在加载产品明细清单...</body>`,
    );
    targetWindow.document.close();
    try {
      const detail = await loadInvoiceDetailData(record);
      await openInvoiceServiceConfirmationPdfWindow(detail, { targetWindow });
    } catch (error) {
      targetWindow.document.open();
      targetWindow.document.write(
        invoiceText`<!doctype html><meta charset="utf-8"><title>产品明细清单</title><body style="font:14px sans-serif;padding:24px;color:#b91c1c;">${escapeHtml(
          error?.message || invoiceText`加载产品明细清单失败`,
        )}</body>`,
      );
      targetWindow.document.close();
    }
  };

  const renderStatusBadge = (status) => {
    const config = STATUS_CONFIG[status] || {
      type: 'primary',
      label: status || '-',
    };
    return (
      <span className='flex items-center gap-2'>
        <Badge dot type={config.type} />
        <span>{t(config.label)}</span>
      </span>
    );
  };

  const renderPaymentMethod = (paymentMethod) => {
    const displayName = PAYMENT_METHOD_MAP[paymentMethod];
    return (
      <Tag shape='circle' color={paymentMethod === 'wallet' ? 'blue' : 'grey'}>
        {t(displayName || paymentMethod || '-')}
      </Tag>
    );
  };

  const renderRiskStatusTag = (status) => {
    const config = RISK_STATUS_CONFIG[status] || {
      color: 'grey',
      label: status || '-',
    };
    return (
      <Tag color={config.color} shape='circle' size='small'>
        {t(config.label)}
      </Tag>
    );
  };

  const renderRiskReason = (reason) =>
    t(RISK_REASON_MAP[reason] || reason || '-');

  const renderWithdrawalStatusTag = (status) => {
    const config = WITHDRAWAL_STATUS_CONFIG[status] || {
      color: 'grey',
      label: status || '-',
    };
    return (
      <Tag color={config.color} shape='circle' size='small'>
        {t(config.label)}
      </Tag>
    );
  };

  const renderInvoiceStatusTag = (status) => {
    const config = INVOICE_STATUS_CONFIG[status] || {
      color: 'grey',
      label: status || '-',
    };
    return (
      <Tag color={config.color} shape='circle' size='small'>
        {t(config.label)}
      </Tag>
    );
  };

  const renderInvoiceSendStatusTag = (status) => {
    const config = INVOICE_SEND_STATUS_CONFIG[status] || {
      color: 'grey',
      label: status || '-',
    };
    return (
      <Tag color={config.color} shape='circle' size='small'>
        {t(config.label)}
      </Tag>
    );
  };

  const getInvoiceFileUrl = (record) => {
    const id = Number(record?.id || 0);
    if (!id || !record?.invoice_file_name) {
      return '';
    }
    return userIsAdmin
      ? `/api/user/invoices/${id}/file`
      : `/api/user/invoices/self/${id}/file`;
  };

  const viewInvoicePdf = async (record) => {
    const url = getInvoiceFileUrl(record);
    if (!url) return;
    try {
      await openInvoicePdf(API, url, {
        title: record.invoice_file_name || t('发票 PDF'),
        loading: t('正在加载发票 PDF...'),
        popupBlocked: t('浏览器阻止了发票窗口，请允许弹出窗口后重试'),
        failed: t('加载发票 PDF 失败，请重试'),
      });
    } catch (error) {
      Toast.error({ content: t(error.message || '加载发票 PDF 失败，请重试') });
    }
  };

  const renderInvoiceRecordStatus = (record) => {
    if (record?.status !== 'success') {
      return <Text type='tertiary'>-</Text>;
    }
    const status = invoiceStatusMap[getInvoiceOrderKey(record)];
    if (!status) {
      if (userIsAdmin) {
        return <Text type='tertiary'>-</Text>;
      }
      return (
        <Button
          size='small'
          type='primary'
          theme='outline'
          onClick={() => openInvoiceApplyModal(record)}
        >
          {t('申请发票')}
        </Button>
      );
    }
    return (
      <Space wrap>
        {renderInvoiceStatusTag(status)}
        {!userIsAdmin && status === 'rejected' ? (
          <Button
            size='small'
            theme='outline'
            onClick={() => openInvoiceApplyModal(record)}
          >
            {t('重新申请')}
          </Button>
        ) : null}
      </Space>
    );
  };

  const isSellableTokenPurchase = (record) =>
    record?.record_type === 'sellable_token_purchase';

  const isSubscriptionTopup = (record) =>
    resolveOrderType(record) === 'subscription';

  const renderRecordType = (record) => {
    if (isSellableTokenPurchase(record)) {
      return (
        <div className='min-w-0'>
          <Tag color='blue' shape='circle' size='small'>
            {t('钱包购买')}
          </Tag>
          <div className='mt-1'>
            <Text
              ellipsis={{ showTooltip: true }}
              style={{ maxWidth: 180, display: 'inline-block' }}
            >
              {record?.product_name || t('可售令牌')}
            </Text>
          </div>
        </div>
      );
    }

    if (isSubscriptionTopup(record)) {
      return (
        <Tag color='purple' shape='circle' size='small'>
          {t('订阅套餐')}
        </Tag>
      );
    }

    return (
      <Tag color='green' shape='circle' size='small'>
        {t('在线充值')}
      </Tag>
    );
  };

  const renderRecordNo = (record) => {
    const text = record?.trade_no || '-';
    return (
      <Text
        copyable={text !== '-'}
        ellipsis={{
          showTooltip: { opts: { style: { wordBreak: 'break-all' } } },
        }}
        style={{ width: 170, display: 'inline-block' }}
      >
        {text}
      </Text>
    );
  };

  const renderRiskSummary = (record) => {
    if (!record?.risk_case_id) {
      return <Text type='tertiary'>-</Text>;
    }
    return (
      <div className='flex flex-col gap-1'>
        {renderRiskStatusTag(record.risk_status)}
        <Text type='tertiary' size='small'>
          {renderRiskReason(record.risk_reason)}
        </Text>
      </div>
    );
  };

  const recordColumns = useMemo(() => {
    const columns = [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        width: 200,
        render: (_, record) => renderRecordNo(record),
      },
      {
        title: t('类型 / 商品'),
        key: 'record_type',
        width: 200,
        render: (_, record) => renderRecordType(record),
      },
    ];

    if (userIsAdmin) {
      columns.push({
        title: t('用户名'),
        dataIndex: 'username',
        key: 'username',
        render: (_, record) => {
          const username = record?.username || '';
          const displayName = record?.display_name || '';
          if (!username) {
            return <Text type='tertiary'>-</Text>;
          }

          return (
            <Space spacing={8} align='center'>
              <Avatar size='extra-small' color={stringToColor(username)}>
                {username.slice(0, 1).toUpperCase()}
              </Avatar>
              <div className='flex flex-col leading-5'>
                {record?.user_id > 0 ? (
                  <Text type='tertiary' size='small'>
                    ID: {record.user_id}
                  </Text>
                ) : null}
                <Text
                  link
                  size='small'
                  style={{ cursor: 'pointer' }}
                  onClick={() => filterByUsername(username)}
                >
                  {username}
                </Text>
                {displayName ? (
                  <Text type='tertiary'>{displayName}</Text>
                ) : null}
              </div>
            </Space>
          );
        },
      });
    }

    columns.push(
      {
        title: t('支付方式'),
        dataIndex: 'payment_method',
        key: 'payment_method',
        render: renderPaymentMethod,
      },
      {
        title: t('充值额度'),
        dataIndex: 'amount',
        key: 'amount',
        render: (amount, record) => {
          if (isSellableTokenPurchase(record)) {
            return <Text type='tertiary'>-</Text>;
          }
          if (isSubscriptionTopup(record)) {
            return (
              <Tag color='purple' shape='circle' size='small'>
                SUB
              </Tag>
            );
          }
          return (
            <span className='flex items-center gap-1'>
              <Coins size={16} />
              <Text>{renderQuotaWithAmount(amount ?? 0)}</Text>
            </span>
          );
        },
      },
      {
        title: t('支付金额'),
        dataIndex: 'money',
        key: 'money',
        render: (money, record) => {
          if (isSellableTokenPurchase(record)) {
            return (
              <Text type='danger'>{renderQuota(record?.amount ?? 0)}</Text>
            );
          }
          if (record?.payment_amount_known === false) {
            return <Text type='tertiary'>-</Text>;
          }
          const currency = record?.currency || 'CNY';
          const paidCurrency = record?.paid_currency || '';
          const showSettlement =
            Number(record?.paid_money || 0) > 0 &&
            paidCurrency &&
            (paidCurrency !== currency ||
              Math.abs(Number(record.paid_money) - Number(money || 0)) > 1e-6);
          return (
            <div className='flex flex-col'>
              <Text type='danger'>{formatMoney(money, currency)}</Text>
              {showSettlement ? (
                <Text type='tertiary' size='small'>
                  {t('渠道结算')}：
                  {formatMoney(record.paid_money, paidCurrency)}
                </Text>
              ) : null}
            </div>
          );
        },
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        key: 'status',
        render: renderStatusBadge,
      },
      {
        title: t('发票'),
        key: 'invoice',
        width: 130,
        render: (_, record) => renderInvoiceRecordStatus(record),
      },
      {
        title: t('创建时间'),
        dataIndex: 'create_time',
        key: 'create_time',
        render: (time) => (time ? timestamp2string(time) : '-'),
      },
    );

    if (userIsAdmin) {
      columns.push({
        title: t('风控'),
        key: 'risk',
        width: 150,
        render: (_, record) => renderRiskSummary(record),
      });
      columns.push({
        title: t('操作'),
        key: 'action',
        width: 220,
        render: (_, record) => {
          const actionButtons = [];

          if (
            record?.record_type === 'topup' &&
            record?.status === 'pending' &&
            !record?.risk_case_id
          ) {
            actionButtons.push(
              <Button
                key='complete'
                size='small'
                type='primary'
                theme='outline'
                onClick={() => confirmAdminComplete(record.trade_no)}
              >
                {t('补单')}
              </Button>,
            );
          }

          if (record?.risk_case_id) {
            actionButtons.push(
              <Button
                key='detail'
                size='small'
                theme='outline'
                onClick={() =>
                  openRiskCaseDetail(record.risk_case_id, {
                    id: record.risk_case_id,
                    trade_no: record.trade_no,
                    record_type:
                      resolveRiskRecordType(record) || resolveOrderType(record),
                    status: record.risk_status,
                    reason: record.risk_reason,
                    user_id: record.user_id,
                    username: record.username,
                    display_name: record.display_name,
                    payment_method: record.payment_method,
                    expected_amount: record.amount,
                    expected_money: record.paid_money || record.money,
                    currency: record.paid_currency || record.currency,
                    order_status: record.status,
                  })
                }
              >
                {t('查看异常')}
              </Button>,
            );
          } else if (canCreateRiskCase(record)) {
            actionButtons.push(
              <Button
                key='mark-risk'
                size='small'
                theme='outline'
                type='danger'
                onClick={() => confirmCreateRiskCase(record)}
              >
                {t('标记异常')}
              </Button>,
            );
          }

          if (actionButtons.length === 0) {
            return null;
          }
          return <Space wrap>{actionButtons}</Space>;
        },
      });
    }

    return columns;
  }, [userIsAdmin, filters, riskFilters, invoiceStatusMap]);

  const dashboardData = useMemo(
    () => normalizeDashboardStats(dashboardStats),
    [dashboardStats],
  );

  const dashboardSummaryCards = useMemo(() => {
    const totals = dashboardData.totals;
    const statuses = dashboardData.statuses || {};
    return [
      {
        key: 'total-money',
        label: '总支付金额',
        value: formatMoneyAmounts(totals.amounts, totals.money),
        helper: `${t('总订单数')} ${formatCount(totals.order_count)}`,
      },
      {
        key: 'success-money',
        label: '成功支付金额',
        value: formatMoneyAmounts(
          statuses.success?.amounts,
          statuses.success?.money,
        ),
        helper: `${t('成功订单')} ${formatCount(statuses.success?.order_count)}`,
      },
      {
        key: 'pending-money',
        label: '待支付金额',
        value: formatMoneyAmounts(
          statuses.pending?.amounts,
          statuses.pending?.money,
        ),
        helper: `${t('待支付订单')} ${formatCount(statuses.pending?.order_count)}`,
      },
      {
        key: 'expired-money',
        label: '失效金额',
        value: formatMoneyAmounts(
          statuses.expired?.amounts,
          statuses.expired?.money,
        ),
        helper: `${t('失效订单')} ${formatCount(statuses.expired?.order_count)}`,
      },
      {
        key: 'cancelled-money',
        label: '已取消金额',
        value: formatMoneyAmounts(
          statuses.cancelled?.amounts,
          statuses.cancelled?.money,
        ),
        helper: `${t('已取消订单')} ${formatCount(statuses.cancelled?.order_count)}`,
      },
    ];
  }, [dashboardData, t]);

  const dashboardPaymentMethods = useMemo(() => {
    const items = Object.entries(dashboardData.payment_methods || {}).map(
      ([method, stats]) => ({
        method,
        money: Number(stats?.money || 0),
        amounts: stats?.amounts,
        orderCount: Number(stats?.order_count || 0),
      }),
    );
    items.sort((left, right) => {
      if (left.orderCount !== right.orderCount) {
        return right.orderCount - left.orderCount;
      }
      return right.money - left.money;
    });
    return items;
  }, [dashboardData]);

  const dashboardRankingColumns = useMemo(
    () => [
      {
        title: t('排名'),
        key: 'rank',
        width: 72,
        render: (_, __, index) => (
          <Tag color={index < 3 ? 'orange' : 'grey'} shape='circle'>
            #{index + 1}
          </Tag>
        ),
      },
      {
        title: t('用户'),
        key: 'username',
        render: (_, record) => {
          if (!record?.username) {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Space spacing={8} align='center'>
              <Avatar size='extra-small' color={stringToColor(record.username)}>
                {record.username.slice(0, 1).toUpperCase()}
              </Avatar>
              <div className='flex flex-col leading-5'>
                <Text
                  link
                  size='small'
                  style={{ cursor: 'pointer' }}
                  onClick={() => openRecordTabForUsername(record.username)}
                >
                  {record.username}
                </Text>
                {record.user_id ? (
                  <Text type='tertiary' size='small'>
                    ID: {record.user_id}
                  </Text>
                ) : null}
                {record.display_name ? (
                  <Text type='tertiary'>{record.display_name}</Text>
                ) : null}
              </div>
            </Space>
          );
        },
      },
      {
        title: t('充值金额'),
        key: 'money',
        width: 120,
        render: (_, record) => (
          <Text strong>
            {formatMoneyAmounts(record?.amounts, record?.money)}
          </Text>
        ),
      },
      {
        title: t('成功金额'),
        key: 'success_money',
        width: 120,
        render: (_, record) => (
          <Text type='success'>
            {formatMoneyAmounts(record?.success_amounts, record?.success_money)}
          </Text>
        ),
      },
      {
        title: t('订单数'),
        key: 'order_count',
        width: 100,
        render: (_, record) => formatCount(record?.order_count),
      },
      {
        title: t('待支付金额'),
        key: 'pending_money',
        width: 120,
        render: (_, record) => (
          <Text type='warning'>
            {formatMoneyAmounts(record?.pending_amounts, record?.pending_money)}
          </Text>
        ),
      },
    ],
    [t, openRecordTabForUsername],
  );

  const riskCaseColumns = useMemo(() => {
    return [
      {
        title: t('异常单'),
        key: 'trade',
        width: 260,
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Space wrap>
              <Tag shape='circle' color='grey' size='small'>
                {t(
                  RECORD_TYPE_MAP[record.record_type] ||
                    record.record_type ||
                    '-',
                )}
              </Tag>
              {renderRiskStatusTag(record.status)}
            </Space>
            <Text copyable>{record.trade_no || '-'}</Text>
          </div>
        ),
      },
      {
        title: t('用户'),
        key: 'username',
        render: (_, record) => {
          if (!record?.username) {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Space spacing={8} align='center'>
              <Avatar size='extra-small' color={stringToColor(record.username)}>
                {record.username.slice(0, 1).toUpperCase()}
              </Avatar>
              <div className='flex flex-col leading-5'>
                <Text
                  link
                  size='small'
                  style={{ cursor: 'pointer' }}
                  onClick={() => filterByUsername(record.username)}
                >
                  {record.username}
                </Text>
                {record.user_id ? (
                  <Text type='tertiary' size='small'>
                    ID: {record.user_id}
                  </Text>
                ) : null}
                {record.display_name ? (
                  <Text type='tertiary'>{record.display_name}</Text>
                ) : null}
              </div>
            </Space>
          );
        },
      },
      {
        title: t('异常原因'),
        key: 'reason',
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Text>{renderRiskReason(record.reason)}</Text>
            <Text type='tertiary' size='small'>
              {t('订单状态')}:{' '}
              {t(
                STATUS_CONFIG[record.order_status]?.label ||
                  record.order_status ||
                  '-',
              )}
            </Text>
          </div>
        ),
      },
      {
        title: t('金额对比'),
        key: 'money',
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Text type='tertiary' size='small'>
              {t('预期')}: {formatMoney(record.expected_money, record.currency)}
            </Text>
            <Text size='small'>
              {t('回调')}: {formatMoney(record.received_money, record.currency)}
            </Text>
          </div>
        ),
      },
      {
        title: t('创建时间'),
        dataIndex: 'created_at',
        key: 'created_at',
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('操作'),
        key: 'action',
        render: (_, record) => (
          <Button
            size='small'
            theme='outline'
            onClick={() => openRiskCaseDetail(record.id, record)}
          >
            {t('查看详情')}
          </Button>
        ),
      },
    ];
  }, [riskFilters]);

  const withdrawalColumns = useMemo(() => {
    const isReviewMode = userIsAdmin && activeTab === 'withdrawals';
    const columns = [
      {
        title: t('状态'),
        dataIndex: 'status',
        key: 'status',
        width: 100,
        render: renderWithdrawalStatusTag,
      },
    ];

    if (isReviewMode) {
      columns.push({
        title: t('用户'),
        key: 'username',
        width: 180,
        render: (_, record) => {
          if (!record?.username) {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Space spacing={8} align='center'>
              <Avatar size='extra-small' color={stringToColor(record.username)}>
                {record.username.slice(0, 1).toUpperCase()}
              </Avatar>
              <div className='flex flex-col leading-5'>
                <Text type='tertiary' size='small'>
                  ID: {record.user_id}
                </Text>
                <Text
                  link
                  size='small'
                  style={{ cursor: 'pointer' }}
                  onClick={() => filterByUsername(record.username)}
                >
                  {record.username}
                </Text>
                {record.display_name ? (
                  <Text type='tertiary'>{record.display_name}</Text>
                ) : null}
              </div>
            </Space>
          );
        },
      });
    }

    columns.push(
      {
        title: t('提现额度'),
        dataIndex: 'quota',
        key: 'quota',
        width: 130,
        render: (quota) => <Text strong>{renderQuota(quota || 0)}</Text>,
      },
      {
        title: t('预计到账'),
        dataIndex: 'amount_cents',
        key: 'amount_cents',
        width: 120,
        render: (amountCents) => (
          <Text type='danger'>{formatAmountCents(amountCents)}</Text>
        ),
      },
      {
        title: t('支付宝信息'),
        key: 'alipay',
        width: 220,
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Text copyable={userIsAdmin}>
              {userIsAdmin
                ? record?.alipay_account || '-'
                : maskAlipayAccount(record?.alipay_account)}
            </Text>
            <Text type='tertiary' size='small'>
              {record?.alipay_name || '-'}
            </Text>
          </div>
        ),
      },
      {
        title: t('提交时间'),
        dataIndex: 'created_at',
        key: 'created_at',
        width: 150,
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('审核时间'),
        dataIndex: 'reviewed_at',
        key: 'reviewed_at',
        width: 150,
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('备注'),
        dataIndex: 'admin_remark',
        key: 'admin_remark',
        render: (value) => value || <Text type='tertiary'>-</Text>,
      },
    );

    if (isReviewMode) {
      columns.push({
        title: t('操作'),
        key: 'action',
        width: 150,
        render: (_, record) => {
          if (record?.status !== 'pending') {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Space wrap>
              <Button
                size='small'
                type='primary'
                theme='outline'
                onClick={() => confirmApproveWithdrawal(record)}
              >
                {t('通过')}
              </Button>
              <Button
                size='small'
                type='danger'
                theme='outline'
                onClick={() => confirmRejectWithdrawal(record)}
              >
                {t('驳回')}
              </Button>
            </Space>
          );
        },
      });
    }

    return columns;
  }, [
    userIsAdmin,
    activeTab,
    withdrawalFilters,
    withdrawalPage,
    withdrawalPageSize,
    withdrawalAppliedFilters,
    t,
  ]);

  const invoiceOrderColumns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        width: 220,
        render: (_, record) => renderRecordNo(record),
      },
      {
        title: t('类型 / 商品'),
        key: 'record_type',
        render: (_, record) => renderRecordType(record),
      },
      {
        title: t('支付金额'),
        key: 'money',
        width: 140,
        render: (_, record) =>
          isSellableTokenPurchase(record) ? (
            <Text type='danger'>{renderQuota(record?.amount || 0)}</Text>
          ) : (
            <Text type='danger'>{formatMoney(record?.money)}</Text>
          ),
      },
      {
        title: t('支付时间'),
        dataIndex: 'complete_time',
        key: 'complete_time',
        width: 160,
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
    ],
    [t],
  );

  const invoiceDetailItemColumns = useMemo(
    () => [
      {
        title: t('交易类型'),
        dataIndex: 'order_type',
        key: 'order_type',
        width: 110,
        render: (value) => (
          <Tag shape='circle' size='small' color='blue'>
            {t(getInvoiceOrderTypeLabel(value))}
          </Tag>
        ),
      },
      {
        title: t('业务编号'),
        key: 'order_id',
        width: 100,
        render: (_, item) => formatInvoiceBusinessId(item),
      },
      {
        title: t('付款方 / 收款方'),
        key: 'transfer_parties',
        width: 200,
        render: (_, item) =>
          item?.payer_name || item?.payee_name ? (
            <div>
              <div>{item?.payer_name || '-'}</div>
              <Text type='tertiary' size='small'>
                → {item?.payee_name || '-'}
              </Text>
            </div>
          ) : (
            <Text type='tertiary'>-</Text>
          ),
      },
      {
        title: t('订单编码/交易号'),
        key: 'trade_no',
        width: 220,
        render: (_, item) => (
          <Text copyable ellipsis style={{ maxWidth: 190 }}>
            {formatInvoiceCode(item)}
          </Text>
        ),
      },
      {
        title: t('商品/套餐'),
        dataIndex: 'product_name',
        key: 'product_name',
        width: 160,
        render: (value) => value || <Text type='tertiary'>-</Text>,
      },
      {
        title: t('支付渠道'),
        dataIndex: 'payment_method',
        key: 'payment_method',
        width: 120,
        render: (_, item) => (
          <Tag shape='circle' color='grey'>
            {t(getInvoicePaymentLabelForItem(item))}
          </Tag>
        ),
      },
      {
        title: t('支付金额'),
        dataIndex: 'money',
        key: 'money',
        width: 120,
        render: (value) => <Text type='danger'>{formatMoney(value)}</Text>,
      },
      {
        title: t('额度'),
        dataIndex: 'amount',
        key: 'amount',
        width: 120,
        render: (value) =>
          Number(value || 0) > 0 ? (
            <Text>{renderQuota(value)}</Text>
          ) : (
            <Text type='tertiary'>-</Text>
          ),
      },
      {
        title: t('支付时间'),
        key: 'complete_time',
        width: 160,
        render: (_, item) =>
          formatInvoiceTime(item?.complete_time || item?.create_time),
      },
    ],
    [t],
  );

  const invoiceProductItemColumns = useMemo(
    () => [
      {
        title: t('产品名称'),
        dataIndex: 'product_name',
        key: 'product_name',
        width: 180,
      },
      {
        title: t('规格说明'),
        dataIndex: 'specification',
        key: 'specification',
        width: 180,
        render: (value) => value || <Text type='tertiary'>-</Text>,
      },
      {
        title: t('数量'),
        key: 'quantity',
        width: 110,
        render: (_, item) => formatInvoiceProductQuantity(item),
      },
      {
        title: t('单价'),
        dataIndex: 'unit_price',
        key: 'unit_price',
        width: 120,
        render: (value) => formatMoney(value),
      },
      {
        title: t('金额'),
        dataIndex: 'money',
        key: 'money',
        width: 120,
        render: (value) => <Text type='danger'>{formatMoney(value)}</Text>,
      },
      {
        title: t('额度'),
        dataIndex: 'quota',
        key: 'quota',
        width: 130,
        render: (value) => (Number(value || 0) > 0 ? renderQuota(value) : '-'),
      },
      {
        title: t('产品备注'),
        dataIndex: 'remark',
        key: 'remark',
        width: 180,
        render: (value) => value || <Text type='tertiary'>-</Text>,
      },
    ],
    [t],
  );

  const invoiceColumns = useMemo(() => {
    const isReviewMode = userIsAdmin && activeTab === 'invoices';
    const columns = [
      {
        title: t('状态'),
        dataIndex: 'status',
        key: 'status',
        width: 100,
        render: renderInvoiceStatusTag,
      },
    ];

    if (isReviewMode) {
      columns.push({
        title: t('用户'),
        key: 'username',
        width: 180,
        render: (_, record) => {
          if (!record?.username) {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Space spacing={8} align='center'>
              <Avatar size='extra-small' color={stringToColor(record.username)}>
                {record.username.slice(0, 1).toUpperCase()}
              </Avatar>
              <div className='flex flex-col leading-5'>
                <Text type='tertiary' size='small'>
                  ID: {record.user_id}
                </Text>
                <Text
                  link
                  size='small'
                  style={{ cursor: 'pointer' }}
                  onClick={() => filterByUsername(record.username)}
                >
                  {record.username}
                </Text>
                {record.display_name ? (
                  <Text type='tertiary'>{record.display_name}</Text>
                ) : null}
              </div>
            </Space>
          );
        },
      });
    }

    columns.push(
      {
        title: t('发票抬头'),
        key: 'title',
        width: 240,
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Space wrap>
              <Tag
                shape='circle'
                size='small'
                color={record?.invoice_type === 'special' ? 'red' : 'green'}
              >
                {t(getInvoiceTypeLabel(record?.invoice_type))}
              </Tag>
              <Tag shape='circle' size='small' color='blue'>
                {t(record?.title_type === 'company' ? '企业' : '个人')}
              </Tag>
              <Tag
                shape='circle'
                size='small'
                color={
                  record?.source_type === 'manual_transfer' ? 'orange' : 'grey'
                }
              >
                {t(
                  record?.source_type === 'manual_transfer'
                    ? '转账'
                    : '平台订单',
                )}
              </Tag>
              <Text
                strong
                copyable={{ content: buildInvoiceTitleCopyText(record) }}
              >
                {record?.title || '-'}
              </Text>
            </Space>
            {record?.need_detail_bill !== false ? (
              <Tag shape='circle' size='small' color='cyan'>
                {t('已申请明细账单')}
              </Tag>
            ) : null}
            {record?.need_service_confirmation ? (
              <Tag shape='circle' size='small' color='purple'>
                {t('已申请产品明细清单')}
              </Tag>
            ) : null}
            {record?.tax_number ? (
              <Text
                type='tertiary'
                size='small'
                copyable={{ content: buildInvoiceTitleCopyText(record) }}
              >
                {record.tax_number}
              </Text>
            ) : null}
          </div>
        ),
      },
      {
        title: t('申请金额'),
        key: 'total',
        width: 140,
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Text type='danger'>{formatMoney(record?.total_money)}</Text>
            {Number(record?.total_quota || 0) > 0 ? (
              <Text type='tertiary' size='small'>
                {renderQuota(record.total_quota)}
              </Text>
            ) : null}
          </div>
        ),
      },
      {
        title: t('订单'),
        key: 'items',
        width: 260,
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Text type='tertiary' size='small'>
              {(record?.items || []).length} {t('笔订单')}
            </Text>
            {(record?.items || []).slice(0, 2).map((item) => (
              <Text key={item.id} copyable ellipsis style={{ maxWidth: 220 }}>
                {item.trade_no || `${item.order_type}-${item.order_id}`}
              </Text>
            ))}
          </div>
        ),
      },
      {
        title: t('接收方式'),
        key: 'contact',
        width: 180,
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Text copyable>{record?.email || '-'}</Text>
            {record?.phone ? (
              <Text type='tertiary' size='small'>
                {record.phone}
              </Text>
            ) : null}
          </div>
        ),
      },
      {
        title: t('发票信息'),
        key: 'invoice_info',
        width: 240,
        render: (_, record) => {
          const fileUrl = getInvoiceFileUrl(record);
          const hasInvoiceInfo =
            fileUrl ||
            record?.invoice_url ||
            record?.invoice_send_status ||
            record?.invoice_send_error;
          return (
            <div className='flex flex-col gap-1'>
              {!hasInvoiceInfo ? <Text type='tertiary'>-</Text> : null}
              <Space wrap spacing={4}>
                {fileUrl ? (
                  <Button
                    size='small'
                    theme='borderless'
                    style={{ padding: 0 }}
                    onClick={() => viewInvoicePdf(record)}
                  >
                    {t('查看 PDF')}
                  </Button>
                ) : null}
                {record?.invoice_url ? (
                  <a href={record.invoice_url} target='_blank' rel='noreferrer'>
                    {t('查看链接')}
                  </a>
                ) : null}
              </Space>
              {record?.invoice_send_status ? (
                <Space wrap spacing={4}>
                  {renderInvoiceSendStatusTag(record.invoice_send_status)}
                  {record?.invoice_sent_to ? (
                    <Text type='tertiary' size='small' copyable>
                      {record.invoice_sent_to}
                    </Text>
                  ) : null}
                </Space>
              ) : null}
              {record?.invoice_send_status === 'failed' &&
              record?.invoice_send_error ? (
                <Text
                  type='danger'
                  size='small'
                  ellipsis={{ showTooltip: true }}
                  style={{ maxWidth: 200 }}
                >
                  {record.invoice_send_error}
                </Text>
              ) : null}
            </div>
          );
        },
      },
      {
        title: t('提交时间'),
        dataIndex: 'created_at',
        key: 'created_at',
        width: 150,
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('备注'),
        key: 'remark',
        render: (_, record) => record?.admin_remark || record?.remark || '-',
      },
    );

    if (isReviewMode) {
      columns.push({
        title: t('操作'),
        key: 'action',
        width: 260,
        render: (_, record) => (
          <Space wrap>
            <Button
              size='small'
              theme='outline'
              onClick={() => viewInvoiceDetailBill(record)}
            >
              {t('明细账单')}
            </Button>
            <Button
              size='small'
              theme='outline'
              onClick={() => viewInvoiceServiceConfirmation(record)}
            >
              {t('产品明细清单')}
            </Button>
            {record?.status === 'pending' ? (
              <>
                <Button
                  size='small'
                  type='primary'
                  theme='outline'
                  onClick={() => openInvoiceReviewModal(record, 'approve')}
                >
                  {t('通过')}
                </Button>
                <Button
                  size='small'
                  type='danger'
                  theme='outline'
                  onClick={() => openInvoiceReviewModal(record, 'reject')}
                >
                  {t('驳回')}
                </Button>
              </>
            ) : null}
            {record?.status === 'invoiced' && record?.invoice_file_name ? (
              <Button
                size='small'
                type='warning'
                theme='outline'
                onClick={() => openResendInvoiceEmailModal(record)}
              >
                {t('重新发送')}
              </Button>
            ) : null}
          </Space>
        ),
      });
    }

    return columns;
  }, [userIsAdmin, activeTab, t]);

  const renderRecordFilterPanel = () => (
    <div className='mb-3'>
      <div className='flex items-center gap-2'>
        <Input
          prefix={<IconSearch />}
          placeholder={t('订单号 / 商品名')}
          value={filters.keyword}
          onChange={(value) => handleFilterChange('keyword', value)}
          onEnterPress={() => applyFilters()}
          showClear
          style={{ flex: 1 }}
        />
        <Button
          icon={<IconFilter />}
          theme={showFilters ? 'solid' : 'light'}
          type={activeFilterTags.length > 0 ? 'primary' : 'tertiary'}
          onClick={() => setShowFilters((current) => !current)}
        >
          {t('筛选')}
          {activeFilterTags.length > 0 ? ` (${activeFilterTags.length})` : ''}
        </Button>
        <Button type='primary' onClick={() => applyFilters()}>
          {t('搜索')}
        </Button>
        {activeFilterTags.length > 0 ? (
          <Button theme='borderless' type='tertiary' onClick={resetFilters}>
            {t('重置')}
          </Button>
        ) : null}
      </div>

      {!showFilters && activeFilterTags.length > 0 ? (
        <div className='flex flex-wrap items-center gap-1 mt-2'>
          {activeFilterTags.map((tag) => (
            <Tag
              key={tag.key}
              closable
              size='small'
              color='blue'
              shape='circle'
              onClose={() => removeFilterTag(tag.key)}
            >
              {tag.label}
            </Tag>
          ))}
        </div>
      ) : null}

      <Collapsible isOpen={showFilters} keepDOM>
        <div
          className='mt-2 rounded-lg p-3 flex flex-wrap items-end gap-3'
          style={{
            background: 'var(--semi-color-fill-0)',
            border: '1px solid var(--semi-color-border)',
          }}
        >
          {userIsAdmin ? (
            <div style={{ minWidth: 160, flex: 1 }}>
              <div
                className='text-xs mb-1'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                {t('ID/用户名')}
              </div>
              <Input
                placeholder={t('ID/用户名')}
                value={filters.username}
                onChange={(value) => handleFilterChange('username', value)}
                onEnterPress={() => applyFilters()}
                showClear
                size='small'
              />
            </div>
          ) : null}
          <div style={{ minWidth: 120, flex: 1 }}>
            <div
              className='text-xs mb-1'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t('状态')}
            </div>
            <Select
              value={filters.status}
              optionList={STATUS_OPTIONS.map((item) => ({
                ...item,
                label: t(item.label),
              }))}
              onChange={(value) => handleFilterChange('status', value)}
              size='small'
              style={{ width: '100%' }}
            />
          </div>
          <div style={{ minWidth: 130, flex: 1 }}>
            <div
              className='text-xs mb-1'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t('支付方式')}
            </div>
            <Select
              value={filters.paymentMethod}
              optionList={PAYMENT_OPTIONS.map((item) => ({
                ...item,
                label: t(item.label),
              }))}
              onChange={(value) => handleFilterChange('paymentMethod', value)}
              size='small'
              style={{ width: '100%' }}
            />
          </div>
        </div>
      </Collapsible>
    </div>
  );

  const renderRiskFilterPanel = () => (
    <div className='mb-3'>
      <div
        className='rounded-lg p-3 flex flex-wrap items-end gap-3'
        style={{
          background: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <div style={{ minWidth: 220, flex: 2 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('订单号')}
          </div>
          <Input
            prefix={<IconSearch />}
            placeholder={t('订单号')}
            value={riskFilters.keyword}
            onChange={(value) => handleRiskFilterChange('keyword', value)}
            onEnterPress={() => applyRiskFilters()}
            showClear
            size='small'
          />
        </div>
        <div style={{ minWidth: 160, flex: 1 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('ID/用户名')}
          </div>
          <Input
            placeholder={t('ID/用户名')}
            value={riskFilters.username}
            onChange={(value) => handleRiskFilterChange('username', value)}
            onEnterPress={() => applyRiskFilters()}
            showClear
            size='small'
          />
        </div>
        <div style={{ minWidth: 120, flex: 1 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('状态')}
          </div>
          <Select
            value={riskFilters.status}
            optionList={RISK_STATUS_OPTIONS.map((item) => ({
              ...item,
              label: t(item.label),
            }))}
            onChange={(value) => handleRiskFilterChange('status', value)}
            size='small'
            style={{ width: '100%' }}
          />
        </div>
        <div style={{ minWidth: 130, flex: 1 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('订单类型')}
          </div>
          <Select
            value={riskFilters.recordType}
            optionList={RISK_RECORD_TYPE_OPTIONS.map((item) => ({
              ...item,
              label: t(item.label),
            }))}
            onChange={(value) => handleRiskFilterChange('recordType', value)}
            size='small'
            style={{ width: '100%' }}
          />
        </div>
        <div style={{ minWidth: 160, flex: 1 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('异常原因')}
          </div>
          <Select
            value={riskFilters.reason}
            optionList={RISK_REASON_OPTIONS.map((item) => ({
              ...item,
              label: t(item.label),
            }))}
            onChange={(value) => handleRiskFilterChange('reason', value)}
            size='small'
            style={{ width: '100%' }}
          />
        </div>
        <Space>
          <Button type='primary' onClick={() => applyRiskFilters()}>
            {t('搜索')}
          </Button>
          <Button theme='borderless' type='tertiary' onClick={resetRiskFilters}>
            {t('重置')}
          </Button>
        </Space>
      </div>
    </div>
  );

  const renderWithdrawalFilterPanel = () => (
    <div className='mb-3'>
      <div
        className='rounded-lg p-3 flex flex-wrap items-end gap-3'
        style={{
          background: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
        }}
      >
        {userIsAdmin && activeTab === 'withdrawals' ? (
          <div style={{ minWidth: 180, flex: 1 }}>
            <div
              className='text-xs mb-1'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t('ID/用户名')}
            </div>
            <Input
              placeholder={t('ID/用户名')}
              value={withdrawalFilters.username}
              onChange={(value) =>
                handleWithdrawalFilterChange('username', value)
              }
              onEnterPress={() => applyWithdrawalFilters()}
              showClear
              size='small'
            />
          </div>
        ) : null}
        <div style={{ minWidth: 140, flex: 1 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('状态')}
          </div>
          <Select
            value={withdrawalFilters.status}
            optionList={WITHDRAWAL_STATUS_OPTIONS.map((item) => ({
              ...item,
              label: t(item.label),
            }))}
            onChange={(value) => handleWithdrawalFilterChange('status', value)}
            size='small'
            style={{ width: '100%' }}
          />
        </div>
        <Space>
          <Button type='primary' onClick={() => applyWithdrawalFilters()}>
            {t('搜索')}
          </Button>
          <Button
            theme='borderless'
            type='tertiary'
            onClick={resetWithdrawalFilters}
          >
            {t('重置')}
          </Button>
        </Space>
      </div>
    </div>
  );

  const renderInvoiceFilterPanel = () => (
    <div className='mb-3 flex flex-wrap items-end gap-3'>
      {userIsAdmin ? (
        <div style={{ minWidth: 180, flex: 1 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('ID/用户名')}
          </div>
          <Input
            placeholder={t('ID/用户名')}
            value={invoiceFilters.username}
            onChange={(value) => handleInvoiceFilterChange('username', value)}
            onEnterPress={() => applyInvoiceFilters()}
            showClear
            size='small'
          />
        </div>
      ) : null}
      <div style={{ minWidth: 140, flex: 1 }}>
        <div
          className='text-xs mb-1'
          style={{ color: 'var(--semi-color-text-2)' }}
        >
          {t('状态')}
        </div>
        <Select
          value={invoiceFilters.status}
          optionList={INVOICE_STATUS_OPTIONS.map((item) => ({
            ...item,
            label: t(item.label),
          }))}
          onChange={(value) => handleInvoiceFilterChange('status', value)}
          size='small'
          style={{ width: '100%' }}
        />
      </div>
      <Space>
        <Button type='primary' onClick={() => applyInvoiceFilters()}>
          {t('搜索')}
        </Button>
        <Button
          theme='borderless'
          type='tertiary'
          onClick={resetInvoiceFilters}
        >
          {t('重置')}
        </Button>
        {!userIsAdmin ? (
          <Button
            type='primary'
            theme='outline'
            onClick={() => openInvoiceApplyModal()}
          >
            {t('申请发票')}
          </Button>
        ) : null}
      </Space>
    </div>
  );

  const renderRecordsTable = () => (
    <>
      {renderRecordFilterPanel()}
      <Table
        columns={recordColumns}
        dataSource={topups}
        loading={loading}
        rowKey={(record) =>
          `${record?.record_type || 'topup'}-${record?.id || '0'}`
        }
        size='small'
        pagination={{
          currentPage: page,
          pageSize,
          total,
          showSizeChanger: true,
          pageSizeOpts: [10, 20, 50, 100],
          onPageChange: (currentPage) => setPage(currentPage),
          onPageSizeChange: (currentPageSize) => {
            setPageSize(currentPageSize);
            setPage(1);
          },
        }}
        scroll={{ x: '100%' }}
        empty={buildTableEmpty(t, '暂无支付记录')}
      />
    </>
  );

  const renderDashboardBoard = () => (
    <div className='space-y-4'>
      <div
        className='rounded-lg p-3 flex flex-wrap items-end gap-3'
        style={{
          background: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <div style={{ minWidth: 260, flex: 2 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('时间范围')}
          </div>
          <Space wrap>
            {DASHBOARD_PRESET_OPTIONS.map((item) => (
              <Button
                key={item.key}
                size='small'
                type={dashboardPreset === item.key ? 'primary' : 'tertiary'}
                theme={dashboardPreset === item.key ? 'solid' : 'outline'}
                onClick={() => handleDashboardPresetChange(item.key)}
              >
                {t(item.label)}
              </Button>
            ))}
          </Space>
        </div>
        <div style={{ minWidth: 280, flex: 2 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('自定义时间')}
          </div>
          <DatePicker
            type='dateTimeRange'
            value={dashboardDateRange}
            onChange={handleDashboardDateRangeChange}
            style={{ width: '100%' }}
          />
        </div>
        <div style={{ minWidth: 120 }}>
          <div
            className='text-xs mb-1'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('榜单条数')}
          </div>
          <Select
            value={dashboardRankLimit}
            optionList={DASHBOARD_RANK_LIMIT_OPTIONS.map((item) => ({
              ...item,
              label: item.label,
            }))}
            onChange={(value) => setDashboardRankLimit(Number(value || 10))}
            size='small'
            style={{ width: '100%' }}
          />
        </div>
        <Button
          type='primary'
          loading={dashboardLoading}
          onClick={refreshDashboard}
        >
          {t('刷新')}
        </Button>
      </div>

      <div
        className='grid gap-3'
        style={{
          gridTemplateColumns: isMobile
            ? '1fr'
            : 'repeat(auto-fit, minmax(180px, 1fr))',
        }}
      >
        {dashboardSummaryCards.map((item) => (
          <Card
            key={item.key}
            bordered={false}
            bodyStyle={{ padding: 16 }}
            style={{
              background: 'var(--semi-color-bg-1)',
              border: '1px solid var(--semi-color-border)',
            }}
          >
            <Text type='tertiary' size='small'>
              {t(item.label)}
            </Text>
            <div className='mt-2 text-xl font-semibold'>{item.value}</div>
            <Text type='tertiary' size='small'>
              {item.helper}
            </Text>
          </Card>
        ))}
      </div>

      <div
        className='grid gap-4'
        style={{
          gridTemplateColumns: isMobile
            ? '1fr'
            : 'minmax(0, 1.6fr) minmax(280px, 1fr)',
        }}
      >
        <Card
          bordered={false}
          bodyStyle={{ padding: 0 }}
          style={{
            background: 'var(--semi-color-bg-1)',
            border: '1px solid var(--semi-color-border)',
          }}
          title={t('充值金额榜单')}
        >
          <Table
            columns={dashboardRankingColumns}
            dataSource={dashboardRankings}
            loading={dashboardLoading}
            rowKey={(record) =>
              String(record?.user_id || record?.username || '')
            }
            size='small'
            pagination={false}
            scroll={{ x: '100%' }}
            empty={buildTableEmpty(t, '暂无充值榜单数据')}
          />
        </Card>

        <Card
          bordered={false}
          bodyStyle={{ padding: 16 }}
          style={{
            background: 'var(--semi-color-bg-1)',
            border: '1px solid var(--semi-color-border)',
          }}
          title={t('支付方式分布')}
        >
          {dashboardPaymentMethods.length === 0 ? (
            buildTableEmpty(t, '暂无统计数据')
          ) : (
            <div className='space-y-3'>
              {dashboardPaymentMethods.map((item) => (
                <div
                  key={item.method}
                  className='flex items-center justify-between gap-3'
                >
                  <div className='flex items-center gap-2'>
                    {renderPaymentMethod(item.method)}
                    <Text type='tertiary' size='small'>
                      {formatCount(item.orderCount)} {t('单')}
                    </Text>
                  </div>
                  <Text strong>
                    {formatMoneyAmounts(item.amounts, item.money)}
                  </Text>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>
    </div>
  );

  const renderRiskCaseTable = () => (
    <>
      {renderRiskFilterPanel()}
      <Table
        columns={riskCaseColumns}
        dataSource={riskCases}
        loading={riskLoading}
        rowKey={(record) => String(record?.id || 0)}
        size='small'
        pagination={{
          currentPage: riskPage,
          pageSize: riskPageSize,
          total: riskTotal,
          showSizeChanger: true,
          pageSizeOpts: [10, 20, 50, 100],
          onPageChange: (currentPage) => setRiskPage(currentPage),
          onPageSizeChange: (currentPageSize) => {
            setRiskPageSize(currentPageSize);
            setRiskPage(1);
          },
        }}
        scroll={{ x: '100%' }}
        empty={buildTableEmpty(t, '暂无异常单')}
      />
    </>
  );

  const renderWithdrawalTable = () => (
    <>
      {renderWithdrawalFilterPanel()}
      <Table
        columns={withdrawalColumns}
        dataSource={withdrawals}
        loading={withdrawalLoading}
        rowKey={(record) => String(record?.id || 0)}
        size='small'
        pagination={{
          currentPage: withdrawalPage,
          pageSize: withdrawalPageSize,
          total: withdrawalTotal,
          showSizeChanger: true,
          pageSizeOpts: [10, 20, 50, 100],
          onPageChange: (currentPage) => setWithdrawalPage(currentPage),
          onPageSizeChange: (currentPageSize) => {
            setWithdrawalPageSize(currentPageSize);
            setWithdrawalPage(1);
          },
        }}
        scroll={{ x: '100%' }}
        empty={buildTableEmpty(
          t,
          activeTab === 'withdrawals' ? '暂无提现审核记录' : '暂无提现订单',
        )}
      />
    </>
  );

  const renderInvoiceDetailValue = (label, content, copyable = false) => {
    const empty = content === undefined || content === null || content === '';
    return (
      <div
        className='rounded-lg p-3'
        style={{
          background: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <Text type='tertiary' size='small'>
          {t(label)}
        </Text>
        <div className='mt-1 break-all'>
          {React.isValidElement(content) ? (
            content
          ) : (
            <Text copyable={copyable && !empty}>{displayValue(content)}</Text>
          )}
        </div>
      </div>
    );
  };

  const renderInvoiceDetailModalContent = () => {
    const detail = invoiceDetail;
    if (!detail && !invoiceDetailLoading) {
      return buildTableEmpty(t, '暂无发票详情');
    }

    const items = detail?.items || [];
    const productItems = detail?.product_items || [];
    const infoGridStyle = {
      gridTemplateColumns: isMobile ? '1fr' : 'repeat(3, minmax(0, 1fr))',
    };

    return (
      <div className='space-y-4'>
        <Card
          title={t('申请信息')}
          bordered={false}
          bodyStyle={{ padding: 12 }}
          style={{ border: '1px solid var(--semi-color-border)' }}
        >
          <div className='grid gap-3' style={infoGridStyle}>
            {renderInvoiceDetailValue(
              '申请编号',
              `#${detail?.id || '-'}`,
              true,
            )}
            {renderInvoiceDetailValue(
              '状态',
              renderInvoiceStatusTag(detail?.status),
            )}
            {renderInvoiceDetailValue(
              '申请时间',
              formatInvoiceTime(detail?.created_at),
            )}
            {renderInvoiceDetailValue(
              '申请用户',
              formatInvoiceUser(detail),
              true,
            )}
            {renderInvoiceDetailValue(
              '凭证来源',
              detail?.source_type === 'manual_transfer'
                ? t('转账')
                : t('平台订单'),
            )}
            {renderInvoiceDetailValue(
              '交易数量',
              `${items.length} ${t('笔交易')}`,
            )}
            {renderInvoiceDetailValue(
              '合计支付金额',
              <Text type='danger'>{formatMoney(detail?.total_money)}</Text>,
            )}
            {renderInvoiceDetailValue(
              '合计额度',
              Number(detail?.total_quota || 0) > 0
                ? renderQuota(detail.total_quota)
                : '-',
            )}
            {renderInvoiceDetailValue(
              '手续费',
              Number(detail?.service_fee_quota || 0) > 0 ? (
                <Text type='warning'>
                  {renderQuota(detail.service_fee_quota)}
                  {detail?.status === 'rejected' ? `（${t('已退还')}）` : ''}
                </Text>
              ) : (
                '-'
              ),
            )}
            {renderInvoiceDetailValue(
              '明细账单',
              detail?.need_detail_bill !== false
                ? t('需要随发票发送')
                : t('不需要'),
            )}
            {renderInvoiceDetailValue(
              '产品明细清单',
              detail?.need_service_confirmation
                ? t('需要随发票发送')
                : t('不需要'),
            )}
          </div>
          {isManualInvoice(detail) ? (
            <div
              className='mt-3 rounded-lg p-3'
              style={{
                background: 'var(--semi-color-warning-light-default)',
                border: '1px solid var(--semi-color-warning)',
              }}
            >
              <Text type='warning'>
                {t(
                  '该申请包含人工录入的银行转账资料，审核时必须核对流水号、付款方、金额和转账时间；该记录不会影响钱包余额。',
                )}
              </Text>
            </div>
          ) : null}
        </Card>

        <Card
          title={t('发票抬头与接收信息')}
          bordered={false}
          bodyStyle={{ padding: 12 }}
          style={{ border: '1px solid var(--semi-color-border)' }}
        >
          <div className='grid gap-3' style={infoGridStyle}>
            {renderInvoiceDetailValue(
              '发票类型',
              t(getInvoiceTypeLabel(detail?.invoice_type)),
            )}
            {renderInvoiceDetailValue(
              '抬头类型',
              t(detail?.title_type === 'company' ? '企业' : '个人'),
            )}
            {renderInvoiceDetailValue(
              detail?.invoice_type === 'special' ? '单位名称' : '抬头名称',
              <Text copyable={{ content: buildInvoiceTitleCopyText(detail) }}>
                {displayValue(detail?.title)}
              </Text>,
            )}
            {renderInvoiceDetailValue(
              '税号',
              <Text copyable={{ content: buildInvoiceTitleCopyText(detail) }}>
                {displayValue(detail?.tax_number)}
              </Text>,
            )}
            {renderInvoiceDetailValue(
              '注册地址',
              detail?.registered_address,
              true,
            )}
            {renderInvoiceDetailValue(
              '注册电话',
              detail?.registered_phone,
              true,
            )}
            {renderInvoiceDetailValue('开户银行', detail?.bank_name, true)}
            {renderInvoiceDetailValue('银行账号', detail?.bank_account, true)}
            {renderInvoiceDetailValue('接收邮箱', detail?.email, true)}
            {renderInvoiceDetailValue('手机号', detail?.phone, true)}
            {renderInvoiceDetailValue('用户备注', detail?.remark)}
          </div>
        </Card>

        <Card
          title={t('交易明细')}
          bordered={false}
          bodyStyle={{ padding: 0 }}
          style={{ border: '1px solid var(--semi-color-border)' }}
        >
          <Table
            columns={invoiceDetailItemColumns}
            dataSource={items}
            loading={invoiceDetailLoading}
            rowKey={(item) => String(item?.id || getInvoiceItemKey(item))}
            size='small'
            pagination={false}
            scroll={{ x: '100%' }}
            empty={buildTableEmpty(t, '暂无交易明细')}
          />
        </Card>

        <Card
          title={t('产品明细清单')}
          bordered={false}
          bodyStyle={{ padding: 0 }}
          style={{ border: '1px solid var(--semi-color-border)' }}
        >
          <Table
            columns={invoiceProductItemColumns}
            dataSource={productItems}
            loading={invoiceDetailLoading}
            rowKey={(item, index) => String(item?.id || `product-${index}`)}
            size='small'
            pagination={false}
            scroll={{ x: '100%' }}
            empty={buildTableEmpty(t, '暂无产品明细')}
          />
        </Card>

        <Card
          title={t('审核与开票信息')}
          bordered={false}
          bodyStyle={{ padding: 12 }}
          style={{ border: '1px solid var(--semi-color-border)' }}
        >
          <div className='grid gap-3' style={infoGridStyle}>
            {renderInvoiceDetailValue(
              '审核人',
              formatInvoiceReviewer(detail),
              true,
            )}
            {renderInvoiceDetailValue(
              '审核时间',
              formatInvoiceTime(detail?.reviewed_at),
            )}
            {renderInvoiceDetailValue('发票链接', detail?.invoice_url, true)}
            {renderInvoiceDetailValue(
              '发票 PDF',
              getInvoiceFileUrl(detail) ? (
                <Button
                  size='small'
                  theme='borderless'
                  style={{ padding: 0 }}
                  onClick={() => viewInvoicePdf(detail)}
                >
                  {detail?.invoice_file_name || t('查看 PDF')}
                </Button>
              ) : (
                '-'
              ),
            )}
            {renderInvoiceDetailValue(
              '邮件状态',
              detail?.invoice_send_status
                ? renderInvoiceSendStatusTag(detail.invoice_send_status)
                : '-',
            )}
            {renderInvoiceDetailValue(
              '发送邮箱',
              detail?.invoice_sent_to,
              true,
            )}
            {renderInvoiceDetailValue(
              '发送时间',
              formatInvoiceTime(detail?.invoice_sent_at),
            )}
            {renderInvoiceDetailValue('发送错误', detail?.invoice_send_error)}
            {renderInvoiceDetailValue(
              '管理员备注/驳回原因',
              detail?.admin_remark,
            )}
          </div>
        </Card>
      </div>
    );
  };

  const renderInvoiceTable = () => (
    <>
      {renderInvoiceFilterPanel()}
      <Table
        columns={invoiceColumns}
        dataSource={invoices}
        loading={invoiceLoading}
        rowKey={(record) => String(record?.id || 0)}
        size='small'
        pagination={{
          currentPage: invoicePage,
          pageSize: invoicePageSize,
          total: invoiceTotal,
          showSizeChanger: true,
          pageSizeOpts: [10, 20, 50, 100],
          onPageChange: (currentPage) => setInvoicePage(currentPage),
          onPageSizeChange: (currentPageSize) => {
            setInvoicePageSize(currentPageSize);
            setInvoicePage(1);
          },
        }}
        scroll={{ x: '100%' }}
        empty={buildTableEmpty(t, '暂无发票申请')}
      />
    </>
  );

  return (
    <>
      <Modal
        title={t('支付记录')}
        visible={visible}
        onCancel={onCancel}
        footer={null}
        size={isMobile ? 'full-width' : 'large'}
        style={isMobile ? undefined : { width: '1400px', maxWidth: '95vw' }}
      >
        {userIsAdmin ? (
          <Tabs type='card' activeKey={activeTab} onChange={setActiveTab}>
            <Tabs.TabPane tab={t('支付记录')} itemKey='records'>
              {renderRecordsTable()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('发票')} itemKey='invoices'>
              {renderInvoiceTable()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('我的提现记录')} itemKey='my-withdrawals'>
              {renderWithdrawalTable()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('提现审核')} itemKey='withdrawals'>
              {renderWithdrawalTable()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('对账看板')} itemKey='dashboard'>
              {renderDashboardBoard()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('异常单')} itemKey='risk'>
              {renderRiskCaseTable()}
            </Tabs.TabPane>
          </Tabs>
        ) : (
          <Tabs type='card' activeKey={activeTab} onChange={setActiveTab}>
            <Tabs.TabPane tab={t('支付记录')} itemKey='records'>
              {renderRecordsTable()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('发票')} itemKey='invoices'>
              {renderInvoiceTable()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('提现订单')} itemKey='my-withdrawals'>
              {renderWithdrawalTable()}
            </Tabs.TabPane>
          </Tabs>
        )}
      </Modal>

      <PaymentRiskCaseDetailModal
        visible={riskDetailVisible}
        riskCaseId={selectedRiskCaseId}
        initialRiskCase={selectedRiskCaseSeed}
        onCancel={() => setRiskDetailVisible(false)}
        onResolved={handleRiskCaseResolved}
        t={t}
      />

      <Modal
        title={t('发票申请详情')}
        visible={invoiceDetailVisible}
        onCancel={closeInvoiceDetail}
        footer={
          <Space>
            <Button onClick={closeInvoiceDetail}>{t('关闭')}</Button>
            <Button
              loading={invoiceDetailLoading}
              disabled={!invoiceDetail}
              onClick={printInvoiceServiceConfirmation}
            >
              {t('产品明细清单')}
            </Button>
            <Button
              type='primary'
              loading={invoiceDetailLoading}
              disabled={!invoiceDetail}
              onClick={printInvoiceDetail}
            >
              {t('打印明细账单')}
            </Button>
          </Space>
        }
        size={isMobile ? 'full-width' : 'large'}
        style={isMobile ? undefined : { width: '1100px', maxWidth: '95vw' }}
      >
        {renderInvoiceDetailModalContent()}
      </Modal>

      <Modal
        title={t('申请发票')}
        visible={invoiceApplyVisible}
        onOk={submitInvoiceRequest}
        onCancel={closeInvoiceApplyModal}
        confirmLoading={invoiceSubmitting}
        maskClosable={false}
        size={isMobile ? 'full-width' : 'large'}
        style={isMobile ? undefined : { width: '1000px', maxWidth: '95vw' }}
      >
        <div className='space-y-4'>
          <Card
            bordered={false}
            bodyStyle={{ padding: 12 }}
            style={{
              background: 'var(--semi-color-fill-0)',
              border: '1px solid var(--semi-color-border)',
            }}
          >
            <div className='text-xs mb-1'>{t('付款凭证来源')}</div>
            <Select
              value={invoiceForm.sourceType}
              optionList={[
                { label: t('平台订单'), value: 'system_order' },
                { label: t('转账'), value: 'manual_transfer' },
              ]}
              onChange={handleInvoiceSourceChange}
              style={{ width: '100%' }}
            />
            <div className='mt-2'>
              <Text type='tertiary' size='small'>
                {t(
                  invoiceForm.sourceType === 'manual_transfer'
                    ? '人工转账仅作为开票凭证，不会自动增加或扣减钱包余额，提交后需管理员核验。'
                    : '平台订单信息将自动复制为不可变的开票快照。',
                )}
              </Text>
            </div>
          </Card>

          <div
            className='grid gap-3'
            style={{
              gridTemplateColumns: isMobile
                ? '1fr'
                : 'repeat(2, minmax(0, 1fr))',
            }}
          >
            <div>
              <div className='text-xs mb-1'>{t('发票类型')}</div>
              <Select
                value={invoiceForm.invoiceType}
                optionList={[
                  { label: t('普票'), value: 'normal' },
                  { label: t('专票'), value: 'special' },
                ]}
                onChange={(value) =>
                  handleInvoiceFormChange('invoiceType', value)
                }
                style={{ width: '100%' }}
              />
            </div>
            <div>
              <div className='text-xs mb-1'>{t('抬头类型')}</div>
              <Select
                value={invoiceForm.titleType}
                disabled={invoiceForm.invoiceType === 'special'}
                optionList={[
                  { label: t('企业'), value: 'company' },
                  { label: t('个人'), value: 'personal' },
                ]}
                onChange={(value) =>
                  handleInvoiceFormChange('titleType', value)
                }
                style={{ width: '100%' }}
              />
            </div>
            <div>
              <div className='text-xs mb-1'>
                {t(
                  invoiceForm.invoiceType === 'special'
                    ? '单位名称'
                    : '发票抬头',
                )}
              </div>
              <Input
                value={invoiceForm.title}
                onChange={(value) => handleInvoiceFormChange('title', value)}
                placeholder={t(
                  invoiceForm.invoiceType === 'special'
                    ? '请输入单位名称'
                    : '请输入发票抬头',
                )}
                maxLength={128}
                showClear
              />
            </div>
            <div>
              <div className='text-xs mb-1'>{t('税号')}</div>
              <Input
                value={invoiceForm.taxNumber}
                onChange={(value) =>
                  handleInvoiceFormChange('taxNumber', value)
                }
                placeholder={t(
                  invoiceForm.invoiceType === 'special'
                    ? '专票必填'
                    : '企业抬头必填',
                )}
                maxLength={64}
                showClear
              />
            </div>
            {invoiceForm.invoiceType === 'special' ? (
              <>
                <div>
                  <div className='text-xs mb-1'>{t('注册地址')}</div>
                  <Input
                    value={invoiceForm.registeredAddress}
                    onChange={(value) =>
                      handleInvoiceFormChange('registeredAddress', value)
                    }
                    placeholder={t('可选')}
                    maxLength={255}
                    showClear
                  />
                </div>
                <div>
                  <div className='text-xs mb-1'>{t('注册电话')}</div>
                  <Input
                    value={invoiceForm.registeredPhone}
                    onChange={(value) =>
                      handleInvoiceFormChange('registeredPhone', value)
                    }
                    placeholder={t('可选')}
                    maxLength={64}
                    showClear
                  />
                </div>
                <div>
                  <div className='text-xs mb-1'>{t('开户银行')}</div>
                  <Input
                    value={invoiceForm.bankName}
                    onChange={(value) =>
                      handleInvoiceFormChange('bankName', value)
                    }
                    placeholder={t('可选')}
                    maxLength={128}
                    showClear
                  />
                </div>
                <div>
                  <div className='text-xs mb-1'>{t('银行账号')}</div>
                  <Input
                    value={invoiceForm.bankAccount}
                    onChange={(value) =>
                      handleInvoiceFormChange('bankAccount', value)
                    }
                    placeholder={t('可选')}
                    maxLength={128}
                    showClear
                  />
                </div>
              </>
            ) : null}
            <div>
              <div className='text-xs mb-1'>{t('接收邮箱')}</div>
              <Input
                value={invoiceForm.email}
                onChange={(value) => handleInvoiceFormChange('email', value)}
                placeholder={t('用于接收电子发票')}
                maxLength={128}
                showClear
              />
            </div>
            <div>
              <div className='text-xs mb-1'>{t('手机号')}</div>
              <Input
                value={invoiceForm.phone}
                onChange={(value) => handleInvoiceFormChange('phone', value)}
                placeholder={t('可选')}
                maxLength={32}
                showClear
              />
            </div>
            <div>
              <div className='text-xs mb-1'>{t('备注')}</div>
              <Input
                value={invoiceForm.remark}
                onChange={(value) => handleInvoiceFormChange('remark', value)}
                placeholder={t('可选')}
                maxLength={1000}
                showClear
              />
            </div>
          </div>

          <Card
            bordered={false}
            bodyStyle={{ padding: 12 }}
            style={{
              background: 'var(--semi-color-fill-0)',
              border: '1px solid var(--semi-color-border)',
            }}
          >
            <div className='flex flex-col gap-2'>
              <Checkbox
                checked={invoiceForm.needDetailBill}
                disabled={invoiceForm.sourceType === 'manual_transfer'}
                onChange={(event) =>
                  handleInvoiceFormChange(
                    'needDetailBill',
                    event.target.checked,
                  )
                }
              >
                {t('同时申请明细账单')}
              </Checkbox>
              <Checkbox
                checked={invoiceForm.needServiceConfirmation}
                disabled={invoiceForm.sourceType === 'manual_transfer'}
                onChange={(event) =>
                  handleInvoiceFormChange(
                    'needServiceConfirmation',
                    event.target.checked,
                  )
                }
              >
                {t('同时申请产品明细清单')}
              </Checkbox>
            </div>
            <div className='mt-1'>
              <Text type='tertiary' size='small'>
                {t('审核通过后可按申请选项随发票邮件一并发送。')}
              </Text>
            </div>
          </Card>

          {invoiceForm.sourceType === 'manual_transfer' ? (
            <>
              <Card
                title={t('银行转账明细')}
                bordered={false}
                bodyStyle={{ padding: 12 }}
                style={{ border: '1px solid var(--semi-color-border)' }}
              >
                <div className='space-y-3'>
                  {invoiceForm.manualTransactions.map((item, index) => (
                    <div
                      key={item.key}
                      className='rounded-lg p-3'
                      style={{
                        background: 'var(--semi-color-fill-0)',
                        border: '1px solid var(--semi-color-border)',
                      }}
                    >
                      <div className='mb-3 flex items-center justify-between gap-2'>
                        <Text strong>
                          {t('转账记录')} #{index + 1}
                        </Text>
                        <Button
                          size='small'
                          type='danger'
                          theme='borderless'
                          disabled={invoiceForm.manualTransactions.length <= 1}
                          onClick={() =>
                            removeManualInvoiceTransaction(item.key)
                          }
                        >
                          {t('删除')}
                        </Button>
                      </div>
                      <div
                        className='grid gap-3'
                        style={{
                          gridTemplateColumns: isMobile
                            ? '1fr'
                            : 'repeat(2, minmax(0, 1fr))',
                        }}
                      >
                        <div>
                          <div className='text-xs mb-1'>{t('银行流水号')}</div>
                          <Input
                            value={item.tradeNo}
                            onChange={(value) =>
                              updateManualInvoiceTransaction(
                                item.key,
                                'tradeNo',
                                value,
                              )
                            }
                            maxLength={255}
                            showClear
                          />
                        </div>
                        <div>
                          <div className='text-xs mb-1'>{t('转账时间')}</div>
                          <DatePicker
                            type='dateTime'
                            value={item.paidAt}
                            onChange={(value) =>
                              updateManualInvoiceTransaction(
                                item.key,
                                'paidAt',
                                value,
                              )
                            }
                            disabledDate={(date) =>
                              date instanceof Date &&
                              date.getTime() > Date.now()
                            }
                            style={{ width: '100%' }}
                          />
                        </div>
                        <div>
                          <div className='text-xs mb-1'>{t('付款方名称')}</div>
                          <Input
                            value={item.payerName}
                            onChange={(value) =>
                              updateManualInvoiceTransaction(
                                item.key,
                                'payerName',
                                value,
                              )
                            }
                            maxLength={128}
                            showClear
                          />
                        </div>
                        <div>
                          <div className='text-xs mb-1'>{t('收款方名称')}</div>
                          <Input
                            value={item.payeeName}
                            disabled
                            maxLength={128}
                          />
                        </div>
                        <div>
                          <div className='text-xs mb-1'>{t('付款银行')}</div>
                          <Input
                            value={item.transferBankName}
                            onChange={(value) =>
                              updateManualInvoiceTransaction(
                                item.key,
                                'transferBankName',
                                value,
                              )
                            }
                            maxLength={128}
                            showClear
                          />
                        </div>
                        <div>
                          <div className='text-xs mb-1'>{t('转账金额')}</div>
                          <Input
                            type='number'
                            value={item.money}
                            onChange={(value) =>
                              updateManualInvoiceTransaction(
                                item.key,
                                'money',
                                value,
                              )
                            }
                            prefix='¥'
                          />
                        </div>
                        <div
                          style={{
                            gridColumn: isMobile ? undefined : '1 / -1',
                          }}
                        >
                          <div className='text-xs mb-1'>{t('转账备注')}</div>
                          <Input
                            value={item.remark}
                            onChange={(value) =>
                              updateManualInvoiceTransaction(
                                item.key,
                                'remark',
                                value,
                              )
                            }
                            maxLength={500}
                            showClear
                          />
                        </div>
                      </div>
                    </div>
                  ))}
                  <Button
                    theme='outline'
                    disabled={
                      invoiceForm.manualTransactions.length >=
                      MAX_MANUAL_INVOICE_ROWS
                    }
                    onClick={addManualInvoiceTransaction}
                  >
                    {t('新增转账记录')}
                  </Button>
                </div>
              </Card>

              <Card
                title={t('产品明细清单')}
                bordered={false}
                bodyStyle={{ padding: 12 }}
                style={{ border: '1px solid var(--semi-color-border)' }}
              >
                <Text type='tertiary'>
                  {t(
                    '系统将自动生成一条“AI API 调用服务”产品明细，数量为 1 项，单价和金额取转账总额；不会自动生成或增加钱包额度。',
                  )}
                </Text>
              </Card>
            </>
          ) : null}

          <Card
            bordered={false}
            bodyStyle={{ padding: 12 }}
            style={{
              background: 'var(--semi-color-fill-0)',
              border: '1px solid var(--semi-color-border)',
            }}
          >
            <Space wrap>
              <Text strong>
                {invoiceForm.sourceType === 'manual_transfer'
                  ? `${t('已录入')} ${invoiceForm.manualTransactions.length} ${t('笔转账')}`
                  : `${t('已选')} ${selectedInvoiceOrders.length} ${t('笔订单')}`}
              </Text>
              <Text type='danger'>
                {t('金额')} {formatMoney(invoiceApplySummary.money)}
              </Text>
              {invoiceApplySummary.quota > 0 ? (
                <Text type='tertiary'>
                  {t('额度')} {renderQuota(invoiceApplySummary.quota)}
                </Text>
              ) : null}
              {estimatedInvoiceFeeQuota > 0 ? (
                <Text type={invoiceFeeInsufficient ? 'danger' : 'warning'}>
                  {t('预计手续费')} {renderQuota(estimatedInvoiceFeeQuota)}
                </Text>
              ) : null}
              {invoiceFeeInsufficient ? (
                <Text type='danger'>
                  {t('余额不足，无法支付发票手续费')}（{t('当前余额')}{' '}
                  {renderQuota(userWalletQuota)}）
                </Text>
              ) : null}
            </Space>
          </Card>

          {invoiceForm.sourceType === 'system_order' ? (
            <Table
              columns={invoiceOrderColumns}
              dataSource={eligibleInvoiceOrders}
              loading={eligibleInvoiceLoading}
              rowKey={(record) => getInvoiceOrderKey(record)}
              size='small'
              pagination={false}
              scroll={{ x: '100%' }}
              rowSelection={{
                selectedRowKeys: selectedInvoiceOrderKeys,
                onChange: (keys) => setSelectedInvoiceOrderKeys(keys),
              }}
              empty={buildTableEmpty(t, '暂无可开票订单')}
            />
          ) : null}
        </div>
      </Modal>

      <Modal
        title={t(
          invoiceReviewState.action === 'approve'
            ? '通过发票申请'
            : '驳回发票申请',
        )}
        visible={invoiceReviewState.visible}
        onOk={submitInvoiceReview}
        onCancel={closeInvoiceReviewModal}
        confirmLoading={invoiceReviewSubmitting}
        cancelButtonProps={{ disabled: invoiceReviewSubmitting }}
        closable={!invoiceReviewSubmitting}
        closeOnEsc={!invoiceReviewSubmitting}
        maskClosable={false}
        okButtonProps={
          invoiceReviewState.action === 'reject'
            ? { type: 'danger' }
            : undefined
        }
      >
        <fieldset
          disabled={invoiceReviewSubmitting}
          className='space-y-3 border-0 m-0 p-0 min-w-0'
        >
          {invoiceReviewState.action === 'approve' ? (
            <>
              <Input
                placeholder={t('发票链接')}
                value={invoiceReviewForm.invoiceUrl}
                onChange={(value) =>
                  setInvoiceReviewForm((prev) => ({
                    ...prev,
                    invoiceUrl: value,
                  }))
                }
                showClear
              />
              <Input
                placeholder={t('接收邮箱')}
                value={invoiceReviewForm.invoiceSentTo}
                onChange={(value) =>
                  setInvoiceReviewForm((prev) => ({
                    ...prev,
                    invoiceSentTo: value,
                  }))
                }
                maxLength={128}
                showClear
              />
              <div
                className='rounded-lg p-3'
                style={{
                  background: 'var(--semi-color-fill-0)',
                  border: '1px solid var(--semi-color-border)',
                }}
              >
                <div className='text-xs mb-2'>{t('发票 PDF')}</div>
                <input
                  type='file'
                  accept='application/pdf,.pdf'
                  onChange={(event) =>
                    setInvoiceReviewFile(event.target.files?.[0] || null)
                  }
                />
                <div className='mt-2'>
                  {invoiceReviewFile ? (
                    <Text size='small' copyable>
                      {invoiceReviewFile.name}
                    </Text>
                  ) : (
                    <Text type='tertiary' size='small'>
                      {t('请选择需要发送给用户的发票 PDF，最大 10MB')}
                    </Text>
                  )}
                </div>
              </div>
              <Checkbox
                checked={invoiceReviewForm.sendEmail}
                onChange={(event) =>
                  setInvoiceReviewForm((prev) => ({
                    ...prev,
                    sendEmail: event.target.checked,
                  }))
                }
              >
                {t('通过后自动发送到用户邮箱')}
              </Checkbox>
              <Checkbox
                checked={invoiceReviewForm.sendDetailBill}
                disabled={!invoiceReviewForm.sendEmail}
                onChange={(event) =>
                  setInvoiceReviewForm((prev) => ({
                    ...prev,
                    sendDetailBill: event.target.checked,
                  }))
                }
              >
                {t('同时发送明细账单 PDF 附件')}
              </Checkbox>
              {invoiceReviewForm.sendEmail &&
              invoiceReviewForm.sendDetailBill ? (
                <div
                  className='rounded-lg p-3'
                  style={{
                    background: 'var(--semi-color-fill-0)',
                    border: '1px solid var(--semi-color-border)',
                  }}
                >
                  <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
                    <Text type='tertiary' size='small'>
                      {t('发送时自动生成明细账单 PDF；也可手动上传替换。')}
                    </Text>
                    <Button
                      size='small'
                      theme='outline'
                      onClick={() =>
                        viewInvoiceDetailBill(invoiceReviewState.record)
                      }
                    >
                      {t('预览明细账单')}
                    </Button>
                  </div>
                  <input
                    type='file'
                    accept='application/pdf,.pdf'
                    onChange={(event) =>
                      setInvoiceReviewDetailBillFile(
                        event.target.files?.[0] || null,
                      )
                    }
                  />
                  <div className='mt-2'>
                    {invoiceReviewDetailBillFile ? (
                      <Text size='small'>
                        {invoiceReviewDetailBillFile.name}
                      </Text>
                    ) : (
                      <Text type='tertiary' size='small'>
                        {t('未上传替换文件，将自动生成 PDF')}
                      </Text>
                    )}
                  </div>
                </div>
              ) : null}
              <Checkbox
                checked={invoiceReviewForm.sendServiceConfirmation}
                disabled={!invoiceReviewForm.sendEmail}
                onChange={(event) =>
                  setInvoiceReviewForm((prev) => ({
                    ...prev,
                    sendServiceConfirmation: event.target.checked,
                  }))
                }
              >
                {t('同时发送产品明细清单 PDF 附件')}
              </Checkbox>
              {invoiceReviewForm.sendEmail &&
              invoiceReviewForm.sendServiceConfirmation ? (
                <div
                  className='rounded-lg p-3'
                  style={{
                    background: 'var(--semi-color-fill-0)',
                    border: '1px solid var(--semi-color-border)',
                  }}
                >
                  <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
                    <Text type='tertiary' size='small'>
                      {t('发送时自动生成产品明细清单 PDF；也可手动上传替换。')}
                    </Text>
                    <Button
                      size='small'
                      theme='outline'
                      onClick={() =>
                        viewInvoiceServiceConfirmation(
                          invoiceReviewState.record,
                        )
                      }
                    >
                      {t('预览产品明细清单')}
                    </Button>
                  </div>
                  <input
                    type='file'
                    accept='application/pdf,.pdf'
                    onChange={(event) =>
                      setInvoiceReviewServiceConfirmationFile(
                        event.target.files?.[0] || null,
                      )
                    }
                  />
                  <div className='mt-2'>
                    {invoiceReviewServiceConfirmationFile ? (
                      <Text size='small'>
                        {invoiceReviewServiceConfirmationFile.name}
                      </Text>
                    ) : (
                      <Text type='tertiary' size='small'>
                        {t('未上传替换文件，将自动生成 PDF')}
                      </Text>
                    )}
                  </div>
                </div>
              ) : null}
            </>
          ) : null}
          <TextArea
            placeholder={t(
              invoiceReviewState.action === 'approve'
                ? '审核备注，可选'
                : '驳回原因，必填',
            )}
            value={invoiceReviewForm.adminRemark}
            onChange={(value) =>
              setInvoiceReviewForm((prev) => ({
                ...prev,
                adminRemark: value,
              }))
            }
            maxCount={1000}
            autosize
          />
        </fieldset>
      </Modal>

      <Modal
        title={t(
          reviewState.action === 'approve' ? '通过提现申请' : '驳回提现申请',
        )}
        visible={reviewState.visible}
        onOk={submitReviewModal}
        onCancel={closeReviewModal}
        confirmLoading={reviewSubmitting}
        maskClosable={false}
        okButtonProps={
          reviewState.action === 'reject' ? { type: 'danger' } : undefined
        }
      >
        <div className='space-y-3'>
          <div>
            {t(
              reviewState.action === 'approve'
                ? '请确认已通过支付宝完成转账后再点击通过。'
                : '驳回后会把冻结的待使用收益退回给用户。',
            )}
          </div>
          <Input
            placeholder={t('审核备注，可选')}
            value={reviewRemark}
            onChange={setReviewRemark}
            maxLength={255}
            showClear
          />
        </div>
      </Modal>

      <Modal
        title={t('重发发票邮件')}
        size={isMobile ? 'full-width' : 'small'}
        visible={invoiceEmailState.visible}
        onOk={submitResendInvoiceEmail}
        onCancel={closeResendInvoiceEmailModal}
        okText={t('重新发送')}
        confirmLoading={invoiceEmailState.submitting}
        cancelButtonProps={{ disabled: invoiceEmailState.submitting }}
        closable={!invoiceEmailState.submitting}
        closeOnEsc={!invoiceEmailState.submitting}
        maskClosable={false}
      >
        <fieldset
          disabled={invoiceEmailState.submitting}
          className='space-y-3 border-0 m-0 p-0 min-w-0'
        >
          <Text type='secondary'>
            {t(
              '重发时复用已保存的附件，缺失的明细账单和产品明细清单会自动生成；支持修改收件邮箱或上传替换文件。',
            )}
          </Text>
          <Input
            value={invoiceEmailState.invoiceSentTo}
            onChange={(value) =>
              setInvoiceEmailState((prev) => ({
                ...prev,
                invoiceSentTo: value,
              }))
            }
            placeholder={t('发票接收邮箱')}
            type='email'
            maxLength={128}
            showClear
          />
          <Checkbox
            checked={invoiceEmailState.sendDetailBill}
            onChange={(event) =>
              setInvoiceEmailState((prev) => ({
                ...prev,
                sendDetailBill: event.target.checked,
              }))
            }
          >
            {t('同时发送明细账单 PDF 附件')}
          </Checkbox>
          {invoiceEmailState.sendDetailBill ? (
            <div
              className='rounded-lg p-3'
              style={{
                background: 'var(--semi-color-fill-0)',
                border: '1px solid var(--semi-color-border)',
              }}
            >
              <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
                <Text type='tertiary' size='small'>
                  {t(
                    invoiceEmailState.record?.detail_bill_file_name
                      ? '将复用已保存的 PDF；如需更换，可上传新文件。'
                      : '发送时将自动生成并保存此附件，无需另存或上传。',
                  )}
                </Text>
                <Button
                  size='small'
                  theme='outline'
                  onClick={() =>
                    viewInvoiceDetailBill(invoiceEmailState.record)
                  }
                >
                  {t('预览明细账单')}
                </Button>
              </div>
              <input
                type='file'
                className='max-w-full'
                accept='application/pdf,.pdf'
                onChange={(event) =>
                  setInvoiceEmailState((prev) => ({
                    ...prev,
                    detailBillFile: event.target.files?.[0] || null,
                  }))
                }
              />
              <div className='mt-2 break-all'>
                {invoiceEmailState.detailBillFile ? (
                  <Text size='small'>
                    {invoiceEmailState.detailBillFile.name}
                  </Text>
                ) : (
                  <Text type='tertiary' size='small'>
                    {invoiceEmailState.record?.detail_bill_file_name
                      ? t('已保存：{{filename}}', {
                          filename:
                            invoiceEmailState.record.detail_bill_file_name,
                        })
                      : t('未上传替换文件，将自动生成 PDF')}
                  </Text>
                )}
              </div>
            </div>
          ) : null}
          <Checkbox
            checked={invoiceEmailState.sendServiceConfirmation}
            onChange={(event) =>
              setInvoiceEmailState((prev) => ({
                ...prev,
                sendServiceConfirmation: event.target.checked,
              }))
            }
          >
            {t('同时发送产品明细清单 PDF 附件')}
          </Checkbox>
          {invoiceEmailState.sendServiceConfirmation ? (
            <div
              className='rounded-lg p-3'
              style={{
                background: 'var(--semi-color-fill-0)',
                border: '1px solid var(--semi-color-border)',
              }}
            >
              <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
                <Text type='tertiary' size='small'>
                  {t(
                    invoiceEmailState.record?.service_confirmation_file_name
                      ? '将复用已保存的 PDF；如需更换，可上传新文件。'
                      : '发送时将自动生成并保存此附件，无需另存或上传。',
                  )}
                </Text>
                <Button
                  size='small'
                  theme='outline'
                  onClick={() =>
                    viewInvoiceServiceConfirmation(invoiceEmailState.record)
                  }
                >
                  {t('预览产品明细清单')}
                </Button>
              </div>
              <input
                type='file'
                className='max-w-full'
                accept='application/pdf,.pdf'
                onChange={(event) =>
                  setInvoiceEmailState((prev) => ({
                    ...prev,
                    serviceConfirmationFile: event.target.files?.[0] || null,
                  }))
                }
              />
              <div className='mt-2 break-all'>
                {invoiceEmailState.serviceConfirmationFile ? (
                  <Text size='small'>
                    {invoiceEmailState.serviceConfirmationFile.name}
                  </Text>
                ) : (
                  <Text type='tertiary' size='small'>
                    {invoiceEmailState.record?.service_confirmation_file_name
                      ? t('已保存：{{filename}}', {
                          filename:
                            invoiceEmailState.record
                              .service_confirmation_file_name,
                        })
                      : t('未上传替换文件，将自动生成 PDF')}
                  </Text>
                )}
              </div>
            </div>
          ) : null}
        </fieldset>
      </Modal>
    </>
  );
};

export default TopupHistoryModal;
