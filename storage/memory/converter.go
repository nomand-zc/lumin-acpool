package memory

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

const (
	valueTypeString = "string"
	valueTypeNumber = "number"
	valueTypeBool   = "bool"
	valueTypeTime   = "time"
)

// AccountFilterFunc is the in-memory filter function type for Account.
type AccountFilterFunc = func(*account.Account) bool

// ProviderFilterFunc is the in-memory filter function type for ProviderInfo.
type ProviderFilterFunc = func(*provider.ProviderInfo) bool

// ============================================================
// AccountConverter converts Filter conditions to in-memory filter functions for Account.
// ============================================================

// AccountConverter converts Filter conditions to in-memory filter functions for Account.
// Implements filtercond.Converter[AccountFilterFunc].
type AccountConverter struct{}

// Convert converts a Filter condition to an in-memory filter function for Account.
func (c *AccountConverter) Convert(cond *filtercond.Filter) (AccountFilterFunc, error) {
	if cond == nil {
		return func(*account.Account) bool { return true }, nil
	}
	return c.convertCondition(cond)
}

// convertCondition is the unified dispatch entry point that routes to the corresponding builder method based on operator type.
func (c *AccountConverter) convertCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
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
func (c *AccountConverter) buildLogicalCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
	subs, ok := cond.Value.([]*filtercond.Filter)
	if !ok {
		return nil, fmt.Errorf("logical operator value must be []*Filter: %v", cond.Value)
	}

	var predicates []AccountFilterFunc
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

	return func(a *account.Account) bool {
		isAnd := cond.Operator == filtercond.OperatorAnd
		for _, p := range predicates {
		result := p(a)
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
func (c *AccountConverter) buildComparisonCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
	extractor, err := accountFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	return func(a *account.Account) bool {
		docValue := extractor(a)
		switch valueType(cond.Value) {
		case valueTypeString:
			return compareString(docValue, cond.Value, cond.Operator)
		case valueTypeNumber:
			return compareNumber(docValue, cond.Value, cond.Operator)
		case valueTypeTime:
			return compareTime(docValue, cond.Value, cond.Operator)
		case valueTypeBool:
			return compareBool(docValue, cond.Value, cond.Operator)
		}
		return false
	}, nil
}

// buildInCondition builds IN/NOT IN conditions.
func (c *AccountConverter) buildInCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
	extractor, err := accountFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	s := reflect.ValueOf(cond.Value)
	if s.Kind() != reflect.Slice || s.Len() <= 0 {
		return nil, fmt.Errorf("in operator value must be a slice with at least one value: %v", cond.Value)
	}

	itemNum := s.Len()
	return func(a *account.Account) bool {
		docValue := extractor(a)
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
func (c *AccountConverter) buildBetweenCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() != 2 {
		return nil, fmt.Errorf("between operator value must be a slice with two elements: %v", cond.Value)
	}

	var condFuncs []AccountFilterFunc
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

	return func(a *account.Account) bool {
		for _, fn := range condFuncs {
			if !fn(a) {
				return false
			}
		}
		return true
	}, nil
}

// buildLikeCondition builds LIKE/NOT LIKE conditions.
func (c *AccountConverter) buildLikeCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
	extractor, err := accountFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	pattern, ok := cond.Value.(string)
	if !ok {
		return nil, fmt.Errorf("like operator requires a string pattern")
	}

	return func(a *account.Account) bool {
		docValue := extractor(a)
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

// accountFieldExtractor returns an Account field value extractor function based on field name.
func accountFieldExtractor(field string) (func(*account.Account) any, error) {
	switch field {
	case storage.AccountFieldID:
		return func(a *account.Account) any { return a.ID }, nil
	case storage.AccountFieldProviderType:
		return func(a *account.Account) any { return a.ProviderType }, nil
	case storage.AccountFieldProviderName:
		return func(a *account.Account) any { return a.ProviderName }, nil
	case storage.AccountFieldStatus:
		return func(a *account.Account) any { return int(a.Status) }, nil
	case storage.AccountFieldPriority:
		return func(a *account.Account) any { return a.Priority }, nil
	case storage.AccountFieldTotalCalls:
		return func(a *account.Account) any { return a.TotalCalls }, nil
	case storage.AccountFieldSuccessCalls:
		return func(a *account.Account) any { return a.SuccessCalls }, nil
	case storage.AccountFieldFailedCalls:
		return func(a *account.Account) any { return a.FailedCalls }, nil
	case storage.AccountFieldConsecutiveFailures:
		return func(a *account.Account) any { return a.ConsecutiveFailures }, nil
	case storage.AccountFieldLastUsedAt:
		return func(a *account.Account) any {
			if a.LastUsedAt == nil {
				return time.Time{}
			}
			return *a.LastUsedAt
		}, nil
	case storage.AccountFieldLastErrorAt:
		return func(a *account.Account) any {
			if a.LastErrorAt == nil {
				return time.Time{}
			}
			return *a.LastErrorAt
		}, nil
	case storage.AccountFieldLastErrorMsg:
		return func(a *account.Account) any { return a.LastErrorMsg }, nil
	case storage.AccountFieldCooldownUntil:
		return func(a *account.Account) any {
			if a.CooldownUntil == nil {
				return time.Time{}
			}
			return *a.CooldownUntil
		}, nil
	case storage.AccountFieldCircuitOpenUntil:
		return func(a *account.Account) any {
			if a.CircuitOpenUntil == nil {
				return time.Time{}
			}
			return *a.CircuitOpenUntil
		}, nil
	case storage.AccountFieldCreatedAt:
		return func(a *account.Account) any { return a.CreatedAt }, nil
	case storage.AccountFieldUpdatedAt:
		return func(a *account.Account) any { return a.UpdatedAt }, nil
	default:
		return nil, fmt.Errorf("memory: unsupported account field: %s", field)
	}
}

// ============================================================
// ProviderConverter converts Filter conditions to in-memory filter functions for ProviderInfo.
// ============================================================

// ProviderConverter converts Filter conditions to in-memory filter functions for ProviderInfo.
// Implements filtercond.Converter[ProviderFilterFunc].
type ProviderConverter struct{}

// Convert converts a Filter condition to an in-memory filter function for ProviderInfo.
func (c *ProviderConverter) Convert(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	if cond == nil {
		return func(*provider.ProviderInfo) bool { return true }, nil
	}
	return c.convertCondition(cond)
}

// convertCondition is the unified dispatch entry point that routes to the corresponding builder method based on operator type.
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

// buildLogicalCondition builds AND/OR logical conditions.
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

	return func(p *provider.ProviderInfo) bool {
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
func (c *ProviderConverter) buildComparisonCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	// supported_model field uses special array containment logic
	if cond.Field == storage.ProviderFieldSupportedModel {
		return c.buildModelFilter(cond)
	}

	extractor, err := providerFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	return func(p *provider.ProviderInfo) bool {
		docValue := extractor(p)
		switch valueType(cond.Value) {
		case valueTypeString:
			return compareString(docValue, cond.Value, cond.Operator)
		case valueTypeNumber:
			return compareNumber(docValue, cond.Value, cond.Operator)
		case valueTypeTime:
			return compareTime(docValue, cond.Value, cond.Operator)
		case valueTypeBool:
			return compareBool(docValue, cond.Value, cond.Operator)
		}
		return false
	}, nil
}

// buildInCondition builds IN/NOT IN conditions.
func (c *ProviderConverter) buildInCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	// supported_model field uses special array containment logic
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
	return func(p *provider.ProviderInfo) bool {
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

	return func(p *provider.ProviderInfo) bool {
		for _, fn := range condFuncs {
			if !fn(p) {
				return false
			}
		}
		return true
	}, nil
}

// buildLikeCondition builds LIKE/NOT LIKE conditions.
func (c *ProviderConverter) buildLikeCondition(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	extractor, err := providerFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	pattern, ok := cond.Value.(string)
	if !ok {
		return nil, fmt.Errorf("like operator requires a string pattern")
	}

	return func(p *provider.ProviderInfo) bool {
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
func (c *ProviderConverter) buildModelFilter(cond *filtercond.Filter) (ProviderFilterFunc, error) {
	switch cond.Operator {
	case filtercond.OperatorEqual:
		model, ok := cond.Value.(string)
		if !ok {
			return nil, fmt.Errorf("memory: supported_model eq value must be string")
		}
		return func(p *provider.ProviderInfo) bool {
			return p.SupportsModel(model)
		}, nil
	case filtercond.OperatorIn:
		models, err := toStringSlice(cond.Value)
		if err != nil {
			return nil, fmt.Errorf("memory: supported_model in value must be string slice: %w", err)
		}
		return func(p *provider.ProviderInfo) bool {
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

// providerFieldExtractor returns a ProviderInfo field value extractor function based on field name.
func providerFieldExtractor(field string) (func(*provider.ProviderInfo) any, error) {
	switch field {
	case storage.ProviderFieldType:
		return func(p *provider.ProviderInfo) any { return p.ProviderType }, nil
	case storage.ProviderFieldName:
		return func(p *provider.ProviderInfo) any { return p.ProviderName }, nil
	case storage.ProviderFieldStatus:
		return func(p *provider.ProviderInfo) any { return int(p.Status) }, nil
	case storage.ProviderFieldPriority:
		return func(p *provider.ProviderInfo) any { return p.Priority }, nil
	case storage.ProviderFieldWeight:
		return func(p *provider.ProviderInfo) any { return p.Weight }, nil
	case storage.ProviderFieldAccountCount:
		return func(p *provider.ProviderInfo) any { return p.AccountCount }, nil
	case storage.ProviderFieldAvailableAccountCount:
		return func(p *provider.ProviderInfo) any { return p.AvailableAccountCount }, nil
	case storage.ProviderFieldCreatedAt:
		return func(p *provider.ProviderInfo) any { return p.CreatedAt }, nil
	case storage.ProviderFieldUpdatedAt:
		return func(p *provider.ProviderInfo) any { return p.UpdatedAt }, nil
	default:
		return nil, fmt.Errorf("memory: unsupported provider field: %s", field)
	}
}
