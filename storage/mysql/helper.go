package mysql

import (
	"encoding/json"
	"strings"
)

// MarshalJSON 将任意值序列化为 JSON，nil 返回 nil。
func MarshalJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// IsDuplicateEntry 判断是否为 MySQL 主键冲突错误。
func IsDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "1062")
}
