package memory

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	"github.com/nomand-zc/lumin-acpool/storage/memory/internal/utils"
)

// ProviderFilterFunc 是内存中 ProviderInfo 的过滤函数类型。
type ProviderFilterFunc = func(*account.ProviderInfo) bool

// ProviderConverter 将 Filter 条件转换为内存中 ProviderInfo 的过滤函数。
type ProviderConverter struct{}

// Convert 将 Filter 条件转换为内存中 ProviderInfo 的过滤函数。
func (c *ProviderConverter) Convert(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	if cond == nil {
		return func(*account.ProviderInfo) bool { return true }, nil
	}
	return c.convertCondition(cond)
}

func (c *ProviderConverter) convertCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
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
	case filtercond.OperatorBetween:
		return c.buildBetweenCondition(cond)
	case filtercond.OperatorLike, filtercond.OperatorNotLike:
		return c.buildLikeCondition(cond)
	default:
		return nil, fmt.Errorf("unsupported operator: %s", cond.Operator)
	}
}

func (c *ProviderConverter) buildLogicalCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	subs, ok := cond.Value.([]*filtercond.Filter)
	if !ok {
		return nil, fmt.Errorf("logical operator value must be []*Filter: %v", cond.Value)
	}

	var predicates []ProviderFilterFunc
	for _, sub := range subs {
		p, err := c.convertCondition(sub)
		if err != nil {
			return nil, err
		}
		if p != nil {
			predicates = append(predicates, p)
		}
	}

	if len(predicates) == 0 {
		return nil, fmt.Errorf("no valid sub-conditions in logical condition")
	}

	return func(p *account.ProviderInfo) bool {
		isAnd := cond.Operator == filtercond.OperatorAnd
		for _, pred := range predicates {
			result := pred(p)
			if !isAnd && result {
				return true
			}
			if isAnd && !result {
				return false
			}
		}
		return isAnd
	}, nil
}

func (c *ProviderConverter) buildComparisonCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	// supported_model 字段使用特殊的数组包含逻辑
	if cond.Field == storage.ProviderFieldSupportedModel {
		return c.buildModelFilter(cond)
	}

	extractor, err := providerFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	return func(p *account.ProviderInfo) bool {
		docValue := extractor(p)
		switch utils.ValueType(cond.Value) {
		case utils.ValueTypeString:
			return utils.CompareString(docValue, cond.Value, cond.Operator)
		case utils.ValueTypeNumber:
			return utils.CompareNumber(docValue, cond.Value, cond.Operator)
		case utils.ValueTypeTime:
			return utils.CompareTime(docValue, cond.Value, cond.Operator)
		case utils.ValueTypeBool:
			return utils.CompareBool(docValue, cond.Value, cond.Operator)
		}
		return false
	}, nil
}

func (c *ProviderConverter) buildInCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	if cond.Field == storage.ProviderFieldSupportedModel {
		return c.buildModelFilter(cond)
	}

	extractor, err := providerFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	s := reflect.ValueOf(cond.Value)
	if s.Kind() != reflect.Slice || s.Len() <= 0 {
		return nil, fmt.Errorf("in operator value must be a slice with at least one value: %v", cond.Value)
	}

	itemNum := s.Len()
	return func(p *account.ProviderInfo) bool {
		docValue := extractor(p)
		var found bool
		for i := 0; i < itemNum; i++ {
			if reflect.DeepEqual(docValue, s.Index(i).Interface()) {
				found = true
				break
			}
		}
		if cond.Operator == filtercond.OperatorIn {
			return found
		}
		return !found
	}, nil
}

func (c *ProviderConverter) buildBetweenCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() != 2 {
		return nil, fmt.Errorf("between operator value must be a slice with two elements: %v", cond.Value)
	}

	var condFuncs []ProviderFilterFunc
	for i := 0; i < 2; i++ {
		op := filtercond.OperatorGreaterThanOrEqual
		if i == 1 {
			op = filtercond.OperatorLessThanOrEqual
		}
		fn, err := c.buildComparisonCondition(&filtercond.Filter{
			Field:    cond.Field,
			Operator: op,
			Value:    value.Index(i).Interface(),
		})
		if err != nil {
			return nil, err
		}
		if fn != nil {
			condFuncs = append(condFuncs, fn)
		}
	}

	return func(p *account.ProviderInfo) bool {
		for _, fn := range condFuncs {
			if !fn(p) {
				return false
			}
		}
		return true
	}, nil
}

func (c *ProviderConverter) buildLikeCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	extractor, err := providerFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	pattern, ok := cond.Value.(string)
	if !ok {
		return nil, fmt.Errorf("like operator requires a string pattern")
	}

	return func(p *account.ProviderInfo) bool {
		docValue := extractor(p)
		docStr, ok := docValue.(string)
		if !ok {
			return false
		}
		matched := strings.Contains(docStr, pattern)
		if cond.Operator == filtercond.OperatorLike {
			return matched
		}
		return !matched
	}, nil
}

// buildModelFilter 处理 supported_model 字段的特殊逻辑。
func (c *ProviderConverter) buildModelFilter(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	switch cond.Operator {
	case filtercond.OperatorEqual:
		model, ok := cond.Value.(string)
		if !ok {
			return nil, fmt.Errorf("memory: supported_model eq value must be string")
		}
		return func(p *account.ProviderInfo) bool {
			return p.SupportsModel(model)
		}, nil
	case filtercond.OperatorIn:
		models, err := utils.ToStringSlice(cond.Value)
		if err != nil {
			return nil, fmt.Errorf("memory: supported_model in value must be string slice: %w", err)
		}
		return func(p *account.ProviderInfo) bool {
			for _, m := range models {
				if p.SupportsModel(m) {
					return true
				}
			}
			return false
		}, nil
	default:
		return nil, fmt.Errorf("memory: unsupported operator %q for supported_model field", cond.Operator)
	}
}

// providerFieldExtractor 根据字段名返回 ProviderInfo 字段值提取函数。
func providerFieldExtractor(field string) (func(*account.ProviderInfo) any, error) {
	switch field {
	case storage.ProviderFieldType:
		return func(p *account.ProviderInfo) any { return p.ProviderType }, nil
	case storage.ProviderFieldName:
		return func(p *account.ProviderInfo) any { return p.ProviderName }, nil
	case storage.ProviderFieldStatus:
		return func(p *account.ProviderInfo) any { return int(p.Status) }, nil
	case storage.ProviderFieldPriority:
		return func(p *account.ProviderInfo) any { return p.Priority }, nil
	case storage.ProviderFieldWeight:
		return func(p *account.ProviderInfo) any { return p.Weight }, nil
	case storage.ProviderFieldAccountCount:
		return func(p *account.ProviderInfo) any { return p.AccountCount }, nil
	case storage.ProviderFieldAvailableAccountCount:
		return func(p *account.ProviderInfo) any { return p.AvailableAccountCount }, nil
	case storage.ProviderFieldCreatedAt:
		return func(p *account.ProviderInfo) any { return p.CreatedAt }, nil
	case storage.ProviderFieldUpdatedAt:
		return func(p *account.ProviderInfo) any { return p.UpdatedAt }, nil
	default:
		return nil, fmt.Errorf("memory: unsupported provider field: %s", field)
	}
}
