package mysql

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// MySQL 比较运算符映射。
var comparisonOperators = map[string]string{
	filtercond.OperatorEqual:              "=",
	filtercond.OperatorNotEqual:           "!=",
	filtercond.OperatorGreaterThan:        ">",
	filtercond.OperatorGreaterThanOrEqual: ">=",
	filtercond.OperatorLessThan:           "<",
	filtercond.OperatorLessThanOrEqual:    "<=",
}

// MySQL JSON操作符映射。
var jsonOperators = map[string]string{
	filtercond.OperatorJSONContains:    "JSON_CONTAINS",
	filtercond.OperatorJSONNotContains: "NOT JSON_CONTAINS",
}

// CondConvertResult 是条件转换的结果，包含 SQL 条件片段和对应的参数。
type CondConvertResult struct {
	Cond string
	Args []any
}

// MysqlConverter 将 filtercond.Filter 转换为 MySQL WHERE 子句。
type MysqlConverter struct {
	// fieldMapping 字段名映射，将 storage.Fields 中的逻辑字段名映射到数据库列名。
	fieldMapping map[string]string
	// jsonFields 标记哪些字段是JSON类型，需要特殊处理
	jsonFields map[string]bool
}

// Compile-time interface compliance check.
var _ filtercond.Converter[*CondConvertResult] = (*MysqlConverter)(nil)

// NewConditionConverter 创建一个新的 MySQL 条件转换器。
// fieldMapping 用于将逻辑字段名映射到实际数据库列名，nil 表示直接使用字段名。
// jsonFields 用于标记JSON类型的字段，nil 表示没有JSON字段。
func NewConditionConverter(fieldMapping map[string]string, jsonFields map[string]bool) *MysqlConverter {
	if fieldMapping == nil {
		fieldMapping = make(map[string]string)
	}
	if jsonFields == nil {
		jsonFields = make(map[string]bool)
	}
	return &MysqlConverter{
		fieldMapping: fieldMapping,
		jsonFields:   jsonFields,
	}
}

// Convert 将 filtercond.Filter 转换为 MySQL WHERE 子句。
// 当 filter 为 nil 时，返回空条件（匹配所有记录）。
func (c *MysqlConverter) Convert(filter *filtercond.Filter) (*CondConvertResult, error) {
	if filter == nil {
		return &CondConvertResult{Cond: "1=1"}, nil
	}
	return c.convertCondition(filter)
}

// convertFieldName 将逻辑字段名转换为实际数据库列名。
func (c *MysqlConverter) convertFieldName(field string) string {
	if mapped, ok := c.fieldMapping[field]; ok {
		return mapped
	}
	return field
}

// isJSONField 检查字段是否为JSON类型。
func (c *MysqlConverter) isJSONField(field string) bool {
	return c.jsonFields[field]
}

func (c *MysqlConverter) convertCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
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

// buildLogicalCondition 构建逻辑条件（AND/OR）。
func (c *MysqlConverter) buildLogicalCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
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

// buildComparisonCondition 构建比较条件（=, !=, >, >=, <, <=）。
func (c *MysqlConverter) buildComparisonCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	operator, ok := comparisonOperators[cond.Operator]
	if !ok {
		return nil, fmt.Errorf("unsupported comparison operator: %s", cond.Operator)
	}

	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}

	fieldName := c.convertFieldName(cond.Field)
	return &CondConvertResult{
		Cond: fmt.Sprintf("`%s` %s ?", fieldName, operator),
		Args: []any{cond.Value},
	}, nil
}

// buildInCondition 构建IN/NOT IN条件。
func (c *MysqlConverter) buildInCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
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
	for i := 0; i < itemNum; i++ {
		args = append(args, value.Index(i).Interface())
		placeholders = append(placeholders, "?")
	}

	operator := strings.ToUpper(cond.Operator)
	return &CondConvertResult{
		Cond: fmt.Sprintf("`%s` %s (%s)", fieldName, operator, strings.Join(placeholders, ", ")),
		Args: args,
	}, nil
}

// buildLikeCondition 构建LIKE/NOT LIKE条件。
func (c *MysqlConverter) buildLikeCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}
	if cond.Value == nil || reflect.TypeOf(cond.Value).Kind() != reflect.String {
		return nil, fmt.Errorf("like operator value must be a string: %v", cond.Value)
	}

	fieldName := c.convertFieldName(cond.Field)
	operator := strings.ToUpper(cond.Operator)
	return &CondConvertResult{
		Cond: fmt.Sprintf("`%s` %s ?", fieldName, operator),
		Args: []any{cond.Value},
	}, nil
}

// buildBetweenCondition 构建BETWEEN条件。
func (c *MysqlConverter) buildBetweenCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}
	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() != 2 {
		return nil, fmt.Errorf("between operator value must be a slice with two elements: %v", cond.Value)
	}

	fieldName := c.convertFieldName(cond.Field)
	return &CondConvertResult{
		Cond: fmt.Sprintf("`%s` BETWEEN ? AND ?", fieldName),
		Args: []any{value.Index(0).Interface(), value.Index(1).Interface()},
	}, nil
}

// buildJSONCondition 构建JSON条件（JSON_CONTAINS/NOT JSON_CONTAINS）。
func (c *MysqlConverter) buildJSONCondition(cond *filtercond.Filter) (*CondConvertResult, error) {
	operator, ok := jsonOperators[cond.Operator]
	if !ok {
		return nil, fmt.Errorf("unsupported JSON operator: %s", cond.Operator)
	}

	if cond.Field == "" {
		return nil, fmt.Errorf("field is empty")
	}

	fieldName := c.convertFieldName(cond.Field)

	// 对于JSON数组查询，我们需要将值转换为JSON格式
	jsonValue, err := c.convertToJSONValue(cond.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to convert value to JSON: %w", err)
	}

	return &CondConvertResult{
		Cond: fmt.Sprintf("%s(`%s`, ?)", operator, fieldName),
		Args: []any{jsonValue},
	}, nil
}

// convertToJSONValue 将单个值转换为JSON格式的字符串。
func (c *MysqlConverter) convertToJSONValue(value any) (string, error) {
	if value == nil {
		return "null", nil
	}

	switch v := value.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, v), nil
	case int, int32, int64, float32, float64, bool:
		return fmt.Sprintf("%v", v), nil
	default:
		return "", fmt.Errorf("unsupported value type for JSON conversion: %T", value)
	}
}
