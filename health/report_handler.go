package health

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// 健康检查结果 Data 中的标准 Key。
// 与 checks 包中的常量保持一致，定义在此处避免 health ↔ checks 循环导入。
const (
	// ReportDataKeyUsageStats 用量统计数据的 Key，对应 []*usagerule.UsageStats。
	ReportDataKeyUsageStats = "usage_stats"
	// ReportDataKeyCooldownUntil 冷却到期时间的 Key，对应 *time.Time。
	ReportDataKeyCooldownUntil = "cooldown_until"
	// ReportDataKeySupportedModels 支持的模型列表的 Key，对应 []string。
	ReportDataKeySupportedModels = "supported_models"
	// ReportDataKeyUsageRules 用量规则的 Key，对应 []*usagerule.UsageRule。
	ReportDataKeyUsageRules = "usage_rules"
	// ReportDataKeyCredentialRefreshed 凭证已刷新标记的 Key，对应 bool。
	// 由 CredentialRefreshCheck 在成功刷新后设置，ReportHandler 据此决定是否持久化凭证字段。
	ReportDataKeyCredentialRefreshed = "credential_refreshed"
)

// ReportHandlerDeps 是构建默认 ReportCallback 所需的依赖。
type ReportHandlerDeps struct {
	// AccountStorage 账号存储（必选，用于获取和更新账号）。
	AccountStorage storage.AccountStorage
	// ProviderStorage Provider 存储（可选，用于更新 SupportedModels）。
	ProviderStorage storage.ProviderStorage
	// UsageTracker 用量追踪器（可选，用于校准用量数据）。
	UsageTracker usagetracker.UsageTracker
	// CooldownManager 冷却管理器（可选，用于触发冷却）。
	CooldownManager cooldown.CooldownManager
}

// NewDefaultReportCallback 构建一个默认的 ReportCallback。
// 该回调消费健康检查产出的结果，执行以下操作：
//   - 处理 SuggestedStatus：将账号状态变更为建议状态
//   - 处理 UsageStats（Data[UsageStatKey]）：调用 UsageTracker.Calibrate() 校准本地计数
//   - 处理 CooldownUntil（Data[CooldownUntilKey]）：调用 CooldownManager.StartCooldown() 触发冷却
//   - 持久化：调用 AccountStorage.Update() 保存变更
func NewDefaultReportCallback(deps ReportHandlerDeps) ReportCallback {
	return func(ctx context.Context, report *HealthReport) {
		if report == nil || len(report.Results) == 0 {
			return
		}

		acct, err := deps.AccountStorage.GetAccount(ctx, report.AccountID)
		if err != nil {
			return // 获取失败静默忽略
		}

		needUpdate := false
		credentialChanged := false

		for _, result := range report.Results {
			if result == nil {
				continue
			}

			// 1. 处理 UsageStats 校准
			if deps.UsageTracker != nil && result.Data != nil {
				needUpdate = handleUsageStats(ctx, deps.UsageTracker, report.AccountID, result) || needUpdate
			}

			// 2. 处理 SupportedModels 动态发现
			if deps.ProviderStorage != nil && result.Data != nil {
				handleSupportedModels(ctx, deps.ProviderStorage, report.ProviderKey, result)
			}

			// 3. 处理 UsageRules 动态刷新
			if result.Data != nil {
				needUpdate = handleUsageRulesRefresh(ctx, deps, acct, result) || needUpdate
			}

			// 4. 检查凭证是否已刷新（由 CredentialRefreshCheck 在 Data 中显式标记）
			if result.Data != nil {
				if dataMap, ok := result.Data.(map[string]any); ok {
					if refreshed, ok := dataMap[ReportDataKeyCredentialRefreshed].(bool); ok && refreshed {
						credentialChanged = true
					}
				}
			}

			// 5. 处理 SuggestedStatus 状态变更
			if result.SuggestedStatus != nil {
				needUpdate = handleSuggestedStatus(ctx, deps, acct, result) || needUpdate
			}
		}

		// 持久化变更：仅更新实际发生变化的字段
		if needUpdate || credentialChanged {
			acct.UpdatedAt = time.Now()

			// 按实际变更构建字段掩码，避免不必要的覆盖写
			var updateFields storage.UpdateField
			if needUpdate {
				updateFields |= storage.UpdateFieldStatus | storage.UpdateFieldUsageRules
			}
			if credentialChanged {
				updateFields |= storage.UpdateFieldCredential
			}
			_ = deps.AccountStorage.UpdateAccount(ctx, acct, updateFields)
		}
	}
}

// handleUsageStats 处理检查结果中的 UsageStats 数据，校准 UsageTracker。
func handleUsageStats(ctx context.Context, tracker usagetracker.UsageTracker, accountID string, result *CheckResult) bool {
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		return false
	}

	stats, ok := dataMap[ReportDataKeyUsageStats]
	if !ok {
		return false
	}

	usageStats, ok := stats.([]*usagerule.UsageStats)
	if !ok {
		return false
	}

	_ = tracker.Calibrate(ctx, accountID, usageStats)
	return false // Calibrate 只更新 UsageTracker 内部数据，不影响 Account 持久化
}

// handleSuggestedStatus 处理检查结果中的 SuggestedStatus，变更账号状态。
func handleSuggestedStatus(ctx context.Context, deps ReportHandlerDeps, acct *account.Account, result *CheckResult) bool {
	suggested := *result.SuggestedStatus

	// 如果建议状态与当前状态相同，则无需变更
	if acct.Status == suggested {
		return false
	}

	// 特殊处理：冷却状态需要通过 CooldownManager 设置冷却时间
	if suggested == account.StatusCoolingDown {
		return handleCooldown(ctx, deps, acct, result)
	}

	// 普通状态变更
	acct.Status = suggested

	// 如果恢复为 Available，清除冷却/熔断时间
	if suggested == account.StatusAvailable {
		acct.CooldownUntil = nil
		acct.CircuitOpenUntil = nil
	}

	return true
}

// handleCooldown 处理冷却状态变更。
func handleCooldown(ctx context.Context, deps ReportHandlerDeps, acct *account.Account, result *CheckResult) bool {
	// 仅对 Available 状态的账号触发冷却，避免覆盖更严重的状态
	if acct.Status != account.StatusAvailable && acct.Status != account.StatusCircuitOpen {
		return false
	}

	// 尝试从 Data 中提取 CooldownUntil
	var cooldownUntil *time.Time
	if result.Data != nil {
		if dataMap, ok := result.Data.(map[string]any); ok {
			if until, ok := dataMap[ReportDataKeyCooldownUntil]; ok {
				if t, ok := until.(*time.Time); ok {
					cooldownUntil = t
				}
			}
		}
	}

	// 通过 CooldownManager 设置冷却（会根据规则动态计算冷却时长）
	if deps.CooldownManager != nil {
		deps.CooldownManager.StartCooldown(acct, cooldownUntil)
	} else if cooldownUntil != nil {
		// 没有 CooldownManager，直接设置冷却时间
		acct.CooldownUntil = cooldownUntil
	}

	acct.Status = account.StatusCoolingDown
	return true
}

// handleSupportedModels 处理模型发现检查结果，更新 ProviderInfo.SupportedModels。
func handleSupportedModels(ctx context.Context, providerStorage storage.ProviderStorage, providerKey account.ProviderKey, result *CheckResult) {
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		return
	}

	modelsRaw, ok := dataMap[ReportDataKeySupportedModels]
	if !ok {
		return
	}

	models, ok := modelsRaw.([]string)
	if !ok || len(models) == 0 {
		return
	}

	// 获取当前 ProviderInfo 并更新 SupportedModels
	provInfo, err := providerStorage.GetProvider(ctx, providerKey)
	if err != nil {
		return
	}

	provInfo.SupportedModels = models
	provInfo.UpdatedAt = time.Now()
	_ = providerStorage.UpdateProvider(ctx, provInfo)
}

// handleUsageRulesRefresh 处理用量规则刷新检查结果，更新 Account.UsageRules 并重新初始化 UsageTracker。
func handleUsageRulesRefresh(ctx context.Context, deps ReportHandlerDeps, acct *account.Account, result *CheckResult) bool {
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		return false
	}

	rulesRaw, ok := dataMap[ReportDataKeyUsageRules]
	if !ok {
		return false
	}

	rules, ok := rulesRaw.([]*usagerule.UsageRule)
	if !ok || len(rules) == 0 {
		return false
	}

	// 更新 Account.UsageRules
	acct.UsageRules = rules

	// 重新初始化 UsageTracker 规则
	if deps.UsageTracker != nil {
		_ = deps.UsageTracker.InitRules(ctx, acct.ID, rules)
	}

	return true
}
