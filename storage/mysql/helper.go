package mysql

import (
	"encoding/json"
	"strings"
)

// MarshalJSON 将任意值序列化为 JSON 字符串，nil 返回 nil。
// 注意：返回 *string 而非 []byte，因为 database/sql 驱动会将 []byte 存为 blob 类型，
// 导致 JSON 函数无法正确解析（SQLite 中会报 malformed JSON 错误）。
func MarshalJSON(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := string(data)
	return &s, nil
}

// IsDuplicateEntry 判断是否为 MySQL 主键冲突错误。
func IsDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "1062")
}
