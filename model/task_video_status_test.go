package model

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInitialAndTerminalVideoStatusMappings(t *testing.T) {
	for _, tc := range []struct {
		input TaskStatus
		want  string
	}{
		{TaskStatusNotStart, dto.VideoStatusQueued},
		{TaskStatusSubmitted, dto.VideoStatusQueued},
		{TaskStatusQueued, dto.VideoStatusQueued},
		{TaskStatusInProgress, dto.VideoStatusInProgress},
		{TaskStatusSuccess, dto.VideoStatusCompleted},
		{TaskStatusFailure, dto.VideoStatusFailed},
		{TaskStatusUnknown, dto.VideoStatusUnknown},
		{TaskStatus("unexpected"), dto.VideoStatusUnknown},
	} {
		t.Run(string(tc.input), func(t *testing.T) { require.Equal(t, tc.want, tc.input.ToVideoStatus()) })
	}
}
