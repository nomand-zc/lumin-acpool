package redis

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// FilterEvaluator 在客户端内存中评估 filtercond.Filter 条件。
// 由于 Redis 不支持 SQL 查询，需要先取回全量数据再在内存中过滤。
type FilterEvaluator struct {
	// fieldExtractor 用于从对象中提取指定字段的值。
	fieldExtractor func(obj any, field string) (any, bool)
}

// NewFilterEvaluator 创建一个新的过滤评估器。
// fieldExtractor 用于从具体对象中提取指定字段的值，返回 (value, exists)。
func NewFilterEvaluator(fieldExtractor func(obj any, field string) (any, bool)) *FilterEvaluator {
	return &FilterEvaluator{fieldExtractor: fieldExtractor}
}

// Match 判断 obj 是否满足 filter 条件。
// filter 为 nil 时返回 true（匹配全部）。
func (e *FilterEvaluator) Match(obj any, filter *filtercond.Filter) bool {
	if filter == nil {
		return true
	}
	result, err := e.evaluate(obj, filter)
	if err != nil {
		return false
	}
	return result
}

func (e *FilterEvaluator) evaluate(obj any, cond *filtercond.Filter) (bool, error) {
	if cond == nil {
		return true, nil
	}

	switch cond.Operator {
	case filtercond.OperatorAnd:
		return e.evalLogical(obj, cond, true)
	case filtercond.OperatorOr:
		return e.evalLogical(obj, cond, false)
	case filtercond.OperatorEqual:
		return e.evalComparison(obj, cond, func(cmp int) bool { return cmp == 0 })
	case filtercond.OperatorNotEqual:
		return e.evalComparison(obj, cond, func(cmp int) bool { return cmp != 0 })
	case filtercond.OperatorGreaterThan:
		return e.evalComparison(obj, cond, func(cmp int) bool { return cmp > 0 })
	case filtercond.OperatorGreaterThanOrEqual:
		return e.evalComparison(obj, cond, func(cmp int) bool { return cmp >= 0 })
	case filtercond.OperatorLessThan:
		return e.evalComparison(obj, cond, func(cmp int) bool { return cmp < 0 })
	case filtercond.OperatorLessThanOrEqual:
		return e.evalComparison(obj, cond, func(cmp int) bool { return cmp <= 0 })
	case filtercond.OperatorIn:
		return e.evalIn(obj, cond, false)
	case filtercond.OperatorNotIn:
		return e.evalIn(obj, cond, true)
	case filtercond.OperatorLike:
		return e.evalLike(obj, cond, false)
	case filtercond.OperatorNotLike:
		return e.evalLike(obj, cond, true)
	case filtercond.OperatorBetween:
		return e.evalBetween(obj, cond)
	default:
		return false, fmt.Errorf("unsupported operator: %s", cond.Operator)
	}
}

func (e *FilterEvaluator) evalLogical(obj any, cond *filtercond.Filter, isAnd bool) (bool, error) {
	children, ok := cond.Value.([]*filtercond.Filter)
	if !ok {
		return false, fmt.Errorf("logical operator requires []*filtercond.Filter value")
	}

	for _, child := range children {
		result, err := e.evaluate(obj, child)
		if err != nil {
			return false, err
		}
		if isAnd && !result {
			return false, nil
		}
		if !isAnd && result {
			return true, nil
		}
	}

	return isAnd, nil
}

func (e *FilterEvaluator) evalComparison(obj any, cond *filtercond.Filter, check func(int) bool) (bool, error) {
	fieldVal, exists := e.fieldExtractor(obj, cond.Field)
	if !exists {
		return false, nil
	}
	cmp := compareValues(fieldVal, cond.Value)
	return check(cmp), nil
}

func (e *FilterEvaluator) evalIn(obj any, cond *filtercond.Filter, negate bool) (bool, error) {
	fieldVal, exists := e.fieldExtractor(obj, cond.Field)
	if !exists {
		return negate, nil
	}

	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice {
		return false, fmt.Errorf("in operator requires a slice value")
	}

	for i := range value.Len() {
		if compareValues(fieldVal, value.Index(i).Interface()) == 0 {
			return !negate, nil
		}
	}
	return negate, nil
}

func (e *FilterEvaluator) evalLike(obj any, cond *filtercond.Filter, negate bool) (bool, error) {
	fieldVal, exists := e.fieldExtractor(obj, cond.Field)
	if !exists {
		return negate, nil
	}

	fieldStr := fmt.Sprintf("%v", fieldVal)
	pattern := fmt.Sprintf("%v", cond.Value)

	// 简单的 LIKE 实现：%xxx% → Contains, %xxx → HasSuffix, xxx% → HasPrefix
	matched := matchLikePattern(fieldStr, pattern)
	if negate {
		return !matched, nil
	}
	return matched, nil
}

func (e *FilterEvaluator) evalBetween(obj any, cond *filtercond.Filter) (bool, error) {
	fieldVal, exists := e.fieldExtractor(obj, cond.Field)
	if !exists {
		return false, nil
	}

	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() != 2 {
		return false, fmt.Errorf("between operator requires a slice with 2 elements")
	}

	low := value.Index(0).Interface()
	high := value.Index(1).Interface()

	return compareValues(fieldVal, low) >= 0 && compareValues(fieldVal, high) <= 0, nil
}

// compareValues 比较两个值，返回 -1/0/1。
// 支持 int、float64、string、time.Time 类型的比较。
func compareValues(a, b any) int {
	af := toFloat64(a)
	bf := toFloat64(b)
	if af != nil && bf != nil {
		if *af < *bf {
			return -1
		}
		if *af > *bf {
			return 1
		}
		return 0
	}

	// 字符串比较
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}

// toFloat64 尝试将值转为 float64。
func toFloat64(v any) *float64 {
	switch val := v.(type) {
	case int:
		f := float64(val)
		return &f
	case int32:
		f := float64(val)
		return &f
	case int64:
		f := float64(val)
		return &f
	case float32:
		f := float64(val)
		return &f
	case float64:
		return &val
	default:
		return nil
	}
}

// matchLikePattern 实现简单的 SQL LIKE 模式匹配。
func matchLikePattern(s, pattern string) bool {
	// 处理 %xxx%，%xxx，xxx% 三种模式。
	hasPrefix := strings.HasPrefix(pattern, "%")
	hasSuffix := strings.HasSuffix(pattern, "%")
	core := strings.Trim(pattern, "%")

	if hasPrefix && hasSuffix {
		return strings.Contains(s, core)
	}
	if hasPrefix {
		return strings.HasSuffix(s, core)
	}
	if hasSuffix {
		return strings.HasPrefix(s, core)
	}
	return s == pattern
}
