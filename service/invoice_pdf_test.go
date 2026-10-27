package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func invoicePDFTestImage(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, img))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
}

func TestInvoicePDFSanitizesActiveMarkupAndPreservesPrintContent(t *testing.T) {
	stamp := invoicePDFTestImage(t)
	document, err := sanitizeInvoiceHTML(`<html><head><meta http-equiv="refresh" content="0;url=file:///etc/passwd"><base href="https://example.test/"><link rel="stylesheet" href="https://example.test/x"><style>@page { size:A4 landscape } .seal {width:110px}</style><title>Invoice</title></head><body onload="alert(1)"><main class="certificate"><p style="color:red">可读的账单文本</p><table><tr><td colspan="2">100.00</td></tr></table><img class="seal" src="` + stamp + `" onerror="alert(2)"><img src="file:///etc/passwd"><img src="data:image/svg+xml;base64,PHN2Zz4="><iframe srcdoc="&lt;script&gt;alert(3)&lt;/script&gt;"></iframe><script>alert(4)</script><svg><image href="https://example.test/remote"/></svg><form action="https://example.test/"><input></form></main></body></html>`)
	require.NoError(t, err)
	for _, blocked := range []string{"refresh", "<base", "<link", "onload", "onerror", "<iframe", "<script", "<svg", "<form", "<input", "file:///", "image/svg+xml"} {
		require.NotContains(t, document, blocked)
	}
	for _, preserved := range []string{"@page { size:A4 landscape }", `class="certificate"`, `colspan="2"`, "可读的账单文本", stamp} {
		require.Contains(t, document, preserved)
	}
	parsed, err := html.Parse(strings.NewReader(document))
	require.NoError(t, err)
	var head *html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if node.Data == "head" {
			head = node
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(parsed)
	require.NotNil(t, head)
	require.Equal(t, "meta", head.FirstChild.Data)
	require.Contains(t, head.FirstChild.Attr, html.Attribute{Key: "content", Val: invoicePDFCSP})
}

func TestInvoicePDFLimitsAndCancellation(t *testing.T) {
	for _, source := range []string{"", "  ", strings.Repeat("x", MaxInvoiceHTMLSize+1), strings.Repeat("<p>x</p>", 10001)} {
		_, err := RenderInvoicePDF(context.Background(), source)
		require.ErrorIs(t, err, ErrInvoicePDFInput)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RenderInvoicePDF(ctx, "<p>invoice</p>")
	require.ErrorIs(t, err, context.Canceled)
	invoicePDFSlots <- struct{}{}
	invoicePDFSlots <- struct{}{}
	t.Cleanup(func() { <-invoicePDFSlots; <-invoicePDFSlots })
	_, err = RenderInvoicePDF(context.Background(), "<p>invoice</p>")
	require.ErrorIs(t, err, ErrInvoicePDFBusy)
}

func TestInvoicePDFMissingBrowserReturnsOnlySafeError(t *testing.T) {
	t.Setenv("INVOICE_PDF_CHROME_PATH", "/nonexistent/invoice-chromium")
	_, err := RenderInvoicePDF(context.Background(), "<p>private-invoice-content</p>")
	require.Equal(t, ErrInvoicePDFRender, err)
	require.Zero(t, len(invoicePDFSlots))
}

func TestInvoicePDFBrowserPrintAndIsolation(t *testing.T) {
	if os.Getenv("INVOICE_PDF_INTEGRATION") != "1" {
		t.Skip("set INVOICE_PDF_INTEGRATION=1 for real Chromium")
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	// Also exercise CDP restrictions without the sanitizer/CSP, so the second
	// layer is independently verified. Title metadata reveals script execution.
	raw := fmt.Sprintf(`<html><head><title>SAFE-INVOICE</title><style>@import url('%s/style'); @page {size:A4 landscape;margin:14mm} body {background-image:url('%s/image');color:#234}</style></head><body><script>document.title='INVOICE-SCRIPT-RAN';fetch('%s/script')</script><h1>Invoice text</h1><img src="%s"></body></html>`, server.URL, server.URL, server.URL, invoicePDFTestImage(t))
	data, err := printInvoicePDF(ctx, raw)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("%PDF-")))
	require.Contains(t, string(data), "/Title (SAFE-INVOICE)")
	require.NotContains(t, string(data), "INVOICE-SCRIPT-RAN")
	require.Contains(t, string(data), "/ToUnicode")
	box := regexp.MustCompile(`/MediaBox\s*\[0 0 ([0-9.]+) ([0-9.]+)\]`).FindSubmatch(data)
	require.Len(t, box, 3)
	width, err := strconv.ParseFloat(string(box[1]), 64)
	require.NoError(t, err)
	height, err := strconv.ParseFloat(string(box[2]), 64)
	require.NoError(t, err)
	require.InDelta(t, 841.89, width, 1)
	require.InDelta(t, 595.28, height, 1)
	require.Zero(t, requests.Load())
	// The normal public service additionally strips navigation, frame, file,
	// foreign markup, and JavaScript before Chromium sees the document.
	data, err = RenderInvoicePDF(ctx, strings.ReplaceAll(raw, "<body>", `<body><meta http-equiv="refresh" content="0;url=`+server.URL+`"><iframe src="`+server.URL+`/frame"></iframe><img src="file:///etc/passwd"><svg><image href="`+server.URL+`/svg"></image></svg>`))
	require.NoError(t, err)
	require.Contains(t, string(data), "/Title (SAFE-INVOICE)")
	require.Zero(t, requests.Load())
	short, stop := context.WithTimeout(context.Background(), time.Millisecond)
	defer stop()
	_, err = RenderInvoicePDF(short, raw)
	require.Error(t, err)
	require.Zero(t, len(invoicePDFSlots))
}
