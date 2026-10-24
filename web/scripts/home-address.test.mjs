import assert from 'node:assert/strict';
import test from 'node:test';
import { getPublicHomeServerAddress } from '../src/pages/Home/homeAddress.js';

test('known public roots use the main domain with one protocol suffix', () => {
  for (const address of [
    'https://meta-api.vip',
    'https://meta-api.vip/v1/',
    'https://nan.meta-api.vip',
    'https://nan.meta-api.vip/',
    'https://cn.meta-api.vip',
    'https://cn.meta-api.vip/v1/',
    ' https://CN.META-API.VIP:443/v1/ ',
  ]) {
    const server = getPublicHomeServerAddress(address, 'http://localhost:5173');
    assert.equal(server, 'https://meta-api.vip');
    assert.equal(`${server}/v1`, 'https://meta-api.vip/v1');
    assert.equal(
      `${server}/v1/chat/completions`,
      'https://meta-api.vip/v1/chat/completions',
    );
  }
});

test('custom domains, paths, ports and other routes retain their configured destination', () => {
  for (const [address, expected] of [
    ['https://tenant.example/v1/', 'https://tenant.example'],
    ['https://tenant.example/gateway/v1/', 'https://tenant.example/gateway'],
    ['http://localhost:3000/v1/', 'http://localhost:3000'],
    ['https://cn.meta-api.vip/gateway/v1/', 'https://cn.meta-api.vip/gateway'],
    ['https://nan.meta-api.vip:8443/v1/', 'https://nan.meta-api.vip:8443'],
    ['http://cn.meta-api.vip', 'http://cn.meta-api.vip'],
    ['https://edu.meta-api.vip', 'https://edu.meta-api.vip'],
    ['https://cn.meta-api.vip.example', 'https://cn.meta-api.vip.example'],
  ]) {
    assert.equal(
      getPublicHomeServerAddress(address, 'https://meta-api.vip'),
      expected,
    );
  }
});

test('missing or invalid config falls back to the current origin', () => {
  for (const address of [
    undefined,
    null,
    '',
    '   ',
    'javascript:alert(1)',
    '/api',
  ]) {
    assert.equal(
      getPublicHomeServerAddress(address, 'https://tenant.example'),
      'https://tenant.example',
    );
    assert.equal(
      getPublicHomeServerAddress(address, 'https://cn.meta-api.vip'),
      'https://meta-api.vip',
    );
  }
});

test('address normalization follows runtime docs without carrying credentials or query strings', () => {
  assert.equal(
    getPublicHomeServerAddress(
      'https://user:secret@tenant.example/gateway/v1/?token=private#config',
      'https://meta-api.vip',
    ),
    'https://tenant.example/gateway',
  );
});
