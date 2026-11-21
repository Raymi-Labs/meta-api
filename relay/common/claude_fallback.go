package common

import "github.com/QuantumNous/new-api/dto"

const ClaudeFallbackPrimary = "claude-opus-5-5"
const ClaudeFallbackTarget = "claude-opus-4-8"

// This state is local to one upstream attempt. Only the original RelayInfo
// owns a billing session; the target carries its independent pricing snapshot.
type ClaudeFallback struct {
	Target          *RelayInfo
	ServingModel    string
	PrimaryCategory string
	FinalCategory   string
	StopReason      string
	Attempts        []ClaudeFallbackAttempt
	IncompleteUsage bool
}

type ClaudeFallbackAttempt struct {
	Model string
	Usage dto.Usage
}
