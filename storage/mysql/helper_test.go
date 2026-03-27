package mysql

import (
	"errors"
	"testing"
)

// ---------- MarshalJSON ----------

func TestMarshalJSON_Nil(t *testing.T) {
	result, err := MarshalJSON(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMarshalJSON_String(t *testing.T) {
	result, err := MarshalJSON("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != `"hello"` {
		t.Errorf("result = %q, want %q", *result, `"hello"`)
	}
}

func TestMarshalJSON_Map(t *testing.T) {
	m := map[string]any{"key": "value", "num": 42}
	result, err := MarshalJSON(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(*result) == 0 {
		t.Error("expected non-empty JSON string")
	}
}

func TestMarshalJSON_Slice(t *testing.T) {
	s := []string{"a", "b", "c"}
	result, err := MarshalJSON(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != `["a","b","c"]` {
		t.Errorf("result = %q", *result)
	}
}

func TestMarshalJSON_Int(t *testing.T) {
	result, err := MarshalJSON(123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != "123" {
		t.Errorf("result = %q, want \"123\"", *result)
	}
}

func TestMarshalJSON_Bool(t *testing.T) {
	result, err := MarshalJSON(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || *result != "true" {
		t.Errorf("result = %v", result)
	}
}

// ---------- IsDuplicateEntry ----------

func TestIsDuplicateEntry_Nil(t *testing.T) {
	if IsDuplicateEntry(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsDuplicateEntry_DuplicateMsg(t *testing.T) {
	err := errors.New("Error 1062: Duplicate entry 'foo' for key 'PRIMARY'")
	if !IsDuplicateEntry(err) {
		t.Error("expected true for Duplicate entry error")
	}
}

func TestIsDuplicateEntry_1062Code(t *testing.T) {
	err := errors.New("mysql error 1062: something")
	if !IsDuplicateEntry(err) {
		t.Error("expected true for 1062 error")
	}
}

func TestIsDuplicateEntry_OtherError(t *testing.T) {
	err := errors.New("connection refused")
	if IsDuplicateEntry(err) {
		t.Error("expected false for unrelated error")
	}
}
