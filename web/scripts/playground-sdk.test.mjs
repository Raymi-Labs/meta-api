// Run with PLAYGROUND_SDK_PYTHON pointing to a Python with openai + anthropic,
// and PLAYGROUND_SDK_NODE_DIR to a directory containing those Node SDKs.
// No credentials or external model calls: every request targets this loopback fixture.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import {
  buildPlaygroundCode,
  convertPlaygroundPayload,
} from '../src/helpers/playgroundCode';
const run = promisify(execFile);
assert.ok(
  process.env.PLAYGROUND_SDK_PYTHON && process.env.PLAYGROUND_SDK_NODE_DIR,
  'Set PLAYGROUND_SDK_PYTHON and PLAYGROUND_SDK_NODE_DIR; see this file header.',
);
const requests = [];
const server = createServer(async (req, res) => {
  let raw = '';
  for await (const part of req) raw += part;
  const body = JSON.parse(raw);
  requests.push({ body, headers: req.headers, url: req.url });
  res.setHeader(
    'Content-Type',
    body.stream ? 'text/event-stream' : 'application/json',
  );
  const kind = req.url.split('/').at(-1);
  const message = {
    id: 'msg_fixture',
    type: 'message',
    role: 'assistant',
    model: body.model,
    content: [{ type: 'text', text: 'fixture-ok' }],
    stop_reason: 'end_turn',
    stop_sequence: null,
    usage: { input_tokens: 10, output_tokens: 1 },
  };
  if (!body.stream)
    res.end(
      JSON.stringify(
        kind === 'messages'
          ? message
          : kind === 'responses'
            ? {
                id: 'resp_fixture',
                object: 'response',
                output: [
                  {
                    type: 'message',
                    role: 'assistant',
                    content: [{ type: 'output_text', text: 'fixture-ok' }],
                  },
                ],
              }
            : {
                id: 'chat_fixture',
                object: 'chat.completion',
                choices: [
                  {
                    index: 0,
                    finish_reason: 'stop',
                    message: { role: 'assistant', content: 'fixture-ok' },
                  },
                ],
              },
      ),
    );
  else if (kind === 'messages')
    res.end(
      `event: message_start\ndata: ${JSON.stringify({ type: 'message_start', message })}\n\nevent: content_block_delta\ndata: ${JSON.stringify({ type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: 'fixture-ok' } })}\n\nevent: message_stop\ndata: {"type":"message_stop"}\n\n`,
    );
  else if (kind === 'responses')
    res.end(
      'event: response.output_text.delta\ndata: {"type":"response.output_text.delta","delta":"fixture-ok","output_index":0,"content_index":0,"item_id":"msg_fixture","sequence_number":1}\n\n',
    );
  else
    res.end(
      'data: {"id":"chat_fixture","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"fixture-ok"}}]}\n\ndata: [DONE]\n\n',
    );
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
try {
  const address = `http://127.0.0.1:${server.address().port}/gateway/v1`;
  for (const stream of [true, false])
    for (const protocol of ['chat', 'messages', 'responses'])
      for (const language of ['python', 'javascript', 'curl']) {
        const payload = {
          model: 'fixture-model',
          group: 'vip',
          stream,
          temperature: 0,
          max_tokens: 100,
          messages: [
            { role: 'system', content: 'Be helpful' },
            {
              role: 'user',
              content: `中文 ' " \\ \n $HOME $(printf injected) </code><script>bad()</script>`,
            },
          ],
          ...(protocol === 'chat'
            ? {
                custom_option: { enabled: true, empty: null },
                input: 'extension',
              }
            : {}),
        };
        const code = buildPlaygroundCode(payload, language, address, protocol);
        assert.ok(code);
        const [cmd, args] =
          language === 'python'
            ? [process.env.PLAYGROUND_SDK_PYTHON, ['-c', code]]
            : language === 'javascript'
              ? ['node', ['--input-type=module', '-e', code]]
              : ['bash', ['-c', code]];
        const { stdout } = await run(cmd, args, {
          cwd: process.env.PLAYGROUND_SDK_NODE_DIR,
          env: {
            ...process.env,
            META_API_KEY: 'fixture-key',
            NO_PROXY: '127.0.0.1',
          },
          timeout: 15000,
        });
        assert.match(stdout, /fixture-ok/);
        const actual = requests.at(-1);
        assert.deepEqual(
          actual.body,
          convertPlaygroundPayload(payload, protocol).body,
        );
        assert.equal(
          actual.url,
          '/gateway/v1/' +
            (protocol === 'chat' ? 'chat/completions' : protocol),
        );
        if (protocol === 'messages') {
          assert.ok(
            actual.headers['x-api-key'] === 'fixture-key' ||
              actual.headers.authorization === 'Bearer fixture-key',
          );
          assert.equal(actual.headers['anthropic-version'], '2023-06-01');
        } else assert.equal(actual.headers.authorization, 'Bearer fixture-key');
        assert.equal(actual.headers['new-api-user'], undefined);
        console.log(`PASS ${protocol} ${language} stream=${stream}`);
      }
  assert.equal(requests.length, 18);
} finally {
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
