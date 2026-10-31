package ali

import "github.com/QuantumNous/new-api/common"

var ModelList = append([]string{
	"qwen-turbo",
	"qwen-plus",
	"qwen-max",
	"qwen-max-longcontext",
	"qwq-32b",
	"qwen3-235b-a22b",
	"text-embedding-v1",
	"gte-rerank-v2",
}, common.AliTTSModels...)

var ChannelName = "ali"
