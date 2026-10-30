package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

var (
	modelCapabilitySyncMutex sync.Mutex
	modelCapabilityTaskOnce  sync.Once
	modelCatalogChanged      = make(chan struct{}, 1)
	errModelCapabilityBusy   = errors.New("模型能力同步正在进行，请稍后重试")
)

// The existing metadata export omits limits and modalities. The full catalog
// from the same configured source supplies these fields; it is community
// metadata, not evidence of a channel's actual routing or billing capabilities.
func modelCapabilitySourceURL() string {
	return strings.TrimRight(getUpstreamBase(), "/") + "/api/all.json"
}

type capabilityProvider struct {
	ID      string `json:"id"`
	Billing struct {
		Priority int `json:"priority"`
	} `json:"billing"`
	Models map[string]json.RawMessage `json:"models"`
}

type capabilityModel struct {
	ID               string          `json:"id"`
	Reasoning        json.RawMessage `json:"reasoning"`
	ToolCall         json.RawMessage `json:"tool_call"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	ReleaseDate      json.RawMessage `json:"release_date"`
	Limit            struct {
		Output  json.RawMessage `json:"output"`
		Context json.RawMessage `json:"context"`
	} `json:"limit"`
	Modalities struct {
		Input  json.RawMessage `json:"input"`
		Output json.RawMessage `json:"output"`
	} `json:"modalities"`
}

func decodeCapabilityModalities(raw json.RawMessage) model.ModelModalities {
	var values model.ModelModalities
	if len(raw) == 0 || common.Unmarshal(raw, &values) != nil {
		return nil
	}
	for i, value := range values {
		// This source calls PDF input "pdf". The local catalog groups document
		// inputs under "file"; this is not a promise to support every file format.
		if strings.EqualFold(strings.TrimSpace(value), "pdf") {
			values[i] = "file"
		}
	}
	validated := model.Model{InputModalities: values}
	if validated.NormalizeDisplayMetadata() != nil {
		return nil
	}
	return validated.InputModalities
}

func parseModelCapabilities(data []byte) (map[string]model.ModelCapabilityMetadata, error) {
	var providers map[string]json.RawMessage
	if err := common.Unmarshal(data, &providers); err != nil {
		return nil, fmt.Errorf("invalid model capability catalog: %w", err)
	}
	// Equal priorities follow the source export's stable provider-ID order.
	providerIDs := make([]string, 0, len(providers))
	for id := range providers {
		providerIDs = append(providerIDs, id)
	}
	sort.Strings(providerIDs)
	entries := make(map[string]model.ModelCapabilityMetadata)
	priorities := make(map[string]int)
	for _, providerID := range providerIDs {
		var provider capabilityProvider
		if providerID == "" || strings.TrimSpace(providerID) != providerID ||
			common.Unmarshal(providers[providerID], &provider) != nil || provider.ID != providerID {
			continue
		}
		for id, raw := range provider.Models {
			var upstream capabilityModel
			if id == "" || strings.TrimSpace(id) != id || common.Unmarshal(raw, &upstream) != nil || upstream.ID != id {
				continue
			}
			var contextLength int64
			if common.Unmarshal(upstream.Limit.Context, &contextLength) != nil || contextLength < 0 || contextLength > 9007199254740991 {
				contextLength = 0
			}
			var maxOutput int64
			if common.Unmarshal(upstream.Limit.Output, &maxOutput) != nil || maxOutput < 0 || maxOutput > 9007199254740991 {
				maxOutput = 0
			}
			flags := model.ModelCapabilities{}
			for key, raw := range map[string]json.RawMessage{"reasoning": upstream.Reasoning, "function_calling": upstream.ToolCall, "structured_output": upstream.StructuredOutput} {
				var value *bool
				if common.Unmarshal(raw, &value) == nil && value != nil {
					flags[key] = *value
				}
			}
			var releaseDate string
			if common.Unmarshal(upstream.ReleaseDate, &releaseDate) != nil || !model.ValidModelReleaseDate(releaseDate) {
				releaseDate = ""
			}
			entry := model.ModelCapabilityMetadata{
				ReleaseDate:     releaseDate,
				ContextLength:   contextLength,
				MaxOutputTokens: maxOutput, Capabilities: flags,
				InputModalities:  decodeCapabilityModalities(upstream.Modalities.Input),
				OutputModalities: decodeCapabilityModalities(upstream.Modalities.Output),
				Provider:         providerID,
			}
			if entry.ReleaseDate == "" && entry.ContextLength == 0 && entry.MaxOutputTokens == 0 && len(entry.Capabilities) == 0 && len(entry.InputModalities) == 0 && len(entry.OutputModalities) == 0 {
				continue
			}
			if previous, exists := priorities[id]; exists && previous >= provider.Billing.Priority {
				continue
			}
			entries[id], priorities[id] = entry, provider.Billing.Priority
		}
	}
	if len(entries) == 0 {
		return nil, errors.New("model capability catalog contains no valid entries")
	}
	return entries, nil
}

func fetchModelCapabilities(ctx context.Context, source string) (map[string]model.ModelCapabilityMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	resp, err := getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// A failed or rate-limited source is left alone until the next scheduled
		// run. Never change endpoints or retry in a tight loop.
		return nil, fmt.Errorf("model capability source returned HTTP %d", resp.StatusCode)
	}
	maxMB := common.GetEnvOrDefault("SYNC_HTTP_MAX_MB", 10)
	if maxMB < 1 || maxMB > 64 {
		maxMB = 10
	}
	maxBytes := int64(maxMB) << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("model capability catalog exceeds size limit")
	}
	return parseModelCapabilities(data)
}

// List reads never wait on remote sources. A buffered signal coalesces new rows.
func reconcileModelCatalog(ctx context.Context) (int, error) {
	created, err := model.EnsureModelCatalog(ctx)
	if created > 0 {
		model.RefreshPricing()
		select {
		case modelCatalogChanged <- struct{}{}:
		default:
		}
	}
	return created, err
}

func runModelCapabilitySync(parent context.Context) (result model.ModelCapabilitySyncResult, syncErr error) {
	if !modelCapabilitySyncMutex.TryLock() {
		return model.ModelCapabilitySyncResult{}, errModelCapabilityBusy
	}
	defer modelCapabilitySyncMutex.Unlock()
	defer func() {
		reconcileCtx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		created, err := model.EnsureModelCatalog(reconcileCtx)
		result.Created += created
		if syncErr == nil {
			syncErr = err
		}
		if result.Updated > 0 || result.Created > 0 {
			model.RefreshPricing()
		}
	}()
	timeout := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 30)
	if timeout < 1 || timeout > 300 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Second)
	defer cancel()
	source := modelCapabilitySourceURL()
	var entries, official map[string]model.ModelCapabilityMetadata
	var communityErr, officialErr error
	var warnings []string
	var fetches sync.WaitGroup
	fetches.Add(2)
	go func() { defer fetches.Done(); entries, communityErr = fetchModelCapabilities(ctx, source) }()
	go func() { defer fetches.Done(); official, warnings, officialErr = fetchOfficialModelAttributes(ctx) }()
	fetches.Wait()
	if entries == nil {
		entries = map[string]model.ModelCapabilityMetadata{}
	}
	for id, entry := range official {
		entries[id] = mergeOfficialAttributes(entries[id], entry)
	}
	if len(entries) == 0 && communityErr != nil {
		return model.ModelCapabilitySyncResult{}, communityErr
	}
	if officialErr != nil {
		return model.ModelCapabilitySyncResult{}, officialErr
	}
	if communityErr != nil {
		warnings = append(warnings, "Community metadata unavailable")
	}
	// Network timeouts must not prevent applying independently successful sources.
	applyCtx, applyCancel := context.WithTimeout(parent, 30*time.Second)
	defer applyCancel()
	result, err := model.ApplyModelCapabilityMetadata(applyCtx, entries, source)
	result.SourceFailures = len(warnings)
	if len(warnings) > 0 {
		common.SysError(fmt.Sprintf("model metadata sources incomplete: %d unavailable requests; prior official values retained", len(warnings)))
	}

	return result, err
}

func SyncModelCapabilities(c *gin.Context) {
	result, err := runModelCapabilitySync(c.Request.Context())
	if err != nil {
		if errors.Is(err, errModelCapabilityBusy) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.SysError("model capability sync failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "模型能力同步未完成，请稍后重试；未同步字段保留原有配置"})
		return
	}
	common.ApiSuccess(c, result)
}

func modelCapabilitySyncInterval() time.Duration {
	hours := common.GetEnvOrDefault("MODEL_METADATA_SYNC_INTERVAL_HOURS", 24)
	if hours < 1 || hours > 8760 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

func StartModelCapabilitySyncTask() {
	modelCapabilityTaskOnce.Do(func() {
		if !common.IsMasterNode || !common.GetEnvOrDefaultBool("MODEL_METADATA_SYNC_ENABLED", true) {
			return
		}
		interval := modelCapabilitySyncInterval()
		go func() {
			// Local inventory is cheap and independent of the daily network refresh.
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			runModelCatalogSyncTask(context.Background(), ticker.C, interval)
		}()
	})
}

// A cancellable loop lets lifecycle and cadence be exercised without real timers.
func runModelCatalogSyncTask(ctx context.Context, ticks <-chan time.Time, interval time.Duration) {
	nextSync := time.Now().Add(time.Minute)
	for {
		changed := false
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			created, err := reconcileModelCatalog(ctx)
			cancel()
			if err != nil {
				common.SysError("model catalog reconciliation failed: " + err.Error())
			}
			changed = created > 0
		case <-modelCatalogChanged:
			changed = true
		}
		if !changed && time.Now().Before(nextSync) {
			continue
		}
		// The local reconciliation above may itself have queued a signal.
		select {
		case <-modelCatalogChanged:
		default:
		}
		result, err := runModelCapabilitySync(ctx)
		if errors.Is(err, errModelCapabilityBusy) {
			nextSync = time.Now() // retry the coalesced change on the next local tick
			continue
		}
		nextSync = time.Now().Add(interval)
		if err != nil {
			common.SysError("scheduled model capability sync failed: " + err.Error())
		} else if result.Updated > 0 || result.Created > 0 {
			common.SysLog(fmt.Sprintf("model capability sync completed: updated=%d created=%d", result.Updated, result.Created))
		}
	}
}
