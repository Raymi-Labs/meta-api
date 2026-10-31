// Validate an operational plan through the same override engine used by relay.
package main

import (
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/tidwall/gjson"
)

type modelRow struct {
	ChannelID         int                    `json:"channel_id"`
	Model             string                 `json:"model"`
	Upstream          string                 `json:"upstream_model"`
	Status            string                 `json:"status"`
	Price             map[string]interface{} `json:"official_price"`
	Providers         []string               `json:"providers"`
	ImageTokenPricing bool                   `json:"image_token_pricing"`
}

func must(ok bool, message string) {
	if !ok {
		panic(message)
	}
}

func read(path string, value interface{}) {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	if err = common.Unmarshal(b, value); err != nil {
		panic(err)
	}
}

func main() {
	must(len(os.Args) == 3, "usage: openrouter-price-check AUDIT.json PLAN.json")
	var audit struct {
		Models []modelRow `json:"models"`
	}
	var plan struct {
		Channels []struct {
			Patch struct {
				ID       int    `json:"id"`
				Override string `json:"param_override"`
			} `json:"patch"`
		} `json:"channels"`
	}
	read(os.Args[1], &audit)
	read(os.Args[2], &plan)
	overrides := map[int]map[string]interface{}{}
	for _, ch := range plan.Channels {
		var override map[string]interface{}
		must(common.UnmarshalJsonStr(ch.Patch.Override, &override) == nil, "bad override")
		overrides[ch.Patch.ID] = override
	}
	checks := 0
	apply := func(row modelRow, body map[string]interface{}, wantError bool) []byte {
		b, err := common.Marshal(body)
		if err != nil {
			panic(err)
		}
		out, err := relaycommon.ApplyParamOverride(b, overrides[row.ChannelID],
			map[string]interface{}{"original_model": row.Model, "model": row.Upstream})
		checks++
		must((err != nil) == wantError, fmt.Sprintf("%s error=%v expected=%v", row.Model, err, wantError))
		return out
	}
	for _, row := range audit.Models {
		if row.Status != "enabled" {
			apply(row, map[string]interface{}{"model": row.Upstream}, true)
			continue
		}
		for _, supplied := range []float64{999999, 0} {
			body := map[string]interface{}{"model": row.Upstream, "service_tier": "priority", "speed": "fast",
				"tools": []interface{}{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "web_search", "parameters": map[string]interface{}{"type": "object"}}}},
				"provider": map[string]interface{}{"sort": "throughput", "order": []string{"mara"}, "allow_fallbacks": false,
					"zdr": true, "data_collection": "deny", "ignore": []string{"untrusted"},
					"max_price": map[string]interface{}{"prompt": supplied, "completion": supplied}}}
			out := apply(row, body, false)
			must(gjson.GetBytes(out, "provider.sort").String() == "price", "sort")
			must(gjson.GetBytes(out, "provider.allow_fallbacks").Bool(), "fallbacks")
			must(gjson.GetBytes(out, "service_tier").String() == "default", "tier")
			must(!gjson.GetBytes(out, "speed").Exists() && !gjson.GetBytes(out, "provider.order").Exists(), "priority/order escape")
			must(gjson.GetBytes(out, "provider.zdr").Bool() && gjson.GetBytes(out, "provider.data_collection").String() == "deny", "privacy weakened")
			must(gjson.GetBytes(out, "provider.ignore.0").String() == "untrusted", "ignore lost")
			for _, key := range []string{"prompt", "completion"} {
				want := row.Price[key].(float64)
				if supplied < want {
					want = supplied
				}
				must(gjson.GetBytes(out, "provider.max_price."+key).Float() == want, "wrong cap or stricter cap raised")
			}
			for _, key := range []string{"request", "image"} {
				if key == "image" && row.ImageTokenPricing {
					must(!gjson.GetBytes(out, "provider.max_price.image").Exists(), "Gemini per-token image quote must not be filtered as zero per-image cost")
					continue
				}
				p := gjson.GetBytes(out, "provider.max_price."+key)
				must(p.Exists() && p.Float() == 0, "extra fee cap")
			}
			var actual []string
			must(common.UnmarshalJsonStr(gjson.GetBytes(out, "provider.only").Raw, &actual) == nil, "only array")
			must(fmt.Sprint(actual) == fmt.Sprint(row.Providers), "provider allowlist")
			must(gjson.GetBytes(out, "tools.0.function.name").String() == "web_search", "function tool lost")
		}
		out := apply(row, map[string]interface{}{"model": row.Upstream}, false)
		must(gjson.GetBytes(out, "provider.max_price.prompt").Float() == row.Price["prompt"].(float64), "missing cap")
		for _, extra := range []map[string]interface{}{
			{"model": "unreviewed/new-model"},
			{"model": row.Upstream + ":nitro"},
			{"plugins": []interface{}{map[string]interface{}{"id": "web"}}},
			{"tools": []interface{}{map[string]interface{}{"type": "web_search"}}},
			{"models": []string{"unreviewed/new-model"}},
			{"provider": map[string]interface{}{"only": []string{"mara"}}},
			{"provider": map[string]interface{}{"options": map[string]interface{}{"search": true}}},
			{"provider": map[string]interface{}{"max_price": map[string]interface{}{"prompt": "unlimited"}}},
			{"web_search_options": map[string]interface{}{}},
			{"preset": "expensive"},
		} {
			body := map[string]interface{}{"model": row.Upstream}
			for k, v := range extra {
				body[k] = v
			}
			apply(row, body, true)
		}
	}
	fmt.Printf("PASS: %d wire-policy checks across %d models and %d channels\n", checks, len(audit.Models), len(plan.Channels))
}
