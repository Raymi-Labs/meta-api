// References are CNY list prices for Beijing, checked 2026-10-31. They never
// become local selling prices. Model IDs are exact to avoid applying Pro rates
// to other versions, regions or aliases.
export const IMAGE_PRICE_REFERENCES = {
  'qwen-image-3.0-pro': {
    url: 'https://help.aliyun.com/zh/model-studio/qwen-image-3-0-pro',
    rows: [
      { label: '1K', input: 0.02, output: 0.25 },
      { label: '2K', input: 0.02, output: 0.5 },
    ],
  },
  'wan2.7-image-pro': {
    url: 'https://help.aliyun.com/zh/model-studio/wan2-7-image-pro',
    rows: [{ label: '1K / 2K / 4K', output: 0.5 }],
  },
  'z-image-turbo': {
    url: 'https://help.aliyun.com/zh/model-studio/z-image-turbo',
    rows: [
      { label: 'prompt_extend=false', output: 0.1 },
      { label: 'prompt_extend=true', output: 0.2 },
    ],
  },
};
export const supportsImagePricing = (name) =>
  ['qwen-image-3.0', 'qwen-image-3.0-pro'].includes(name);
export const isImagePriceExpression = (expr = '') =>
  expr.includes('tier("image_config_preview"');
export const emptyImagePrices = () => [
  { resolution: '1K', input: '', output: '' },
  { resolution: '2K', input: '', output: '' },
];

export function buildImagePriceExpression(rows) {
  if (
    rows?.length !== 2 ||
    rows.some(
      (r, i) =>
        r.resolution !== ['1K', '2K'][i] ||
        ['input', 'output'].some(
          (k) =>
            r[k] === '' ||
            r[k] == null ||
            !['number', 'string'].includes(typeof r[k]) ||
            String(r[k]).trim() === '' ||
            !Number.isFinite(Number(r[k])) ||
            Number(r[k]) < 0 ||
            Number(r[k]) > 1000000,
        ),
    )
  )
    return null;
  const cost = rows.map(
    (r) =>
      `param("_image_billing.input_count") * ${Number((Number(r.input) * 1e6).toPrecision(15))} + param("_image_billing.output_count") * ${Number((Number(r.output) * 1e6).toPrecision(15))}`,
  );
  return `param("_image_billing") == nil ? tier("image_config_preview", 0) : (param("_image_billing.resolution") == "1K" ? tier("image_1k", ${cost[0]}) : (param("_image_billing.resolution") == "2K" ? tier("image_2k", ${cost[1]}) : (param("_image_billing.resolution") == "auto" ? tier("image_auto", max(${cost[0]}, ${cost[1]})) : tier("unsupported_image_parameters", -1))))`;
}
export function parseImagePriceExpression(expr = '') {
  const matches = [
    ...expr.matchAll(
      /tier\("image_[12]k", param\("_image_billing.input_count"\) \* ([\d.e+-]+) \+ param\("_image_billing.output_count"\) \* ([\d.e+-]+)\)/g,
    ),
  ];
  if (matches.length !== 2) return null;
  const rows = matches.map((m, i) => ({
    resolution: ['1K', '2K'][i],
    input: Number(m[1]) / 1e6,
    output: Number(m[2]) / 1e6,
  }));
  // Only exact generated expressions round-trip; custom rules must stay intact.
  return buildImagePriceExpression(rows) === expr ? rows : null;
}
