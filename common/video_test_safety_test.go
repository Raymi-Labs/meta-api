package common

import "testing"

func TestVideoTestSafetyDetection(t *testing.T) {
	for _, name := range []string{"wan2.6-t2v", "wanx2.1-i2v", "veo3.1", "org/veo-3.1", "kling-v2.6", "MiniMax-Hailuo-02", "grok-imagine-video-1.5", "happyhorse-1.1-t2v", "doubao-seedance-2.0", "sora-2"} {
		if !IsVideoGenerationTest(1, name) {
			t.Errorf("missed video model %s", name)
		}
	}
	for _, name := range []string{"gpt-4o", "kimi-k3", "qwen-vl", "MiniMax-M2.5", "seedream-4.0"} {
		if IsVideoGenerationTest(1, name) {
			t.Errorf("misclassified text/image model %s", name)
		}
	}
}
