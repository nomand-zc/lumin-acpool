package redis

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// MarshalJSON 将任意值序列化为 JSON 字符串，nil 返回空字符串。
func MarshalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatTime 将时间格式化为字符串，适用于 Redis 存储。
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

// FormatTimePtr 将时间指针格式化为字符串。
func FormatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return FormatTime(*t)
}

// ParseTime 从字符串解析时间。
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %s", s)
}

// ParseTimePtr 从字符串解析为时间指针，空字符串返回 nil。
func ParseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := ParseTime(s)
	if err != nil {
		return nil
	}
	return &t
}

// ParseInt 从字符串解析整数，默认返回 0。
func ParseInt(s string) int {
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

// ParseInt64 从字符串解析 int64，默认返回 0。
func ParseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// ParseFloat64 从字符串解析 float64，默认返回 0。
func ParseFloat64(s string) float64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// IsNotFound 判断 go-redis 返回的错误是否为 key 不存在。
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "redis: nil"
}
