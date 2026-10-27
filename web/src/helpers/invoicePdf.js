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

async function invoicePdfErrorMessage(data, fallback) {
  try {
    const payload = data instanceof Blob ? JSON.parse(await data.text()) : data;
    return typeof payload?.message === 'string' && payload.message
      ? payload.message
      : fallback;
  } catch {
    return fallback;
  }
}

// Use the shared API client so the session and New-Api-User header are retained.
export async function loadInvoicePdf(api, url, fallback) {
  let response;
  try {
    response = await api.get(url, {
      responseType: 'blob',
      skipErrorHandler: true,
    });
  } catch (error) {
    throw new Error(
      await invoicePdfErrorMessage(error?.response?.data, fallback),
    );
  }
  return validateInvoicePdf(response.data, fallback);
}

async function validateInvoicePdf(blob, fallback) {
  // Business errors can be JSON with HTTP 200; never open them as a PDF.
  if (!(blob instanceof Blob) || (await blob.slice(0, 5).text()) !== '%PDF-') {
    throw new Error(await invoicePdfErrorMessage(blob, fallback));
  }
  return new Blob([blob], { type: 'application/pdf' });
}

// The server uses Chromium's native print engine with this same print HTML.
export async function renderInvoicePdf(api, id, kind, html, fallback) {
  let response;
  try {
    response = await api.post(
      `/api/user/invoices/${id}/render-pdf`,
      { kind, html },
      { responseType: 'blob', skipErrorHandler: true, timeout: 60000 },
    );
  } catch (error) {
    throw new Error(
      await invoicePdfErrorMessage(error?.response?.data, fallback),
    );
  }
  const blob = await validateInvoicePdf(response.data, fallback);
  if (blob.size > 10 * 1024 * 1024) throw new Error(fallback);
  return blob;
}

// Prepare every selected attachment before the approval/send request starts.
// A null file means the server should reuse the already saved attachment.
export async function prepareInvoiceAttachmentFiles({
  selected,
  files,
  saved = {},
  loadDetail,
  loadStamp,
  buildHtml,
  renderPdf,
}) {
  const result = {};
  const missing = [];
  for (const kind of ['detail_bill', 'service_confirmation']) {
    if (!selected[kind]) continue;
    result[kind] = files[kind] || null;
    if (!result[kind] && !saved[kind]) missing.push(kind);
  }
  if (missing.length === 0) return result;
  const snapshot = await loadDetail();
  // Freeze the issue date across both documents, even when rendering crosses midnight.
  const detail = {
    ...snapshot,
    reviewed_at: snapshot.reviewed_at || Math.floor(Date.now() / 1000),
  };
  const stamp = await loadStamp();
  // Sequential rendering keeps browser memory bounded on small servers.
  for (const kind of missing) {
    const blob = await renderPdf(kind, buildHtml(kind, detail, stamp));
    result[kind] = new File([blob], `${kind}-${detail.id}.pdf`, {
      type: 'application/pdf',
    });
  }
  return result;
}

export async function openInvoicePdf(api, url, messages) {
  // Open synchronously during the click to avoid popup blocking after await.
  const preview = window.open('', '_blank');
  if (!preview) throw new Error(messages.popupBlocked);
  preview.opener = null;
  preview.document.title = messages.title;
  preview.document.body.textContent = messages.loading;

  let objectUrl;
  try {
    const blob = await loadInvoicePdf(api, url, messages.failed);
    if (preview.closed) return;
    objectUrl = URL.createObjectURL(blob);
    preview.location.replace(objectUrl);
    // Keep the URL valid for the viewer's lazy reads and download action.
    const timer = window.setInterval(() => {
      if (preview.closed) {
        URL.revokeObjectURL(objectUrl);
        window.clearInterval(timer);
      }
    }, 1000);
  } catch (error) {
    if (objectUrl) URL.revokeObjectURL(objectUrl);
    if (!preview.closed) preview.close();
    throw error;
  }
}
