package channel

import (
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// DoTaskPollingRequest uses the credential selected for this task and its
// persisted owner. There is deliberately no client request/header context.
func DoTaskPollingRequest(info *relaycommon.RelayInfo, key string, req *http.Request, client *http.Client) (*http.Response, error) {
	if err := applyTaskPollingHeaderOverride(info, key, req); err != nil {
		return nil, err
	}
	return taskHeaderOverrideHTTPClient(client, info).Do(req)
}

func applyTaskPollingHeaderOverride(info *relaycommon.RelayInfo, key string, req *http.Request) error {
	if info != nil {
		pollingInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: key, HeadersOverride: relaycommon.GetEffectiveHeaderOverride(info),
		}}
		identity := &gin.Context{}
		identity.Set("id", info.UserId)
		if err := ApplyHeaderOverride(pollingInfo, identity, req); err != nil {
			return err
		}
	}
	return nil
}

// Content URLs retain their own host. Call only for credentialed content;
// public download URLs must not inherit a channel's private headers.
func DoTaskContentRequest(info *relaycommon.RelayInfo, key string, req *http.Request, client *http.Client) (*http.Response, error) {
	overrides := make(map[string]any)
	for name, value := range relaycommon.GetEffectiveHeaderOverride(info) {
		if name != "host" {
			overrides[name] = value
		}
	}
	contentInfo := &relaycommon.RelayInfo{UserId: info.UserId, ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: overrides}}
	if err := applyTaskPollingHeaderOverride(contentInfo, key, req); err != nil {
		return nil, err
	}
	// Native Authorization/x-goog-api-key and query credentials also require
	// redirect isolation, even when the channel has no Header Override.
	return taskSameOriginHTTPClient(client).Do(req)
}

// Custom credential headers are not among net/http's built-in sensitive names.
// Preserve same-origin redirects and the existing SSRF redirect checks, but do
// not forward configured task headers to another origin.
func taskHeaderOverrideHTTPClient(client *http.Client, info *relaycommon.RelayInfo) *http.Client {
	if len(relaycommon.GetEffectiveHeaderOverride(info)) == 0 {
		return client
	}
	return taskSameOriginHTTPClient(client)
}

func taskSameOriginHTTPClient(client *http.Client) *http.Client {
	copyClient := *client
	previous := client.CheckRedirect
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if previous != nil {
			if err := previous(req, via); err != nil {
				return err
			}
		}
		if len(via) > 0 && (req.URL.Scheme != via[0].URL.Scheme || req.URL.Host != via[0].URL.Host) {
			return http.ErrUseLastResponse
		}
		if previous == nil && len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return &copyClient
}
