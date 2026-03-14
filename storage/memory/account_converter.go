package memory

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	"github.com/nomand-zc/lumin-acpool/storage/memory/internal/utils"
)

// AccountFilterFunc 是内存中 Account 的过滤函数类型。
type AccountFilterFunc = func(*account.Account) bool

// AccountConverter 将 Filter 条件转换为内存中 Account 的过滤函数。
type AccountConverter struct{}

// Convert 将 Filter 条件转换为内存中 Account 的过滤函数。
func (c *AccountConverter) Convert(cond *filtercond.Filter) (AccountFilterFunc, error) {
	if cond == nil {
		return func(*account.Account) bool { return true }, nil
	}
	return c.convertCondition(cond)
}

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
				return true
			}
			if isAnd && !result {
				return false
			}
		}
		return isAnd
	}, nil
}

func (c *AccountConverter) buildComparisonCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
	extractor, err := accountFieldExtractor(cond.Field)
	if err != nil {
		return nil, err
	}

	return func(a *account.Account) bool {
		docValue := extractor(a)
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
		for i := range itemNum {
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

func (c *AccountConverter) buildBetweenCondition(cond *filtercond.Filter) (AccountFilterFunc, error) {
	value := reflect.ValueOf(cond.Value)
	if value.Kind() != reflect.Slice || value.Len() != 2 {
		return nil, fmt.Errorf("between operator value must be a slice with two elements: %v", cond.Value)
	}

	var condFuncs []AccountFilterFunc
	for i := range 2 {
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

// accountFieldExtractor 根据字段名返回 Account 字段值提取函数。
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
