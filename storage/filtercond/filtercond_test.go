package filtercond

import "testing"

func TestEqual(t *testing.T) {
	f := Equal("name", "alice")
	if f.Field != "name" {
		t.Errorf("Equal Field = %q, want \"name\"", f.Field)
	}
	if f.Operator != OperatorEqual {
		t.Errorf("Equal Operator = %q, want %q", f.Operator, OperatorEqual)
	}
	if f.Value != "alice" {
		t.Errorf("Equal Value = %v, want \"alice\"", f.Value)
	}
}

func TestNotEqual(t *testing.T) {
	f := NotEqual("status", "disabled")
	if f.Operator != OperatorNotEqual {
		t.Errorf("NotEqual Operator = %q, want %q", f.Operator, OperatorNotEqual)
	}
	if f.Field != "status" || f.Value != "disabled" {
		t.Errorf("NotEqual unexpected field/value: %+v", f)
	}
}

func TestGreaterThan(t *testing.T) {
	f := GreaterThan("priority", 5)
	if f.Operator != OperatorGreaterThan {
		t.Errorf("GreaterThan Operator = %q, want %q", f.Operator, OperatorGreaterThan)
	}
	if f.Value != 5 {
		t.Errorf("GreaterThan Value = %v, want 5", f.Value)
	}
}

func TestGreaterThanOrEqual(t *testing.T) {
	f := GreaterThanOrEqual("count", 10)
	if f.Operator != OperatorGreaterThanOrEqual {
		t.Errorf("GreaterThanOrEqual Operator = %q, want %q", f.Operator, OperatorGreaterThanOrEqual)
	}
	if f.Value != 10 {
		t.Errorf("GreaterThanOrEqual Value = %v, want 10", f.Value)
	}
}

func TestLessThan(t *testing.T) {
	f := LessThan("age", 18)
	if f.Operator != OperatorLessThan {
		t.Errorf("LessThan Operator = %q, want %q", f.Operator, OperatorLessThan)
	}
	if f.Value != 18 {
		t.Errorf("LessThan Value = %v, want 18", f.Value)
	}
}

func TestLessThanOrEqual(t *testing.T) {
	f := LessThanOrEqual("score", 100)
	if f.Operator != OperatorLessThanOrEqual {
		t.Errorf("LessThanOrEqual Operator = %q, want %q", f.Operator, OperatorLessThanOrEqual)
	}
	if f.Value != 100 {
		t.Errorf("LessThanOrEqual Value = %v, want 100", f.Value)
	}
}

func TestIn(t *testing.T) {
	f := In("env", "prod", "staging")
	if f.Operator != OperatorIn {
		t.Errorf("In Operator = %q, want %q", f.Operator, OperatorIn)
	}
	if f.Field != "env" {
		t.Errorf("In Field = %q, want \"env\"", f.Field)
	}
	vals, ok := f.Value.([]any)
	if !ok || len(vals) != 2 {
		t.Errorf("In Value should be []any of len 2, got %T %v", f.Value, f.Value)
	}
}

func TestNotIn(t *testing.T) {
	f := NotIn("status", "banned", "disabled")
	if f.Operator != OperatorNotIn {
		t.Errorf("NotIn Operator = %q, want %q", f.Operator, OperatorNotIn)
	}
	vals, ok := f.Value.([]any)
	if !ok || len(vals) != 2 {
		t.Errorf("NotIn Value should be []any of len 2, got %T %v", f.Value, f.Value)
	}
}

func TestLike(t *testing.T) {
	f := Like("name", "%admin%")
	if f.Operator != OperatorLike {
		t.Errorf("Like Operator = %q, want %q", f.Operator, OperatorLike)
	}
	if f.Value != "%admin%" {
		t.Errorf("Like Value = %v, want \"%%admin%%\"", f.Value)
	}
}

func TestNotLike(t *testing.T) {
	f := NotLike("name", "%test%")
	if f.Operator != OperatorNotLike {
		t.Errorf("NotLike Operator = %q, want %q", f.Operator, OperatorNotLike)
	}
	if f.Value != "%test%" {
		t.Errorf("NotLike Value = %v, want \"%%test%%\"", f.Value)
	}
}

func TestBetween(t *testing.T) {
	f := Between("age", 18, 65)
	if f.Operator != OperatorBetween {
		t.Errorf("Between Operator = %q, want %q", f.Operator, OperatorBetween)
	}
	vals, ok := f.Value.([]any)
	if !ok || len(vals) != 2 {
		t.Errorf("Between Value should be []any of len 2, got %T %v", f.Value, f.Value)
	}
	if vals[0] != 18 || vals[1] != 65 {
		t.Errorf("Between values = %v, want [18 65]", vals)
	}
}

func TestJSONContains(t *testing.T) {
	f := JSONContains("tags", "vip")
	if f.Operator != OperatorJSONContains {
		t.Errorf("JSONContains Operator = %q, want %q", f.Operator, OperatorJSONContains)
	}
	if f.Value != "vip" {
		t.Errorf("JSONContains Value = %v, want \"vip\"", f.Value)
	}
}

func TestJSONNotContains(t *testing.T) {
	f := JSONNotContains("tags", "blocked")
	if f.Operator != OperatorJSONNotContains {
		t.Errorf("JSONNotContains Operator = %q, want %q", f.Operator, OperatorJSONNotContains)
	}
	if f.Value != "blocked" {
		t.Errorf("JSONNotContains Value = %v, want \"blocked\"", f.Value)
	}
}

func TestAnd(t *testing.T) {
	c1 := Equal("env", "prod")
	c2 := GreaterThan("priority", 0)
	f := And(c1, c2)

	if f.Operator != OperatorAnd {
		t.Errorf("And Operator = %q, want %q", f.Operator, OperatorAnd)
	}
	if f.Field != "" {
		t.Errorf("And Field should be empty, got %q", f.Field)
	}
	conditions, ok := f.Value.([]*Filter)
	if !ok || len(conditions) != 2 {
		t.Errorf("And Value should be []*Filter of len 2, got %T %v", f.Value, f.Value)
	}
}

func TestOr(t *testing.T) {
	c1 := Equal("status", "active")
	c2 := Equal("status", "degraded")
	f := Or(c1, c2)

	if f.Operator != OperatorOr {
		t.Errorf("Or Operator = %q, want %q", f.Operator, OperatorOr)
	}
	conditions, ok := f.Value.([]*Filter)
	if !ok || len(conditions) != 2 {
		t.Errorf("Or Value should be []*Filter of len 2, got %T %v", f.Value, f.Value)
	}
}

func TestAnd_Empty(t *testing.T) {
	f := And()
	if f.Operator != OperatorAnd {
		t.Errorf("And() Operator = %q, want %q", f.Operator, OperatorAnd)
	}
	conditions, ok := f.Value.([]*Filter)
	if !ok || len(conditions) != 0 {
		t.Errorf("And() Value should be empty []*Filter, got %T %v", f.Value, f.Value)
	}
}

func TestOr_Empty(t *testing.T) {
	f := Or()
	if f.Operator != OperatorOr {
		t.Errorf("Or() Operator = %q, want %q", f.Operator, OperatorOr)
	}
	conditions, ok := f.Value.([]*Filter)
	if !ok || len(conditions) != 0 {
		t.Errorf("Or() Value should be empty []*Filter, got %T %v", f.Value, f.Value)
	}
}

func TestIn_NoValues(t *testing.T) {
	f := In("field")
	if f.Operator != OperatorIn {
		t.Errorf("In() Operator = %q, want %q", f.Operator, OperatorIn)
	}
	vals, ok := f.Value.([]any)
	if !ok || len(vals) != 0 {
		t.Errorf("In() with no values should have empty []any, got %T %v", f.Value, f.Value)
	}
}

// 验证所有常量值正确
func TestOperatorConstants(t *testing.T) {
	cases := map[string]string{
		OperatorAnd:              "and",
		OperatorOr:               "or",
		OperatorEqual:            "eq",
		OperatorNotEqual:         "ne",
		OperatorGreaterThan:      "gt",
		OperatorGreaterThanOrEqual: "gte",
		OperatorLessThan:         "lt",
		OperatorLessThanOrEqual:  "lte",
		OperatorIn:               "in",
		OperatorNotIn:            "not in",
		OperatorLike:             "like",
		OperatorNotLike:          "not like",
		OperatorBetween:          "between",
		OperatorJSONContains:     "json_contains",
		OperatorJSONNotContains:  "json_not_contains",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("operator constant = %q, want %q", got, want)
		}
	}
}
