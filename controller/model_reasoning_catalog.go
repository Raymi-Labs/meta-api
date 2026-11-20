package controller

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var officialReasoningURLs = map[string]string{
	model.CodexCatalog:  "https://raw.githubusercontent.com/openai/codex/main/codex-rs/models-manager/models.json",
	model.GeminiCatalog: "https://aistudio.google.com/docs/thinking.md",
	model.ClaudeCatalog: "https://platform.claude.com/docs/en/build-with-claude/effort.md",
}

// Operator-configurable relay TLS exceptions must never apply to catalog provenance.
func reasoningCatalogClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func fetchOfficialReasoningCatalog(ctx context.Context, client *http.Client, source, etag string) (string, error) {
	const maxBytes = 8 << 20
	url, ok := officialReasoningURLs[source]
	if !ok {
		return etag, errors.New("unrecognized official source")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return etag, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := client.Do(req)
	if err != nil {
		return etag, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified && etag != "" {
		return etag, nil
	}
	if res.StatusCode != http.StatusOK {
		return etag, errors.New("official reasoning catalog unavailable: " + http.StatusText(res.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return etag, err
	}
	if len(data) > maxBytes {
		return etag, errors.New("official reasoning catalog too large")
	}
	if err = model.ReplaceOfficialReasoningCatalog(source, data); err != nil {
		return etag, err
	}
	return res.Header.Get("ETag"), nil
}

var reasoningCatalogOnce sync.Once

// Replicas share the last-good projections in the database; only the master contacts publishers.
func StartOfficialReasoningCatalogTask() {
	if !common.IsMasterNode || !common.GetEnvOrDefaultBool("MODEL_METADATA_SYNC_ENABLED", true) {
		return
	}
	reasoningCatalogOnce.Do(func() {
		for source := range officialReasoningURLs {
			go runOfficialReasoningCatalogTask(context.Background(), reasoningCatalogClient(), source, modelCapabilitySyncInterval())
		}
	})
}

func runOfficialReasoningCatalogTask(ctx context.Context, client *http.Client, source string, interval time.Duration) {
	defer client.CloseIdleConnections()
	etag := ""
	failures := 0
	for ctx.Err() == nil {
		next, err := fetchOfficialReasoningCatalog(ctx, client, source, etag)
		if err == nil {
			etag = next
			failures = 0
		} else if ctx.Err() == nil {
			failures++
			common.SysError("官方模型推理目录刷新未完成，继续使用已有资料: " + source)
		}
		timer := time.NewTimer(modelMetadataRetryDelay(interval, failures))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
