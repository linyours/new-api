package service

import (
	"net/http"

	chselector "github.com/QuantumNous/new-api/pkg/channel_selector"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// RecordRelayAttempt records one upstream attempt for multi-factor selection stats.
// No-op when the selector flag is off.
func RecordRelayAttempt(channelID int, modelName string, apiErr *types.NewAPIError, latencyMs int64) {
	if !chselector.Enabled() || channelID <= 0 || modelName == "" {
		return
	}
	outcome := classifyAttemptOutcome(apiErr)
	chselector.RecordAttempt(channelID, modelName, outcome, latencyMs)
}

func classifyAttemptOutcome(apiErr *types.NewAPIError) chselector.AttemptOutcome {
	if apiErr == nil {
		return chselector.OutcomeSuccess
	}
	code := apiErr.StatusCode
	if code == http.StatusBadRequest {
		return chselector.OutcomeIgnored
	}
	// Client / non-retryable errors below 500 (except 429) do not reflect channel health.
	if types.IsSkipRetryError(apiErr) && code > 0 && code < 500 && code != http.StatusTooManyRequests {
		return chselector.OutcomeIgnored
	}
	return chselector.OutcomeChannelFault
}
