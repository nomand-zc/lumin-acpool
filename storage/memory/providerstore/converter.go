package providerstore

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	"github.com/nomand-zc/lumin-acpool/storage/memory/internal/utils"
)

// FilterFunc is the in-memory filter function type for ProviderInfo.
type FilterFunc = func(*account.ProviderInfo) bool

// Converter converts Filter conditions to in-memory filter functions for ProviderInfo.
type Converter struct{}

// Convert converts a Filter condition to an in-memory filter function for ProviderInfo.
func (c *Converter) Convert(cond *filtercond.Filter) (FilterFunc, error) {
	if cond == nil {
		return func(*account.ProviderInfo) bool { return true }, nil
	}
	return c.convertCondition(cond)
}

// convertCondition is the unified dispatch entry point that routes to the corresponding builder method based on operator type.
func (c *Converter) convertCondition(cond *filtercond.Filter) (FilterFunc, error) {
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

// buildLogicalCondition builds AND/OR logical conditions.
func (c *Converter) buildLogicalCondition(cond *filtercond.Filter) (FilterFunc, error) {
	subs, ok := cond.Value.([]*filtercond.Filter)
	if !ok {
		return nil, fmt.Errorf("logical operator value must be []*Filter: %v", cond.Value)
	}

	var predicates []FilterFunc
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
				return true // OR short-circuit
			}
			if isAnd && !result {
				return false // AND short-circuit
			}
		}
		return isAnd
	}, nil
}

// buildComparisonCondition builds comparison conditions (eq, ne, gt, gte, lt, lte).
// Uses special logic for the supported_model field.
func (c *Converter) buildComparisonCondition(cond *filtercond.Filter) (FilterFunc, error) {
	// supported_model field uses special array containment logic
	if cond.Field == storage.ProviderFieldSupportedModel {
		return c.buildModelFilter(cond)
	}

	extractor, err := fieldExtractor(cond.Field)
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

// buildInCondition builds IN/NOT IN conditions.
func (c *Converter) buildInCondition(cond *filtercond.Filter) (FilterFunc, error) {
	// supported_model field uses special array containment logic
	if cond.Field == storage.ProviderFieldSupportedModel {
		return c.buildModelFilter(cond)
	}

	extractor, err := fieldExtractor(cond.Field)
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

// buildBetweenCondition builds BETWEEN conditions.
func (c *Converter) buildBetweenCondition(cond *filtercond.Filter) (FilterFunc, error) {
	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() != 2 {
		return nil, fmt.Errorf("between operator value must be a slice with two elements: %v", cond.Value)
	}

	var condFuncs []FilterFunc
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

// buildLikeCondition builds LIKE/NOT LIKE conditions.
func (c *Converter) buildLikeCondition(cond *filtercond.Filter) (FilterFunc, error) {
	extractor, err := fieldExtractor(cond.Field)
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

// buildModelFilter handles special logic for the supported_model field.
// For eq operation: checks if SupportedModels contains the specified model.
// For in operation: checks if SupportedModels contains any model from the list.
func (c *Converter) buildModelFilter(cond *filtercond.Filter) (FilterFunc, error) {
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

// fieldExtractor returns a ProviderInfo field value extractor function based on field name.
func fieldExtractor(field string) (func(*account.ProviderInfo) any, error) {
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
