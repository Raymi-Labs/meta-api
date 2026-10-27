package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

const (
	MaxInvoiceHTMLSize         = 2 * 1024 * 1024
	MaxGeneratedInvoicePDFSize = 10 * 1024 * 1024
	invoicePDFTimeout          = 30 * time.Second
	invoicePDFCSP              = "default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src 'none'; connect-src 'none'; frame-src 'none'; child-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"
)

var (
	ErrInvoicePDFInput  = errors.New("PDF 生成参数无效")
	ErrInvoicePDFBusy   = errors.New("PDF 生成服务繁忙，请稍后重试")
	ErrInvoicePDFRender = errors.New("自动生成 PDF 失败，请重试或手动上传 PDF")
	invoicePDFSlots     = make(chan struct{}, 2)
)

// RenderInvoicePDF prints a self-contained administrator-supplied document.
// It never navigates to a supplied URL or uses the application's browser/session.
func RenderInvoicePDF(ctx context.Context, source string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case invoicePDFSlots <- struct{}{}:
		defer func() { <-invoicePDFSlots }()
	default:
		return nil, ErrInvoicePDFBusy
	}
	document, err := sanitizeInvoiceHTML(source)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, invoicePDFTimeout)
	defer cancel()
	data, err := printInvoicePDF(ctx, document)
	if err != nil {
		// Never expose Chrome's output: it can contain document text or data URLs.
		return nil, ErrInvoicePDFRender
	}
	if len(data) == 0 || len(data) > MaxGeneratedInvoicePDFSize || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, ErrInvoicePDFRender
	}
	return data, nil
}

func printInvoicePDF(ctx context.Context, document string) ([]byte, error) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("no-first-run", true),
		// No external connection is needed, including browser background traffic.
		chromedp.Flag("proxy-server", "http://127.0.0.1:9"),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND"),
	)
	if executable := strings.TrimSpace(os.Getenv("INVOICE_PDF_CHROME_PATH")); executable != "" {
		opts = append(opts, chromedp.ExecPath(executable))
	}
	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer stopAllocator()
	// Protocol errors may include HTML or data URLs; keep them out of server logs.
	browser, stopBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...interface{}) {}))
	defer stopBrowser()
	chromedp.ListenTarget(browser, func(event interface{}) {
		if request, ok := event.(*fetch.EventRequestPaused); ok {
			go func() {
				executor := cdp.WithExecutor(browser, chromedp.FromContext(browser).Target)
				_ = fetch.FailRequest(request.RequestID, network.ErrorReasonBlockedByClient).Do(executor)
			}()
		}
	})
	var data []byte
	err := chromedp.Run(browser,
		emulation.SetScriptExecutionDisabled(true),
		network.Enable(),
		network.SetBlockedURLs([]string{"http://*", "https://*", "file://*", "ftp://*", "ws://*", "wss://*"}),
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}),
		emulation.SetEmulatedMedia().WithMedia("print"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			frame, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(frame.Frame.ID, document).Do(ctx)
		}),
		// This fixed DevTools expression only waits for trusted local fonts and
		// validated inline image bytes. Page scripts stay disabled throughout.
		chromedp.Evaluate(`Promise.all([document.fonts.ready, ...Array.from(document.images, image => image.decode())]).then(() => true)`, nil,
			func(params *runtime.EvaluateParams) *runtime.EvaluateParams { return params.WithAwaitPromise(true) }),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			data, _, err = page.PrintToPDF().WithPrintBackground(true).WithPreferCSSPageSize(true).WithDisplayHeaderFooter(false).Do(ctx)
			return err
		}),
	)
	return data, err
}

// Rebuild the tree instead of injecting a meta tag into attacker-controlled head
// markup. Active/foreign elements and navigation attributes never reach Chrome.
func sanitizeInvoiceHTML(source string) (string, error) {
	if len(source) == 0 || len(source) > MaxInvoiceHTMLSize || strings.TrimSpace(source) == "" {
		return "", ErrInvoicePDFInput
	}
	parsed, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", ErrInvoicePDFInput
	}
	document := &html.Node{Type: html.DocumentNode}
	document.AppendChild(&html.Node{Type: html.DoctypeNode, Data: "html"})
	root := &html.Node{Type: html.ElementNode, Data: "html"}
	head := &html.Node{Type: html.ElementNode, Data: "head"}
	body := &html.Node{Type: html.ElementNode, Data: "body"}
	document.AppendChild(root)
	root.AppendChild(head)
	root.AppendChild(body)
	head.AppendChild(&html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "http-equiv", Val: "Content-Security-Policy"}, {Key: "content", Val: invoicePDFCSP}}})
	head.AppendChild(&html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "charset", Val: "utf-8"}}})
	allowed := map[string]bool{"main": true, "section": true, "div": true, "span": true, "p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true, "td": true, "colgroup": true, "col": true, "caption": true, "br": true, "hr": true, "strong": true, "b": true, "em": true, "i": true, "small": true, "ul": true, "ol": true, "li": true, "img": true, "style": true, "title": true}
	count := 0
	var copyNode func(*html.Node, *html.Node) error
	copyNode = func(node, parent *html.Node) error {
		count++
		if count > 20000 {
			return ErrInvoicePDFInput
		}
		if node.Type == html.TextNode {
			parent.AppendChild(&html.Node{Type: html.TextNode, Data: node.Data})
			return nil
		}
		if node.Type == html.DocumentNode || (node.Namespace == "" && (node.Data == "html" || node.Data == "head" || node.Data == "body")) {
			if node.Data == "head" {
				parent = head
			}
			if node.Data == "body" {
				parent = body
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if err := copyNode(child, parent); err != nil {
					return err
				}
			}
			return nil
		}
		if node.Type != html.ElementNode || node.Namespace != "" || !allowed[node.Data] {
			return nil
		}
		out := &html.Node{Type: html.ElementNode, Data: node.Data}
		validImage := false
		for _, attr := range node.Attr {
			if attr.Namespace != "" {
				continue
			}
			switch attr.Key {
			case "class", "id", "style", "lang", "dir", "colspan", "rowspan", "width", "height", "alt":
				out.Attr = append(out.Attr, attr)
			case "src":
				if node.Data == "img" && validInvoiceInlineImage(attr.Val) {
					out.Attr = append(out.Attr, attr)
					validImage = true
				}
			}
		}
		if node.Data == "img" && !validImage {
			return nil
		}
		parent.AppendChild(out)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := copyNode(child, out); err != nil {
				return err
			}
		}
		return nil
	}
	if err := copyNode(parsed, body); err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := html.Render(&output, document); err != nil {
		return "", ErrInvoicePDFInput
	}
	return output.String(), nil
}

func validInvoiceInlineImage(source string) bool {
	prefix, encoded, ok := strings.Cut(source, ",")
	if !ok || (prefix != "data:image/png;base64" && prefix != "data:image/jpeg;base64") {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	return err == nil && (format == "png" || format == "jpeg") && config.Width > 0 && config.Height > 0 && config.Width <= 4096 && config.Height <= 4096
}
