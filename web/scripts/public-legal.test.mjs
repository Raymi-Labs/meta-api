import test from 'node:test';
import assert from 'node:assert/strict';
import {
  PUBLIC_POLICIES,
  getPublicPolicy,
} from '../src/pages/PublicLegal/content.js';
import { isPublicLegalPath } from '../src/helpers/publicRoutes.js';

test('supplemental legal pages do not intercept operator-configured agreements or private routes', () => {
  for (const path of [
    '/legal/cookies',
    '/legal/zdr/',
    '/legal/missing',
    '/privacy-choices/',
  ]) {
    assert.equal(isPublicLegalPath(path), true);
  }
  for (const path of [
    '/user-agreement',
    '/privacy-policy',
    '/console',
    '/legal-settings',
    '/privacy-choices/settings',
    '/docs',
  ]) {
    assert.equal(isPublicLegalPath(path), false);
  }
});

test('legal directory has unique supplemental documents and rejects unknown paths', () => {
  assert.equal(new Set(PUBLIC_POLICIES.map(({ slug }) => slug)).size, 5);
  for (const policy of PUBLIC_POLICIES) {
    assert.equal(getPublicPolicy(policy.slug), policy);
    assert.ok(policy.intro);
    assert.ok(policy.sections.length >= 5);
    assert.ok(
      policy.sections.every(
        (section) => section.title && section.paragraphs.every(Boolean),
      ),
    );
  }
  assert.equal(getPublicPolicy('missing'), undefined);
  assert.equal(getPublicPolicy('cookies/extra'), undefined);
  assert.equal(getPublicPolicy('__proto__'), undefined);
});
