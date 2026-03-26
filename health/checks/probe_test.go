package checks

import (
	"context"
	"sync"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-client/providers"
)

// --- Fix-7: ProbeCheck.buildProbeRequest 并发安全 ---

// TestProbeCheck_BuildProbeRequest_ConcurrentSafe_WithExplicitModel 并发调用不产生 data race
// 当 c.Model != "" 时，resolveModel 直接返回，不调用 client.Models，因此 client 可为 nil。
func TestProbeCheck_BuildProbeRequest_ConcurrentSafe_WithExplicitModel(t *testing.T) {
	// 共享的 ProbeRequest，Model 为空（将由 c.Model 填充到副本中）
	sharedReq := &providers.Request{
		Model: "",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "shared probe msg"},
		},
	}

	c := &ProbeCheck{
		ProbeRequest: sharedReq,
		Model:        "gpt-4", // 设置 Model，resolveModel 不会调用 client
	}

	acct := &account.Account{ID: "acc-1"}
	target := health.NewCheckTarget(acct, nil) // nil client，因为 c.Model != "" 不会调用它

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req := c.buildProbeRequest(context.Background(), target)
			if req == nil {
				t.Error("expected non-nil request")
				return
			}
			if req.Model != "gpt-4" {
				t.Errorf("expected model gpt-4, got %s", req.Model)
			}
		}()
	}

	wg.Wait()
}

// TestProbeCheck_BuildProbeRequest_ReturnsCopy 验证返回副本而非共享指针
func TestProbeCheck_BuildProbeRequest_ReturnsCopy(t *testing.T) {
	sharedReq := &providers.Request{
		Model: "",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "test probe"},
		},
	}

	c := &ProbeCheck{
		ProbeRequest: sharedReq,
		Model:        "gpt-4",
	}

	acct := &account.Account{ID: "acc-1"}
	target := health.NewCheckTarget(acct, nil)

	req1 := c.buildProbeRequest(context.Background(), target)
	req2 := c.buildProbeRequest(context.Background(), target)

	if req1 == sharedReq {
		t.Fatal("buildProbeRequest must return a copy, not the original sharedReq pointer")
	}
	if req2 == sharedReq {
		t.Fatal("buildProbeRequest must return a copy, not the original sharedReq pointer")
	}
	if req1 == req2 {
		t.Fatal("expected two separate copies on each call")
	}
}

// TestProbeCheck_BuildProbeRequest_SharedReqNotMutated 并发调用时共享 ProbeRequest 不被修改
func TestProbeCheck_BuildProbeRequest_SharedReqNotMutated(t *testing.T) {
	sharedReq := &providers.Request{
		Model: "", // 空 Model，触发副本中的 model 继承
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "original"},
		},
	}

	c := &ProbeCheck{
		ProbeRequest: sharedReq,
		Model:        "gpt-4",
	}

	acct := &account.Account{ID: "acc-1"}
	target := health.NewCheckTarget(acct, nil)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req := c.buildProbeRequest(context.Background(), target)
			_ = req
		}()
	}

	wg.Wait()

	// 并发调用后，共享 ProbeRequest.Model 不应被修改
	if sharedReq.Model != "" {
		t.Fatalf("shared ProbeRequest.Model was mutated to %q; expected empty string", sharedReq.Model)
	}
}

// TestProbeCheck_BuildProbeRequest_NilProbeRequest_DefaultRequest nil ProbeRequest 时返回默认请求
func TestProbeCheck_BuildProbeRequest_NilProbeRequest_DefaultRequest(t *testing.T) {
	c := &ProbeCheck{
		ProbeRequest: nil,
		Model:        "gpt-4",
	}

	acct := &account.Account{ID: "acc-1"}
	target := health.NewCheckTarget(acct, nil)

	req := c.buildProbeRequest(context.Background(), target)
	if req == nil {
		t.Fatal("expected non-nil default request when ProbeRequest is nil")
	}
	if req.Model != "gpt-4" {
		t.Fatalf("expected model gpt-4, got %s", req.Model)
	}
	if len(req.Messages) == 0 {
		t.Fatal("expected default probe message to be set")
	}
}

// TestProbeCheck_BuildProbeRequest_ModelInheritedFromConfig ProbeRequest.Model 为空时从 c.Model 继承
func TestProbeCheck_BuildProbeRequest_ModelInheritedFromConfig(t *testing.T) {
	sharedReq := &providers.Request{
		Model: "", // 空，将从 c.Model 填充
	}

	c := &ProbeCheck{
		ProbeRequest: sharedReq,
		Model:        "claude-3",
	}

	acct := &account.Account{ID: "acc-1"}
	target := health.NewCheckTarget(acct, nil)

	req := c.buildProbeRequest(context.Background(), target)
	if req.Model != "claude-3" {
		t.Fatalf("expected model claude-3 (inherited), got %s", req.Model)
	}

	// 原始共享对象 Model 不应被修改
	if sharedReq.Model != "" {
		t.Fatalf("original sharedReq.Model should stay empty, got %s", sharedReq.Model)
	}
}

// TestProbeCheck_BuildProbeRequest_ExistingModelNotOverridden ProbeRequest.Model 非空时不覆盖
func TestProbeCheck_BuildProbeRequest_ExistingModelNotOverridden(t *testing.T) {
	sharedReq := &providers.Request{
		Model: "existing-model",
	}

	c := &ProbeCheck{
		ProbeRequest: sharedReq,
		Model:        "other-model", // 不应覆盖 ProbeRequest 已有的 model
	}

	acct := &account.Account{ID: "acc-1"}
	target := health.NewCheckTarget(acct, nil)

	req := c.buildProbeRequest(context.Background(), target)
	// 当 ProbeRequest.Model 非空时，不覆盖
	if req.Model != "existing-model" {
		t.Fatalf("expected model existing-model (from ProbeRequest), got %s", req.Model)
	}
}

// --- Fix-7 直接暴露 buildProbeRequest 行为的额外测试 ---

// TestProbeCheck_Name 验证检查名称
func TestProbeCheck_Name(t *testing.T) {
	c := &ProbeCheck{}
	if c.Name() != ProbeCheckName {
		t.Fatalf("expected %q, got %q", ProbeCheckName, c.Name())
	}
}

// TestProbeCheck_Severity 验证严重等级
func TestProbeCheck_Severity(t *testing.T) {
	c := &ProbeCheck{}
	if c.Severity() != health.SeverityWarning {
		t.Fatalf("expected SeverityWarning, got %v", c.Severity())
	}
}

// TestProbeCheck_DependsOn 验证依赖关系
func TestProbeCheck_DependsOn(t *testing.T) {
	c := &ProbeCheck{}
	deps := c.DependsOn()
	if len(deps) != 1 || deps[0] != CredentialRefreshCheckName {
		t.Fatalf("expected depends on [%s], got %v", CredentialRefreshCheckName, deps)
	}
}
