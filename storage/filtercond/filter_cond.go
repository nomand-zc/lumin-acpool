package filtercond

const (
	// OperatorAnd is the "and" operator.
	OperatorAnd = "and"

	// OperatorOr is the "or" operator.
	OperatorOr = "or"

	// OperatorEqual is the "equal" operator.
	OperatorEqual = "eq"

	// OperatorNotEqual is the "not equal" operator.
	OperatorNotEqual = "ne"

	// OperatorGreaterThan is the "greater than" operator.
	OperatorGreaterThan = "gt"

	// OperatorGreaterThanOrEqual is the "greater than or equal" operator.
	OperatorGreaterThanOrEqual = "gte"

	// OperatorLessThan is the "less than" operator.
	OperatorLessThan = "lt"

	// OperatorLessThanOrEqual is the "less than or equal" operator.
	OperatorLessThanOrEqual = "lte"

	// OperatorIn is the "in" operator.
	OperatorIn = "in"

	// OperatorNotIn is the "not in" operator.
	OperatorNotIn = "not in"

	// OperatorLike is the "contains" operator.
	OperatorLike = "like"

	// OperatorNotLike is the "not contains" operator.
	OperatorNotLike = "not like"

	// OperatorBetween is the "between" operator.
	OperatorBetween = "between"

	// OperatorJSONContains is the "JSON_CONTAINS" operator for JSON array fields.
	OperatorJSONContains = "json_contains"

	// OperatorJSONNotContains is the "NOT JSON_CONTAINS" operator for JSON array fields.
	OperatorJSONNotContains = "json_not_contains"
)

// Converter is an interface for converting universal filter conditions to specific query formats.
type Converter[T any] interface {
	// Convert converts a universal filter condition to a specific query format.
	Convert(condition *Filter) (T, error)
}

// Filter represents a single condition for a search filter.
type Filter struct {
	// Field is the metadata field to filter on.
	// Required for comparison operators, not used for logical operators (and/or).
	Field string

	// Operator is the comparison or logical operator.
	// Comparison operators: eq, ne, gt, gte, lt, lte, in, not in, like, not like, between
	// Logical operators: and, or
	Operator string

	// Value is the value to compare against or sub-conditions for logical operators.
	// For comparison operators: single value, array for "in"/"not in"/"between"
	// For logical operators (and/or): array of UniversalFilter objects
	Value any
}
