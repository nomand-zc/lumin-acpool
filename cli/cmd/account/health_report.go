package account

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/health/checks"
)

// healthReportJSON 用于 JSON 序列化的健康检查报告。
type healthReportJSON struct {
	AccountID   string             `json:"account_id"`
	ProviderKey string             `json:"provider_key"`
	FinalStatus string             `json:"final_status"`
	Checks      []string           `json:"checks"`
	Results     []*checkResultJSON `json:"results"`
	Summary     *reportSummary     `json:"summary"`
	Duration    string             `json:"total_duration"`
	Timestamp   string             `json:"timestamp"`
}

// checkResultJSON 单个检查项的 JSON 输出格式。
type checkResultJSON struct {
	CheckName       string `json:"check_name"`
	Status          string `json:"status"`
	Severity        string `json:"severity"`
	Message         string `json:"message"`
	SuggestedStatus string `json:"suggested_status,omitempty"`
	Duration        string `json:"duration"`
}

// reportSummary 报告摘要统计。
type reportSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Warning int `json:"warning"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Error   int `json:"error"`
}

// summaryReport 输出到文件的汇总报告。
type summaryReport struct {
	TotalAccounts  int                   `json:"total_accounts"`
	HealthyCount   int                   `json:"healthy_count"`
	UnhealthyCount int                   `json:"unhealthy_count"`
	WarningCount   int                   `json:"warning_count"`
	StatusCounts   map[string]int        `json:"status_counts"`
	Accounts       []*accountHealthBrief `json:"accounts"`
	Timestamp      string                `json:"timestamp"`
}

// accountHealthBrief 汇总中的单账号摘要。
type accountHealthBrief struct {
	AccountID   string         `json:"account_id"`
	ProviderKey string         `json:"provider_key"`
	Status      string         `json:"status"`
	FinalStatus string         `json:"final_status"`
	Summary     *reportSummary `json:"summary"`
	Duration    string         `json:"duration"`
}

// buildReportJSON 将 HealthReport 转为 JSON 输出结构。
// finalStatus 为账号健康检查后的最终状态。
func (c *healthCmd) buildReportJSON(report *health.HealthReport, specifiedChecks []string, finalStatus string) *healthReportJSON {
	checkNames := specifiedChecks
	if len(checkNames) == 0 {
		checkNames = []string{"all"}
	}

	summary := &reportSummary{Total: len(report.Results)}
	results := make([]*checkResultJSON, 0, len(report.Results))

	for _, r := range report.Results {
		if r == nil {
			continue
		}

		// 统计汇总
		switch r.Status {
		case health.CheckPassed:
			summary.Passed++
		case health.CheckWarning:
			summary.Warning++
		case health.CheckFailed:
			summary.Failed++
		case health.CheckSkipped:
			summary.Skipped++
		case health.CheckError:
			summary.Error++
		}

		rj := &checkResultJSON{
			CheckName: r.CheckName,
			Status:    r.Status.String(),
			Severity:  r.Severity.String(),
			Message:   r.Message,
			Duration:  r.Duration.Round(time.Millisecond).String(),
		}
		if r.SuggestedStatus != nil {
			rj.SuggestedStatus = r.SuggestedStatus.String()
		}
		results = append(results, rj)
	}

	return &healthReportJSON{
		AccountID:   report.AccountID,
		ProviderKey: report.ProviderKey.String(),
		FinalStatus: finalStatus,
		Checks:      checkNames,
		Results:     results,
		Summary:     summary,
		Duration:    report.TotalDuration.Round(time.Millisecond).String(),
		Timestamp:   report.Timestamp.Format(time.RFC3339),
	}
}

// printAvailableChecks 输出所有可用的检查项。
func (c *healthCmd) printAvailableChecks() error {
	fmt.Println("可用的健康检查项：")
	fmt.Println()

	// 按类别分组输出
	categories := []struct {
		title  string
		checks []string
	}{
		{
			title:  "凭证相关",
			checks: []string{checks.CredentialValidityCheckName, checks.CredentialRefreshCheckName},
		},
		{
			title:  "可用性检查",
			checks: []string{checks.ProbeCheckName, checks.RecoveryCheckName},
		},
		{
			title:  "用量相关",
			checks: []string{checks.UsageQuotaCheckName, checks.UsageRulesRefreshCheckName},
		},
		{
			title:  "信息发现",
			checks: []string{checks.ModelDiscoveryCheckName},
		},
	}

	for _, cat := range categories {
		fmt.Printf("  [%s]\n", cat.title)
		for _, name := range cat.checks {
			desc := allCheckNames[name]
			fmt.Printf("    %-25s %s\n", name, desc)
		}
		fmt.Println()
	}

	return nil
}

// printReports 在终端打印所有报告的汇总。
func (c *healthCmd) printReports(reports []*healthReportJSON) error {
	if len(reports) <= 1 {
		// 单账号场景已在 checkAccount 中输出了详细报告，只打印汇总
		if len(reports) == 1 {
			c.printSummaryLine(reports[0])
		}
		return nil
	}

	// 多账号场景，打印汇总表格
	fmt.Printf("\n══════════════════════════════════════\n")
	fmt.Printf("  健康检查汇总（共 %d 个账号）\n", len(reports))
	fmt.Printf("══════════════════════════════════════\n\n")

	var totalPassed, totalFailed, totalWarning int
	for _, r := range reports {
		c.printSummaryLine(r)
		totalPassed += r.Summary.Passed
		totalFailed += r.Summary.Failed
		totalWarning += r.Summary.Warning
	}

	fmt.Printf("\n── 总计: %d 通过, %d 警告, %d 失败 ──\n",
		totalPassed, totalWarning, totalFailed)

	return nil
}

// printSummaryLine 打印单个报告的一行摘要。
func (c *healthCmd) printSummaryLine(r *healthReportJSON) {
	icon := "✓"
	if r.Summary.Failed > 0 {
		icon = "✗"
	} else if r.Summary.Warning > 0 {
		icon = "⚠"
	}

	fmt.Printf("  %s [%s] %s — 通过:%d 警告:%d 失败:%d 跳过:%d 错误:%d (%s)\n",
		icon, r.AccountID, r.ProviderKey,
		r.Summary.Passed, r.Summary.Warning, r.Summary.Failed,
		r.Summary.Skipped, r.Summary.Error, r.Duration)
}

// writeReports 将健康检查报告输出到指定目录。
func (c *healthCmd) writeReports(reports []*healthReportJSON) error {
	// 拼接供应商子目录，避免多供应商数据互相污染
	dir := c.outputDir
	if c.providerType != "" {
		dir = filepath.Join(dir, c.providerType)
	}
	if c.providerName != "" {
		dir = filepath.Join(dir, c.providerName)
	}

	// 确保基础输出目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 按账号最终状态分子目录输出 JSON 报告文件
	for _, r := range reports {
		statusDir := filepath.Join(dir, r.FinalStatus)
		if err := os.MkdirAll(statusDir, 0755); err != nil {
			return fmt.Errorf("创建状态子目录 %s 失败: %w", r.FinalStatus, err)
		}
		fileName := fmt.Sprintf("%s.health.json", r.AccountID)
		filePath := filepath.Join(statusDir, fileName)
		if err := ioutil.SaveJSONFile(filePath, r); err != nil {
			return fmt.Errorf("导出 Account %s 健康报告失败: %w", r.AccountID, err)
		}
	}

	// 输出汇总报告
	summaryPath := filepath.Join(dir, "_summary.json")
	summary := c.buildSummaryReport(reports)
	if err := ioutil.SaveJSONFile(summaryPath, summary); err != nil {
		return fmt.Errorf("导出汇总报告失败: %w", err)
	}

	fmt.Printf("已导出 %d 个 Account 的健康检查报告到目录 %s\n", len(reports), dir)
	return nil
}

// buildSummaryReport 构建多账号汇总报告。
func (c *healthCmd) buildSummaryReport(reports []*healthReportJSON) *summaryReport {
	sr := &summaryReport{
		TotalAccounts: len(reports),
		StatusCounts:  make(map[string]int),
		Timestamp:     time.Now().Format(time.RFC3339),
	}

	for _, r := range reports {
		status := "healthy"
		if r.Summary.Failed > 0 || r.Summary.Error > 0 {
			status = "unhealthy"
			sr.UnhealthyCount++
		} else if r.Summary.Warning > 0 {
			status = "warning"
			sr.WarningCount++
		} else {
			sr.HealthyCount++
		}

		// 按账号最终状态统计
		sr.StatusCounts[r.FinalStatus]++

		sr.Accounts = append(sr.Accounts, &accountHealthBrief{
			AccountID:   r.AccountID,
			ProviderKey: r.ProviderKey,
			Status:      status,
			FinalStatus: r.FinalStatus,
			Summary:     r.Summary,
			Duration:    r.Duration,
		})
	}

	return sr
}
