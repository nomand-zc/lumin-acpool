package pool_test

import (
	"context"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/balancer"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/pool"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/queue"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// exampleProvider 示例用的 mock Provider。
type exampleProvider struct{}

func (p *exampleProvider) Type() string { return "example" }
func (p *exampleProvider) Name() string { return "team-demo" }
func (p *exampleProvider) GenerateContent(_ context.Context, _ credentials.Credential, _ providers.Request) (*providers.Response, error) {
	return &providers.Response{}, nil
}
func (p *exampleProvider) GenerateContentStream(_ context.Context, _ credentials.Credential, _ providers.Request) (queue.Consumer[*providers.Response], error) {
	return nil, nil
}
func (p *exampleProvider) Refresh(_ context.Context, _ credentials.Credential) error { return nil }
func (p *exampleProvider) CheckAvailability(_ context.Context, _ credentials.Credential) (credentials.CredentialStatus, error) {
	return 0, nil
}
func (p *exampleProvider) Models(_ context.Context) ([]string, error) { return nil, nil }
func (p *exampleProvider) ListModels(_ context.Context, _ credentials.Credential) ([]string, error) {
	return nil, nil
}
func (p *exampleProvider) GetUsageRules(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageRule, error) {
	return nil, nil
}
func (p *exampleProvider) GetUsageStats(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageStats, error) {
	return nil, nil
}

// Example_basicUsage 展示 Pool 的基本使用流程。
func Example_basicUsage() {
	ctx := context.Background()

	// 1. 创建 Pool（使用默认配置，所有存储自动使用内存实现）
	p, err := pool.New(
		pool.WithCooldownManager(cooldown.NewCooldownManager()),
		pool.WithDefaultMaxRetries(2),
		pool.WithDefaultFailover(true),
	)
	if err != nil {
		panic(err)
	}
	defer p.Close()

	// 2. 注册 Provider（元数据 + SDK Client）
	err = p.RegisterProvider(ctx, &account.ProviderInfo{
		ProviderType:    "example",
		ProviderName:    "team-demo",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4", "gpt-3.5"},
		Priority:        10,
		Weight:          1,
	}, &exampleProvider{})
	if err != nil {
		panic(err)
	}

	// 3. 注册 Account
	err = p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-001",
		ProviderType: "example",
		ProviderName: "team-demo",
		Status:       account.StatusAvailable,
	})
	if err != nil {
		panic(err)
	}

	// 4. 选号
	result, err := p.Pick(ctx, &balancer.PickRequest{
		Model: "gpt-4",
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("选中账号: %s\n", result.Account.ID)
	fmt.Printf("Provider: %s\n", result.ProviderKey)
	fmt.Printf("Client 可用: %v\n", result.Client != nil)

	// 5. 使用 Client 发起 API 调用（此处为 mock，直接成功）
	// resp, err := result.Client.GenerateContent(ctx, result.Account.Credential, providers.Request{...})

	// 6. 上报成功
	err = p.ReportSuccess(ctx, result.Account.ID)
	if err != nil {
		panic(err)
	}

	fmt.Println("上报成功")

	// Output:
	// 选中账号: acct-001
	// Provider: example/team-demo
	// Client 可用: true
	// 上报成功
}

// Example_multiProvider 展示多 Provider 场景下的自动路由。
func Example_multiProvider() {
	ctx := context.Background()

	p, _ := pool.New(pool.WithDefaultFailover(true))
	defer p.Close()

	// 注册两个 Provider（高优先级和低优先级）
	p.RegisterProvider(ctx, &account.ProviderInfo{
		ProviderType:    "primary",
		ProviderName:    "fast-team",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
		Priority:        10,
	}, &exampleProvider{})

	p.RegisterProvider(ctx, &account.ProviderInfo{
		ProviderType:    "backup",
		ProviderName:    "slow-team",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
		Priority:        1,
	}, &exampleProvider{})

	// 各注册一个 Account
	p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-primary",
		ProviderType: "primary",
		ProviderName: "fast-team",
		Status:       account.StatusAvailable,
	})
	p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-backup",
		ProviderType: "backup",
		ProviderName: "slow-team",
		Status:       account.StatusAvailable,
	})

	// 自动选号（优先选择高优先级 Provider）
	result, _ := p.Pick(ctx, &balancer.PickRequest{Model: "gpt-4"})
	fmt.Printf("自动选号: %s (Provider: %s)\n", result.Account.ID, result.ProviderKey)

	// 指定 Provider 选号
	key := account.BuildProviderKey("backup", "slow-team")
	result2, _ := p.Pick(ctx, &balancer.PickRequest{
		Model:       "gpt-4",
		ProviderKey: &key,
	})
	fmt.Printf("指定选号: %s (Provider: %s)\n", result2.Account.ID, result2.ProviderKey)

	// Output:
	// 自动选号: acct-primary (Provider: primary/fast-team)
	// 指定选号: acct-backup (Provider: backup/slow-team)
}

// Example_lifecycle 展示 Pool 的完整生命周期管理。
func Example_lifecycle() {
	ctx := context.Background()

	p, _ := pool.New()

	// 注册
	client := &exampleProvider{}
	p.RegisterProvider(ctx, &account.ProviderInfo{
		ProviderType:    "example",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
	}, client)

	p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "example",
		ProviderName: "team-a",
	})
	p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-2",
		ProviderType: "example",
		ProviderName: "team-a",
	})

	accounts, _ := p.ListAccounts(ctx)
	fmt.Printf("注册后账号数: %d\n", len(accounts))

	// 注销单个 Account
	p.UnregisterAccount(ctx, "acct-1")
	accounts, _ = p.ListAccounts(ctx)
	fmt.Printf("注销 acct-1 后: %d\n", len(accounts))

	// 注销整个 Provider（级联删除所有 Account）
	key := account.BuildProviderKey("example", "team-a")
	p.UnregisterProvider(ctx, key)
	accounts, _ = p.ListAccounts(ctx)
	providers, _ := p.ListProviders(ctx)
	fmt.Printf("注销 Provider 后: providers=%d, accounts=%d\n", len(providers), len(accounts))

	p.Close()
	fmt.Println("Pool 已关闭")

	// Output:
	// 注册后账号数: 2
	// 注销 acct-1 后: 1
	// 注销 Provider 后: providers=0, accounts=0
	// Pool 已关闭
}
