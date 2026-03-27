package selector

import (
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
)

// helpers

func pkPtr(typ, name string) *account.ProviderKey {
	pk := account.BuildProviderKey(typ, name)
	return &pk
}

// --- IsExactProvider ---

func TestSelectRequest_IsExactProvider_BothSet(t *testing.T) {
	r := &SelectRequest{ProviderKey: pkPtr("kiro", "kiro-team-a")}
	if !r.IsExactProvider() {
		t.Error("expected true when both Type and Name are set")
	}
}

func TestSelectRequest_IsExactProvider_TypeOnly(t *testing.T) {
	r := &SelectRequest{ProviderKey: pkPtr("kiro", "")}
	if r.IsExactProvider() {
		t.Error("expected false when only Type is set")
	}
}

func TestSelectRequest_IsExactProvider_NilKey(t *testing.T) {
	r := &SelectRequest{ProviderKey: nil}
	if r.IsExactProvider() {
		t.Error("expected false when ProviderKey is nil")
	}
}

func TestSelectRequest_IsExactProvider_EmptyType(t *testing.T) {
	r := &SelectRequest{ProviderKey: pkPtr("", "kiro-team-a")}
	if r.IsExactProvider() {
		t.Error("expected false when Type is empty")
	}
}

// --- IsProviderTypeOnly ---

func TestSelectRequest_IsProviderTypeOnly_TypeOnly(t *testing.T) {
	r := &SelectRequest{ProviderKey: pkPtr("kiro", "")}
	if !r.IsProviderTypeOnly() {
		t.Error("expected true when only Type is set")
	}
}

func TestSelectRequest_IsProviderTypeOnly_BothSet(t *testing.T) {
	r := &SelectRequest{ProviderKey: pkPtr("kiro", "kiro-team-a")}
	if r.IsProviderTypeOnly() {
		t.Error("expected false when both Type and Name are set")
	}
}

func TestSelectRequest_IsProviderTypeOnly_NilKey(t *testing.T) {
	r := &SelectRequest{ProviderKey: nil}
	if r.IsProviderTypeOnly() {
		t.Error("expected false when ProviderKey is nil")
	}
}

func TestSelectRequest_IsProviderTypeOnly_EmptyType(t *testing.T) {
	r := &SelectRequest{ProviderKey: pkPtr("", "kiro-team-a")}
	if r.IsProviderTypeOnly() {
		t.Error("expected false when Type is empty")
	}
}

// --- IsAutoSelect ---

func TestSelectRequest_IsAutoSelect_NilKey(t *testing.T) {
	r := &SelectRequest{ProviderKey: nil}
	if !r.IsAutoSelect() {
		t.Error("expected true when ProviderKey is nil")
	}
}

func TestSelectRequest_IsAutoSelect_HasKey(t *testing.T) {
	r := &SelectRequest{ProviderKey: pkPtr("kiro", "")}
	if r.IsAutoSelect() {
		t.Error("expected false when ProviderKey is not nil")
	}
}

// --- MutualExclusion ---

func TestSelectRequest_MutualExclusion(t *testing.T) {
	cases := []struct {
		name        string
		req         *SelectRequest
		wantExact   bool
		wantTypeOnly bool
		wantAuto    bool
	}{
		{
			name:        "AutoSelect",
			req:         &SelectRequest{ProviderKey: nil},
			wantExact:   false,
			wantTypeOnly: false,
			wantAuto:    true,
		},
		{
			name:        "TypeOnly",
			req:         &SelectRequest{ProviderKey: pkPtr("kiro", "")},
			wantExact:   false,
			wantTypeOnly: true,
			wantAuto:    false,
		},
		{
			name:        "ExactProvider",
			req:         &SelectRequest{ProviderKey: pkPtr("kiro", "kiro-team-a")},
			wantExact:   true,
			wantTypeOnly: false,
			wantAuto:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.IsExactProvider(); got != tc.wantExact {
				t.Errorf("IsExactProvider() = %v, want %v", got, tc.wantExact)
			}
			if got := tc.req.IsProviderTypeOnly(); got != tc.wantTypeOnly {
				t.Errorf("IsProviderTypeOnly() = %v, want %v", got, tc.wantTypeOnly)
			}
			if got := tc.req.IsAutoSelect(); got != tc.wantAuto {
				t.Errorf("IsAutoSelect() = %v, want %v", got, tc.wantAuto)
			}

			trueCount := 0
			if tc.req.IsExactProvider() {
				trueCount++
			}
			if tc.req.IsProviderTypeOnly() {
				trueCount++
			}
			if tc.req.IsAutoSelect() {
				trueCount++
			}
			if trueCount != 1 {
				t.Errorf("expected exactly one method to return true, got %d", trueCount)
			}
		})
	}
}

// --- error constants compile check ---

func TestErrors_Defined(t *testing.T) {
	if ErrNoAvailableAccount == nil {
		t.Error("ErrNoAvailableAccount should not be nil")
	}
	if ErrNoAvailableProvider == nil {
		t.Error("ErrNoAvailableProvider should not be nil")
	}
	if ErrEmptyCandidates == nil {
		t.Error("ErrEmptyCandidates should not be nil")
	}
}
