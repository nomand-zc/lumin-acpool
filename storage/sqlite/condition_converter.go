package sqlite

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// SQLite 比较运算符映射。
var comparisonOperators = map[string]string{
	filtercond.OperatorEqual:              "=",
	filtercond.OperatorNotEqual:           "!=",
	filtercond.OperatorGreaterThan:        ">",
	filtercond.OperatorGreaterThanOrEqual: ">=",
	filtercond.OperatorLessThan:           "<",
	filtercond.OperatorLessThanOrEqual:    "<=",
}

// CondConvertResult 是条件转换的结果，包含 SQL 条件片段和对应的参数。
type CondConvertResult struct {
	Cond string
	Args []any
}

// SqliteConverter 将 filtercond.Filter 转换为 SQLite WHERE 子句。
type SqliteConverter struct {
	// fieldMapping 字段名映射，将 storage.Fields 中的逻辑字段名映射到数据库列名。
	fieldMapping map[string]string
	// jsonFields 标记哪些字段是JSON类型（SQLite中存储为TEXT），需要特殊处理。
	jsonFields map[string]bool
}

// Compile-time interface compliance check.
var _ filtercond.Converter[*CondConvertResult] = (*SqliteConverter)(nil)

// NewConditionConverter 创建一个新的 SQLite 条件转换器。
// fieldMapping 用于将逻辑字段名映射到实际数据库列名，nil 表示直接使用字段名。
// jsonFields 用于标记JSON类型的字段（SQLite中为TEXT类型），nil 表示没有JSON字段。
func NewConditionConverter(fieldMapping map[string]string, jsonFields map[string]bool) *SqliteConverter {
	if fieldMapping == nil {
		fieldMapping = make(map[string]string)
	}
	if jsonFields == nil {
		jsonFields = make(map[string]bool)
	}
	return &SqliteConverter{
		fieldMapping: fieldMapping,
		jsonFields:   jsonFields,
	}
}

// Convert 将 filtercond.Filter 转换为 SQLite WHERE 子句。
// 当 filter 为 nil 时，返回空条件（匹配所有记录）。
func (c *SqliteConverter) Convert(filter *filtercond.Filter) (*CondConvertResult, error) {
	if filter == nil {
		return &CondConvertResult{Cond: "1=1"}, nil
	}
	return c.convertCondition(filter)
}

// convertFieldName 将逻辑字段名转换为实际数据库列名。
func (c *SqliteConverter) convertFieldName(field string) string {
	if mapped, ok := c.fieldMapping[field]; ok {
		return mapped
	}
	return field
}

// isJSONField 检查字段是否为JSON类型。
func (c *SqliteConverter) isJSONField(field string) bool {
	return c.jsonFields[field]
}

func (c *SqliteConverter) convertCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	if cond == nil {
		return nil, fmt.Errorf("nil condition")
	}

	switch cond.Operator {
	case filtercond.OperatorAnd, filtercond.OperatorOr:
		return c.buildLogicalCondition(cond)
	case filtercond.OperatorEqual, filtercond.OperatorNotEqual,
		filtercond.OperatorGreaterThan, filtercond.OperatorGreaterThanOrEqual,
		filtercond.OperatorLessThan, filtercond.OperatorLessThanOrEqual:
		return c.buildComparisonCondition(cond)
	case filtercond.OperatorIn, filtercond.OperatorNotIn:
		return c.buildInCondition(cond)
	case filtercond.OperatorLike, filtercond.OperatorNotLike:
		return c.buildLikeCondition(cond)
	case filtercond.OperatorBetween:
		return c.buildBetweenCondition(cond)
	case filtercond.OperatorJSONContains, filtercond.OperatorJSONNotContains:
		return c.buildJSONCondition(cond)
	default:
		return nil, fmt.Errorf("unsupported operation: %s", cond.Operator)
	}
}

func (c *SqliteConverter) buildLogicalCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	conds, ok := cond.Value.([]*filtercond.Filter)
	if !ok {
		return nil, fmt.Errorf("invalid logical condition: value must be of type []*filtercond.Filter: %v", cond.Value)
	}

	var result *CondConvertResult
	for _, child := range conds {
		childResult, err := c.convertCondition(child)
		if err != nil {
			return nil, err
		}
		if childResult == nil || childResult.Cond == "" {
			continue
		}
		if result == nil {
			result = childResult
			continue
		}
		result.Cond = fmt.Sprintf("(%s) %s (%s)", result.Cond, strings.ToUpper(cond.Operator), childResult.Cond)
		result.Args = append(result.Args, childResult.Args...)
	}

	if result == nil {
		return nil, fmt.Errorf("empty logical condition")
	}

	return result, nil
}

func (c *SqliteConverter) buildComparisonCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	operator, ok := comparisonOperators[cond.Operator]
	if !ok {
		return nil, fmt.Errorf("unsupported comparison operator: %s", cond.Operator)
	}

	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}

	fieldName := c.convertFieldName(cond.Field)
	return &CondConvertResult{
		Cond: fmt.Sprintf(`"%s" %s ?`, fieldName, operator),
		Args: []any{cond.Value},
	}, nil
}

func (c *SqliteConverter) buildInCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}

	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() <= 0 {
		return nil, fmt.Errorf("in operator value must be a slice with at least one value: %v", cond.Value)
	}

	fieldName := c.convertFieldName(cond.Field)
	itemNum := value.Len()
	args := make([]any, 0, itemNum)
	placeholders := make([]string, 0, itemNum)
	for i := range itemNum {
		args = append(args, value.Index(i).Interface())
		placeholders = append(placeholders, "?")
	}

	operator := strings.ToUpper(cond.Operator)
	return &CondConvertResult{
		Cond: fmt.Sprintf(`"%s" %s (%s)`, fieldName, operator, strings.Join(placeholders, ", ")),
		Args: args,
	}, nil
}

func (c *SqliteConverter) buildLikeCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}
	if cond.Value == nil || reflect.TypeOf(cond.Value).Kind() != reflect.String {
		return nil, fmt.Errorf("like operator value must be a string: %v", cond.Value)
	}

	fieldName := c.convertFieldName(cond.Field)
	operator := strings.ToUpper(cond.Operator)
	return &CondConvertResult{
		Cond: fmt.Sprintf(`"%s" %s ?`, fieldName, operator),
		Args: []any{cond.Value},
	}, nil
}

func (c *SqliteConverter) buildBetweenCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}
	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() != 2 {
		return nil, fmt.Errorf("between operator value must be a slice with two elements: %v", cond.Value)
	}

	fieldName := c.convertFieldName(cond.Field)
	return &CondConvertResult{
		Cond: fmt.Sprintf(`"%s" BETWEEN ? AND ?`, fieldName),
		Args: []any{value.Index(0).Interface(), value.Index(1).Interface()},
	}, nil
}

// buildJSONCondition 构建JSON条件。
// SQLite 中 JSON 数据存储为 TEXT 类型，使用 json_each() 函数 + EXISTS 子查询来实现类似 MySQL JSON_CONTAINS 的功能。
// 生成的 SQL 形如: EXISTS(SELECT 1 FROM json_each("column") WHERE json_each.value = ?)
func (c *SqliteConverter) buildJSONCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}

	fieldName := c.convertFieldName(cond.Field)

	// SQLite 使用 json_each() 表值函数来遍历 JSON 数组，配合 EXISTS 子查询实现包含判断。
	// 例如: EXISTS(SELECT 1 FROM json_each("supported_models") WHERE json_each.value = ?)
	notPrefix := ""
	if cond.Operator == filtercond.OperatorJSONNotContains {
		notPrefix = "NOT "
	}

	return &CondConvertResult{
		Cond: fmt.Sprintf(`%sEXISTS(SELECT 1 FROM json_each("%s") WHERE json_each.value = ?)`, notPrefix, fieldName),
		Args: []any{cond.Value},
	}, nil
}
