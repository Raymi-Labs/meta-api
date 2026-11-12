package model

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

type OpenRouterSiteActivity struct {
	Date     string `json:"date"`
	Model    string `json:"model"`
	Quota    int64  `json:"quota"`
	Requests int64  `json:"requests"`
}

// Current key ownership is a reporting scope, not proof of historical ownership.
func openRouterOwnedChannels(channels []Channel, hashes map[string]bool) []int {
	ids := []int{}
	for _, ch := range channels {
		keys := ch.GetKeys()
		owned := len(keys) > 0
		for _, key := range keys {
			if !hashes[cashKeyHash(strings.TrimSpace(key))] {
				owned = false
				break
			}
		}
		if owned {
			ids = append(ids, ch.Id)
		}
	}
	return ids
}

func fetchOpenRouterSiteActivity(ctx context.Context, client *http.Client, account *UpstreamAccount, start, end time.Time) ([]OpenRouterSiteActivity, int, error) {
	hashes, err := cashRemoteKeyHashes(ctx, client, account)
	if err != nil {
		return nil, 0, err
	}
	var channels []Channel
	if err = DB.WithContext(ctx).Select("id", "key").Where("type = ?", constant.ChannelTypeOpenRouter).Find(&channels).Error; err != nil {
		return nil, 0, errors.New("本站渠道查询失败")
	}
	ids := openRouterOwnedChannels(channels, hashes)
	if len(ids) == 0 {
		return nil, 0, errors.New("未找到当前密钥归属此账户的本站渠道")
	}
	rows, err := queryOpenRouterSiteActivity(ctx, ids, start, end)
	return rows, len(ids), err
}

func queryOpenRouterSiteActivity(ctx context.Context, ids []int, start, end time.Time) ([]OpenRouterSiteActivity, error) {
	totals := map[string]*OpenRouterSiteActivity{}
	// Stream only required fields; bounded time and indexed channel/type/time filter.
	rows, err := LOG_DB.WithContext(ctx).Model(&Log{}).Select("created_at", "model_name", "quota", "other").Where("channel_id IN ? AND type = ? AND created_at >= ? AND created_at < ?", ids, LogTypeConsume, start.Unix(), end.Unix()).Rows()
	if err != nil {
		return nil, errors.New("本站消费日志查询失败")
	}
	defer rows.Close()
	for rows.Next() {
		var log Log
		if err = LOG_DB.ScanRows(rows, &log); err != nil {
			return nil, errors.New("本站消费日志读取失败")
		}
		name := log.ModelName
		var meta struct {
			UpstreamModel string `json:"upstream_model_name"`
		}
		if common.UnmarshalJsonStr(log.Other, &meta) == nil && meta.UpstreamModel != "" {
			name = meta.UpstreamModel
		}
		date := time.Unix(log.CreatedAt, 0).UTC().Format("2006-01-02")
		key := date + "\x00" + name
		if totals[key] == nil {
			totals[key] = &OpenRouterSiteActivity{Date: date, Model: name}
		}
		totals[key].Quota += int64(log.Quota)
		totals[key].Requests++
	}
	if rows.Err() != nil {
		return nil, errors.New("本站消费日志读取失败")
	}
	result := make([]OpenRouterSiteActivity, 0, len(totals))
	for _, row := range totals {
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Date == result[j].Date {
			return result[i].Model < result[j].Model
		}
		return result[i].Date > result[j].Date
	})
	return result, nil
}
