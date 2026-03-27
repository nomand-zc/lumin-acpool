//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// IntegrationTest_MySQLAccountCRUD 演示完整的 Account CRUD 集成测试。
//
// 本测试验证以下功能点：
// - 真实 MySQL 连接与数据库初始化
// - Account 聚合根的完整 CRUD 操作
// - 外键关系与级联删除
// - 乐观锁版本冲突检测
// - 状态变更时 Provider.AvailableAccountCount 同步
// - 数据隔离与测试清理
func IntegrationTest_MySQLAccountCRUD(t *testing.T) {
	// ========== 1. 初始化：连接到真实 MySQL 数据库 ==========

	// 从环境变量读取连接配置，默认值为本地测试数据库
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "acpool:acpool123@tcp(localhost:3306)/lumin_acpool_test?parseTime=true"
	}

	// 创建 MySQL Store（使用 DSN 连接）
	store, err := NewStore(WithDSN(dsn), WithSkipInitDB(false))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// 验证连接可用（ping）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 通过尝试查询来验证连接
	_, err = store.GetAccount(ctx, "non-existent-account-for-ping-test")
	if err != nil && err != storage.ErrNotFound {
		t.Fatalf("failed to connect to mysql: %v, make sure mysql is running with 'make env-up'", err)
	}

	// ========== 2. 数据隔离：清理测试数据 ==========
	// 集成测试必须确保测试前环境干净，测试后无污染
	t.Cleanup(func() {
		cleanupTestData(t, store)
	})

	// 清理测试数据（确保测试开始时表为空或不存在污染）
	cleanupTestData(t, store)

	// ========== 3. 准备测试数据 ==========

	// 3.1 添加 Provider 元数据（Account 依赖 Provider 外键）
	providerInfo := &account.ProviderInfo{
		ProviderType:    testProviderTypeIT,
		ProviderName:    "test-provider-it",
		Status:          account.ProviderStatusActive,
		Priority:        100,
		Weight:          1,
		Tags:            map[string]string{"env": "test"},
		SupportedModels: []string{"gpt-3.5", "gpt-4"},
	}
	if err := store.AddProvider(ctx, providerInfo); err != nil {
		t.Fatalf("failed to add provider: %v", err)
	}

	// 3.2 创建 Account 聚合根
	acct := &account.Account{
		ID:           "test-account-1",
		ProviderType: testProviderTypeIT,
		ProviderName: "test-provider-it",
		Credential:   &mockCredentialIT{token: "test-token-123"},
		Status:       account.StatusAvailable,
		Priority:     10,
		Tags:         map[string]string{"region": "us-west"},
		Metadata:     map[string]any{"quota": 1000},
		UsageRules:   []*usagerule.UsageRule{}, // 空用量规则
	}

	// ========== 4. CREATE 操作测试 ==========
	t.Run("CreateAccount", func(t *testing.T) {
		err := store.AddAccount(ctx, acct)
		if err != nil {
			t.Fatalf("failed to add account: %v", err)
		}

		// 验证账号确实被插入
		retrieved, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to retrieve account: %v", err)
		}
		if retrieved.ID != acct.ID {
			t.Errorf("ID mismatch: got %q, want %q", retrieved.ID, acct.ID)
		}
		if retrieved.Status != account.StatusAvailable {
			t.Errorf("Status mismatch: got %v, want %v", retrieved.Status, account.StatusAvailable)
		}

		// 验证 Provider.AvailableAccountCount 被递增
		provider, err := store.GetProvider(ctx, account.ProviderKey{
			Type: testProviderTypeIT,
			Name: "test-provider-it",
		})
		if err != nil {
			t.Fatalf("failed to get provider: %v", err)
		}
		if provider.AvailableAccountCount != 1 {
			t.Errorf("AvailableAccountCount mismatch: got %d, want 1", provider.AvailableAccountCount)
		}
	})

	// ========== 5. READ 操作测试 ==========
	t.Run("ReadAccount", func(t *testing.T) {
		retrieved, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to get account: %v", err)
		}

		// 验证所有字段
		if retrieved.ProviderType != acct.ProviderType {
			t.Errorf("ProviderType mismatch: got %q, want %q", retrieved.ProviderType, acct.ProviderType)
		}
		if retrieved.Priority != acct.Priority {
			t.Errorf("Priority mismatch: got %d, want %d", retrieved.Priority, acct.Priority)
		}
		if len(retrieved.Tags) != 1 || retrieved.Tags["region"] != "us-west" {
			t.Errorf("Tags mismatch: got %v", retrieved.Tags)
		}
	})

	// ========== 6. UPDATE 操作测试 ==========
	t.Run("UpdateAccount_Priority", func(t *testing.T) {
		// 获取当前版本
		current, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to get account: %v", err)
		}

		// 更新优先级
		current.Priority = 20
		err = store.UpdateAccount(ctx, current, storage.UpdateFieldPriority)
		if err != nil {
			t.Fatalf("failed to update account priority: %v", err)
		}

		// 验证更新生效
		updated, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to retrieve updated account: %v", err)
		}
		if updated.Priority != 20 {
			t.Errorf("Priority update failed: got %d, want 20", updated.Priority)
		}
		// Version 应该被递增
		if updated.Version <= current.Version {
			t.Errorf("Version not incremented: got %d, want > %d", updated.Version, current.Version)
		}
	})

	// ========== 7. UPDATE 操作测试：状态变更与 Provider 计数同步 ==========
	t.Run("UpdateAccount_StatusChange", func(t *testing.T) {
		// 获取当前账号
		current, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to get account: %v", err)
		}

		// 验证初始状态
		if current.Status != account.StatusAvailable {
			t.Fatalf("initial status should be Available, got %v", current.Status)
		}

		// 变更为 CoolingDown 状态
		current.Status = account.StatusCoolingDown
		cooldownTime := time.Now().Add(1 * time.Hour)
		current.CooldownUntil = &cooldownTime
		err = store.UpdateAccount(ctx, current, storage.UpdateFieldStatus)
		if err != nil {
			t.Fatalf("failed to update account status: %v", err)
		}

		// 验证状态更新
		updated, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to retrieve updated account: %v", err)
		}
		if updated.Status != account.StatusCoolingDown {
			t.Errorf("Status update failed: got %v, want CoolingDown", updated.Status)
		}

		// 验证 Provider.AvailableAccountCount 被递减（从 1 降至 0）
		provider, err := store.GetProvider(ctx, account.ProviderKey{
			Type: testProviderTypeIT,
			Name: "test-provider-it",
		})
		if err != nil {
			t.Fatalf("failed to get provider: %v", err)
		}
		if provider.AvailableAccountCount != 0 {
			t.Errorf("AvailableAccountCount should be decremented: got %d, want 0", provider.AvailableAccountCount)
		}

		// 恢复为 Available，验证 AvailableAccountCount 递增
		updated.Status = account.StatusAvailable
		updated.CooldownUntil = nil
		err = store.UpdateAccount(ctx, updated, storage.UpdateFieldStatus)
		if err != nil {
			t.Fatalf("failed to restore account status: %v", err)
		}

		provider, err = store.GetProvider(ctx, account.ProviderKey{
			Type: testProviderTypeIT,
			Name: "test-provider-it",
		})
		if err != nil {
			t.Fatalf("failed to get provider after restore: %v", err)
		}
		if provider.AvailableAccountCount != 1 {
			t.Errorf("AvailableAccountCount should be restored: got %d, want 1", provider.AvailableAccountCount)
		}
	})

	// ========== 8. 乐观锁版本冲突测试 ==========
	t.Run("UpdateAccount_VersionConflict", func(t *testing.T) {
		// 获取当前账号
		account1, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to get account: %v", err)
		}
		version1 := account1.Version

		// 模拟并发更新：第一次更新（成功）
		account1.Priority = 30
		err = store.UpdateAccount(ctx, account1, storage.UpdateFieldPriority)
		if err != nil {
			t.Fatalf("first update failed: %v", err)
		}

		// 模拟并发冲突：用旧版本号重新尝试更新
		account1.Priority = 40
		account1.Version = version1 // 恢复到旧版本号，模拟并发冲突
		err = store.UpdateAccount(ctx, account1, storage.UpdateFieldPriority)
		if err != storage.ErrVersionConflict {
			t.Errorf("expected ErrVersionConflict, got %v", err)
		}

		// 验证数据未被修改（优先级仍然是 30，不是 40）
		current, err := store.GetAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to get account: %v", err)
		}
		if current.Priority != 30 {
			t.Errorf("concurrent update should fail, priority should be 30, got %d", current.Priority)
		}
	})

	// ========== 9. DELETE 操作测试 ==========
	t.Run("DeleteAccount", func(t *testing.T) {
		// 添加第二个账号（便于后续测试）
		acct2 := &account.Account{
			ID:           "test-account-2",
			ProviderType: testProviderTypeIT,
			ProviderName: "test-provider-it",
			Credential:   &mockCredentialIT{token: "test-token-456"},
			Status:       account.StatusAvailable,
			Priority:     15,
		}
		if err := store.AddAccount(ctx, acct2); err != nil {
			t.Fatalf("failed to add second account: %v", err)
		}

		// 删除第一个账号
		err := store.RemoveAccount(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to remove account: %v", err)
		}

		// 验证账号已被删除
		_, err = store.GetAccount(ctx, acct.ID)
		if err != storage.ErrNotFound {
			t.Errorf("account should be deleted, but got: %v", err)
		}

		// 验证 Provider.AvailableAccountCount 被递减
		provider, err := store.GetProvider(ctx, account.ProviderKey{
			Type: testProviderTypeIT,
			Name: "test-provider-it",
		})
		if err != nil {
			t.Fatalf("failed to get provider: %v", err)
		}
		if provider.AvailableAccountCount != 1 {
			t.Errorf("AvailableAccountCount should be 1 (only acct2 left), got %d", provider.AvailableAccountCount)
		}

		// 验证级联删除：账号关联的统计数据也应被删除
		// （MySQL 配置了 ON DELETE CASCADE，stats 表中对应记录应该消失）
		stats, _ := store.GetStats(ctx, acct.ID)
		if stats != nil {
			t.Errorf("stats should be cascade-deleted, but still exists")
		}
	})

	// ========== 10. SEARCH 操作测试 ==========
	t.Run("SearchAccounts", func(t *testing.T) {
		// 搜索所有账号
		all, err := store.SearchAccounts(ctx, nil)
		if err != nil {
			t.Fatalf("failed to search accounts: %v", err)
		}
		if len(all) < 1 {
			t.Errorf("should have at least 1 account, got %d", len(all))
		}

		// 按 Provider 过滤搜索
		filter := &storage.SearchFilter{
			ProviderType: testProviderTypeIT,
			ProviderName: "test-provider-it",
		}
		filtered, err := store.SearchAccounts(ctx, filter)
		if err != nil {
			t.Fatalf("failed to search with filter: %v", err)
		}
		for _, a := range filtered {
			if a.ProviderType != testProviderTypeIT {
				t.Errorf("filter not working: got provider_type=%q", a.ProviderType)
			}
		}
	})

	// ========== 11. COUNT 操作测试 ==========
	t.Run("CountAccounts", func(t *testing.T) {
		count, err := store.CountAccounts(ctx, nil)
		if err != nil {
			t.Fatalf("failed to count accounts: %v", err)
		}
		if count < 1 {
			t.Errorf("count should be at least 1, got %d", count)
		}
	})
}

// IntegrationTest_MySQLAccountStats 演示 Stats 存储的集成测试。
func IntegrationTest_MySQLAccountStats(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "acpool:acpool123@tcp(localhost:3306)/lumin_acpool_test?parseTime=true"
	}

	store, err := NewStore(WithDSN(dsn), WithSkipInitDB(false))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	t.Cleanup(func() {
		cleanupTestData(t, store)
	})
	cleanupTestData(t, store)

	// 添加 Provider 和 Account
	provider := &account.ProviderInfo{
		ProviderType: testProviderTypeIT,
		ProviderName: "stats-test-provider",
		Status:       account.ProviderStatusActive,
	}
	if err := store.AddProvider(ctx, provider); err != nil {
		t.Fatalf("failed to add provider: %v", err)
	}

	acct := &account.Account{
		ID:           "stats-test-account",
		ProviderType: testProviderTypeIT,
		ProviderName: "stats-test-provider",
		Credential:   &mockCredentialIT{token: "test"},
		Status:       account.StatusAvailable,
	}
	if err := store.AddAccount(ctx, acct); err != nil {
		t.Fatalf("failed to add account: %v", err)
	}

	// ========== Stats 操作测试 ==========
	t.Run("IncrSuccess", func(t *testing.T) {
		// 初始统计应该不存在或为 0
		stats, err := store.GetStats(ctx, acct.ID)
		if err != nil && err != storage.ErrNotFound {
			t.Fatalf("failed to get stats: %v", err)
		}
		if stats != nil && stats.SuccessCalls != 0 {
			t.Fatalf("initial success calls should be 0, got %d", stats.SuccessCalls)
		}

		// 记录成功
		if err := store.IncrSuccess(ctx, acct.ID); err != nil {
			t.Fatalf("failed to incr success: %v", err)
		}

		// 验证成功计数递增且连续失败被重置
		stats, err = store.GetStats(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}
		if stats.SuccessCalls != 1 {
			t.Errorf("success calls should be 1, got %d", stats.SuccessCalls)
		}
		if stats.ConsecutiveFailures != 0 {
			t.Errorf("consecutive failures should be 0, got %d", stats.ConsecutiveFailures)
		}
	})

	t.Run("IncrFailure", func(t *testing.T) {
		// 记录失败
		consecutive, err := store.IncrFailure(ctx, acct.ID, "test error")
		if err != nil {
			t.Fatalf("failed to incr failure: %v", err)
		}

		// 验证连续失败计数
		if consecutive < 1 {
			t.Errorf("consecutive failures should be >= 1, got %d", consecutive)
		}

		// 再次记录失败
		consecutive, err = store.IncrFailure(ctx, acct.ID, "another error")
		if err != nil {
			t.Fatalf("failed to incr failure again: %v", err)
		}
		if consecutive < 2 {
			t.Errorf("consecutive failures should be >= 2, got %d", consecutive)
		}
	})

	t.Run("ResetConsecutiveFailures", func(t *testing.T) {
		// 重置连续失败计数
		if err := store.ResetConsecutiveFailures(ctx, acct.ID); err != nil {
			t.Fatalf("failed to reset consecutive failures: %v", err)
		}

		stats, err := store.GetStats(ctx, acct.ID)
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}
		if stats.ConsecutiveFailures != 0 {
			t.Errorf("consecutive failures should be 0 after reset, got %d", stats.ConsecutiveFailures)
		}
	})
}

// ========== 测试辅助函数 ==========

const testProviderTypeIT = "test-provider-type-it"

// mockCredentialIT 用于集成测试的凭证实现
type mockCredentialIT struct {
	token string
}

func (m *mockCredentialIT) Clone() credentials.Credential { return &mockCredentialIT{token: m.token} }
func (m *mockCredentialIT) Validate() error               { return nil }
func (m *mockCredentialIT) GetAccessToken() string        { return m.token }
func (m *mockCredentialIT) GetRefreshToken() string       { return "" }
func (m *mockCredentialIT) GetExpiresAt() *time.Time      { return nil }
func (m *mockCredentialIT) IsExpired() bool               { return false }
func (m *mockCredentialIT) GetUserInfo() (credentials.UserInfo, error) {
	return credentials.UserInfo{}, nil
}
func (m *mockCredentialIT) ToMap() map[string]any { return map[string]any{"token": m.token} }

// 注册测试凭证类型
func init() {
	credentials.Register(testProviderTypeIT, func(data []byte) credentials.Credential {
		return &mockCredentialIT{token: "test-token-it"}
	})
}

// cleanupTestData 清理测试数据，确保幂等性和数据隔离。
// 注意：此函数在测试前后都可调用，会删除所有与测试 Provider 相关的数据。
func cleanupTestData(t *testing.T, store *Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 通过 SearchAccounts + RemoveAccount 方式删除账号
	filter := &storage.SearchFilter{
		ProviderType: testProviderTypeIT,
	}
	accounts, err := store.SearchAccounts(ctx, filter)
	if err != nil && err != sql.ErrNoRows {
		t.Logf("warning: failed to search accounts for cleanup: %v", err)
		return
	}

	for _, acct := range accounts {
		if err := store.RemoveAccount(ctx, acct.ID); err != nil {
			t.Logf("warning: failed to clean account %s: %v", acct.ID, err)
		}
	}

	// 删除 Provider
	key := account.BuildProviderKey(testProviderTypeIT, "test-provider-it")
	if err := store.RemoveProvider(ctx, key); err != nil {
		if err != storage.ErrNotFound {
			t.Logf("warning: failed to clean provider: %v", err)
		}
	}

	// 删除 stats-test-provider
	key2 := account.BuildProviderKey(testProviderTypeIT, "stats-test-provider")
	if err := store.RemoveProvider(ctx, key2); err != nil {
		if err != storage.ErrNotFound {
			t.Logf("warning: failed to clean stats provider: %v", err)
		}
	}
}
