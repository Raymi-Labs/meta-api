package dto

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestImageRequestAcceptsQuotedN(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-image-2","prompt":"cat","n":"2"}`,
		`{"model":"gpt-image-2","prompt":"cat","n":" 2 "}`,
		`{"model":"gpt-image-2","prompt":"cat","n":2}`,
	} {
		req := &ImageRequest{}
		if err := common.Unmarshal([]byte(body), req); err != nil {
			t.Fatalf("%s: unmarshal: %v", body, err)
		}
		if req.N == nil || *req.N != 2 {
			t.Fatalf("%s: expected n=2, got %v", body, req.N)
		}
		if _, ok := req.Extra["n"]; ok {
			t.Fatalf("%s: n must not leak into Extra", body)
		}
		out, err := common.Marshal(req)
		if err != nil || !strings.Contains(string(out), `"n":2`) {
			t.Fatalf("%s: expected integer n in output, got %s (%v)", body, out, err)
		}
	}
}

func TestImageRequestNullN(t *testing.T) {
	req := &ImageRequest{}
	if err := common.Unmarshal([]byte(`{"model":"m","prompt":"cat","n":null}`), req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.N != nil {
		t.Fatalf("expected nil n, got %d", *req.N)
	}
}

func TestImageRequestRejectsInvalidN(t *testing.T) {
	for _, n := range []string{`"abc"`, `"1.5"`, `"-1"`, `-1`, `1.5`, `true`, `""`} {
		req := &ImageRequest{}
		err := common.Unmarshal([]byte(`{"model":"m","prompt":"cat","n":`+n+`}`), req)
		if err == nil || !strings.Contains(err.Error(), "n must be an integer between 1 and") {
			t.Fatalf("n=%s: expected friendly error, got %v", n, err)
		}
	}
}

func TestImageBillingQuantityAcceptsQuotedAliParametersN(t *testing.T) {
	req := &ImageRequest{}
	if err := common.Unmarshal([]byte(`{"model":"wan","prompt":"cat","parameters":{"n":"3"}}`), req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	count, _, err := req.ImageBillingQuantity(true)
	if err != nil || count != 3 {
		t.Fatalf("expected count 3, got %d (%v)", count, err)
	}
	req.Extra["parameters"] = []byte(`{"n":"x"}`)
	if _, _, err := req.ImageBillingQuantity(true); err == nil || !strings.Contains(err.Error(), "parameters.n must be an integer") {
		t.Fatalf("expected friendly parameters.n error, got %v", err)
	}
}
