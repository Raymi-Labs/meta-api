package types

import "fmt"

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
}

type TimeRatioInfo struct {
	Ratio     float64 `json:"ratio,omitempty"`
	RuleID    string  `json:"rule_id,omitempty"`
	Timezone  string  `json:"timezone,omitempty"`
	MatchedAt string  `json:"matched_at,omitempty"`
}

func (t TimeRatioInfo) EffectiveRatio() float64 {
	if t.Ratio <= 0 {
		return 1
	}
	return t.Ratio
}

func (t TimeRatioInfo) Matched() bool {
	return t.RuleID != ""
}

type PriceData struct {
	FreeModel                     bool
	ModelPrice                    float64
	ModelRatio                    float64
	CompletionRatio               float64
	CacheRatio                    float64
	CacheCreationRatio            float64
	CacheCreation5mRatio          float64
	CacheCreation1hRatio          float64
	ImageRatio                    float64
	AudioRatio                    float64
	AudioCompletionRatio          float64
	AudioDurationPrice            float64
	AudioDurationSeconds          int64
	OtherRatios                   map[string]float64
	UsePrice                      bool
	UseAudioDurationPrice         bool
	Quota                         int // 按次计费的最终额度（MJ / Task）
	QuotaToPreConsume             int // 按量计费的预消耗额度
	ConservativeQuotaToPreConsume int // 套餐令牌前置拦截使用的保守预估额度
	GroupRatioInfo                GroupRatioInfo
	TimeRatioInfo                 TimeRatioInfo
	EstimatedPromptTokens         int
	RequestedMaxTokens            int
	InitialChatQuota              int
	InitialCompletionTokens       int
}

func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if p.OtherRatios == nil {
		p.OtherRatios = make(map[string]float64)
	}
	if ratio <= 0 {
		return
	}
	p.OtherRatios[key] = ratio
}

func (p *PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, TimeRatio: %f, UsePrice: %t, UseAudioDurationPrice: %t, AudioDurationPrice: %f, AudioDurationSeconds: %d, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ConservativeQuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.TimeRatioInfo.EffectiveRatio(), p.UsePrice, p.UseAudioDurationPrice, p.AudioDurationPrice, p.AudioDurationSeconds, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ConservativeQuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio)
}
