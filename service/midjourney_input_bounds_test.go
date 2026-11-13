package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMidjourneyMalformedActionDoesNotPanic(t *testing.T) {
	for _, id := range []string{"MJ", "MJ::JOB", "MJ::JOB::upsample", "MJ::JOB::variation"} {
		require.NotPanics(t, func() { require.NotNil(t, CoverPlusActionToNormalAction(&dto.MidjourneyRequest{CustomId: id})) }, id)
	}
	for _, content := range []string{"task ", "task u", "task v", " U1", "task U111"} {
		require.NotPanics(t, func() { require.Nil(t, ConvertSimpleChangeParams(content)) }, content)
	}
	require.Equal(t, "UPSCALE", ConvertSimpleChangeParams("task U1").Action)
	require.Equal(t, "REROLL", ConvertSimpleChangeParams("task R").Action)
}
