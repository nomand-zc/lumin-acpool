package account

import "testing"

func TestStatus_IsSelectable(t *testing.T) {
	cases := []struct {
		status   Status
		expected bool
	}{
		{StatusAvailable, true},
		{StatusCoolingDown, false},
		{StatusCircuitOpen, false},
		{StatusExpired, false},
		{StatusInvalidated, false},
		{StatusBanned, false},
		{StatusDisabled, false},
	}
	for _, c := range cases {
		got := c.status.IsSelectable()
		if got != c.expected {
			t.Errorf("Status(%d).IsSelectable() = %v, want %v", c.status, got, c.expected)
		}
	}
}

func TestStatus_IsRecoverable(t *testing.T) {
	recoverable := []Status{StatusCoolingDown, StatusCircuitOpen, StatusExpired}
	notRecoverable := []Status{StatusAvailable, StatusInvalidated, StatusBanned, StatusDisabled}

	for _, s := range recoverable {
		if !s.IsRecoverable() {
			t.Errorf("Status(%d).IsRecoverable() = false, want true", s)
		}
	}
	for _, s := range notRecoverable {
		if s.IsRecoverable() {
			t.Errorf("Status(%d).IsRecoverable() = true, want false", s)
		}
	}
}

func TestStatus_String(t *testing.T) {
	cases := []struct {
		status   Status
		expected string
	}{
		{StatusAvailable, "available"},
		{StatusCoolingDown, "cooling_down"},
		{StatusCircuitOpen, "circuit_open"},
		{StatusExpired, "expired"},
		{StatusInvalidated, "invalidated"},
		{StatusBanned, "banned"},
		{StatusDisabled, "disabled"},
		{Status(999), "unknown"},
	}
	for _, c := range cases {
		got := c.status.String()
		if got != c.expected {
			t.Errorf("Status(%d).String() = %q, want %q", c.status, got, c.expected)
		}
	}
}
