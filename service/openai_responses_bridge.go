package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	responsesBridgeResultContextKey = "responses_bridge_result"
	responsesMediaTransportBytesKey = "responses_media_transport_total_base64"

	defaultResponsesMediaTTL        = 15 * time.Minute
	defaultResponsesMediaSweep      = 2 * time.Minute
	defaultResponsesSessionTTL      = 30 * time.Minute
	defaultResponsesSessionSweep    = 5 * time.Minute
	defaultResponsesSessionPerScope = 64

	responsesMediaTransportModeOption       = "OpenAIResponsesMediaTransportMode"
	responsesMediaBridgeEnabledOption       = "OpenAIResponsesMediaBridgeEnabled"
	responsesMediaBridgeTTLOption           = "OpenAIResponsesMediaBridgeTTLSeconds"
	responsesMediaBridgeSweepOption         = "OpenAIResponsesMediaBridgeSweepSeconds"
	responsesMediaBridgePathOption          = "OpenAIResponsesMediaBridgePath"
	responsesSessionBridgeEnabledOption     = "OpenAIResponsesSessionBridgeEnabled"
	responsesSessionBridgeTTLOption         = "OpenAIResponsesSessionBridgeTTLSeconds"
	responsesSessionBridgeSweepOption       = "OpenAIResponsesSessionBridgeSweepSeconds"
	responsesSessionBridgeMaxOption         = "OpenAIResponsesSessionBridgeMaxPerScope"
	responsesSessionBridgeUseRedisOption    = "OpenAIResponsesSessionBridgeUseRedis"
	responsesBridgeDetailedLogEnabledOption = "OpenAIResponsesBridgeLogEnabled"

	responsesMediaTransportModeEnv       = "OPENAI_RESPONSES_MEDIA_TRANSPORT_MODE"
	responsesMediaBridgeEnabledEnv       = "OPENAI_RESPONSES_MEDIA_BRIDGE_ENABLED"
	responsesMediaBridgeTTLEnv           = "OPENAI_RESPONSES_MEDIA_BRIDGE_TTL_SECONDS"
	responsesMediaBridgeSweepEnv         = "OPENAI_RESPONSES_MEDIA_BRIDGE_SWEEP_SECONDS"
	responsesMediaBridgePathEnv          = "OPENAI_RESPONSES_MEDIA_BRIDGE_PATH"
	responsesSessionBridgeEnabledEnv     = "OPENAI_RESPONSES_SESSION_BRIDGE_ENABLED"
	responsesSessionBridgeTTLEnv         = "OPENAI_RESPONSES_SESSION_TTL_SECONDS"
	responsesSessionBridgeSweepEnv       = "OPENAI_RESPONSES_SESSION_SWEEP_SECONDS"
	responsesSessionBridgeMaxEnv         = "OPENAI_RESPONSES_SESSION_MAX_PER_SCOPE"
	responsesSessionBridgeUseRedisEnv    = "OPENAI_RESPONSES_SESSION_BRIDGE_USE_REDIS"
	responsesBridgeDetailedLogEnabledEnv = "OPENAI_RESPONSES_BRIDGE_LOG_ENABLED"

	responsesSessionRedisValuePrefix = "responses_bridge:session"
	responsesSessionRedisIndexPrefix = "responses_bridge:session_index"
)

var (
	responsesMediaAutoSingleInlineLimit = 2 * 1024 * 1024
	responsesMediaAutoTotalInlineLimit  = 8 * 1024 * 1024
)

type ResponsesBridgeResult struct {
	ResponseID       string
	AssistantMessage dto.Message
}

type responsesMediaEntry struct {
	ID        string
	FilePath  string
	MimeType  string
	Size      int64
	ExpiresAt time.Time
}

type responsesSessionEntry struct {
	ResponseID string
	Hash       string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

type responsesMediaStore struct {
	mu      sync.RWMutex
	entries map[string]*responsesMediaEntry
	once    sync.Once
}

type responsesSessionStore struct {
	mu      sync.RWMutex
	entries map[string]map[string]*responsesSessionEntry
	once    sync.Once
}

type responsesMediaCleanupStats struct {
	DeletedFiles int
	DeletedBytes int64
	ScanErrors   int
}

type ResponsesSessionMatch struct {
	ResponseID    string
	Trimmed       *dto.GeneralOpenAIRequest
	PrefixLength  int
	ScopeKey      string
	Conversation  string
	OriginalCount int
}

var (
	errResponsesBridgeURLMissing = errors.New("public bridge URL is unavailable")

	openAIResponsesMediaStore = &responsesMediaStore{
		entries: make(map[string]*responsesMediaEntry),
	}
	openAIResponsesSessionStore = &responsesSessionStore{
		entries: make(map[string]map[string]*responsesSessionEntry),
	}
	responsesBridgeInitOnce                     sync.Once
	responsesPreviousResponseIDUnsupportedStore sync.Map
)

func InitOpenAIResponsesBridge() {
	responsesBridgeInitOnce.Do(func() {
		stats, err := openAIResponsesMediaStore.cleanupOrphanedFiles(true)
		if err != nil {
			common.SysError("responses bridge startup cleanup failed: " + err.Error())
		} else if stats.DeletedFiles > 0 {
			responsesBridgeInfoLog("startup cleanup removed %d residual media files (%s)", stats.DeletedFiles, formatResponsesBridgeBytes(stats.DeletedBytes))
		}

		mediaDir, dirErr := getResponsesMediaBridgeDir()
		if dirErr != nil {
			mediaDir = "unavailable: " + dirErr.Error()
		}
		responsesBridgeInfoLog(
			"initialized: media_enabled=%t media_ttl=%s media_sweep=%s media_dir=%s session_enabled=%t session_backend=%s session_ttl=%s session_sweep=%s detailed_log=%t",
			getResponsesMediaBridgeEnabled(),
			getResponsesMediaBridgeTTL(),
			getResponsesMediaBridgeSweepInterval(),
			mediaDir,
			getResponsesSessionBridgeEnabled(),
			getResponsesSessionBackendName(),
			getResponsesSessionTTL(),
			getResponsesSessionSweepInterval(),
			getResponsesBridgeDetailedLogEnabled(),
		)

		openAIResponsesMediaStore.startCleanupLoop()
		openAIResponsesSessionStore.startCleanupLoop()
	})
}

func SetResponsesBridgeResult(c *gin.Context, responseID string, assistant dto.Message) {
	if c == nil || strings.TrimSpace(responseID) == "" {
		return
	}
	c.Set(responsesBridgeResultContextKey, ResponsesBridgeResult{
		ResponseID:       strings.TrimSpace(responseID),
		AssistantMessage: assistant,
	})
}

func GetResponsesBridgeResult(c *gin.Context) (ResponsesBridgeResult, bool) {
	if c == nil {
		return ResponsesBridgeResult{}, false
	}
	value, ok := c.Get(responsesBridgeResultContextKey)
	if !ok || value == nil {
		return ResponsesBridgeResult{}, false
	}
	result, ok := value.(ResponsesBridgeResult)
	return result, ok
}

func ClaudeImageSourceToMessageImageURL(c *gin.Context, source *dto.ClaudeMessageSource) (*dto.MessageImageUrl, error) {
	if source == nil {
		return nil, nil
	}

	if remoteURL := strings.TrimSpace(source.Url); remoteURL != "" {
		return &dto.MessageImageUrl{
			Url:      remoteURL,
			Detail:   "auto",
			MimeType: strings.TrimSpace(source.MediaType),
		}, nil
	}

	rawData := strings.TrimSpace(common.Interface2String(source.Data))
	if rawData == "" {
		return nil, nil
	}

	mimeType := strings.TrimSpace(source.MediaType)
	cleanBase64 := rawData
	if strings.HasPrefix(strings.ToLower(cleanBase64), "data:") {
		idx := strings.Index(cleanBase64, ",")
		if idx > 0 {
			header := cleanBase64[:idx]
			cleanBase64 = cleanBase64[idx+1:]
			if mimeType == "" {
				mimeType = extractMimeTypeFromDataURLHeader(header)
			}
		}
	}

	if strings.HasPrefix(cleanBase64, "http://") || strings.HasPrefix(cleanBase64, "https://") {
		return &dto.MessageImageUrl{
			Url:      cleanBase64,
			Detail:   "auto",
			MimeType: mimeType,
		}, nil
	}

	effectiveMode := getEffectiveClaudeImageTransportMode(c)
	if effectiveMode == dto.ClaudeImageTransportModeData ||
		(effectiveMode == dto.ClaudeImageTransportModeAuto && shouldUseResponsesAutoDataURL(c, len(cleanBase64))) {
		imageURL, err := buildResponsesDataURLFromBase64(cleanBase64, mimeType)
		if err != nil {
			return nil, err
		}
		if effectiveMode == dto.ClaudeImageTransportModeAuto {
			incrementResponsesMediaTransportBytes(c, len(cleanBase64))
		}
		responsesBridgeDebugLog("media transport mode=%s selected data url size=%d", effectiveMode, len(cleanBase64))
		return imageURL, nil
	}

	if !getResponsesMediaBridgeEnabled() {
		responsesBridgeInfoLog("media bridge disabled under mode=%s, falling back to data URL", effectiveMode)
		return buildResponsesDataURLFromBase64(cleanBase64, mimeType)
	}

	imageBytes, err := decodeResponsesMediaBase64(cleanBase64)
	if err != nil {
		return nil, err
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(imageBytes)
	}

	entry, err := openAIResponsesMediaStore.put(imageBytes, mimeType)
	if err != nil {
		responsesBridgeInfoLog("media bridge write failed, falling back to data URL: %v", err)
		return buildResponsesDataURL(cleanBase64, mimeType, imageBytes)
	}

	signedURL, err := buildResponsesMediaBridgeURL(c, entry.ID, entry.ExpiresAt)
	if err != nil {
		_ = openAIResponsesMediaStore.delete(entry.ID)
		responsesBridgeInfoLog("media bridge URL unavailable, falling back to data URL: %v", err)
		return buildResponsesDataURL(cleanBase64, mimeType, imageBytes)
	}

	responsesBridgeDebugLog("media transport mode=%s selected bridge url size=%d", effectiveMode, len(cleanBase64))

	return &dto.MessageImageUrl{
		Url:      signedURL,
		Detail:   "auto",
		MimeType: mimeType,
	}, nil
}

func ServeOpenAIResponsesMedia(c *gin.Context) {
	if c == nil {
		return
	}

	entry, file, err := openAIResponsesMediaStore.open(c.Param("id"), c.Query("e"), c.Query("s"))
	if err != nil {
		statusCode := http.StatusNotFound
		if errors.Is(err, os.ErrPermission) {
			statusCode = http.StatusForbidden
		}
		c.AbortWithStatusJSON(statusCode, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "bridge_media_error",
			},
		})
		return
	}
	defer file.Close()

	c.Header("Cache-Control", "private, no-store, max-age=0")
	c.Header("Content-Type", entry.MimeType)
	c.Header("Content-Length", strconv.FormatInt(entry.Size, 10))
	c.DataFromReader(http.StatusOK, entry.Size, entry.MimeType, file, nil)
}

func ApplyResponsesSessionBridge(info *relaycommon.RelayInfo, req *dto.GeneralOpenAIRequest) (*ResponsesSessionMatch, error) {
	if !getResponsesSessionBridgeEnabled() || info == nil || req == nil || len(req.Messages) < 2 {
		return nil, nil
	}
	if hasResponsesPreviousResponseIDUnsupported(info) {
		return nil, nil
	}

	scopeKey, err := buildResponsesSessionScopeKey(info, req)
	if err != nil {
		return nil, err
	}

	match, prefixHash, prefixLen := openAIResponsesSessionStore.find(scopeKey, req.Messages)
	if match == nil || strings.TrimSpace(match.ResponseID) == "" || prefixLen <= 0 || prefixLen >= len(req.Messages) {
		return nil, nil
	}

	trimmed := cloneChatRequestWithMessages(req, req.Messages[prefixLen:])
	return &ResponsesSessionMatch{
		ResponseID:    match.ResponseID,
		Trimmed:       trimmed,
		PrefixLength:  prefixLen,
		ScopeKey:      scopeKey,
		Conversation:  prefixHash,
		OriginalCount: len(req.Messages),
	}, nil
}

func StoreResponsesSessionBridge(info *relaycommon.RelayInfo, req *dto.GeneralOpenAIRequest, assistant dto.Message, responseID string) error {
	if !getResponsesSessionBridgeEnabled() || info == nil || req == nil || strings.TrimSpace(responseID) == "" {
		return nil
	}
	if hasResponsesPreviousResponseIDUnsupported(info) {
		return nil
	}

	scopeKey, err := buildResponsesSessionScopeKey(info, req)
	if err != nil {
		return err
	}

	fullMessages := append(copyMessages(req.Messages), assistant)
	if len(fullMessages) == 0 {
		return nil
	}

	fullHash, err := hashResponsesConversation(fullMessages)
	if err != nil {
		return err
	}

	openAIResponsesSessionStore.put(scopeKey, fullHash, strings.TrimSpace(responseID))
	return nil
}

func MarkResponsesPreviousResponseIDUnsupported(info *relaycommon.RelayInfo) {
	key, err := buildResponsesPreviousResponseIDCapabilityKey(info)
	if err != nil || key == "" {
		return
	}
	responsesPreviousResponseIDUnsupportedStore.Store(key, struct{}{})
}

func hasResponsesPreviousResponseIDUnsupported(info *relaycommon.RelayInfo) bool {
	// Native DeepSeek Responses is stateless. Never trim caller history into a
	// reference that the provider ignores, even before a capability probe fails.
	if info != nil && info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeDeepSeek {
		return true
	}
	key, err := buildResponsesPreviousResponseIDCapabilityKey(info)
	if err != nil || key == "" {
		return false
	}
	_, ok := responsesPreviousResponseIDUnsupportedStore.Load(key)
	return ok
}

func (s *responsesMediaStore) put(data []byte, mimeType string) (*responsesMediaEntry, error) {
	s.startCleanupLoop()

	dir, err := getResponsesMediaBridgeDir()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create bridge media directory: %w", err)
	}

	id := uuid.NewString()
	filePath := filepath.Join(dir, id+".bin")
	if err = os.WriteFile(filePath, data, 0600); err != nil {
		return nil, fmt.Errorf("write bridge media file: %w", err)
	}

	entry := &responsesMediaEntry{
		ID:        id,
		FilePath:  filePath,
		MimeType:  normalizeMediaMimeType(mimeType),
		Size:      int64(len(data)),
		ExpiresAt: time.Now().Add(getResponsesMediaBridgeTTL()),
	}

	s.mu.Lock()
	s.entries[id] = entry
	s.mu.Unlock()

	responsesBridgeDebugLog("media bridge stored file=%s size=%s ttl=%s", filePath, formatResponsesBridgeBytes(entry.Size), time.Until(entry.ExpiresAt).Round(time.Second))
	return entry, nil
}

func (s *responsesMediaStore) open(id string, expiresAtRaw string, signature string) (*responsesMediaEntry, *os.File, error) {
	s.startCleanupLoop()

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, os.ErrNotExist
	}

	expiresAtUnix, err := strconv.ParseInt(strings.TrimSpace(expiresAtRaw), 10, 64)
	if err != nil {
		return nil, nil, os.ErrPermission
	}
	expiresAt := time.Unix(expiresAtUnix, 0)
	if time.Now().After(expiresAt) {
		return nil, nil, os.ErrPermission
	}
	if !validateResponsesMediaSignature(id, expiresAtUnix, strings.TrimSpace(signature)) {
		return nil, nil, os.ErrPermission
	}

	s.mu.RLock()
	entry, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok || entry == nil {
		return nil, nil, os.ErrNotExist
	}
	if time.Now().After(entry.ExpiresAt) {
		_ = s.delete(id)
		return nil, nil, os.ErrNotExist
	}

	file, err := os.Open(entry.FilePath)
	if err != nil {
		_ = s.delete(id)
		return nil, nil, os.ErrNotExist
	}
	return entry, file, nil
}

func (s *responsesMediaStore) delete(id string) error {
	_, err := s.deleteAndMeasure(id)
	return err
}

func (s *responsesMediaStore) deleteAndMeasure(id string) (int64, error) {
	s.mu.Lock()
	entry, ok := s.entries[id]
	if ok {
		delete(s.entries, id)
	}
	s.mu.Unlock()
	if !ok || entry == nil {
		return 0, nil
	}

	deletedSize := entry.Size
	if entry.FilePath != "" {
		if err := os.Remove(entry.FilePath); err != nil && !os.IsNotExist(err) {
			return deletedSize, err
		}
	}
	return deletedSize, nil
}

func (s *responsesMediaStore) cleanupExpired() responsesMediaCleanupStats {
	now := time.Now()
	var expiredIDs []string

	s.mu.RLock()
	for id, entry := range s.entries {
		if entry == nil || now.After(entry.ExpiresAt) {
			expiredIDs = append(expiredIDs, id)
		}
	}
	s.mu.RUnlock()

	stats := responsesMediaCleanupStats{}
	for _, id := range expiredIDs {
		deletedSize, err := s.deleteAndMeasure(id)
		if err != nil {
			stats.ScanErrors++
			common.SysError("responses bridge cleanup expired file failed: " + err.Error())
			continue
		}
		stats.DeletedFiles++
		stats.DeletedBytes += deletedSize
	}
	return stats
}

func (s *responsesMediaStore) cleanupOrphanedFiles(deleteAll bool) (responsesMediaCleanupStats, error) {
	stats := responsesMediaCleanupStats{}

	dir, err := getResponsesMediaBridgeDir()
	if err != nil {
		return stats, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return stats, err
	}

	activeEntries := make(map[string]*responsesMediaEntry)
	s.mu.RLock()
	for id, entry := range s.entries {
		activeEntries[id] = entry
	}
	s.mu.RUnlock()

	now := time.Now()
	for _, diskEntry := range entries {
		if diskEntry.IsDir() {
			continue
		}
		info, infoErr := diskEntry.Info()
		if infoErr != nil {
			stats.ScanErrors++
			continue
		}

		filename := diskEntry.Name()
		filePath := filepath.Join(dir, filename)
		id := strings.TrimSuffix(filename, filepath.Ext(filename))
		activeEntry, active := activeEntries[id]

		shouldDelete := deleteAll
		if !shouldDelete {
			if !active || activeEntry == nil {
				shouldDelete = true
			} else if now.After(activeEntry.ExpiresAt) {
				shouldDelete = true
			}
		}
		if !shouldDelete {
			continue
		}

		if err := os.Remove(filePath); err != nil {
			if !os.IsNotExist(err) {
				stats.ScanErrors++
				common.SysError("responses bridge cleanup orphaned file failed: " + err.Error())
			}
			continue
		}

		if active {
			s.mu.Lock()
			delete(s.entries, id)
			s.mu.Unlock()
		}
		stats.DeletedFiles++
		stats.DeletedBytes += info.Size()
	}

	return stats, nil
}

func (s *responsesMediaStore) startCleanupLoop() {
	s.once.Do(func() {
		go func() {
			ticker := time.NewTicker(getResponsesMediaBridgeSweepInterval())
			defer ticker.Stop()
			for range ticker.C {
				expiredStats := s.cleanupExpired()
				orphanStats, err := s.cleanupOrphanedFiles(false)
				merged := mergeResponsesMediaCleanupStats(expiredStats, orphanStats)
				if err != nil {
					common.SysError("responses bridge cleanup scan failed: " + err.Error())
				}
				if merged.DeletedFiles > 0 || merged.ScanErrors > 0 {
					responsesBridgeInfoLog("media cleanup deleted %d files (%s), errors=%d", merged.DeletedFiles, formatResponsesBridgeBytes(merged.DeletedBytes), merged.ScanErrors)
				}
			}
		}()
	})
}

func (s *responsesSessionStore) put(scopeKey string, conversationHash string, responseID string) {
	if scopeKey == "" || conversationHash == "" || responseID == "" {
		return
	}

	if shouldUseResponsesSessionRedis() {
		if err := s.putRedis(scopeKey, conversationHash, responseID); err == nil {
			responsesBridgeDebugLog("session bridge stored in redis scope=%s", scopeKey)
			return
		} else {
			responsesBridgeInfoLog("session bridge redis write failed, falling back to memory: %v", err)
		}
	}

	s.putMemory(scopeKey, conversationHash, responseID)
	responsesBridgeDebugLog("session bridge stored in memory scope=%s", scopeKey)
}

func (s *responsesSessionStore) putMemory(scopeKey string, conversationHash string, responseID string) {
	s.startCleanupLoop()

	now := time.Now()
	entry := &responsesSessionEntry{
		ResponseID: responseID,
		Hash:       conversationHash,
		CreatedAt:  now,
		ExpiresAt:  now.Add(getResponsesSessionTTL()),
	}

	s.mu.Lock()
	scopeEntries := s.entries[scopeKey]
	if scopeEntries == nil {
		scopeEntries = make(map[string]*responsesSessionEntry)
		s.entries[scopeKey] = scopeEntries
	}
	scopeEntries[conversationHash] = entry
	s.trimScopeLocked(scopeEntries)
	s.mu.Unlock()
}

func (s *responsesSessionStore) putRedis(scopeKey string, conversationHash string, responseID string) error {
	ctx := context.Background()
	ttl := getResponsesSessionTTL()
	valueKey := responsesSessionRedisValueKey(scopeKey, conversationHash)
	indexKey := responsesSessionRedisIndexKey(scopeKey)
	now := time.Now()

	pipe := common.RDB.TxPipeline()
	pipe.Set(ctx, valueKey, responseID, ttl)
	pipe.ZAdd(ctx, indexKey, &redis.Z{Score: float64(now.UnixMilli()), Member: conversationHash})
	pipe.Expire(ctx, indexKey, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	if err := trimResponsesSessionRedisScope(ctx, scopeKey, indexKey); err != nil {
		responsesBridgeInfoLog("session bridge redis trim failed: %v", err)
	}
	return nil
}

func (s *responsesSessionStore) find(scopeKey string, messages []dto.Message) (*responsesSessionEntry, string, int) {
	if scopeKey == "" || len(messages) == 0 {
		return nil, "", 0
	}

	if shouldUseResponsesSessionRedis() {
		entry, hash, prefixLen, err := s.findRedis(scopeKey, messages)
		if err == nil {
			if entry != nil {
				responsesBridgeDebugLog("session bridge redis hit scope=%s prefix=%d", scopeKey, prefixLen)
			} else {
				responsesBridgeDebugLog("session bridge redis miss scope=%s", scopeKey)
			}
			return entry, hash, prefixLen
		}
		responsesBridgeInfoLog("session bridge redis read failed, falling back to memory: %v", err)
	}

	entry, hash, prefixLen := s.findMemory(scopeKey, messages)
	if entry != nil {
		responsesBridgeDebugLog("session bridge memory hit scope=%s prefix=%d", scopeKey, prefixLen)
	} else {
		responsesBridgeDebugLog("session bridge memory miss scope=%s", scopeKey)
	}
	return entry, hash, prefixLen
}

func (s *responsesSessionStore) findMemory(scopeKey string, messages []dto.Message) (*responsesSessionEntry, string, int) {
	s.startCleanupLoop()

	var best *responsesSessionEntry
	var bestHash string
	bestLen := 0

	for i := len(messages) - 1; i > 0; i-- {
		hash, err := hashResponsesConversation(messages[:i])
		if err != nil {
			continue
		}

		s.mu.RLock()
		scopeEntries := s.entries[scopeKey]
		entry := scopeEntries[hash]
		s.mu.RUnlock()
		if entry == nil {
			continue
		}
		if time.Now().After(entry.ExpiresAt) {
			continue
		}

		best = entry
		bestHash = hash
		bestLen = i
		break
	}

	return best, bestHash, bestLen
}

func (s *responsesSessionStore) findRedis(scopeKey string, messages []dto.Message) (*responsesSessionEntry, string, int, error) {
	ctx := context.Background()
	ttl := getResponsesSessionTTL()
	indexKey := responsesSessionRedisIndexKey(scopeKey)

	for i := len(messages) - 1; i > 0; i-- {
		hash, err := hashResponsesConversation(messages[:i])
		if err != nil {
			continue
		}
		valueKey := responsesSessionRedisValueKey(scopeKey, hash)
		responseID, err := common.RDB.Get(ctx, valueKey).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, "", 0, err
		}
		responseID = strings.TrimSpace(responseID)
		if responseID == "" {
			continue
		}

		_ = common.RDB.Expire(ctx, valueKey, ttl).Err()
		_ = common.RDB.Expire(ctx, indexKey, ttl).Err()
		now := time.Now()
		return &responsesSessionEntry{
			ResponseID: responseID,
			Hash:       hash,
			CreatedAt:  now,
			ExpiresAt:  now.Add(ttl),
		}, hash, i, nil
	}

	return nil, "", 0, nil
}

func (s *responsesSessionStore) cleanupExpired() {
	now := time.Now()
	removed := 0

	s.mu.Lock()
	for scopeKey, scopeEntries := range s.entries {
		for hash, entry := range scopeEntries {
			if entry == nil || now.After(entry.ExpiresAt) {
				delete(scopeEntries, hash)
				removed++
			}
		}
		if len(scopeEntries) == 0 {
			delete(s.entries, scopeKey)
		}
	}
	s.mu.Unlock()

	if removed > 0 {
		responsesBridgeDebugLog("session bridge memory cleanup removed %d entries", removed)
	}
}

func (s *responsesSessionStore) trimScopeLocked(scopeEntries map[string]*responsesSessionEntry) {
	maxPerScope := getResponsesSessionMaxPerScope()
	if len(scopeEntries) <= maxPerScope {
		return
	}

	type kv struct {
		Hash  string
		Entry *responsesSessionEntry
	}

	items := make([]kv, 0, len(scopeEntries))
	for hash, entry := range scopeEntries {
		items = append(items, kv{Hash: hash, Entry: entry})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Entry == nil {
			return true
		}
		if items[j].Entry == nil {
			return false
		}
		return items[i].Entry.CreatedAt.Before(items[j].Entry.CreatedAt)
	})

	for len(items) > maxPerScope {
		delete(scopeEntries, items[0].Hash)
		items = items[1:]
	}
}

func (s *responsesSessionStore) startCleanupLoop() {
	s.once.Do(func() {
		go func() {
			ticker := time.NewTicker(getResponsesSessionSweepInterval())
			defer ticker.Stop()
			for range ticker.C {
				s.cleanupExpired()
			}
		}()
	})
}

func buildResponsesMediaBridgeURL(c *gin.Context, mediaID string, expiresAt time.Time) (string, error) {
	baseURL, err := getResponsesBridgeBaseURL(c)
	if err != nil {
		return "", err
	}
	expiresAtUnix := expiresAt.Unix()
	signature := signResponsesMediaURL(mediaID, expiresAtUnix)
	return fmt.Sprintf("%s/v1/bridge/media/%s?e=%d&s=%s", baseURL, mediaID, expiresAtUnix, signature), nil
}

func getResponsesBridgeBaseURL(c *gin.Context) (string, error) {
	serverAddress := strings.TrimSpace(system_setting.ServerAddress)
	if serverAddress != "" {
		return strings.TrimRight(serverAddress, "/"), nil
	}
	if c == nil || c.Request == nil {
		return "", errResponsesBridgeURLMissing
	}

	host := getRequestHost(c.Request)
	if host == "" {
		return "", errResponsesBridgeURLMissing
	}
	scheme := getRequestScheme(c.Request)
	if scheme == "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, host), nil
}

func getRequestHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		parts := strings.Split(forwardedHost, ",")
		return strings.TrimSpace(parts[0])
	}
	if host := strings.TrimSpace(r.Host); host != "" {
		return host
	}
	if r.URL != nil {
		return strings.TrimSpace(r.URL.Host)
	}
	return ""
}

func getRequestScheme(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		parts := strings.Split(forwardedProto, ",")
		return strings.ToLower(strings.TrimSpace(parts[0]))
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Protocol")); forwardedProto != "" {
		return strings.ToLower(forwardedProto)
	}
	if r.TLS != nil {
		return "https"
	}
	if r.URL != nil && strings.TrimSpace(r.URL.Scheme) != "" {
		return strings.ToLower(strings.TrimSpace(r.URL.Scheme))
	}
	return "http"
}

func signResponsesMediaURL(mediaID string, expiresAtUnix int64) string {
	return common.GenerateHMAC(fmt.Sprintf("%s:%d", mediaID, expiresAtUnix))
}

func validateResponsesMediaSignature(mediaID string, expiresAtUnix int64, signature string) bool {
	if signature == "" {
		return false
	}
	expected := signResponsesMediaURL(mediaID, expiresAtUnix)
	return signature == expected
}

func getResponsesMediaBridgeDir() (string, error) {
	basePath := strings.TrimSpace(getResponsesBridgeStringOption(responsesMediaBridgePathOption, responsesMediaBridgePathEnv, ""))
	if basePath == "" {
		basePath = common.GetDiskCachePath()
	}
	if basePath == "" {
		basePath = os.TempDir()
	}
	if basePath == "" {
		return "", fmt.Errorf("bridge media base path is empty")
	}
	return filepath.Join(basePath, "new-api-responses-media-bridge"), nil
}

func getResponsesMediaBridgeEnabled() bool {
	return getResponsesBridgeBoolOption(responsesMediaBridgeEnabledOption, responsesMediaBridgeEnabledEnv, true)
}

func getResponsesMediaTransportMode() dto.ClaudeImageTransportMode {
	mode := dto.ClaudeImageTransportMode(
		getResponsesBridgeStringOption(
			responsesMediaTransportModeOption,
			responsesMediaTransportModeEnv,
			string(dto.ClaudeImageTransportModeAuto),
		),
	)
	return dto.NormalizeClaudeImageTransportMode(mode)
}

func getResponsesMediaBridgeTTL() time.Duration {
	seconds := getResponsesBridgeIntOption(responsesMediaBridgeTTLOption, responsesMediaBridgeTTLEnv, int(defaultResponsesMediaTTL/time.Second))
	if seconds <= 0 {
		seconds = int(defaultResponsesMediaTTL / time.Second)
	}
	return time.Duration(seconds) * time.Second
}

func getResponsesMediaBridgeSweepInterval() time.Duration {
	seconds := getResponsesBridgeIntOption(responsesMediaBridgeSweepOption, responsesMediaBridgeSweepEnv, int(defaultResponsesMediaSweep/time.Second))
	if seconds <= 0 {
		seconds = int(defaultResponsesMediaSweep / time.Second)
	}
	return time.Duration(seconds) * time.Second
}

func getResponsesSessionBridgeEnabled() bool {
	return getResponsesBridgeBoolOption(responsesSessionBridgeEnabledOption, responsesSessionBridgeEnabledEnv, true)
}

func getResponsesSessionTTL() time.Duration {
	seconds := getResponsesBridgeIntOption(responsesSessionBridgeTTLOption, responsesSessionBridgeTTLEnv, int(defaultResponsesSessionTTL/time.Second))
	if seconds <= 0 {
		seconds = int(defaultResponsesSessionTTL / time.Second)
	}
	return time.Duration(seconds) * time.Second
}

func getResponsesSessionSweepInterval() time.Duration {
	seconds := getResponsesBridgeIntOption(responsesSessionBridgeSweepOption, responsesSessionBridgeSweepEnv, int(defaultResponsesSessionSweep/time.Second))
	if seconds <= 0 {
		seconds = int(defaultResponsesSessionSweep / time.Second)
	}
	return time.Duration(seconds) * time.Second
}

func getResponsesSessionMaxPerScope() int {
	maxPerScope := getResponsesBridgeIntOption(responsesSessionBridgeMaxOption, responsesSessionBridgeMaxEnv, defaultResponsesSessionPerScope)
	if maxPerScope <= 0 {
		maxPerScope = defaultResponsesSessionPerScope
	}
	return maxPerScope
}

func getResponsesSessionBridgeUseRedis() bool {
	return getResponsesBridgeBoolOption(responsesSessionBridgeUseRedisOption, responsesSessionBridgeUseRedisEnv, true)
}

func getResponsesBridgeDetailedLogEnabled() bool {
	return getResponsesBridgeBoolOption(responsesBridgeDetailedLogEnabledOption, responsesBridgeDetailedLogEnabledEnv, false)
}

func getEffectiveClaudeImageTransportMode(c *gin.Context) dto.ClaudeImageTransportMode {
	if c != nil {
		if channelSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting); ok {
			if override := channelSetting.GetClaudeImageTransportMode(); override != dto.ClaudeImageTransportModeInherit {
				return dto.NormalizeClaudeImageTransportMode(override)
			}
		}
	}
	return getResponsesMediaTransportMode()
}

func shouldUseResponsesAutoDataURL(c *gin.Context, base64Size int) bool {
	if base64Size <= 0 || base64Size > responsesMediaAutoSingleInlineLimit {
		return false
	}
	return getResponsesMediaTransportBytes(c)+base64Size <= responsesMediaAutoTotalInlineLimit
}

func getResponsesMediaTransportBytes(c *gin.Context) int {
	if c == nil {
		return 0
	}
	if value, ok := c.Get(responsesMediaTransportBytesKey); ok {
		if total, ok := value.(int); ok {
			return total
		}
	}
	return 0
}

func incrementResponsesMediaTransportBytes(c *gin.Context, size int) {
	if c == nil || size <= 0 {
		return
	}
	c.Set(responsesMediaTransportBytesKey, getResponsesMediaTransportBytes(c)+size)
}

func shouldUseResponsesSessionRedis() bool {
	return getResponsesSessionBridgeEnabled() && getResponsesSessionBridgeUseRedis() && common.RedisEnabled && common.RDB != nil
}

func getResponsesSessionBackendName() string {
	if !getResponsesSessionBridgeEnabled() {
		return "disabled"
	}
	if shouldUseResponsesSessionRedis() {
		return "redis"
	}
	return "memory"
}

func buildResponsesPreviousResponseIDCapabilityKey(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", nil
	}

	payload := map[string]any{
		"channel_id":     info.ChannelId,
		"api_type":       info.ApiType,
		"base_url":       strings.TrimSpace(info.ChannelBaseUrl),
		"origin_model":   strings.TrimSpace(info.OriginModelName),
		"upstream_model": strings.TrimSpace(info.UpstreamModelName),
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	return common.GenerateHMAC(string(data)), nil
}

func buildResponsesSessionScopeKey(info *relaycommon.RelayInfo, req *dto.GeneralOpenAIRequest) (string, error) {
	toolsHash, err := hashAny(req.Tools)
	if err != nil {
		return "", err
	}
	toolChoiceHash, err := hashAny(req.ToolChoice)
	if err != nil {
		return "", err
	}
	responseFormatHash, err := hashAny(req.ResponseFormat)
	if err != nil {
		return "", err
	}
	metadataHash, err := hashAny(req.Metadata)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"user_id":          info.UserId,
		"token_id":         info.TokenId,
		"channel_id":       info.ChannelId,
		"origin_model":     info.OriginModelName,
		"request_model":    req.Model,
		"tools_hash":       toolsHash,
		"tool_choice_hash": toolChoiceHash,
		"format_hash":      responseFormatHash,
		"metadata_hash":    metadataHash,
		"reasoning_effort": req.ReasoningEffort,
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	return common.GenerateHMAC(string(data)), nil
}

func hashResponsesConversation(messages []dto.Message) (string, error) {
	messageDigests := make([]string, 0, len(messages))
	for _, message := range messages {
		digest, err := hashResponsesMessage(message)
		if err != nil {
			return "", err
		}
		messageDigests = append(messageDigests, digest)
	}
	return common.GenerateHMAC(strings.Join(messageDigests, "|")), nil
}

func hashResponsesMessage(message dto.Message) (string, error) {
	contentSummary, err := summarizeMessageContent(message)
	if err != nil {
		return "", err
	}

	toolCallsSummary, err := summarizeToolCalls(message.ParseToolCalls())
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"role":         message.Role,
		"name":         stringPointerValue(message.Name),
		"tool_call_id": message.ToolCallId,
		"content":      contentSummary,
		"tool_calls":   toolCallsSummary,
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	return common.GenerateHMAC(string(data)), nil
}

func summarizeMessageContent(message dto.Message) ([]map[string]string, error) {
	var contentSummary []map[string]string

	switch {
	case message.Content == nil:
		return nil, nil
	case message.IsStringContent():
		return []map[string]string{{
			"type": "text",
			"hash": digestString(message.StringContent()),
		}}, nil
	default:
		for _, part := range message.ParseContent() {
			item := map[string]string{
				"type": part.Type,
			}
			switch part.Type {
			case dto.ContentTypeText:
				item["hash"] = digestString(part.Text)
			case dto.ContentTypeImageURL:
				item["hash"] = digestString(extractImageURL(part.ImageUrl))
			case dto.ContentTypeInputAudio:
				item["hash"] = digestString(extractInputAudio(part.InputAudio))
			case dto.ContentTypeFile:
				item["hash"] = digestString(extractFileIdentifier(part.File))
			case dto.ContentTypeVideoUrl:
				item["hash"] = digestString(extractVideoURL(part.VideoUrl))
			default:
				blobHash, err := hashAny(part)
				if err != nil {
					return nil, err
				}
				item["hash"] = blobHash
			}
			contentSummary = append(contentSummary, item)
		}
	}

	return contentSummary, nil
}

func summarizeToolCalls(toolCalls []dto.ToolCallRequest) ([]map[string]string, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	result := make([]map[string]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		item := map[string]string{
			"id":       toolCall.ID,
			"type":     toolCall.Type,
			"name":     toolCall.Function.Name,
			"arg_hash": digestString(toolCall.Function.Arguments),
		}
		if len(toolCall.Custom) > 0 {
			item["custom_hash"] = digestString(string(toolCall.Custom))
		}
		result = append(result, item)
	}
	return result, nil
}

func cloneChatRequestWithMessages(req *dto.GeneralOpenAIRequest, messages []dto.Message) *dto.GeneralOpenAIRequest {
	if req == nil {
		return nil
	}
	cloned := *req
	cloned.Messages = copyMessages(messages)
	return &cloned
}

func copyMessages(messages []dto.Message) []dto.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]dto.Message, len(messages))
	copy(out, messages)
	return out
}

func normalizeMediaMimeType(mimeType string) string {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}

func extractMimeTypeFromDataURLHeader(header string) string {
	header = strings.TrimSpace(strings.TrimPrefix(header, "data:"))
	if idx := strings.Index(header, ";"); idx >= 0 {
		header = header[:idx]
	}
	return strings.TrimSpace(header)
}

func decodeResponsesMediaBase64(base64Data string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Data))
	if err != nil {
		return nil, fmt.Errorf("decode claude image data: %w", err)
	}
	return decoded, nil
}

func buildResponsesDataURLFromBase64(base64Data string, mimeType string) (*dto.MessageImageUrl, error) {
	if mimeType != "" {
		return buildResponsesDataURL(base64Data, mimeType, nil)
	}
	imageBytes, err := decodeResponsesMediaBase64(base64Data)
	if err != nil {
		return nil, err
	}
	return buildResponsesDataURL(base64Data, mimeType, imageBytes)
}

func buildResponsesDataURL(base64Data string, mimeType string, imageBytes []byte) (*dto.MessageImageUrl, error) {
	base64Data = strings.TrimSpace(base64Data)
	if base64Data == "" {
		return nil, nil
	}
	if mimeType == "" && len(imageBytes) > 0 {
		mimeType = http.DetectContentType(imageBytes)
	}
	mimeType = normalizeMediaMimeType(mimeType)
	return &dto.MessageImageUrl{
		Url:      fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data),
		Detail:   "auto",
		MimeType: mimeType,
	}, nil
}

func hashAny(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	data, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	return common.GenerateHMAC(string(data)), nil
}

func digestString(value string) string {
	if value == "" {
		return ""
	}
	return common.GenerateHMAC(value)
}

func extractImageURL(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		return common.Interface2String(v["url"])
	case dto.MessageImageUrl:
		return v.Url
	case *dto.MessageImageUrl:
		if v != nil {
			return v.Url
		}
	}
	return ""
}

func extractInputAudio(value any) string {
	switch v := value.(type) {
	case map[string]any:
		return common.Interface2String(v["format"]) + ":" + common.Interface2String(v["data"])
	case dto.MessageInputAudio:
		return v.Format + ":" + v.Data
	case *dto.MessageInputAudio:
		if v != nil {
			return v.Format + ":" + v.Data
		}
	}
	return ""
}

func extractFileIdentifier(value any) string {
	switch v := value.(type) {
	case map[string]any:
		fileID := common.Interface2String(v["file_id"])
		if fileID != "" {
			return fileID
		}
		return common.Interface2String(v["filename"]) + ":" + common.Interface2String(v["file_data"])
	case dto.MessageFile:
		if v.FileId != "" {
			return v.FileId
		}
		return v.FileName + ":" + v.FileData
	case *dto.MessageFile:
		if v != nil {
			if v.FileId != "" {
				return v.FileId
			}
			return v.FileName + ":" + v.FileData
		}
	}
	return ""
}

func extractVideoURL(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		return common.Interface2String(v["url"])
	case dto.MessageVideoUrl:
		return v.Url
	case *dto.MessageVideoUrl:
		if v != nil {
			return v.Url
		}
	}
	return ""
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func getResponsesBridgeStringOption(key string, env string, defaultValue string) string {
	common.OptionMapRWMutex.RLock()
	value, ok := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(common.GetEnvOrDefaultString(env, defaultValue))
}

func getResponsesBridgeIntOption(key string, env string, defaultValue int) int {
	common.OptionMapRWMutex.RLock()
	value, ok := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if ok && strings.TrimSpace(value) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
		common.SysError(fmt.Sprintf("responses bridge invalid int option %s=%q: %v", key, value, err))
		return defaultValue
	}
	return common.GetEnvOrDefault(env, defaultValue)
}

func getResponsesBridgeBoolOption(key string, env string, defaultValue bool) bool {
	common.OptionMapRWMutex.RLock()
	value, ok := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if ok && strings.TrimSpace(value) != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
		common.SysError(fmt.Sprintf("responses bridge invalid bool option %s=%q: %v", key, value, err))
		return defaultValue
	}
	return common.GetEnvOrDefaultBool(env, defaultValue)
}

func responsesSessionRedisValueKey(scopeKey string, conversationHash string) string {
	return fmt.Sprintf("%s:%s:%s", responsesSessionRedisValuePrefix, scopeKey, conversationHash)
}

func responsesSessionRedisIndexKey(scopeKey string) string {
	return fmt.Sprintf("%s:%s", responsesSessionRedisIndexPrefix, scopeKey)
}

func trimResponsesSessionRedisScope(ctx context.Context, scopeKey string, indexKey string) error {
	maxPerScope := int64(getResponsesSessionMaxPerScope())
	if maxPerScope <= 0 {
		return nil
	}
	count, err := common.RDB.ZCard(ctx, indexKey).Result()
	if err != nil {
		return err
	}
	if count <= maxPerScope {
		return nil
	}

	removeCount := count - maxPerScope
	members, err := common.RDB.ZRange(ctx, indexKey, 0, removeCount-1).Result()
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}

	pipe := common.RDB.TxPipeline()
	removals := make([]interface{}, 0, len(members))
	for _, member := range members {
		removals = append(removals, member)
		pipe.Del(ctx, responsesSessionRedisValueKey(scopeKey, member))
	}
	pipe.ZRem(ctx, indexKey, removals...)
	pipe.Expire(ctx, indexKey, getResponsesSessionTTL())
	_, err = pipe.Exec(ctx)
	return err
}

func mergeResponsesMediaCleanupStats(left responsesMediaCleanupStats, right responsesMediaCleanupStats) responsesMediaCleanupStats {
	return responsesMediaCleanupStats{
		DeletedFiles: left.DeletedFiles + right.DeletedFiles,
		DeletedBytes: left.DeletedBytes + right.DeletedBytes,
		ScanErrors:   left.ScanErrors + right.ScanErrors,
	}
}

func responsesBridgeInfoLog(format string, args ...any) {
	common.SysLog(fmt.Sprintf("[responses bridge] "+format, args...))
}

func responsesBridgeDebugLog(format string, args ...any) {
	if !getResponsesBridgeDetailedLogEnabled() {
		return
	}
	responsesBridgeInfoLog(format, args...)
}

func formatResponsesBridgeBytes(size int64) string {
	if size <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(size)
	unit := units[0]
	for i := 1; i < len(units) && value >= 1024; i++ {
		value /= 1024
		unit = units[i]
	}
	if unit == "B" {
		return fmt.Sprintf("%d %s", size, unit)
	}
	return fmt.Sprintf("%.2f %s", value, unit)
}
