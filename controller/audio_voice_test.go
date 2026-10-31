package controller

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func voiceFixtureWAV(seconds int) []byte {
	b := make([]byte, 44+seconds*16000*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(b)-44))
	return b
}

type voiceFixture struct {
	deleteStatus                         int
	deleteCode                           string
	handled                              chan struct{}
	db                                   *gorm.DB
	r                                    *gin.Engine
	mu                                   sync.Mutex
	providerID                           string
	creates, queries, syntheses, deletes int
	createStatus                         int
	channel                              model.Channel
}

func setupVoiceFixture(t *testing.T) *voiceFixture {
	t.Helper()
	require.NoError(t, i18n.Init())
	f := &voiceFixture{db: useControllerCapacityTestDB(t), handled: make(chan struct{}, 4)}
	sqlDB, err := f.db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, f.db.AutoMigrate(&model.AudioVoice{}, &model.User{}, &model.Token{}, &model.Ability{}, &model.ModelPermission{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
	oldRedis, oldBatch, oldCount, oldLog, oldErr, oldSQLite := common.RedisEnabled, common.BatchUpdateEnabled, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite
	common.RedisEnabled, common.BatchUpdateEnabled, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite = false, false, false, true, true, true
	t.Cleanup(func() {
		common.RedisEnabled, common.BatchUpdateEnabled, constant.CountToken, common.LogConsumeEnabled, constant.ErrorLogEnabled, common.UsingSQLite = oldRedis, oldBatch, oldCount, oldLog, oldErr, oldSQLite
	})
	model.InvalidateModelPermissionCache()
	t.Cleanup(model.InvalidateModelPermissionCache)
	oldRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"voice-fixture":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	service.InitHttpClient()
	service.InitTokenEncoders()
	for _, userID := range []int{997801, 997802} {
		require.NoError(t, f.db.Create(&model.User{Id: userID, Username: fmt.Sprint(userID), AffCode: fmt.Sprint(userID), Setting: `{"billing_preference":"wallet_only"}`, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}).Error)
		require.NoError(t, f.db.Create(&model.Token{Id: userID, UserId: userID, Name: "fixture", Key: fmt.Sprintf("voicekey%d", userID), Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000}).Error)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/v1/services/audio/tts/customization" {
			data, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			defer f.mu.Unlock()
			switch gjson.GetBytes(data, "input.action").String() {
			case "create_voice":
				f.creates++
				f.providerID = "cosyvoice-v3-flash-" + gjson.GetBytes(data, "input.prefix").String() + "-fixture"
				if f.createStatus != 0 {
					w.WriteHeader(f.createStatus)
					_, _ = w.Write([]byte(`{"secret":"upstream-secret"}`))
					return
				}
				_, _ = fmt.Fprintf(w, `{"output":{"voice_id":%q}}`, f.providerID)
			case "query_voice":
				f.queries++
				_, _ = w.Write([]byte(`{"output":{"status":"OK","target_model":"cosyvoice-v3-flash","resource_link":"private-audio"}}`))
			case "list_voice":
				_, _ = fmt.Fprintf(w, `{"output":{"voice_list":[{"voice_id":%q}]}}`, f.providerID)
			case "delete_voice":
				f.deletes++
				if f.deleteStatus != 0 {
					w.WriteHeader(f.deleteStatus)
					_, _ = fmt.Fprintf(w, `{"code":%q}`, f.deleteCode)
					return
				}
				_, _ = w.Write([]byte(`{"output":{}}`))
			default:
				w.WriteHeader(400)
			}
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			id := gjson.GetBytes(data, "header.task_id").String()
			switch gjson.GetBytes(data, "header.action").String() {
			case "run-task":
				f.mu.Lock()
				want := f.providerID
				f.mu.Unlock()
				if gjson.GetBytes(data, "payload.parameters.voice").String() != want {
					t.Error("upstream received the wrong voice")
					return
				}
				f.mu.Lock()
				f.syntheses++
				f.mu.Unlock()
				_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"header":{"event":"task-started","task_id":%q},"payload":{}}`, id)))
			case "finish-task":
				_ = conn.WriteMessage(websocket.BinaryMessage, []byte("audio"))
				_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"header":{"event":"task-finished","task_id":%q},"payload":{"usage":{"characters":5}}}`, id)))
			}
		}
	}))
	t.Cleanup(up.Close)
	f.channel = model.Channel{Id: 997803, Name: "voice", Type: constant.ChannelTypeAli, Status: common.ChannelStatusEnabled, Key: "upstream-secret", BaseURL: common.GetPointer(up.URL), Models: "voice-fixture", Group: "default", ModelMapping: common.GetPointer(`{"voice-fixture":"cosyvoice-v3-flash"}`)}
	require.NoError(t, f.db.Create(&f.channel).Error)
	require.NoError(t, f.db.Create(&model.Ability{Group: "default", Model: "voice-fixture", ChannelId: f.channel.Id, Enabled: true}).Error)
	f.r = gin.New()
	f.r.Use(middleware.RequestId(), middleware.BodyStorageCleanup(), middleware.TokenAuth(), middleware.TokenRuntimeLimit(), middleware.AudioVoicePrepare())
	f.r.GET("/v1/audio/voices", ListAudioVoices)
	g := f.r.Group("", middleware.Distribute())
	g.POST("/v1/audio/voices", CreateAudioVoice)
	g.GET("/v1/audio/voices/:voice_id", RetrieveAudioVoice)
	g.DELETE("/v1/audio/voices/:voice_id", DeleteAudioVoice)
	g.POST("/v1/audio/speech", func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIAudio) })
	g.GET("/v1/audio/speech/stream", func(c *gin.Context) { defer func() { f.handled <- struct{}{} }(); Relay(c, types.RelayFormatAliTTS) })
	return f
}

func (f *voiceFixture) call(method, path, key, contentType string, body io.Reader) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	r.Header.Set("Authorization", "Bearer sk-"+key)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, r)
	return w
}

func (f *voiceFixture) create(key, idem, name string, audio []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "voice-fixture")
	_ = writer.WriteField("name", name)
	part, _ := writer.CreateFormFile("file", "sample.wav")
	_, _ = part.Write(audio)
	_ = writer.Close()
	r := httptest.NewRequest("POST", "/v1/audio/voices", &body)
	r.Header.Set("Authorization", "Bearer sk-"+key)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Idempotency-Key", idem)
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, r)
	return w
}

func TestAudioVoiceLifecycleAndIsolation(t *testing.T) {
	f := setupVoiceFixture(t)
	w := f.create("voicekey997801", "first-voice-001", "我的声音", voiceFixtureWAV(6))
	require.Equal(t, 201, w.Code, w.Body.String())
	id := gjson.Get(w.Body.String(), "id").String()
	require.True(t, service.IsManagedAudioVoice(id))
	require.NotContains(t, w.Body.String(), "upstream-secret")
	require.NotContains(t, w.Body.String(), "channel")
	w = f.create("voicekey997801", "first-voice-001", "我的声音", voiceFixtureWAV(6))
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, id, gjson.Get(w.Body.String(), "id").String())
	w = f.create("voicekey997801", "first-voice-001", "不同的名称", voiceFixtureWAV(6))
	require.Equal(t, 409, w.Code)
	f.mu.Lock()
	require.Equal(t, 1, f.creates)
	f.mu.Unlock()
	for _, method := range []string{"GET", "DELETE"} {
		w = f.call(method, "/v1/audio/voices/"+id, "voicekey997802", "", nil)
		require.Equal(t, 404, w.Code, w.Body.String())
	}
	w = f.call("GET", "/v1/audio/voices", "voicekey997802", "", nil)
	require.Equal(t, 0, len(gjson.Get(w.Body.String(), "data").Array()))
	w = f.call("GET", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "ready", gjson.Get(w.Body.String(), "status").String())
	require.NotContains(t, w.Body.String(), "private-audio")
	// Even when a higher-priority account is available and capacity selection is
	// enabled, a managed voice must remain on its original account.
	enableControllerCapacityForTest(t)
	alternative := model.Channel{Id: 997806, Name: "other account", Type: constant.ChannelTypeAli, Status: common.ChannelStatusEnabled, Key: "other-account", Models: "voice-fixture", Group: "default", Priority: common.GetPointer(int64(100)), BaseURL: common.GetPointer("http://127.0.0.1:1")}
	require.NoError(t, f.db.Create(&alternative).Error)
	require.NoError(t, f.db.Create(&model.Ability{Group: "default", Model: "voice-fixture", ChannelId: alternative.Id, Enabled: true, Priority: alternative.Priority}).Error)
	for _, stream := range []string{"audio", "sse"} {
		w = f.call("POST", "/v1/audio/speech", "voicekey997801", "application/json", strings.NewReader(fmt.Sprintf(`{"model":"voice-fixture","voice":%q,"input":"你好","response_format":"pcm","stream_format":%q}`, id, stream)))
		require.Equal(t, 200, w.Code, w.Body.String())
		if stream == "audio" {
			require.Equal(t, "audio", w.Body.String())
		} else {
			require.Contains(t, w.Body.String(), "speech.audio.done")
		}
	}
	var user model.User
	require.NoError(t, f.db.First(&user, 997801).Error)
	require.Equal(t, 999990, user.Quota, "only synthesis consumes quota")
	w = f.call("POST", "/v1/audio/speech", "voicekey997802", "application/json", strings.NewReader(fmt.Sprintf(`{"model":"voice-fixture","voice":%q,"input":"你好"}`, id)))
	require.Equal(t, 404, w.Code)
	f.mu.Lock()
	raw := f.providerID
	f.mu.Unlock()
	w = f.call("POST", "/v1/audio/speech", "voicekey997802", "application/json", strings.NewReader(fmt.Sprintf(`{"model":"voice-fixture","voice":%q,"input":"你好"}`, raw)))
	require.Equal(t, 403, w.Code)
	w = f.call("GET", "/v1/audio/voices?limit=1&offset=0", "voicekey997801", "", nil)
	require.Equal(t, 1, len(gjson.Get(w.Body.String(), "data").Array()))
	w = f.call("DELETE", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = f.call("POST", "/v1/audio/speech", "voicekey997801", "application/json", strings.NewReader(fmt.Sprintf(`{"model":"voice-fixture","voice":%q,"input":"你好"}`, id)))
	require.Equal(t, 400, w.Code)
	w = f.call("DELETE", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 200, w.Code)
	f.mu.Lock()
	require.Equal(t, 1, f.deletes)
	require.Equal(t, 2, f.syntheses)
	f.mu.Unlock()
}

func TestAudioVoiceUnknownCreationRecoversWithoutReplay(t *testing.T) {
	f := setupVoiceFixture(t)
	f.createStatus = 503
	w := f.create("voicekey997801", "unknown-voice-001", "声音", voiceFixtureWAV(6))
	require.Equal(t, 502, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "upstream-secret")
	id := gjson.Get(w.Body.String(), "voice.id").String()
	require.NotEmpty(t, id)
	w = f.create("voicekey997801", "unknown-voice-001", "声音", voiceFixtureWAV(6))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "unknown", gjson.Get(w.Body.String(), "status").String())
	w = f.call("GET", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "ready", gjson.Get(w.Body.String(), "status").String())
	f.mu.Lock()
	require.Equal(t, 1, f.creates)
	f.mu.Unlock()
}

func TestAudioVoiceUploadAndPermissionBoundaries(t *testing.T) {
	f := setupVoiceFixture(t)
	for _, item := range []struct {
		key, idem string
		audio     []byte
		status    int
	}{
		{"invalid", "valid-idem", voiceFixtureWAV(6), 401},
		{"voicekey997801", "", voiceFixtureWAV(6), 400},
		{"voicekey997801", "valid-idem", voiceFixtureWAV(1), 400},
		{"voicekey997801", "valid-idem", voiceFixtureWAV(61), 400},
		{"voicekey997801", "valid-idem", []byte("not an audio file"), 400},
		{"voicekey997801", "valid-idem", make([]byte, service.MaxVoiceAudioBytes+1), 413},
	} {
		w := f.create(item.key, item.idem, "声音", item.audio)
		require.Equal(t, item.status, w.Code, w.Body.String())
	}
	w := f.create("voicekey997801", "valid-idem", "声音", voiceFixtureWAV(6))
	require.Equal(t, 201, w.Code, w.Body.String())
	id := gjson.Get(w.Body.String(), "id").String()
	for _, update := range []map[string]any{{"key": "changed"}, {"status": 2}} {
		require.NoError(t, f.db.Model(&model.Channel{}).Where("id = ?", f.channel.Id).Updates(update).Error)
		w = f.call("GET", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
		require.Equal(t, 403, w.Code, w.Body.String())
		require.NoError(t, f.db.Model(&model.Channel{}).Where("id = ?", f.channel.Id).Updates(map[string]any{"key": "upstream-secret", "status": 1}).Error)
	}
	for _, update := range []map[string]any{
		{"model_limits_enabled": true, "model_limits": "another-model"},
		{"channel_limits_enabled": true, "channel_limits": "999999"},
	} {
		require.NoError(t, f.db.Model(&model.Token{}).Where("id = ?", 997801).Updates(update).Error)
		w = f.call("GET", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
		require.Equal(t, 403, w.Code, w.Body.String())
		require.NoError(t, f.db.Model(&model.Token{}).Where("id = ?", 997801).Updates(map[string]any{"model_limits_enabled": false, "channel_limits_enabled": false}).Error)
	}
	require.NoError(t, f.db.Model(&model.Ability{}).Where("channel_id = ?", f.channel.Id).Update("enabled", false).Error)
	w = f.call("GET", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 403, w.Code, w.Body.String())
}

func TestAudioVoiceSellableTokenScope(t *testing.T) {
	f := setupVoiceFixture(t)
	for _, id := range []int{997804, 997805} {
		require.NoError(t, f.db.Create(&model.Token{Id: id, UserId: 997801, Key: fmt.Sprint("sellablevoice", id), Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000, SourceType: model.TokenSourceTypeSellableToken, BillingMode: model.TokenBillingModeTokenOnly}).Error)
	}
	w := f.create("sellablevoice997804", "sellable-voice-001", "独立声音", voiceFixtureWAV(6))
	require.Equal(t, 201, w.Code, w.Body.String())
	id := gjson.Get(w.Body.String(), "id").String()
	for _, key := range []string{"voicekey997801", "sellablevoice997805"} {
		w = f.call("GET", "/v1/audio/voices/"+id, key, "", nil)
		require.Equal(t, 404, w.Code, w.Body.String())
		w = f.call("GET", "/v1/audio/voices", key, "", nil)
		require.Len(t, gjson.Get(w.Body.String(), "data").Array(), 0)
	}
	w = f.call("GET", "/v1/audio/voices/"+id, "sellablevoice997804", "", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
}

func TestAudioVoiceDuplexPinsAndRechecksDeletedVoice(t *testing.T) {
	f := setupVoiceFixture(t)
	w := f.create("voicekey997801", "duplex-voice-001", "声音", voiceFixtureWAV(6))
	require.Equal(t, 201, w.Code, w.Body.String())
	id := gjson.Get(w.Body.String(), "id").String()
	w = f.call("GET", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 200, w.Code)
	server := httptest.NewServer(f.r)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/audio/speech/stream?model=voice-fixture&voice="+id, http.Header{"Authorization": []string{"Bearer sk-voicekey997801"}})
	require.NoError(t, err)
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	send := func(action, task string, payload map[string]any) {
		b, e := common.Marshal(map[string]any{"header": map[string]any{"action": action, "task_id": task, "streaming": "duplex"}, "payload": payload})
		require.NoError(t, e)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, b))
	}
	start := func(task string) {
		send("run-task", task, map[string]any{"task_group": "audio", "task": "tts", "function": "SpeechSynthesizer", "model": "voice-fixture", "parameters": map[string]any{"voice": id, "format": "pcm", "sample_rate": 24000}, "input": map[string]any{}})
	}
	start("first")
	_, b, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "task-started", gjson.GetBytes(b, "header.event").String())
	send("continue-task", "first", map[string]any{"input": map[string]any{"text": "你好"}})
	send("finish-task", "first", map[string]any{"input": map[string]any{}})
	kind, b, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, kind)
	require.Equal(t, "audio", string(b))
	_, b, err = conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "task-finished", gjson.GetBytes(b, "header.event").String())
	w = f.call("DELETE", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	start("after-delete")
	_, b, err = conn.ReadMessage()
	if err == nil {
		require.Contains(t, string(b), "error")
	}
	_ = conn.Close()
	select {
	case <-f.handled:
	case <-time.After(6 * time.Second):
		t.Fatal("duplex did not stop")
	}
	f.mu.Lock()
	require.Equal(t, 1, f.syntheses)
	f.mu.Unlock()
}

func TestAudioVoiceDeleteFailureAndAlreadyAbsent(t *testing.T) {
	f := setupVoiceFixture(t)
	w := f.create("voicekey997801", "delete-voice-001", "声音", voiceFixtureWAV(6))
	require.Equal(t, 201, w.Code, w.Body.String())
	id := gjson.Get(w.Body.String(), "id").String()
	f.mu.Lock()
	f.deleteStatus = 503
	f.mu.Unlock()
	w = f.call("DELETE", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 409, w.Code)
	require.Equal(t, "delete_failed", gjson.Get(w.Body.String(), "voice.status").String())
	f.mu.Lock()
	f.deleteStatus = 400
	f.deleteCode = "BadRequest.ResourceNotExist"
	f.mu.Unlock()
	w = f.call("DELETE", "/v1/audio/voices/"+id, "voicekey997801", "", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	stored, err := model.GetAudioVoice(id, 997801, 0)
	require.NoError(t, err)
	require.Equal(t, "deleted", stored.Status)
}
