import assert from 'node:assert/strict';
import {
  buildInvitePromotionHTML,
  escapePromotionHTML,
  INVITE_CHARACTER_PATH,
} from '../src/helpers/invitePromotion.js';

for (const type of ['text', 'button', 'banner']) {
  const html = buildInvitePromotionHTML(
    'https://example.test/i/abcd?old=1#ignored',
    type,
  );
  assert.ok(html.includes(`href="https://example.test/i/abcd?src=${type}"`));
  assert.ok(html.includes('noopener noreferrer sponsored'));
  assert.ok(!html.includes('old=') && !html.includes('#ignored'));
  assert.ok(!/<script|on\w+=/i.test(html));
  if (type === 'banner') {
    assert.ok(
      html.includes(`src="https://example.test${INVITE_CHARACTER_PATH}"`),
    );
    assert.ok(html.includes('referrerpolicy="no-referrer"'));
    assert.equal((html.match(/<img /g) || []).length, 1);
  } else assert.ok(!html.includes('<img'));
  const malicious = buildInvitePromotionHTML(
    'https://example.test/i/abcd',
    type,
    () => '<script>alert("x")</script>',
  );
  assert.ok(!malicious.includes('<script>'));
}
for (const url of [
  'javascript:alert(1)',
  'data:text/html,hi',
  'https://user:pass@example.test/i/abcd',
  'https://example.test/register',
  'https://example.test/i/a%22onclick=1',
  'not a url',
]) {
  assert.equal(buildInvitePromotionHTML(url, 'banner'), '');
}
assert.equal(
  buildInvitePromotionHTML('https://example.test/i/abcd', 'evil'),
  '',
);
assert.equal(escapePromotionHTML('<>&"\''), '&lt;&gt;&amp;&quot;&#39;');
console.log(
  'Invitation HTML: safe links, source tags, escaping, invalid inputs passed.',
);
