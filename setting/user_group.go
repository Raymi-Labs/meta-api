package setting

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// DefaultUserGroup is the group new users receive (users.group column
// default). It must stay registered.
const DefaultUserGroup = "default"

// maxUserGroupNameLength matches users.group varchar(64).
const maxUserGroupNameLength = 64

// userGroups is the registered list of user groups (user identity tiers such
// as default/vip). It is separate from channel groups (GroupRatio keys), which
// tokens select and channels belong to. Values are descriptions.
var userGroups = map[string]string{
	DefaultUserGroup: "默认用户分组",
}
var userGroupsMutex sync.RWMutex

func GetUserGroupsCopy() map[string]string {
	userGroupsMutex.RLock()
	defer userGroupsMutex.RUnlock()

	groups := make(map[string]string, len(userGroups))
	for name, desc := range userGroups {
		groups[name] = desc
	}
	return groups
}

func IsRegisteredUserGroup(name string) bool {
	userGroupsMutex.RLock()
	defer userGroupsMutex.RUnlock()

	_, ok := userGroups[name]
	return ok
}

func UserGroups2JSONString() string {
	userGroupsMutex.RLock()
	defer userGroupsMutex.RUnlock()

	data, err := common.Marshal(userGroups)
	if err != nil {
		common.SysLog("error marshalling user groups: " + err.Error())
		return "{}"
	}
	return string(data)
}

// ParseUserGroups validates the UserGroups option: a JSON object of
// name -> description that always contains the default group.
func ParseUserGroups(jsonStr string) (map[string]string, error) {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	var raw map[string]string
	if err := common.UnmarshalJsonStr(jsonStr, &raw); err != nil {
		return nil, fmt.Errorf("用户分组必须是 JSON 对象: %w", err)
	}
	if raw == nil {
		return nil, errors.New("用户分组必须是 JSON 对象")
	}
	for name := range raw {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("用户分组名称不能为空")
		}
		if strings.TrimSpace(name) != name {
			return nil, fmt.Errorf("用户分组「%s」的名称首尾不能有空格", name)
		}
		if utf8.RuneCountInString(name) > maxUserGroupNameLength {
			return nil, fmt.Errorf("用户分组「%s」的名称过长", name)
		}
	}
	if _, ok := raw[DefaultUserGroup]; !ok {
		return nil, errors.New("default 是新用户的默认分组，不能删除")
	}
	return raw, nil
}

func UpdateUserGroupsByJSONString(jsonStr string) error {
	parsed, err := ParseUserGroups(jsonStr)
	if err != nil {
		return err
	}
	userGroupsMutex.Lock()
	defer userGroupsMutex.Unlock()
	userGroups = parsed
	return nil
}
