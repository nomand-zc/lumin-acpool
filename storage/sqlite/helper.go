package sqlite

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

// IsDuplicateEntry 判断是否为 SQLite 主键/唯一键冲突错误。
func IsDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "UNIQUE constraint failed") ||
		strings.Contains(errMsg, "constraint failed")
}
