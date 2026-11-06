export function maskedEmail(value) {
  if (typeof value !== 'string' || !value.includes('@')) return '';
  const [local, domain] = value.split('@');
  return local && domain ? `${local.slice(0, 1)}***@${domain}` : '';
}
