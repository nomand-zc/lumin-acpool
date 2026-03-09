package filtercond

// Equal creates a condition for equality comparison.
func Equal(field string, value any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorEqual,
		Value:    value,
	}
}

// NotEqual creates a condition for inequality comparison.
func NotEqual(field string, value any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorNotEqual,
		Value:    value,
	}
}

// GreaterThan creates a condition for greater than comparison.
func GreaterThan(field string, value any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorGreaterThan,
		Value:    value,
	}
}

// GreaterThanOrEqual creates a condition for greater than or equal comparison.
func GreaterThanOrEqual(field string, value any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorGreaterThanOrEqual,
		Value:    value,
	}
}

// LessThan creates a condition for less than comparison.
func LessThan(field string, value any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorLessThan,
		Value:    value,
	}
}

// LessThanOrEqual creates a condition for less than or equal comparison.
func LessThanOrEqual(field string, value any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorLessThanOrEqual,
		Value:    value,
	}
}

// In creates a condition for IN operator.
func In(field string, values ...any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorIn,
		Value:    values,
	}
}

// NotIn creates a condition for NOT IN operator.
func NotIn(field string, values ...any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorNotIn,
		Value:    values,
	}
}

// Like creates a condition for LIKE operator (pattern matching).
func Like(field string, pattern string) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorLike,
		Value:    pattern,
	}
}

// NotLike creates a condition for NOT LIKE operator.
func NotLike(field string, pattern string) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorNotLike,
		Value:    pattern,
	}
}

// Between creates a condition for BETWEEN operator.
func Between(field string, min, max any) *FilterCondition {
	return &FilterCondition{
		Field:    field,
		Operator: OperatorBetween,
		Value:    []any{min, max},
	}
}

// And creates a condition that combines multiple conditions with AND logic.
func And(conditions ...*FilterCondition) *FilterCondition {
	return &FilterCondition{
		Operator: OperatorAnd,
		Value:    conditions,
	}
}

// Or creates a condition that combines multiple conditions with OR logic.
func Or(conditions ...*FilterCondition) *FilterCondition {
	return &FilterCondition{
		Operator: OperatorOr,
		Value:    conditions,
	}
}
