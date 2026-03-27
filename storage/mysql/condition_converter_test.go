package mysql

import (
	"strings"
	"testing"

	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

func newConverter() *MysqlConverter {
	fieldMapping := map[string]string{
		"status":   "status",
		"priority": "priority",
		"name":     "name",
	}
	jsonFields := map[string]bool{
		"models": true,
	}
	return NewConditionConverter(fieldMapping, jsonFields)
}

func TestConvert_NilFilter(t *testing.T) {
	c := newConverter()
	result, err := c.Convert(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "1=1" {
		t.Errorf("Cond = %q, want \"1=1\"", result.Cond)
	}
	if len(result.Args) != 0 {
		t.Errorf("Args should be empty, got %v", result.Args)
	}
}

func TestConvert_Equal(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "status", Operator: filtercond.OperatorEqual, Value: 1}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`status` = ?" {
		t.Errorf("Cond = %q, want \"`status` = ?\"", result.Cond)
	}
	if len(result.Args) != 1 || result.Args[0] != 1 {
		t.Errorf("Args = %v, want [1]", result.Args)
	}
}

func TestConvert_NotEqual(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "status", Operator: filtercond.OperatorNotEqual, Value: 2}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`status` != ?" {
		t.Errorf("Cond = %q, want \"`status` != ?\"", result.Cond)
	}
}

func TestConvert_GreaterThan(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "priority", Operator: filtercond.OperatorGreaterThan, Value: 5}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`priority` > ?" {
		t.Errorf("Cond = %q, want \"`priority` > ?\"", result.Cond)
	}
}

func TestConvert_GreaterThanOrEqual(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "priority", Operator: filtercond.OperatorGreaterThanOrEqual, Value: 5}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`priority` >= ?" {
		t.Errorf("Cond = %q", result.Cond)
	}
}

func TestConvert_LessThan(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "priority", Operator: filtercond.OperatorLessThan, Value: 10}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`priority` < ?" {
		t.Errorf("Cond = %q", result.Cond)
	}
}

func TestConvert_LessThanOrEqual(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "priority", Operator: filtercond.OperatorLessThanOrEqual, Value: 10}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`priority` <= ?" {
		t.Errorf("Cond = %q", result.Cond)
	}
}

func TestConvert_In(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "status", Operator: filtercond.OperatorIn, Value: []int{1, 2, 3}}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`status` IN (?, ?, ?)" {
		t.Errorf("Cond = %q, want \"`status` IN (?, ?, ?)\"", result.Cond)
	}
	if len(result.Args) != 3 {
		t.Errorf("Args len = %d, want 3", len(result.Args))
	}
}

func TestConvert_NotIn(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "status", Operator: filtercond.OperatorNotIn, Value: []int{1, 2}}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`status` NOT IN (?, ?)" {
		t.Errorf("Cond = %q", result.Cond)
	}
}

func TestConvert_In_EmptySlice(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "status", Operator: filtercond.OperatorIn, Value: []int{}}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for empty slice, got nil")
	}
}

func TestConvert_Like(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "name", Operator: filtercond.OperatorLike, Value: "%test%"}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`name` LIKE ?" {
		t.Errorf("Cond = %q, want \"`name` LIKE ?\"", result.Cond)
	}
	if len(result.Args) != 1 || result.Args[0] != "%test%" {
		t.Errorf("Args = %v", result.Args)
	}
}

func TestConvert_NotLike(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "name", Operator: filtercond.OperatorNotLike, Value: "%test%"}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`name` NOT LIKE ?" {
		t.Errorf("Cond = %q", result.Cond)
	}
}

func TestConvert_Like_NonStringValue(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "name", Operator: filtercond.OperatorLike, Value: 123}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for non-string value, got nil")
	}
}

func TestConvert_Between(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "priority", Operator: filtercond.OperatorBetween, Value: []int{1, 10}}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`priority` BETWEEN ? AND ?" {
		t.Errorf("Cond = %q", result.Cond)
	}
	if len(result.Args) != 2 {
		t.Errorf("Args len = %d, want 2", len(result.Args))
	}
}

func TestConvert_Between_WrongSliceLen(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "priority", Operator: filtercond.OperatorBetween, Value: []int{1}}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for slice with one element, got nil")
	}
}

func TestConvert_JSONContains_String(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "models", Operator: filtercond.OperatorJSONContains, Value: "gemini-pro"}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "JSON_CONTAINS(`models`, ?)" {
		t.Errorf("Cond = %q", result.Cond)
	}
	if len(result.Args) != 1 || result.Args[0] != `"gemini-pro"` {
		t.Errorf("Args = %v", result.Args)
	}
}

func TestConvert_JSONContains_Int(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "models", Operator: filtercond.OperatorJSONContains, Value: 42}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Args[0] != "42" {
		t.Errorf("Args = %v", result.Args)
	}
}

func TestConvert_JSONContains_Bool(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "models", Operator: filtercond.OperatorJSONContains, Value: true}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Args[0] != "true" {
		t.Errorf("Args = %v", result.Args)
	}
}

func TestConvert_JSONContains_Nil(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "models", Operator: filtercond.OperatorJSONContains, Value: nil}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Args[0] != "null" {
		t.Errorf("Args = %v", result.Args)
	}
}

func TestConvert_JSONNotContains(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "models", Operator: filtercond.OperatorJSONNotContains, Value: "gpt-4"}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "NOT JSON_CONTAINS(`models`, ?)" {
		t.Errorf("Cond = %q", result.Cond)
	}
}

func TestConvert_JSONContains_UnsupportedType(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "models", Operator: filtercond.OperatorJSONContains, Value: []string{"a", "b"}}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
}

func TestConvert_And(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{
		Operator: filtercond.OperatorAnd,
		Value: []*filtercond.Filter{
			{Field: "status", Operator: filtercond.OperatorEqual, Value: 1},
			{Field: "priority", Operator: filtercond.OperatorGreaterThan, Value: 5},
		},
	}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Cond, "AND") {
		t.Errorf("Cond should contain AND, got: %q", result.Cond)
	}
	if len(result.Args) != 2 {
		t.Errorf("Args len = %d, want 2", len(result.Args))
	}
}

func TestConvert_Or(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{
		Operator: filtercond.OperatorOr,
		Value: []*filtercond.Filter{
			{Field: "status", Operator: filtercond.OperatorEqual, Value: 1},
			{Field: "status", Operator: filtercond.OperatorEqual, Value: 2},
		},
	}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Cond, "OR") {
		t.Errorf("Cond should contain OR, got: %q", result.Cond)
	}
}

func TestConvert_And_InvalidValue(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{
		Operator: filtercond.OperatorAnd,
		Value:    "invalid",
	}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for invalid logical condition value, got nil")
	}
}

func TestConvert_UnsupportedOperator(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "status", Operator: "unsupported_op", Value: 1}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for unsupported operator, got nil")
	}
}

func TestConvert_EmptyField_Comparison(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "", Operator: filtercond.OperatorEqual, Value: 1}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for empty field, got nil")
	}
}

func TestConvert_EmptyField_In(t *testing.T) {
	c := newConverter()
	filter := &filtercond.Filter{Field: "", Operator: filtercond.OperatorIn, Value: []int{1, 2}}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for empty field in IN, got nil")
	}
}

func TestConvert_FieldMapping(t *testing.T) {
	c := NewConditionConverter(map[string]string{"my_field": "db_column"}, nil)
	filter := &filtercond.Filter{Field: "my_field", Operator: filtercond.OperatorEqual, Value: "val"}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`db_column` = ?" {
		t.Errorf("Cond = %q, want \"`db_column` = ?\"", result.Cond)
	}
}

func TestConvert_NilFieldMapping(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	filter := &filtercond.Filter{Field: "some_field", Operator: filtercond.OperatorEqual, Value: "val"}
	result, err := c.Convert(filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cond != "`some_field` = ?" {
		t.Errorf("Cond = %q", result.Cond)
	}
}

func TestConvert_And_Empty(t *testing.T) {
	// AND 无子条件应该报错
	c := newConverter()
	filter := &filtercond.Filter{
		Operator: filtercond.OperatorAnd,
		Value:    []*filtercond.Filter{},
	}
	_, err := c.Convert(filter)
	if err == nil {
		t.Fatal("expected error for empty AND, got nil")
	}
}
