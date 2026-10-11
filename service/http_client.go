package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"golang.org/x/net/proxy"
)

var (
	httpClient       *http.Client
	imageHttpClient  *http.Client
	proxyClientLock  sync.Mutex
	proxyClients     = make(map[string]*http.Client)
	imageProxyClient = make(map[string]*http.Client)
)

func checkRedirect(req *http.Request, via []*http.Request) error {
	fetchSetting := system_setting.GetFetchSetting()
	urlStr := req.URL.String()
	if err := common.ValidateURLWithFetchSetting(urlStr, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return fmt.Errorf("redirect to %s blocked: %v", urlStr, err)
	}
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

func InitHttpClient() {
	resetHeaderOverrideClients()
	httpClient = newRelayHttpClient(time.Duration(common.RelayResponseHeaderTimeout) * time.Second)
	imageHttpClient = newRelayHttpClient(time.Duration(common.RelayImageResponseHeaderTimeout) * time.Second)
}

func responseHeaderTimeout(client *http.Client) time.Duration {
	if client == nil {
		return 0
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil {
		return 0
	}
	return transport.ResponseHeaderTimeout
}

func GetResponseHeaderTimeout() time.Duration {
	return responseHeaderTimeout(GetHttpClient())
}

func GetImageResponseHeaderTimeout() time.Duration {
	return responseHeaderTimeout(GetImageHttpClient())
}

func newRelayTransport(responseHeaderTimeout time.Duration) *http.Transport {
	if responseHeaderTimeout < 0 {
		responseHeaderTimeout = 0
	}
	transport := &http.Transport{
		MaxIdleConns:          common.RelayMaxIdleConns,
		MaxIdleConnsPerHost:   common.RelayMaxIdleConnsPerHost,
		MaxConnsPerHost:       200,
		IdleConnTimeout:       time.Duration(common.RelayIdleConnTimeout) * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		Proxy:                 http.ProxyFromEnvironment, // Support HTTP_PROXY, HTTPS_PROXY, NO_PROXY env vars
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig.Clone()
	}
	return transport
}

func newRelayHttpClient(responseHeaderTimeout time.Duration) *http.Client {
	transport := newRelayTransport(responseHeaderTimeout)
	if common.RelayTimeout == 0 {
		return &http.Client{
			Transport:     transport,
			CheckRedirect: checkRedirect,
		}
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       time.Duration(common.RelayTimeout) * time.Second,
		CheckRedirect: checkRedirect,
	}
}

func GetHttpClient() *http.Client {
	return httpClient
}

func GetImageHttpClient() *http.Client {
	if imageHttpClient != nil {
		return imageHttpClient
	}
	return GetHttpClient()
}

// GetHttpClientWithProxy returns the default client or a proxy-enabled one when proxyURL is provided.
func GetHttpClientWithProxy(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return GetHttpClient(), nil
	}
	return NewProxyHttpClient(proxyURL, false)
}

func GetImageHttpClientWithProxy(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return GetImageHttpClient(), nil
	}
	return NewProxyHttpClient(proxyURL, true)
}

// ResetProxyClientCache 清空代理客户端缓存，确保下次使用时重新初始化
func ResetProxyClientCache() {
	resetHeaderOverrideClients()
	proxyClientLock.Lock()
	defer proxyClientLock.Unlock()
	for _, client := range proxyClients {
		if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
	}
	for _, client := range imageProxyClient {
		if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
	}
	proxyClients = make(map[string]*http.Client)
	imageProxyClient = make(map[string]*http.Client)
}

// NewProxyHttpClient 创建支持代理的 HTTP 客户端
func NewProxyHttpClient(proxyURL string, imageRequest ...bool) (*http.Client, error) {
	isImageRequest := len(imageRequest) > 0 && imageRequest[0]
	if proxyURL == "" {
		client := GetHttpClient()
		if isImageRequest {
			client = GetImageHttpClient()
		}
		if client != nil {
			return client, nil
		}
		return http.DefaultClient, nil
	}

	clientCache := proxyClients
	responseHeaderTimeout := time.Duration(common.RelayResponseHeaderTimeout) * time.Second
	if isImageRequest {
		clientCache = imageProxyClient
		responseHeaderTimeout = time.Duration(common.RelayImageResponseHeaderTimeout) * time.Second
	}

	proxyClientLock.Lock()
	if client, ok := clientCache[proxyURL]; ok {
		proxyClientLock.Unlock()
		return client, nil
	}
	proxyClientLock.Unlock()

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	switch parsedURL.Scheme {
	case "http", "https":
		transport := newRelayTransport(responseHeaderTimeout)
		transport.Proxy = http.ProxyURL(parsedURL)
		client := &http.Client{
			Transport:     transport,
			CheckRedirect: checkRedirect,
		}
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
		proxyClientLock.Lock()
		clientCache[proxyURL] = client
		proxyClientLock.Unlock()
		return client, nil

	case "socks5", "socks5h":
		// 获取认证信息
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{
				User:     parsedURL.User.Username(),
				Password: "",
			}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}

		// 创建 SOCKS5 代理拨号器
		// proxy.SOCKS5 使用 tcp 参数，所有 TCP 连接包括 DNS 查询都将通过代理进行。行为与 socks5h 相同
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}

		transport := &http.Transport{
			MaxIdleConns:          common.RelayMaxIdleConns,
			MaxIdleConnsPerHost:   common.RelayMaxIdleConnsPerHost,
			MaxConnsPerHost:       200,
			IdleConnTimeout:       time.Duration(common.RelayIdleConnTimeout) * time.Second,
			ResponseHeaderTimeout: responseHeaderTimeout,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.(proxy.ContextDialer).DialContext(ctx, network, addr)
			},
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig.Clone()
		}

		client := &http.Client{Transport: transport, CheckRedirect: checkRedirect}
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
		proxyClientLock.Lock()
		clientCache[proxyURL] = client
		proxyClientLock.Unlock()
		return client, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsedURL.Scheme)
	}
}
