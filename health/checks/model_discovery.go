package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/health"
)

const ModelDiscoveryCheckName = "model_discovery"

// modelDiscoveryDataKey 是 SupportedModels 数据在 CheckResult.Data 中的标准 Key。
const modelDiscoveryDataKey = "supported_models"

// ModelDiscoveryCheck 模型发现检查。
// 调用 lumin-client 的 ListModels() 动态获取当前凭证支持的模型列表，
// 并将结果放入 Data 中供上层回调更新 ProviderInfo.SupportedModels。
//
// 依赖凭证有效性检查（需要有效凭证才能查询模型）。
type ModelDiscoveryCheck struct{}

func (c *ModelDiscoveryCheck) Name() string {
	return ModelDiscoveryCheckName
}

func (c *ModelDiscoveryCheck) Severity() health.CheckSeverity {
	return health.SeverityInfo
}

func (c *ModelDiscoveryCheck) DependsOn() []string {
	return []string{CredentialValidityCheckName}
}

func (c *ModelDiscoveryCheck) Check(ctx context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()

	models, err := target.Client().ListModels(ctx, target.Credential())
	if err != nil {
		return &health.CheckResult{
			CheckName: ModelDiscoveryCheckName,
			Status:    health.CheckError,
			Severity:  health.SeverityInfo,
			Message:   "failed to discover models: " + err.Error(),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	if len(models) == 0 {
		return &health.CheckResult{
			CheckName: ModelDiscoveryCheckName,
			Status:    health.CheckWarning,
			Severity:  health.SeverityInfo,
			Message:   "no models discovered",
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	return &health.CheckResult{
		CheckName: ModelDiscoveryCheckName,
		Status:    health.CheckPassed,
		Severity:  health.SeverityInfo,
		Message:   fmt.Sprintf("discovered %d models", len(models)),
		Data: map[string]any{
			modelDiscoveryDataKey: models,
		},
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
}
