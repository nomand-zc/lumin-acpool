package checks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/queue"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// ============================================================
// 通用 Mock 基础设施
// ============================================================

// mockCredential 实现 credentials.Credential 接口
type mockCredential struct {
	validateErr error
	expired     bool
	expiresAt   *time.Time
}

func (c *mockCredential) Clone() credentials.Credential              { return c }
func (c *mockCredential) Validate() error                            { return c.validateErr }
func (c *mockCredential) GetAccessToken() string                     { return "access-token" }
func (c *mockCredential) GetRefreshToken() string                    { return "refresh-token" }
func (c *mockCredential) GetExpiresAt() *time.Time                   { return c.expiresAt }
func (c *mockCredential) IsExpired() bool                            { return c.expired }
func (c *mockCredential) GetUserInfo() (credentials.UserInfo, error) { return credentials.UserInfo{}, nil }
func (c *mockCredential) ToMap() map[string]any                      { return map[string]any{} }

// mockProvider 实现 providers.Provider 接口
type mockProvider struct {
	refreshErr       error
	usageStats       []*usagerule.UsageStats
	usageStatsErr    error
	usageRules       []*usagerule.UsageRule
	usageRulesErr    error
	models           []string
	modelsErr        error
	listModels       []string
	listModelsErr    error
	generateErr      error
	generateResponse *providers.Response
}

func (p *mockProvider) Type() string { return "mock" }
func (p *mockProvider) Name() string { return "mock" }
func (p *mockProvider) GenerateContent(_ context.Context, _ *providers.Request) (*providers.Response, error) {
	return p.generateResponse, p.generateErr
}
func (p *mockProvider) GenerateContentStream(_ context.Context, _ *providers.Request) (queue.Consumer[*providers.Response], error) {
	return nil, nil
}
func (p *mockProvider) Refresh(_ context.Context, _ credentials.Credential) error {
	return p.refreshErr
}
func (p *mockProvider) Models(_ context.Context) ([]string, error) {
	return p.models, p.modelsErr
}
func (p *mockProvider) ListModels(_ context.Context, _ credentials.Credential) ([]string, error) {
	return p.listModels, p.listModelsErr
}
func (p *mockProvider) DefaultUsageRules(_ context.Context) ([]*usagerule.UsageRule, error) {
	return nil, nil
}
func (p *mockProvider) GetUsageRules(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageRule, error) {
	return p.usageRules, p.usageRulesErr
}
func (p *mockProvider) GetUsageStats(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageStats, error) {
	return p.usageStats, p.usageStatsErr
}

// newTestTarget 创建一个带 credential 和 provider 的测试目标
func newTestTarget(cred credentials.Credential, prov providers.Provider) health.CheckTarget {
	acct := &account.Account{
		ID:           "acc-test",
		ProviderType: "mock",
		ProviderName: "default",
		Credential:   cred,
	}
	return health.NewCheckTarget(acct, prov)
}

// ============================================================
// CredentialValidityCheck 测试
// ============================================================

func TestCredentialValidityCheck_Valid(t *testing.T) {
	cred := &mockCredential{validateErr: nil}
	target := newTestTarget(cred, nil)

	c := &CredentialValidityCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed, got %v", result.Status)
	}
	if result.CheckName != CredentialValidityCheckName {
		t.Fatalf("expected check name %q, got %q", CredentialValidityCheckName, result.CheckName)
	}
	if result.SuggestedStatus != nil {
		t.Fatal("expected no SuggestedStatus for valid credential")
	}
}

func TestCredentialValidityCheck_Invalid(t *testing.T) {
	cred := &mockCredential{validateErr: errors.New("token malformed")}
	target := newTestTarget(cred, nil)

	c := &CredentialValidityCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckFailed {
		t.Fatalf("expected CheckFailed, got %v", result.Status)
	}
	if result.SuggestedStatus == nil {
		t.Fatal("expected SuggestedStatus to be set on invalid credential")
	}
	if *result.SuggestedStatus != account.StatusInvalidated {
		t.Fatalf("expected SuggestedStatus=Invalidated, got %v", *result.SuggestedStatus)
	}
}

func TestCredentialValidityCheck_Name(t *testing.T) {
	c := &CredentialValidityCheck{}
	if c.Name() != CredentialValidityCheckName {
		t.Fatalf("expected name %q, got %q", CredentialValidityCheckName, c.Name())
	}
}

func TestCredentialValidityCheck_Severity(t *testing.T) {
	c := &CredentialValidityCheck{}
	if c.Severity() != health.SeverityCritical {
		t.Fatalf("expected SeverityCritical, got %v", c.Severity())
	}
}

func TestCredentialValidityCheck_DependsOn(t *testing.T) {
	c := &CredentialValidityCheck{}
	if c.DependsOn() != nil {
		t.Fatal("expected no dependencies")
	}
}

// ============================================================
// CredentialRefreshCheck (refresh.go) 测试
// ============================================================

// expiresAtFuture 凭证有效期在未来
func credValidFuture() *mockCredential {
	t := time.Now().Add(24 * time.Hour)
	return &mockCredential{expired: false, expiresAt: &t}
}

// credExpired 凭证已过期
func credExpired() *mockCredential {
	t := time.Now().Add(-time.Hour)
	return &mockCredential{expired: true, expiresAt: &t}
}

func TestRefreshCheck_Name(t *testing.T) {
	c := &CredentialRefreshCheck{}
	if c.Name() != CredentialRefreshCheckName {
		t.Fatalf("expected %q, got %q", CredentialRefreshCheckName, c.Name())
	}
}

func TestRefreshCheck_Severity(t *testing.T) {
	c := &CredentialRefreshCheck{}
	if c.Severity() != health.SeverityCritical {
		t.Fatalf("expected SeverityCritical, got %v", c.Severity())
	}
}

func TestRefreshCheck_DependsOn(t *testing.T) {
	c := &CredentialRefreshCheck{}
	deps := c.DependsOn()
	if len(deps) != 1 || deps[0] != CredentialValidityCheckName {
		t.Fatalf("expected depends on [%s], got %v", CredentialValidityCheckName, deps)
	}
}

// 凭证有效、未到刷新阈值，直接 Pass
func TestRefreshCheck_ValidCredential_NoRefreshNeeded(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{}
	target := newTestTarget(cred, prov)

	c := &CredentialRefreshCheck{RefreshThreshold: 0}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed when credential is valid, got %v", result.Status)
	}
}

// 凭证过期，刷新成功
func TestRefreshCheck_ExpiredCredential_RefreshSuccess(t *testing.T) {
	cred := credExpired()
	prov := &mockProvider{refreshErr: nil}
	target := newTestTarget(cred, prov)

	c := &CredentialRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed after successful refresh, got %v: %s", result.Status, result.Message)
	}
	if result.SuggestedStatus == nil || *result.SuggestedStatus != account.StatusAvailable {
		t.Fatalf("expected SuggestedStatus=Available after refresh, got %v", result.SuggestedStatus)
	}
}

// 凭证过期，刷新失败（普通错误）
func TestRefreshCheck_ExpiredCredential_RefreshFails_WithError(t *testing.T) {
	cred := credExpired()
	prov := &mockProvider{refreshErr: errors.New("network timeout")}
	target := newTestTarget(cred, prov)

	c := &CredentialRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckError {
		t.Fatalf("expected CheckError for network error, got %v", result.Status)
	}
	// 凭证已过期且刷新失败 -> SuggestedStatus=Expired
	if result.SuggestedStatus == nil || *result.SuggestedStatus != account.StatusExpired {
		t.Fatalf("expected SuggestedStatus=Expired when cred expired and refresh fails, got %v", result.SuggestedStatus)
	}
}

// 凭证过期，刷新失败（invalid_grant）
func TestRefreshCheck_ExpiredCredential_RefreshFails_InvalidGrant(t *testing.T) {
	cred := credExpired()
	prov := &mockProvider{refreshErr: providers.ErrInvalidGrant}
	target := newTestTarget(cred, prov)

	c := &CredentialRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckFailed {
		t.Fatalf("expected CheckFailed for invalid_grant, got %v", result.Status)
	}
	if result.SuggestedStatus == nil || *result.SuggestedStatus != account.StatusInvalidated {
		t.Fatalf("expected SuggestedStatus=Invalidated for invalid_grant, got %v", result.SuggestedStatus)
	}
}

// 凭证过期，刷新失败（Forbidden 403）
func TestRefreshCheck_ExpiredCredential_RefreshFails_Forbidden(t *testing.T) {
	cred := credExpired()
	prov := &mockProvider{
		refreshErr: &providers.HTTPError{
			ErrorType: providers.ErrorTypeForbidden,
			Message:   "account banned",
		},
	}
	target := newTestTarget(cred, prov)

	c := &CredentialRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckFailed {
		t.Fatalf("expected CheckFailed for Forbidden, got %v", result.Status)
	}
	if result.SuggestedStatus == nil || *result.SuggestedStatus != account.StatusBanned {
		t.Fatalf("expected SuggestedStatus=Banned for Forbidden, got %v", result.SuggestedStatus)
	}
}

// 凭证过期，刷新失败（RateLimit 429）
func TestRefreshCheck_ExpiredCredential_RefreshFails_RateLimit(t *testing.T) {
	cred := credExpired()
	until := time.Now().Add(time.Hour)
	prov := &mockProvider{
		refreshErr: &providers.HTTPError{
			ErrorType:     providers.ErrorTypeRateLimit,
			CooldownUntil: &until,
		},
	}
	target := newTestTarget(cred, prov)

	c := &CredentialRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckFailed {
		t.Fatalf("expected CheckFailed for RateLimit, got %v", result.Status)
	}
	if result.SuggestedStatus == nil || *result.SuggestedStatus != account.StatusCoolingDown {
		t.Fatalf("expected SuggestedStatus=CoolingDown for RateLimit, got %v", result.SuggestedStatus)
	}
}

// ============================================================
// UsageQuotaCheck (usage.go) 测试
// ============================================================

func TestUsageCheck_Name(t *testing.T) {
	c := &UsageQuotaCheck{}
	if c.Name() != UsageQuotaCheckName {
		t.Fatalf("expected %q, got %q", UsageQuotaCheckName, c.Name())
	}
}

func TestUsageCheck_Severity(t *testing.T) {
	c := &UsageQuotaCheck{}
	if c.Severity() != health.SeverityCritical {
		t.Fatalf("expected SeverityCritical, got %v", c.Severity())
	}
}

func TestUsageCheck_DependsOn(t *testing.T) {
	c := &UsageQuotaCheck{}
	deps := c.DependsOn()
	if len(deps) != 1 || deps[0] != CredentialRefreshCheckName {
		t.Fatalf("expected depends on [%s], got %v", CredentialRefreshCheckName, deps)
	}
}

// GetUsageStats 返回错误时 -> CheckError
func TestUsageCheck_GetUsageStatsFails(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{usageStatsErr: errors.New("api error")}
	target := newTestTarget(cred, prov)

	c := &UsageQuotaCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckError {
		t.Fatalf("expected CheckError when GetUsageStats fails, got %v", result.Status)
	}
}

// 用量充足（无触发规则）-> CheckPassed
func TestUsageCheck_QuotaAvailable(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{
		usageStats: []*usagerule.UsageStats{
			{
				Rule: &usagerule.UsageRule{
					SourceType: usagerule.SourceTypeRequest,
					Total:      100,
				},
				Used:   10,
				Remain: 90,
			},
		},
	}
	target := newTestTarget(cred, prov)

	c := &UsageQuotaCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed for available quota, got %v: %s", result.Status, result.Message)
	}
}

// 用量耗尽（IsTriggered = true）-> CheckFailed + CoolingDown
func TestUsageCheck_QuotaExhausted(t *testing.T) {
	cred := credValidFuture()
	endTime := time.Now().Add(time.Hour)
	prov := &mockProvider{
		usageStats: []*usagerule.UsageStats{
			{
				Rule: &usagerule.UsageRule{
					SourceType: usagerule.SourceTypeRequest,
					Total:      100,
				},
				Used:    100,
				Remain:  0,
				EndTime: &endTime,
			},
		},
	}
	target := newTestTarget(cred, prov)

	c := &UsageQuotaCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckFailed {
		t.Fatalf("expected CheckFailed for exhausted quota, got %v", result.Status)
	}
	if result.SuggestedStatus == nil || *result.SuggestedStatus != account.StatusCoolingDown {
		t.Fatalf("expected SuggestedStatus=CoolingDown, got %v", result.SuggestedStatus)
	}
}

// 用量接近耗尽（余量比例低于 Warning 阈值）-> CheckWarning
func TestUsageCheck_QuotaNearExhausted_Warning(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{
		usageStats: []*usagerule.UsageStats{
			{
				Rule: &usagerule.UsageRule{
					SourceType: usagerule.SourceTypeRequest,
					Total:      100,
				},
				Used:   99,
				Remain: 1, // 1% 剩余，低于默认 1% 阈值边界 -> Warning
			},
		},
	}
	target := newTestTarget(cred, prov)

	c := &UsageQuotaCheck{WarningThreshold: 0.05} // 设置 5% 阈值，1% < 5% -> Warning
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckWarning {
		t.Fatalf("expected CheckWarning for near-exhausted quota, got %v: %s", result.Status, result.Message)
	}
}

// 空 stats 列表 -> CheckPassed
func TestUsageCheck_EmptyStats(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{usageStats: []*usagerule.UsageStats{}}
	target := newTestTarget(cred, prov)

	c := &UsageQuotaCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed for empty stats, got %v", result.Status)
	}
}

// ============================================================
// UsageRulesRefreshCheck (usage_rules_refresh.go) 测试
// ============================================================

func TestUsageRulesRefreshCheck_Name(t *testing.T) {
	c := &UsageRulesRefreshCheck{}
	if c.Name() != UsageRulesRefreshCheckName {
		t.Fatalf("expected %q, got %q", UsageRulesRefreshCheckName, c.Name())
	}
}

func TestUsageRulesRefreshCheck_Severity(t *testing.T) {
	c := &UsageRulesRefreshCheck{}
	if c.Severity() != health.SeverityInfo {
		t.Fatalf("expected SeverityInfo, got %v", c.Severity())
	}
}

func TestUsageRulesRefreshCheck_DependsOn(t *testing.T) {
	c := &UsageRulesRefreshCheck{}
	deps := c.DependsOn()
	if len(deps) != 1 || deps[0] != CredentialRefreshCheckName {
		t.Fatalf("expected depends on [%s], got %v", CredentialRefreshCheckName, deps)
	}
}

// GetUsageRules 成功，有规则 -> CheckPassed + Data 中包含规则
func TestUsageRulesRefreshCheck_Success(t *testing.T) {
	cred := credValidFuture()
	rules := []*usagerule.UsageRule{
		{SourceType: usagerule.SourceTypeRequest, Total: 1000},
	}
	prov := &mockProvider{usageRules: rules}
	target := newTestTarget(cred, prov)

	c := &UsageRulesRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed, got %v: %s", result.Status, result.Message)
	}
	if result.Data == nil {
		t.Fatal("expected Data to be set")
	}
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("expected Data to be map[string]any")
	}
	if _, exists := dataMap[usageRulesDataKey]; !exists {
		t.Fatalf("expected Data to contain key %q", usageRulesDataKey)
	}
}

// GetUsageRules 返回空列表 -> CheckPassed (unlimited)
func TestUsageRulesRefreshCheck_EmptyRules(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{usageRules: []*usagerule.UsageRule{}}
	target := newTestTarget(cred, prov)

	c := &UsageRulesRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed for empty rules (unlimited), got %v", result.Status)
	}
}

// GetUsageRules 失败 -> CheckError
func TestUsageRulesRefreshCheck_Error(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{usageRulesErr: errors.New("api unavailable")}
	target := newTestTarget(cred, prov)

	c := &UsageRulesRefreshCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckError {
		t.Fatalf("expected CheckError when GetUsageRules fails, got %v", result.Status)
	}
}

// ============================================================
// ModelDiscoveryCheck (model_discovery.go) 测试
// ============================================================

func TestModelDiscoveryCheck_Name(t *testing.T) {
	c := &ModelDiscoveryCheck{}
	if c.Name() != ModelDiscoveryCheckName {
		t.Fatalf("expected %q, got %q", ModelDiscoveryCheckName, c.Name())
	}
}

func TestModelDiscoveryCheck_Severity(t *testing.T) {
	c := &ModelDiscoveryCheck{}
	if c.Severity() != health.SeverityInfo {
		t.Fatalf("expected SeverityInfo, got %v", c.Severity())
	}
}

func TestModelDiscoveryCheck_DependsOn(t *testing.T) {
	c := &ModelDiscoveryCheck{}
	deps := c.DependsOn()
	if len(deps) != 1 || deps[0] != CredentialValidityCheckName {
		t.Fatalf("expected depends on [%s], got %v", CredentialValidityCheckName, deps)
	}
}

// ListModels 返回模型列表 -> CheckPassed
func TestModelDiscoveryCheck_Success(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{listModels: []string{"gpt-4", "gpt-3.5"}}
	target := newTestTarget(cred, prov)

	c := &ModelDiscoveryCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckPassed {
		t.Fatalf("expected CheckPassed, got %v: %s", result.Status, result.Message)
	}
}

// ListModels 返回空列表 -> CheckWarning
func TestModelDiscoveryCheck_EmptyModels(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{listModels: []string{}}
	target := newTestTarget(cred, prov)

	c := &ModelDiscoveryCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckWarning {
		t.Fatalf("expected CheckWarning for empty models, got %v", result.Status)
	}
}

// ListModels 失败 -> CheckError
func TestModelDiscoveryCheck_Error(t *testing.T) {
	cred := credValidFuture()
	prov := &mockProvider{listModelsErr: errors.New("not supported")}
	target := newTestTarget(cred, prov)

	c := &ModelDiscoveryCheck{}
	result := c.Check(context.Background(), target)

	if result.Status != health.CheckError {
		t.Fatalf("expected CheckError, got %v", result.Status)
	}
}
