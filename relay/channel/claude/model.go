package claude

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/reasoning"
)

// IsClaude47Model reports whether model is a Claude 4.7 family model (e.g. claude-sonnet-4-7).
func IsClaude47Model(model string) bool {
	baseModel := model
	if trimmed, _, ok := reasoning.TrimEffortSuffix(model); ok {
		baseModel = trimmed
	}
	baseModel = strings.TrimSuffix(baseModel, "-thinking")
	return strings.HasPrefix(baseModel, "claude-") && strings.Contains(baseModel, "-4-7")
}

// ClearSamplingParamsForClaude47 removes temperature, top_p and top_k for Claude 4.7 models.
func ClearSamplingParamsForClaude47(request *dto.ClaudeRequest) {
	if request == nil || !IsClaude47Model(request.Model) {
		return
	}
	request.Temperature = nil
	request.TopP = nil
	request.TopK = nil
}
