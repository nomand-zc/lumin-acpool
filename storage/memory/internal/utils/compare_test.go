package utils

import (
	"testing"
	"time"
)

// --- ValueType 测试 ---

func TestValueType_String(t *testing.T) {
	if got := ValueType("hello"); got != ValueTypeString {
		t.Errorf("ValueType(string) = %q, want %q", got, ValueTypeString)
	}
}

func TestValueType_Int(t *testing.T) {
	if got := ValueType(42); got != ValueTypeNumber {
		t.Errorf("ValueType(int) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Int8(t *testing.T) {
	if got := ValueType(int8(8)); got != ValueTypeNumber {
		t.Errorf("ValueType(int8) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Int16(t *testing.T) {
	if got := ValueType(int16(16)); got != ValueTypeNumber {
		t.Errorf("ValueType(int16) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Int32(t *testing.T) {
	if got := ValueType(int32(32)); got != ValueTypeNumber {
		t.Errorf("ValueType(int32) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Int64(t *testing.T) {
	if got := ValueType(int64(64)); got != ValueTypeNumber {
		t.Errorf("ValueType(int64) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Uint(t *testing.T) {
	if got := ValueType(uint(1)); got != ValueTypeNumber {
		t.Errorf("ValueType(uint) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Uint8(t *testing.T) {
	if got := ValueType(uint8(1)); got != ValueTypeNumber {
		t.Errorf("ValueType(uint8) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Uint16(t *testing.T) {
	if got := ValueType(uint16(1)); got != ValueTypeNumber {
		t.Errorf("ValueType(uint16) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Uint32(t *testing.T) {
	if got := ValueType(uint32(1)); got != ValueTypeNumber {
		t.Errorf("ValueType(uint32) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Uint64(t *testing.T) {
	if got := ValueType(uint64(1)); got != ValueTypeNumber {
		t.Errorf("ValueType(uint64) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Float32(t *testing.T) {
	if got := ValueType(float32(3.14)); got != ValueTypeNumber {
		t.Errorf("ValueType(float32) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Float64(t *testing.T) {
	if got := ValueType(3.14); got != ValueTypeNumber {
		t.Errorf("ValueType(float64) = %q, want %q", got, ValueTypeNumber)
	}
}

func TestValueType_Bool(t *testing.T) {
	if got := ValueType(true); got != ValueTypeBool {
		t.Errorf("ValueType(bool) = %q, want %q", got, ValueTypeBool)
	}
}

func TestValueType_Time(t *testing.T) {
	if got := ValueType(time.Now()); got != ValueTypeTime {
		t.Errorf("ValueType(time.Time) = %q, want %q", got, ValueTypeTime)
	}
}

func TestValueType_Unknown(t *testing.T) {
	if got := ValueType(struct{}{}); got != "" {
		t.Errorf("ValueType(struct{}) = %q, want \"\"", got)
	}
}

// --- CompareString 测试 ---

func TestCompareString_Equal(t *testing.T) {
	if !CompareString("abc", "abc", "eq") {
		t.Error("CompareString equal failed")
	}
	if CompareString("abc", "def", "eq") {
		t.Error("CompareString equal mismatch should be false")
	}
}

func TestCompareString_NotEqual(t *testing.T) {
	if !CompareString("abc", "def", "ne") {
		t.Error("CompareString not equal failed")
	}
	if CompareString("abc", "abc", "ne") {
		t.Error("CompareString not equal same values should be false")
	}
}

func TestCompareString_GreaterThan(t *testing.T) {
	if !CompareString("b", "a", "gt") {
		t.Error("CompareString gt failed")
	}
	if CompareString("a", "b", "gt") {
		t.Error("CompareString gt reverse should be false")
	}
}

func TestCompareString_GreaterThanOrEqual(t *testing.T) {
	if !CompareString("b", "a", "gte") {
		t.Error("CompareString gte failed")
	}
	if !CompareString("a", "a", "gte") {
		t.Error("CompareString gte equal failed")
	}
}

func TestCompareString_LessThan(t *testing.T) {
	if !CompareString("a", "b", "lt") {
		t.Error("CompareString lt failed")
	}
}

func TestCompareString_LessThanOrEqual(t *testing.T) {
	if !CompareString("a", "b", "lte") {
		t.Error("CompareString lte failed")
	}
	if !CompareString("a", "a", "lte") {
		t.Error("CompareString lte equal failed")
	}
}

func TestCompareString_InvalidTypes(t *testing.T) {
	if CompareString(123, "abc", "eq") {
		t.Error("CompareString with non-string docValue should return false")
	}
	if CompareString("abc", 123, "eq") {
		t.Error("CompareString with non-string condValue should return false")
	}
}

func TestCompareString_UnknownOperator(t *testing.T) {
	if CompareString("a", "b", "unknown") {
		t.Error("CompareString with unknown operator should return false")
	}
}

// --- CompareBool 测试 ---

func TestCompareBool_Equal(t *testing.T) {
	if !CompareBool(true, true, "eq") {
		t.Error("CompareBool true==true failed")
	}
	if CompareBool(true, false, "eq") {
		t.Error("CompareBool true==false should be false")
	}
}

func TestCompareBool_NotEqual(t *testing.T) {
	if !CompareBool(true, false, "ne") {
		t.Error("CompareBool true!=false failed")
	}
	if CompareBool(true, true, "ne") {
		t.Error("CompareBool true!=true should be false")
	}
}

func TestCompareBool_InvalidTypes(t *testing.T) {
	if CompareBool("true", true, "eq") {
		t.Error("CompareBool with non-bool docValue should return false")
	}
	if CompareBool(true, "true", "eq") {
		t.Error("CompareBool with non-bool condValue should return false")
	}
}

func TestCompareBool_UnknownOperator(t *testing.T) {
	if CompareBool(true, true, "gt") {
		t.Error("CompareBool with unsupported operator should return false")
	}
}

// --- CompareTime 测试 ---

func TestCompareTime_Equal(t *testing.T) {
	now := time.Now()
	if !CompareTime(now, now, "eq") {
		t.Error("CompareTime equal failed")
	}
	later := now.Add(time.Second)
	if CompareTime(now, later, "eq") {
		t.Error("CompareTime equal different times should be false")
	}
}

func TestCompareTime_NotEqual(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Second)
	if !CompareTime(now, later, "ne") {
		t.Error("CompareTime not equal failed")
	}
	if CompareTime(now, now, "ne") {
		t.Error("CompareTime not equal same time should be false")
	}
}

func TestCompareTime_GreaterThan(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Second)
	if !CompareTime(now, earlier, "gt") {
		t.Error("CompareTime gt failed")
	}
}

func TestCompareTime_GreaterThanOrEqual(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Second)
	if !CompareTime(now, earlier, "gte") {
		t.Error("CompareTime gte gt case failed")
	}
	if !CompareTime(now, now, "gte") {
		t.Error("CompareTime gte equal case failed")
	}
}

func TestCompareTime_LessThan(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Second)
	if !CompareTime(now, later, "lt") {
		t.Error("CompareTime lt failed")
	}
}

func TestCompareTime_LessThanOrEqual(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Second)
	if !CompareTime(now, later, "lte") {
		t.Error("CompareTime lte lt case failed")
	}
	if !CompareTime(now, now, "lte") {
		t.Error("CompareTime lte equal case failed")
	}
}

func TestCompareTime_WithPointer(t *testing.T) {
	now := time.Now()
	ptr := &now
	if !CompareTime(ptr, now, "eq") {
		t.Error("CompareTime with *time.Time failed")
	}
}

func TestCompareTime_InvalidTypes(t *testing.T) {
	if CompareTime("not-a-time", time.Now(), "eq") {
		t.Error("CompareTime with non-time docValue should return false")
	}
}

func TestCompareTime_UnknownOperator(t *testing.T) {
	now := time.Now()
	if CompareTime(now, now, "unknown") {
		t.Error("CompareTime with unknown operator should return false")
	}
}

// --- CompareNumber 测试 ---

func TestCompareNumber_Equal(t *testing.T) {
	if !CompareNumber(42, 42, "eq") {
		t.Error("CompareNumber equal int failed")
	}
	if !CompareNumber(3.14, 3.14, "eq") {
		t.Error("CompareNumber equal float failed")
	}
	if CompareNumber(1, 2, "eq") {
		t.Error("CompareNumber equal different values should be false")
	}
}

func TestCompareNumber_NotEqual(t *testing.T) {
	if !CompareNumber(1, 2, "ne") {
		t.Error("CompareNumber ne failed")
	}
	if CompareNumber(1, 1, "ne") {
		t.Error("CompareNumber ne same values should be false")
	}
}

func TestCompareNumber_GreaterThan(t *testing.T) {
	if !CompareNumber(10, 5, "gt") {
		t.Error("CompareNumber gt failed")
	}
	if CompareNumber(5, 10, "gt") {
		t.Error("CompareNumber gt reverse should be false")
	}
}

func TestCompareNumber_GreaterThanOrEqual(t *testing.T) {
	if !CompareNumber(10, 5, "gte") {
		t.Error("CompareNumber gte gt case failed")
	}
	if !CompareNumber(5, 5, "gte") {
		t.Error("CompareNumber gte equal case failed")
	}
}

func TestCompareNumber_LessThan(t *testing.T) {
	if !CompareNumber(5, 10, "lt") {
		t.Error("CompareNumber lt failed")
	}
}

func TestCompareNumber_LessThanOrEqual(t *testing.T) {
	if !CompareNumber(5, 10, "lte") {
		t.Error("CompareNumber lte lt case failed")
	}
	if !CompareNumber(5, 5, "lte") {
		t.Error("CompareNumber lte equal case failed")
	}
}

func TestCompareNumber_InvalidTypes(t *testing.T) {
	if CompareNumber("not-a-number", 5, "eq") {
		t.Error("CompareNumber with non-number docValue should return false")
	}
}

func TestCompareNumber_UnknownOperator(t *testing.T) {
	if CompareNumber(1, 2, "unknown") {
		t.Error("CompareNumber with unknown operator should return false")
	}
}

func TestCompareNumber_MixedTypes(t *testing.T) {
	// int vs float64
	if !CompareNumber(int64(10), float64(10), "eq") {
		t.Error("CompareNumber int64 vs float64 equal failed")
	}
}

// --- ToFloat64 测试 ---

func TestToFloat64_AllTypes(t *testing.T) {
	cases := []struct {
		input any
		want  float64
	}{
		{int(1), 1},
		{int8(2), 2},
		{int16(3), 3},
		{int32(4), 4},
		{int64(5), 5},
		{uint(6), 6},
		{uint8(7), 7},
		{uint16(8), 8},
		{uint32(9), 9},
		{uint64(10), 10},
		{float32(1.5), float64(float32(1.5))},
		{float64(2.5), 2.5},
	}
	for _, c := range cases {
		got, ok := ToFloat64(c.input)
		if !ok {
			t.Errorf("ToFloat64(%T) ok = false, want true", c.input)
		}
		if got != c.want {
			t.Errorf("ToFloat64(%v) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestToFloat64_Invalid(t *testing.T) {
	_, ok := ToFloat64("not-a-number")
	if ok {
		t.Error("ToFloat64(string) should return ok=false")
	}
}

// --- ToTime 测试 ---

func TestToTime_TimeValue(t *testing.T) {
	now := time.Now()
	got, ok := ToTime(now)
	if !ok {
		t.Error("ToTime(time.Time) ok = false")
	}
	if !got.Equal(now) {
		t.Errorf("ToTime(time.Time) = %v, want %v", got, now)
	}
}

func TestToTime_TimePointer(t *testing.T) {
	now := time.Now()
	got, ok := ToTime(&now)
	if !ok {
		t.Error("ToTime(*time.Time) ok = false")
	}
	if !got.Equal(now) {
		t.Errorf("ToTime(*time.Time) = %v, want %v", got, now)
	}
}

func TestToTime_NilPointer(t *testing.T) {
	var ptr *time.Time
	got, ok := ToTime(ptr)
	if !ok {
		t.Error("ToTime(nil *time.Time) ok = false")
	}
	if !got.IsZero() {
		t.Errorf("ToTime(nil *time.Time) = %v, want zero time", got)
	}
}

func TestToTime_Invalid(t *testing.T) {
	_, ok := ToTime("not-a-time")
	if ok {
		t.Error("ToTime(string) should return ok=false")
	}
}

// --- ToStringSlice 测试 ---

func TestToStringSlice_StringSlice(t *testing.T) {
	input := []string{"a", "b", "c"}
	got, err := ToStringSlice(input)
	if err != nil {
		t.Fatalf("ToStringSlice([]string) error: %v", err)
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("ToStringSlice([]string) = %v, want [a b c]", got)
	}
}

func TestToStringSlice_AnySlice(t *testing.T) {
	input := []any{"x", "y"}
	got, err := ToStringSlice(input)
	if err != nil {
		t.Fatalf("ToStringSlice([]any with strings) error: %v", err)
	}
	if len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("ToStringSlice([]any) = %v, want [x y]", got)
	}
}

func TestToStringSlice_AnySliceWithNonString(t *testing.T) {
	input := []any{"a", 42}
	_, err := ToStringSlice(input)
	if err == nil {
		t.Error("ToStringSlice([]any with non-string) should return error")
	}
}

func TestToStringSlice_InvalidType(t *testing.T) {
	_, err := ToStringSlice(123)
	if err == nil {
		t.Error("ToStringSlice(int) should return error")
	}
}

func TestToStringSlice_EmptySlice(t *testing.T) {
	got, err := ToStringSlice([]string{})
	if err != nil {
		t.Fatalf("ToStringSlice(empty []string) error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ToStringSlice(empty) = %v, want empty", got)
	}
}
