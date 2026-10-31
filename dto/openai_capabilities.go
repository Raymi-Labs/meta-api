package dto

import (
	"strings"
	"time"
)

// OpenAIChatCapabilities keeps each request restriction independent. Unknown
// models retain their fields instead of inheriting future model restrictions.
type OpenAIChatCapabilities struct {
	UseMaxCompletionTokens bool
	UseDeveloperRole       bool
	SupportsTemperature    bool
	SupportsTopP           bool
	SupportsLogProbs       bool
}

func GetOpenAIChatCapabilities(model, effort string) OpenAIChatCapabilities {
	c := OpenAIChatCapabilities{SupportsTemperature: true, SupportsTopP: true, SupportsLogProbs: true}
	if strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4") {
		c.UseMaxCompletionTokens = true
		c.UseDeveloperRole = !strings.HasPrefix(model, "o1-mini") && !strings.HasPrefix(model, "o1-preview")
		c.SupportsTemperature = false
		return c
	}
	isGPT5 := model == "gpt-5" || strings.HasPrefix(model, "gpt-5-") || strings.HasPrefix(model, "gpt-5.")
	isGPT6SolLuna := isOpenAIModelSnapshot(model, "gpt-6-sol") || isOpenAIModelSnapshot(model, "gpt-6-luna")
	isGPT61Terra := isOpenAIModelSnapshot(model, "gpt-6.1-terra")
	if !isGPT5 && !isGPT6SolLuna && !isGPT61Terra && !isOpenAIModelSnapshot(model, "gpt-6-astra") && !isOpenAIModelSnapshot(model, "gpt-6.1-sol") {
		return c
	}
	c.UseMaxCompletionTokens, c.UseDeveloperRole = true, true
	sampling := false
	if effort == "" || effort == "none" {
		// Terra defaults to reasoning; sampling requires explicit none. Sol
		// never inherits the sampling exception, including dated snapshots.
		sampling = isGPT6SolLuna || (isGPT61Terra && effort == "none")
		for _, base := range []string{"gpt-5.1", "gpt-5.2", "gpt-5.4"} {
			if isOpenAIModelSnapshot(model, base) {
				sampling = true
				break
			}
		}
	}
	c.SupportsTemperature, c.SupportsTopP, c.SupportsLogProbs = sampling, sampling, sampling
	return c
}

func isOpenAIModelSnapshot(model, base string) bool {
	if model == base {
		return true
	}
	suffix, ok := strings.CutPrefix(model, base+"-")
	if !ok {
		return false
	}
	_, err := time.Parse(time.DateOnly, suffix)
	return err == nil
}
