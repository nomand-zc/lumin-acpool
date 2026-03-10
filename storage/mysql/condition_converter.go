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

// CondConvertResult 是条件转换的结果，包含 SQL 条件片段和对应的参数。
type CondConvertResult struct {
	Cond string
	Args []any
}

// MysqlConverter 将 filtercond.Filter 转换为 MySQL WHERE 子句。
type MysqlConverter struct {
	// fieldMapping 字段名映射，将 storage.Fields 中的逻辑字段名映射到数据库列名。
	fieldMapping map[string]string
}

// Compile-time interface compliance check.
var _ filtercond.Converter[*CondConvertResult] = (*MysqlConverter)(nil)

// NewConditionConverter 创建一个新的 MySQL 条件转换器。
// fieldMapping 用于将逻辑字段名映射到实际数据库列名，nil 表示直接使用字段名。
func NewConditionConverter(fieldMapping map[string]string) *MysqlConverter {
	if fieldMapping == nil {
		fieldMapping = make(map[string]string)
	}
	return &MysqlConverter{
		fieldMapping: fieldMapping,
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
	default:
		return nil, fmt.Errorf("unsupported operation: %s", cond.Operator)
	}
}

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
	for i := range itemNum {
		args = append(args, value.Index(i).Interface())
		placeholders = append(placeholders, "?")
	}

	operator := strings.ToUpper(cond.Operator)
	return &CondConvertResult{
		Cond: fmt.Sprintf("`%s` %s (%s)", fieldName, operator, strings.Join(placeholders, ", ")),
		Args: args,
	}, nil
}

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
