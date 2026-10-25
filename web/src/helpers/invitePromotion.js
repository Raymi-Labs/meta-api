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

// Shared copy for the invitation landing page and embeddable advertisements.
export const INVITE_CHARACTER_PATH = '/images/yuanheng-invite-character.jpg';

export const INVITE_PROMOTION = {
  title: '满血模型，',
  subtitle: '尽在元衡。',
  description: '满血 CC Max、GPT Pro，让灵感变成作品，让想法跑成代码。',
  action: '立即体验元衡',
  models: [
    {
      name: 'CC Max',
      badge: '满血接入',
      description: '从第一行代码，到整个项目。',
    },
    {
      name: 'GPT Pro',
      badge: '创作搭档',
      description: '写作、推理、创作，放开想。',
    },
  ],
  benefits: ['一 Key 多模型', '用量明细可查', '按需选择模型'],
};

export const INVITE_PROMOTION_SOURCES = {
  link: '邀请链接',
  text: '文字链接',
  button: '推广按钮',
  banner: '横幅卡片',
};

export const escapePromotionHTML = (value) =>
  String(value).replace(
    /[&<>"']/g,
    (char) =>
      ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
      })[char],
  );

export function buildInvitePromotionHTML(affLink, type, t = (value) => value) {
  if (!['text', 'button', 'banner'].includes(type)) return '';
  let url;
  try {
    url = new URL(affLink);
  } catch {
    return '';
  }
  if (
    !['https:', 'http:'].includes(url.protocol) ||
    url.username ||
    url.password ||
    !/^\/i\/[A-Za-z0-9_-]{4,32}$/.test(url.pathname)
  )
    return '';
  url.search = '';
  url.hash = '';
  url.searchParams.set('src', type);
  const href = escapePromotionHTML(url.href);
  const text = (key) => escapePromotionHTML(t(key));
  const attrs = `href="${href}" target="_blank" rel="noopener noreferrer sponsored"`;
  if (type === 'text') {
    return `<a ${attrs}>${text('元衡 AI')} · ${text('满血 CC Max、GPT Pro，一站接入。')}</a>`;
  }
  if (type === 'button') {
    return `<a ${attrs} style="display:inline-flex;align-items:center;gap:16px;padding:14px 22px;border:1px solid #1868ed;border-radius:12px;background:#1868ed;color:#fff;text-decoration:none;font:700 16px/1.5 system-ui,sans-serif;"><span>${text(INVITE_PROMOTION.action)}<span style="display:block;color:#e4edff;font-size:11px;font-weight:400;">CC Max &middot; GPT Pro</span></span><span aria-hidden="true">&rarr;</span></a>`;
  }
  const imageURL = escapePromotionHTML(
    new URL(INVITE_CHARACTER_PATH, url).href,
  );
  const models = INVITE_PROMOTION.models
    .map(
      (model) =>
        `<span style="display:block;margin-top:10px;"><strong style="font-size:18px;color:#234579;">${escapePromotionHTML(model.name)}</strong> <span style="font-size:10px;color:#3673c9;">${text(model.badge)}</span></span>`,
    )
    .join('');
  return `<a ${attrs} style="display:block;box-sizing:border-box;max-width:640px;padding:24px;border:1px solid #d9e9ff;border-radius:22px;background:linear-gradient(120deg,#fff,#edf7ff);color:#1c365b;text-decoration:none;font:14px/1.6 system-ui,sans-serif;">
  <span style="display:block;color:#295b9b;font-size:12px;letter-spacing:1px;">${text('元衡 AI')} &nbsp; / &nbsp; AI, ALL IN ONE.</span>
  <span style="display:flex;align-items:center;gap:8px;">
    <span style="display:block;flex:1;min-width:0;padding:18px 0;">
      <strong style="display:block;font-size:clamp(24px,5vw,38px);line-height:1.25;letter-spacing:-1px;">${text(INVITE_PROMOTION.title)}<br /><span style="color:#1868ed;">${text(INVITE_PROMOTION.subtitle)}</span></strong>
      <span style="display:block;margin:20px 0;">${models}</span>
      <span style="display:inline-block;padding:11px 14px;border-radius:9px;background:#1868ed;color:#fff;font-weight:700;">${text(INVITE_PROMOTION.action)} &rarr;</span>
    </span>
    <span style="display:block;position:relative;flex:none;width:clamp(100px,28vw,170px);aspect-ratio:470/1024;overflow:hidden;mix-blend-mode:multiply;">
      <img src="${imageURL}" alt="" referrerpolicy="no-referrer" width="1536" height="1024" style="display:block;position:absolute;left:0;top:0;height:100%;width:auto;max-width:none;" />
    </span>
  </span>
  <span style="display:block;padding-top:14px;border-top:1px solid #dce9f8;color:#6482a6;font-size:11px;">${INVITE_PROMOTION.benefits.map(text).join(' &nbsp; &middot; &nbsp; ')}</span>
</a>`;
}
