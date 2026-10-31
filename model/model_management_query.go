package model

import (
	"context"
	"errors"
	"sort"
	"strconv"
)

type ModelManagementPage struct {
	Items        []*Model    `json:"items"`
	Total        int         `json:"total"`
	VendorCounts map[int]int `json:"vendor_counts"`
	Vendors      []*Vendor   `json:"vendors"`
}

// Resolve the small inventory before filtering and pagination. Full model metadata
// (FAQ, source snapshots, descriptions) is loaded only for the selected page.
func GetModelManagementPage(ctx context.Context, keyword, vendor string, offset, limit int) (ModelManagementPage, error) {
	result := ModelManagementPage{Items: []*Model{}, Vendors: []*Vendor{}, VendorCounts: map[int]int{}}
	db := DB.WithContext(ctx)
	var inventory []Model
	if err := db.Select("id", "model_name", "vendor_id").Order("id DESC").Find(&inventory).Error; err != nil {
		return result, err
	}
	var vendors []*Vendor
	if err := db.Order("id").Find(&vendors).Error; err != nil {
		return result, err
	}
	vendorMap := make(map[int]*Vendor, len(vendors))
	for _, v := range vendors {
		vendorMap[v.Id] = v
	}
	displayIDs := make(map[int]int, len(inventory))
	for _, row := range inventory {
		id := modelDisplayVendorID(row.ModelName, row.VendorID, vendorMap)
		if id == 0 && getDefaultVendorName(row.ModelName) != "" {
			return result, errors.New("获取模型供应商失败")
		}
		displayIDs[row.Id] = id
		result.VendorCounts[id]++
	}
	for _, v := range vendorMap {
		result.Vendors = append(result.Vendors, v)
	}
	sort.Slice(result.Vendors, func(i, j int) bool { return result.Vendors[i].Id < result.Vendors[j].Id })
	// Keep database LIKE/collation semantics for user-entered search strings.
	keywordIDs := map[int]bool{}
	if keyword != "" {
		var ids []int
		like := "%" + keyword + "%"
		if err := db.Model(&Model{}).Where("model_name LIKE ? OR display_name LIKE ? OR description LIKE ? OR tags LIKE ?", like, like, like, like).Pluck("id", &ids).Error; err != nil {
			return result, err
		}
		for _, id := range ids {
			keywordIDs[id] = true
		}
	}
	vendorIDs := map[int]bool{}
	if vendor != "" {
		if id, err := strconv.Atoi(vendor); err == nil {
			vendorIDs[id] = true
		} else {
			var ids []int
			if err := db.Model(&Vendor{}).Where("name LIKE ?", "%"+vendor+"%").Pluck("id", &ids).Error; err != nil {
				return result, err
			}
			for _, id := range ids {
				vendorIDs[id] = true
			}
		}
	}
	if offset < 0 {
		offset = 0
	}
	pageIDs := []int{}
	for _, row := range inventory {
		if keyword != "" && !keywordIDs[row.Id] {
			continue
		}
		if vendor != "" && !vendorIDs[displayIDs[row.Id]] {
			continue
		}
		if result.Total >= offset && len(pageIDs) < limit {
			pageIDs = append(pageIDs, row.Id)
		}
		result.Total++
	}
	if len(pageIDs) > 0 {
		if err := db.Where("id IN ?", pageIDs).Order("id DESC").Find(&result.Items).Error; err != nil {
			return result, err
		}
		for _, row := range result.Items {
			row.DisplayVendorID = displayIDs[row.Id]
		}
	}
	return result, nil
}
