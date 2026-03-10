package cooldown

import (
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/usagerule"
)

func newTestAccount(id string) *account.Account {
	return &account.Account{
		ID:           id,
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	}
}

func TestNewCooldownManager_Defaults(t *testing.T) {
	cm := NewCooldownManager()
	if cm == nil {
		t.Fatal("expected non-nil CooldownManager")
	}
}

func TestStartCooldown_WithExplicitTime(t *testing.T) {
	cm := NewCooldownManager()
	acct := newTestAccount("acc-1")

	until := time.Now().Add(5 * time.Minute)
	cm.StartCooldown(acct, &until)

	if acct.CooldownUntil == nil {
		t.Fatal("expected CooldownUntil to be set")
	}
	if !acct.CooldownUntil.Equal(until) {
		t.Fatalf("expected CooldownUntil = %v, got %v", until, *acct.CooldownUntil)
	}
}

func TestStartCooldown_DefaultDuration(t *testing.T) {
	cm := NewCooldownManager(WithDefaultDuration(10 * time.Second))
	acct := newTestAccount("acc-1")

	before := time.Now()
	cm.StartCooldown(acct, nil)
	after := time.Now()

	if acct.CooldownUntil == nil {
		t.Fatal("expected CooldownUntil to be set")
	}

	expectedMin := before.Add(10 * time.Second)
	expectedMax := after.Add(10 * time.Second)

	if acct.CooldownUntil.Before(expectedMin) || acct.CooldownUntil.After(expectedMax) {
		t.Fatalf("CooldownUntil %v not in expected range [%v, %v]",
			*acct.CooldownUntil, expectedMin, expectedMax)
	}
}

func TestStartCooldown_WithUsageRules(t *testing.T) {
	cm := NewCooldownManager(WithDefaultDuration(30 * time.Second))
	acct := newTestAccount("acc-1")

	// 使用 1 小时粒度窗口的规则，CalculateWindowTime 会对齐到当前小时起始，
	// 窗口结束时间为当前小时 + 1 小时，所以剩余时间在 (0, 60min] 之间。
	acct.UsageRules = []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
		},
	}

	cm.StartCooldown(acct, nil)

	if acct.CooldownUntil == nil {
		t.Fatal("expected CooldownUntil to be set")
	}

	// 冷却时长应该基于窗口剩余时间（> 默认 30s），除非恰好在小时边界附近
	cooldownDuration := time.Until(*acct.CooldownUntil)
	// 窗口剩余时间一定 > 0 且 <= 1h
	if cooldownDuration <= 0 || cooldownDuration > time.Hour+time.Second {
		t.Fatalf("expected cooldown duration in (0, 1h], got %v", cooldownDuration)
	}
}

func TestIsCooldownExpired_NotExpired(t *testing.T) {
	cm := NewCooldownManager()
	acct := newTestAccount("acc-1")

	until := time.Now().Add(time.Hour)
	acct.CooldownUntil = &until

	if cm.IsCooldownExpired(acct) {
		t.Fatal("should not be expired when CooldownUntil is in the future")
	}
}

func TestIsCooldownExpired_Expired(t *testing.T) {
	cm := NewCooldownManager()
	acct := newTestAccount("acc-1")

	until := time.Now().Add(-time.Second)
	acct.CooldownUntil = &until

	if !cm.IsCooldownExpired(acct) {
		t.Fatal("should be expired when CooldownUntil is in the past")
	}
}

func TestIsCooldownExpired_NilCooldownUntil(t *testing.T) {
	cm := NewCooldownManager()
	acct := newTestAccount("acc-1")

	if !cm.IsCooldownExpired(acct) {
		t.Fatal("should be expired when CooldownUntil is nil")
	}
}
