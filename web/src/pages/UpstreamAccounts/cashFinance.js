export const cashMoney = (value) =>
  typeof value === 'number' && Number.isFinite(value)
    ? `¥${value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`
    : '—';

export function cashDateRange(days = 7) {
  const end = new Date();
  end.setUTCDate(end.getUTCDate() - 1);
  const start = new Date(end);
  start.setUTCDate(start.getUTCDate() - days + 1);
  return [start.toISOString().slice(0, 10), end.toISOString().slice(0, 10)];
}

export const cashRequestID = () => crypto.randomUUID();

export function cashCSV(rows, t, formatQuota = (value) => value) {
  const escape = (value) => {
    let text = String(value ?? '');
    if (/^[\s]*[=+@-]/.test(text)) text = `'${text}`;
    return `"${text.replaceAll('"', '""')}"`;
  };
  return (
    '\uFEFF' +
    [
      [
        t('归集对象'),
        t('分摊净到账收入'),
        t('上游现金成本'),
        t('现金贡献利润'),
        t('请求数'),
        t('本站消耗'),
      ],
      ...rows.map((r) => [
        r.name,
        r.income,
        r.cost,
        r.profit,
        r.requests,
        r.quota == null ? '' : formatQuota(r.quota),
      ]),
    ]
      .map((r) => r.map(escape).join(','))
      .join('\r\n')
  );
}
