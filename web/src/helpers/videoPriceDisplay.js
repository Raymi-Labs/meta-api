import { formatModelPricingUnitPrice } from './modelPricingCurrency.js';

// Do not evaluate arbitrary billing expressions in the browser. Recognize only
// linear duration tiers and retain the existing fallback for everything else.
export function videoSecondRates(expression = '') {
  if (expression.length > 100000) return [];
  const rates = [];
  function scan(text) {
    let depth = 0,
      quoted = false,
      escaped = false;
    const top = [];
    for (let i = 0; i < text.length; i++) {
      const c = text[i];
      if (quoted) {
        if (escaped) escaped = false;
        else if (c === '\\') escaped = true;
        else if (c === '"') quoted = false;
        continue;
      }
      if (c === '"') quoted = true;
      else if (c === '(') depth++;
      else if (c === ')') {
        depth--;
        if (depth < 0) return null;
      }
      if (!quoted && depth === 0) top.push(i);
    }
    return depth || quoted ? null : top;
  }
  function visit(source, nesting = 0) {
    if (nesting > 64) return false;
    let text = source.trim(),
      positions = scan(text);
    if (!positions) return false;
    while (text.startsWith('(') && positions[0] === text.length - 1) {
      text = text.slice(1, -1).trim();
      positions = scan(text);
      if (!positions) return false;
    }
    const question = positions.find(
      (i) => text[i] === '?' && text[i - 1] !== '?' && text[i + 1] !== '?',
    );
    if (question !== undefined) {
      let branches = 0,
        colon;
      for (const i of positions.filter((i) => i > question)) {
        if (text[i] === '?' && text[i - 1] !== '?' && text[i + 1] !== '?')
          branches++;
        if (text[i] === ':') {
          if (branches === 0) {
            colon = i;
            break;
          }
          branches--;
        }
      }
      return (
        colon !== undefined &&
        visit(text.slice(question + 1, colon), nesting + 1) &&
        visit(text.slice(colon + 1), nesting + 1)
      );
    }
    if (
      /^tier\(\s*"(?:video_config_preview|configuration_preview)"\s*,\s*0\s*\)$/.test(
        text,
      ) ||
      /^tier\(\s*"unsupported_video_parameters"\s*,\s*-1\s*\)$/.test(text)
    )
      return true;
    const match = text.match(
      /^tier\(\s*"[^"\n]+"\s*,\s*param\(\s*"duration"\s*\)\s*\*\s*(\d+(?:\.\d+)?)\s*(?:\*\s*\(param\("sampleCount"\)\s*\?\?\s*1\))?\s*\)$/,
    );
    if (!match) return false;
    const price = Number(match[1]) / 1e6;
    if (!Number.isFinite(price) || price < 0) return false;
    rates.push(price);
    return true;
  }
  return visit(expression) ? [...new Set(rates)].sort((a, b) => a - b) : [];
}

export function videoPriceItems(expression, ratio, displayPrice, t = (s) => s) {
  const rates = videoSecondRates(expression);
  if (!rates.length || !Number.isFinite(Number(ratio))) return [];
  const values = rates.map((rate) =>
    formatModelPricingUnitPrice(rate * Number(ratio), displayPrice, 4),
  );
  return [
    {
      key: 'video-second',
      label: t('视频按秒 / 表达式'),
      value:
        values.length === 1
          ? values[0]
          : `${values[0]} – ${values[values.length - 1]}`,
      suffix: ' / s',
      isDynamic: false,
    },
  ];
}

export function isVideoConsumption(record, other) {
  if (
    Number(record?.type) !== 2 ||
    other?.violation_fee === true ||
    other?.violation_fee_code ||
    other?.violation_fee_marker
  )
    return false;
  if (other?.task_billing?.kind === 'video_expression') return true;
  return /^\/v1\/(?:videos(?:\/|$)|video\/generations(?:\/|$))/.test(
    other?.request_path || '',
  );
}

export function videoLogSegments(record, other, quota, t = (s) => s) {
  const result = [
    {
      text: `${t('扣费')}：${quota(Number(record.quota), 6)}`,
      tone: 'primary',
    },
  ];
  const snapshot = other?.task_billing;
  if (
    snapshot &&
    Number.isFinite(snapshot.base_quota) &&
    Number.isFinite(snapshot.group_ratio) &&
    Number.isFinite(snapshot.time_ratio)
  ) {
    result.push({
      text: `${quota(snapshot.base_quota, 6)} × ${snapshot.group_ratio} × ${snapshot.time_ratio} = ${quota(Number(record.quota), 6)}`,
      tone: 'secondary',
    });
  }
  const seconds = Number(other?.task_billing?.requested_seconds);
  if (
    Number.isFinite(seconds) &&
    seconds > 0 &&
    Number.isFinite(Number(record.quota))
  ) {
    result.push({
      text: `${seconds} s · ${quota(Number(record.quota) / seconds, 6)} / s`,
      tone: 'secondary',
    });
  }
  if (other?.task_billing?.matched_tier)
    result.push({
      text: String(other.task_billing.matched_tier),
      tone: 'secondary',
    });
  return result;
}
