import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  getRequestFailureDetails,
  buildRequestFailureReport,
} from '../src/helpers/requestFailure.js';
const t = (text) => text;
test('HTTP 200 stream failure includes diagnosis without private diagnostics', () => {
  const log = {
    type: 5,
    request_id: 'req-123',
    model_name: 'claude-opus-5',
    created_at: 100,
    other: JSON.stringify({
      request_failure: true,
      http_status: 200,
      status_code: 400,
      failure_reason: '请求被内容安全策略拒绝',
      failure_hint: '请调整请求内容',
      latency_ms: 1554,
      retry_after_seconds: 3,
      admin_info: { error_message: 'private-key', upstream: 'private-url' },
    }),
  };
  const report = buildRequestFailureReport(log, t);
  assert.match(report, /req-123/);
  assert.match(report, /HTTP 状态码: 200/);
  assert.match(report, /失败状态码: 400/);
  assert.match(report, /流内错误/);
  assert.match(report, /1554 ms/);
  assert.match(report, /Retry-After: 3 s/);
  assert.match(report, /消费或退款/);
  assert.doesNotMatch(report, /private/);
});
test('legacy errors are readable and successful usage has no failure panel', () => {
  const details = getRequestFailureDetails(
    { type: 5, content: 'private legacy error', other: '{bad' },
    t,
  );
  assert.ok(details.length > 0);
  assert.match(JSON.stringify(details), /不能据此判断最终调用结果/);
  assert.doesNotMatch(JSON.stringify(details), /private/);
  assert.deepEqual(getRequestFailureDetails({ type: 2 }, t), []);
});

test('model and sampling diagnoses translate without exposing adjustment attempts', () => {
  const diagnoses = [
    [
      '当前令牌可用范围内未找到该模型',
      '请使用同一令牌查询 /v1/models，核对模型名称、令牌分组及渠道限制。',
    ],
    ...['temperature', 'top_p', 'top_k'].map((parameter) => [
      '上游模型不支持当前采样参数',
      `请关闭客户端自定义 ${parameter} 参数后重试；若仍失败，请携带请求 ID 联系管理员。`,
    ]),
  ];
  for (const locale of ['zh-CN', 'zh-TW', 'en', 'ja', 'fr', 'ru', 'vi']) {
    const { translation } = JSON.parse(
      readFileSync(
        new URL(`../src/i18n/locales/${locale}.json`, import.meta.url),
      ),
    );
    for (const [reason, hint] of diagnoses) {
      assert.ok(translation[reason], `${locale}: ${reason}`);
      assert.ok(translation[hint], `${locale}: ${hint}`);
      const report = buildRequestFailureReport(
        {
          type: 5,
          created_at: 100,
          request_id: 'diagnosis-fixture',
          other: JSON.stringify({
            request_failure: true,
            failure_reason: reason,
            failure_hint: hint,
            admin_info: {
              attempts: [
                {
                  adjustment: 'private-adjustment',
                  upstream: 'private-provider',
                },
              ],
            },
          }),
        },
        (key) => translation[key] || key,
      );
      assert.ok(report.includes(translation[reason]));
      assert.ok(report.includes(translation[hint]));
      assert.doesNotMatch(report, /private-/);
    }
  }
});
