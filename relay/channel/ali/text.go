package ali

import (
	"github.com/QuantumNous/new-api/dto"
	"strings"
)

// https://help.aliyun.com/document_detail/613695.html?spm=a2c4g.2399480.0.0.1adb778fAdzP9w#341800c0f8w0r

const EnableSearchModelSuffix = "-internet"

func requestOpenAI2Ali(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	// Qwen MT accepts top_p=1 and has its own default when omitted.
	if strings.HasPrefix(request.Model, "qwen-mt-") {
		return &request
	}
	if request.TopP >= 1 {
		request.TopP = 0.999
	} else if request.TopP <= 0 {
		request.TopP = 0.001
	}
	return &request
}
