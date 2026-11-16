export const modelDiscoveryRequest = (inputs, channelId) => ({
  ...(channelId !== undefined ? { channel_id: channelId } : {}),
  type: inputs.type,
  base_url: inputs.base_url || '',
  key: inputs.key || '',
  proxy: inputs.proxy || '',
  header_override: inputs.header_override || '',
  allow_reference_fallback: true,
});

export const modelDiscoveryNotice = (source, t) => {
  if (source === 'official_reference')
    return t('上游列表获取失败，已显示官方参考模型；不代表该渠道支持。');
  if (source === 'builtin_reference')
    return t('上游列表获取失败，已显示内置参考模型；不代表该渠道支持。');
  return t('模型列表来自当前填写的上游地址。');
};
