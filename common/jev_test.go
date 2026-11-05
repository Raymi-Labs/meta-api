package common

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestJevDecisionEndpoints(t *testing.T) {
	for _, name := range []string{"typesafe/jev-1.14", "typesafe/jev-1.13"} {
		require.Equal(t, []constant.EndpointType{constant.EndpointTypeDecisions}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenRouter, name))
	}
	for _, name := range []string{"typesafe/jev-router", "typesafe/jev-latest", "other/jev-1.14"} {
		require.False(t, IsJevDecisionModel(name))
	}
}
