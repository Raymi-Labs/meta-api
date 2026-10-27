import test from 'node:test';
import assert from 'node:assert/strict';
import {
  loadInvoicePdf,
  openInvoicePdf,
  renderInvoicePdf,
  prepareInvoiceAttachmentFiles,
} from '../src/helpers/invoicePdf.js';

const fallback = '加载发票 PDF 失败，请重试';
const pdf = new Blob(['%PDF-1.4\ninvoice'], { type: 'application/pdf' });
const jsonError = (message) =>
  new Blob([JSON.stringify({ success: false, message })], {
    type: 'application/json',
  });

test('loads admin and self PDFs through the supplied authenticated client', async () => {
  for (const url of [
    '/api/user/invoices/1/file',
    '/api/user/invoices/self/1/file',
  ]) {
    const api = {
      async get(actualUrl, options) {
        assert.equal(actualUrl, url);
        assert.equal(options.responseType, 'blob');
        assert.equal(options.skipErrorHandler, true);
        return { data: pdf };
      },
    };
    const result = await loadInvoicePdf(api, url, fallback);
    assert.equal(result.type, 'application/pdf');
    assert.equal(await result.text(), await pdf.text());
  }
});

test('HTTP 200 business errors are shown instead of opening a fake PDF', async () => {
  const api = { get: async () => ({ data: jsonError('发票 PDF 不存在') }) };
  await assert.rejects(
    loadInvoicePdf(api, '/file', fallback),
    /发票 PDF 不存在/,
  );
});

test('HTTP auth errors retain the server message from the blob response', async () => {
  const api = {
    async get() {
      throw { response: { status: 401, data: jsonError('登录已过期') } };
    },
  };
  await assert.rejects(loadInvoicePdf(api, '/file', fallback), /登录已过期/);
});

test('empty files, HTML errors and network failures use a readable fallback', async () => {
  for (const data of [new Blob([]), new Blob(['<html>502</html>']), null]) {
    await assert.rejects(
      loadInvoicePdf({ get: async () => ({ data }) }, '/file', fallback),
      { message: fallback },
    );
  }
  await assert.rejects(
    loadInvoicePdf(
      {
        get: async () => {
          throw new Error('Network Error');
        },
      },
      '/file',
      fallback,
    ),
    { message: fallback },
  );
});

function mockWindow(t, { blocked = false } = {}) {
  let interval;
  const revoked = [];
  const locations = [];
  const preview = {
    opener: {},
    closed: false,
    document: { body: {} },
    location: { replace: (url) => locations.push(url) },
    close() {
      this.closed = true;
    },
  };
  const oldWindow = globalThis.window;
  globalThis.window = {
    open: () => (blocked ? null : preview),
    setInterval: (callback) => {
      interval = callback;
      return 123;
    },
    clearInterval: (id) => assert.equal(id, 123),
  };
  t.after(() => {
    if (oldWindow === undefined) delete globalThis.window;
    else globalThis.window = oldWindow;
  });
  t.mock.method(URL, 'createObjectURL', () => 'blob:invoice');
  t.mock.method(URL, 'revokeObjectURL', (url) => revoked.push(url));
  return { preview, revoked, locations, tick: () => interval() };
}
const messages = {
  title: '<img src=x onerror=alert(1)>.pdf',
  loading: '正在加载',
  popupBlocked: '请允许弹出窗口',
  failed: fallback,
};

test('preopens a safe preview and retains the blob until the viewer is closed', async (t) => {
  const state = mockWindow(t);
  await openInvoicePdf({ get: async () => ({ data: pdf }) }, '/file', messages);
  assert.equal(state.preview.opener, null);
  assert.equal(state.preview.document.title, messages.title);
  assert.equal(state.preview.document.body.textContent, messages.loading);
  assert.deepEqual(state.locations, ['blob:invoice']);
  state.tick();
  assert.deepEqual(state.revoked, []);
  state.preview.close();
  state.tick();
  assert.deepEqual(state.revoked, ['blob:invoice']);
});

test('popup blocking prevents the request', async (t) => {
  mockWindow(t, { blocked: true });
  let fetched = false;
  await assert.rejects(
    openInvoicePdf(
      {
        get: async () => {
          fetched = true;
        },
      },
      '/file',
      messages,
    ),
    /请允许弹出窗口/,
  );
  assert.equal(fetched, false);
});

test('closing the preview during download prevents blob creation', async (t) => {
  const state = mockWindow(t);
  await openInvoicePdf(
    {
      get: async () => {
        state.preview.close();
        return { data: pdf };
      },
    },
    '/file',
    messages,
  );
  assert.deepEqual(state.locations, []);
  assert.deepEqual(state.revoked, []);
});

test('download failure closes the blank preview and propagates the message', async (t) => {
  const state = mockWindow(t);
  await assert.rejects(
    openInvoicePdf(
      { get: async () => ({ data: jsonError('无权限') }) },
      '/file',
      messages,
    ),
    /无权限/,
  );
  assert.equal(state.preview.closed, true);
  assert.deepEqual(state.locations, []);
});

test('native rendering posts the exact print HTML with auth client and a bounded timeout', async () => {
  const html =
    '<!doctype html><style>@page {size:A4 landscape}</style><p>明细</p>';
  const api = {
    async post(url, body, options) {
      assert.equal(url, '/api/user/invoices/7/render-pdf');
      assert.deepEqual(body, { kind: 'detail_bill', html });
      assert.deepEqual(options, {
        responseType: 'blob',
        skipErrorHandler: true,
        timeout: 60000,
      });
      return { data: pdf };
    },
  };
  const result = await renderInvoicePdf(api, 7, 'detail_bill', html, fallback);
  assert.equal(await result.text(), await pdf.text());
  assert.equal(result.type, 'application/pdf');
});

test('render errors retain business and HTTP messages; invalid/oversized PDF never becomes an attachment', async () => {
  for (const api of [
    { post: async () => ({ data: jsonError('渲染器暂不可用') }) },
    {
      post: async () => {
        throw { response: { data: jsonError('渲染器暂不可用') } };
      },
    },
  ]) {
    await assert.rejects(
      renderInvoicePdf(api, 7, 'detail_bill', '<p/>', fallback),
      /渲染器暂不可用/,
    );
  }
  for (const data of [
    new Blob(['<html>502</html>']),
    new Blob(['%PDF-', new Uint8Array(10 * 1024 * 1024)]),
  ]) {
    await assert.rejects(
      renderInvoicePdf(
        { post: async () => ({ data }) },
        7,
        'detail_bill',
        '<p/>',
        fallback,
      ),
      { message: fallback },
    );
  }
});

function attachmentFixture(overrides = {}) {
  const events = [];
  const detail = { id: 42, items: [{ trade_no: 'snapshot-order' }] };
  const stamp = 'data:image/png;base64,stamp';
  const args = {
    selected: { detail_bill: true, service_confirmation: true },
    files: {},
    saved: {},
    loadDetail: async () => {
      events.push('detail');
      return detail;
    },
    loadStamp: async () => {
      events.push('stamp');
      return stamp;
    },
    buildHtml: (kind, actualDetail, actualStamp) => {
      assert.equal(actualDetail.id, detail.id);
      assert.equal(actualDetail.items, detail.items);
      assert.ok(actualDetail.reviewed_at > 0);
      assert.equal(actualStamp, stamp);
      events.push(`html:${kind}`);
      return `<p>${kind}:${detail.items[0].trade_no}</p>`;
    },
    renderPdf: async (kind, html) => {
      events.push(`pdf:${kind}`);
      assert.equal(html, `<p>${kind}:snapshot-order</p>`);
      return pdf;
    },
    ...overrides,
  };
  return { args, events };
}

test('both missing attachments use one latest detail snapshot and stamp, preserving native PDF bytes', async () => {
  const { args, events } = attachmentFixture();
  const files = await prepareInvoiceAttachmentFiles(args);
  assert.deepEqual(events, [
    'detail',
    'stamp',
    'html:detail_bill',
    'pdf:detail_bill',
    'html:service_confirmation',
    'pdf:service_confirmation',
  ]);
  for (const kind of ['detail_bill', 'service_confirmation']) {
    assert.equal(files[kind].name, `${kind}-42.pdf`);
    assert.equal(files[kind].type, 'application/pdf');
    assert.equal(await files[kind].text(), await pdf.text());
  }
});

test('manual replacements and saved PDFs are preferred without loading data, stamp or renderer', async () => {
  const manual = new File([pdf], 'manual.pdf');
  const { args, events } = attachmentFixture({
    files: { detail_bill: manual },
    saved: { detail_bill: 'old.pdf', service_confirmation: 'saved.pdf' },
  });
  const result = await prepareInvoiceAttachmentFiles(args);
  assert.equal(result.detail_bill, manual);
  assert.equal(result.service_confirmation, null);
  assert.deepEqual(events, []);
});

test('unselected attachments are neither generated nor submitted even if a manual file remains selected', async () => {
  const { args, events } = attachmentFixture({
    selected: { detail_bill: false, service_confirmation: false },
    files: { detail_bill: new File([pdf], 'manual.pdf') },
  });
  assert.deepEqual(await prepareInvoiceAttachmentFiles(args), {});
  assert.deepEqual(events, []);
});

test('only missing selected files are generated; saved/unchecked attachments stay untouched', async () => {
  const { args, events } = attachmentFixture({
    saved: { detail_bill: 'saved.pdf' },
  });
  const result = await prepareInvoiceAttachmentFiles(args);
  assert.equal(result.detail_bill, null);
  assert.equal(result.service_confirmation.name, 'service_confirmation-42.pdf');
  assert.deepEqual(events, [
    'detail',
    'stamp',
    'html:service_confirmation',
    'pdf:service_confirmation',
  ]);
});

test('detail, stamp, or second PDF failure prevents a submission with partial attachments', async () => {
  for (const failedStage of ['detail', 'stamp', 'service_confirmation']) {
    const { args, events } = attachmentFixture();
    if (failedStage === 'detail')
      args.loadDetail = async () => {
        throw new Error('详情失败');
      };
    if (failedStage === 'stamp')
      args.loadStamp = async () => {
        throw new Error('印章失败');
      };
    if (failedStage === 'service_confirmation') {
      const original = args.renderPdf;
      args.renderPdf = (kind, html) => {
        if (kind === failedStage) throw new Error('渲染失败');
        return original(kind, html);
      };
    }
    let submitted = false;
    await assert.rejects(async () => {
      await prepareInvoiceAttachmentFiles(args);
      submitted = true;
    }, /失败/);
    assert.equal(submitted, false);
    if (failedStage !== 'service_confirmation') {
      assert.ok(!events.some((event) => event.startsWith('pdf:')));
    }
  }
});
