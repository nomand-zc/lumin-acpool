package checks

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
)

func newCheckTarget(acct *account.Account) health.CheckTarget {
	return health.NewCheckTarget(acct, nil)
}

func TestRecoveryCheck_Name(t *testing.T) {
	c := NewRecoveryCheck()
	if c.Name() != RecoveryCheckName {
		t.Fatalf("expected name %q, got %q", RecoveryCheckName, c.Name())
	}
}

func TestRecoveryCheck_Severity(t *testing.T) {
	c := NewRecoveryCheck()
	if c.Severity() != health.SeverityCritical {
		t.Fatalf("expected severity Critical, got %v", c.Severity())
	}
}

func TestRecoveryCheck_DependsOn(t *testing.T) {
	c := NewRecoveryCheck()
	if c.DependsOn() != nil {
		t.Fatal("expected no dependencies")
	}
}

func TestRecoveryCheck_CoolingDown_Expired(t *testing.T) {
	c := NewRecoveryCheck()

	until := time.Now().Add(-time.Second)
	acct := &account.Account{
		ID:            "acc-1",
		Status:        account.StatusCoolingDown,
		CooldownUntil: &until,
	}

	result := c.Check(context.Background(), newCheckTarget(acct))

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed for expired cooldown, got %v", result.Status)
	}
	if result.SuggestedStatus == nil {
		t.Fatal("expected SuggestedStatus to be set")
	}
	if *result.SuggestedStatus != account.StatusAvailable {
		t.Fatalf("expected SuggestedStatus=Available, got %v", *result.SuggestedStatus)
	}
}

func TestRecoveryCheck_CoolingDown_NotExpired(t *testing.T) {
	c := NewRecoveryCheck()

	until := time.Now().Add(time.Hour)
	acct := &account.Account{
		ID:            "acc-1",
		Status:        account.StatusCoolingDown,
		CooldownUntil: &until,
	}

	result := c.Check(context.Background(), newCheckTarget(acct))

	if result.Status != health.CheckWarning {
		t.Fatalf("expected CheckWarning for active cooldown, got %v", result.Status)
	}
	if result.SuggestedStatus != nil {
		t.Fatal("expected SuggestedStatus to be nil for active cooldown")
	}
}

func TestRecoveryCheck_CircuitOpen_Expired(t *testing.T) {
	c := NewRecoveryCheck()

	until := time.Now().Add(-time.Second)
	acct := &account.Account{
		ID:               "acc-1",
		Status:           account.StatusCircuitOpen,
		CircuitOpenUntil: &until,
	}

	result := c.Check(context.Background(), newCheckTarget(acct))

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed for expired circuit open, got %v", result.Status)
	}
	if result.SuggestedStatus == nil {
		t.Fatal("expected SuggestedStatus to be set")
	}
	if *result.SuggestedStatus != account.StatusAvailable {
		t.Fatalf("expected SuggestedStatus=Available, got %v", *result.SuggestedStatus)
	}
}

func TestRecoveryCheck_CircuitOpen_NotExpired(t *testing.T) {
	c := NewRecoveryCheck()

	until := time.Now().Add(time.Hour)
	acct := &account.Account{
		ID:               "acc-1",
		Status:           account.StatusCircuitOpen,
		CircuitOpenUntil: &until,
	}

	result := c.Check(context.Background(), newCheckTarget(acct))

	if result.Status != health.CheckWarning {
		t.Fatalf("expected CheckWarning for active circuit open, got %v", result.Status)
	}
	if result.SuggestedStatus != nil {
		t.Fatal("expected SuggestedStatus to be nil for active circuit open")
	}
}

func TestRecoveryCheck_AvailableAccount_Skip(t *testing.T) {
	c := NewRecoveryCheck()

	acct := &account.Account{
		ID:     "acc-1",
		Status: account.StatusAvailable,
	}

	result := c.Check(context.Background(), newCheckTarget(acct))

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed for available account, got %v", result.Status)
	}
	if result.SuggestedStatus != nil {
		t.Fatal("expected SuggestedStatus to be nil for available account")
	}
}

func TestRecoveryCheck_DisabledAccount_Skip(t *testing.T) {
	c := NewRecoveryCheck()

	acct := &account.Account{
		ID:     "acc-1",
		Status: account.StatusDisabled,
	}

	result := c.Check(context.Background(), newCheckTarget(acct))

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed for disabled account, got %v", result.Status)
	}
	if result.SuggestedStatus != nil {
		t.Fatal("expected SuggestedStatus to be nil for disabled account")
	}
}
