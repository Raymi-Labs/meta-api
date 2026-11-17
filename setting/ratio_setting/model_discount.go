package ratio_setting

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var noDiscountModels = struct {
	sync.RWMutex
	names map[string]bool
}{names: make(map[string]bool)}

// ParseNoDiscountModels accepts exact public billing IDs, never wildcard rules.
func ParseNoDiscountModels(raw string) ([]string, error) {
	var names []string
	if err := common.UnmarshalJsonStr(raw, &names); err != nil {
		return nil, fmt.Errorf("不参与折扣的模型必须是模型 ID 数组: %w", err)
	}
	if names == nil {
		return nil, fmt.Errorf("模型列表不能为空值，请使用 [] 清空")
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "*?\r\n\t") {
			return nil, fmt.Errorf("无效的精确模型 ID: %q", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("模型 ID 重复: %s", name)
		}
		seen[name] = true
	}
	return names, nil
}

func UpdateNoDiscountModels(raw string) error {
	names, err := ParseNoDiscountModels(raw)
	if err != nil {
		return err
	}
	next := make(map[string]bool, len(names))
	for _, name := range names {
		next[name] = true
	}
	noDiscountModels.Lock()
	noDiscountModels.names = next
	noDiscountModels.Unlock()
	return nil
}

func IsNoDiscountModel(name string) bool {
	noDiscountModels.RLock()
	defer noDiscountModels.RUnlock()
	return noDiscountModels.names[name]
}

func NoDiscountModelsJSON() string {
	noDiscountModels.RLock()
	names := make([]string, 0, len(noDiscountModels.names))
	for name := range noDiscountModels.names {
		names = append(names, name)
	}
	noDiscountModels.RUnlock()
	sort.Strings(names)
	data, _ := common.Marshal(names)
	return string(data)
}
