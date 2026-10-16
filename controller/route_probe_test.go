package controller

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func probeRouter() *gin.Engine {
	r := gin.New()
	r.GET("/api/route-probe", RouteProbe)
	r.GET("/api/route-probe/stream", RouteProbeStream)
	return r
}

func TestRouteProbeNonceAndCache(t *testing.T) {
	r := probeRouter()
	nonce := strings.Repeat("a", 32)
	for _, query := range []string{"", "?nonce=bad", "?nonce=" + nonce + "&nonce=" + nonce, "?nonce=" + strings.Repeat("b", 4096)} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/route-probe"+query, nil))
		if w.Code != 400 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("invalid nonce: %d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/route-probe?nonce="+nonce, nil))
	var value map[string]interface{}
	if err := common.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || value["nonce"] != nonce || value["service"] != "yuanheng-api" || value["version"] != float64(1) || len(value) != 3 {
		t.Fatalf("unexpected probe response: %s", w.Body.String())
	}
}

func TestRouteProbeStreamFlushAndCancel(t *testing.T) {
	server := httptest.NewServer(probeRouter())
	defer server.Close()
	request, _ := http.NewRequest("GET", server.URL+"/api/route-probe/stream?nonce="+strings.Repeat("c", 32), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := http.DefaultClient.Do(request.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	count := 0
	var last time.Time
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data: ") {
			continue
		}
		var value map[string]interface{}
		if err := common.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &value); err != nil {
			t.Fatal(err)
		}
		if value["sequence"] != float64(count) {
			t.Fatalf("invalid sequence: %v", value)
		}
		if count > 0 && time.Since(last) < 150*time.Millisecond {
			t.Fatal("stream was buffered")
		}
		last = time.Now()
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("got %d frames", count)
	}
	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	w := httptest.NewRecorder()
	probeRouter().ServeHTTP(w, httptest.NewRequest("GET", "/api/route-probe/stream?nonce="+strings.Repeat("c", 32), nil).WithContext(cancelled))
	if strings.Count(w.Body.String(), "data:") > 1 {
		t.Fatal("cancelled stream continued")
	}
}
