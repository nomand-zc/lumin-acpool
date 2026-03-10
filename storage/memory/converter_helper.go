package memory

import (
	"fmt"
	"reflect"
	"time"

	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// ============================================================
// Common type checking and comparison functions
// ============================================================

// valueType returns the type identifier of a value.
func valueType(value any) string {
	switch reflect.ValueOf(value).Kind() {
	case reflect.String:
		return valueTypeString
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return valueTypeNumber
	case reflect.Bool:
		return valueTypeBool
	default:
		if _, ok := value.(time.Time); ok {
			return valueTypeTime
		}
	}
	return ""
}

// compareString compares two string values.
func compareString(docValue any, condValue any, operator string) bool {
	docStr, ok1 := docValue.(string)
	condStr, ok2 := condValue.(string)
	if !ok1 || !ok2 {
		return false
	}

	switch operator {
	case filtercond.OperatorEqual:
		return docStr == condStr
	case filtercond.OperatorNotEqual:
		return docStr != condStr
	case filtercond.OperatorGreaterThan:
		return docStr > condStr
	case filtercond.OperatorGreaterThanOrEqual:
		return docStr >= condStr
	case filtercond.OperatorLessThan:
		return docStr < condStr
	case filtercond.OperatorLessThanOrEqual:
		return docStr <= condStr
	}
	return false
}

// compareBool compares two boolean values.
func compareBool(docValue any, condValue any, operator string) bool {
	docBool, ok1 := docValue.(bool)
	condBool, ok2 := condValue.(bool)
	if !ok1 || !ok2 {
		return false
	}

	switch operator {
	case filtercond.OperatorEqual:
		return docBool == condBool
	case filtercond.OperatorNotEqual:
		return docBool != condBool
	}
	return false
}

// compareTime compares two time values.
func compareTime(docValue any, condValue any, operator string) bool {
	docTime, ok1 := toTime(docValue)
	condTime, ok2 := toTime(condValue)
	if !ok1 || !ok2 {
		return false
	}

	switch operator {
	case filtercond.OperatorEqual:
		return docTime.Equal(condTime)
	case filtercond.OperatorNotEqual:
		return !docTime.Equal(condTime)
	case filtercond.OperatorGreaterThan:
		return docTime.After(condTime)
	case filtercond.OperatorGreaterThanOrEqual:
		return docTime.After(condTime) || docTime.Equal(condTime)
	case filtercond.OperatorLessThan:
		return docTime.Before(condTime)
	case filtercond.OperatorLessThanOrEqual:
		return docTime.Before(condTime) || docTime.Equal(condTime)
	}
	return false
}

// compareNumber compares two numeric values.
func compareNumber(docValue any, condValue any, operator string) bool {
	docNum, ok1 := toFloat64(docValue)
	condNum, ok2 := toFloat64(condValue)
	if !ok1 || !ok2 {
		return false
	}

	switch operator {
	case filtercond.OperatorEqual:
		return docNum == condNum
	case filtercond.OperatorNotEqual:
		return docNum != condNum
	case filtercond.OperatorGreaterThan:
		return docNum > condNum
	case filtercond.OperatorGreaterThanOrEqual:
		return docNum >= condNum
	case filtercond.OperatorLessThan:
		return docNum < condNum
	case filtercond.OperatorLessThanOrEqual:
		return docNum <= condNum
	}
	return false
}

// ============================================================
// Type conversion helper functions
// ============================================================

// toFloat64 attempts to convert any to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

// toTime attempts to convert any to time.Time.
func toTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, true
		}
		return *t, true
	default:
		return time.Time{}, false
	}
}

// toStringSlice attempts to convert any to []string.
func toStringSlice(v any) ([]string, error) {
	switch s := v.(type) {
	case []string:
		return s, nil
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			result = append(result, str)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected []string or []any, got %T", v)
	}
}
