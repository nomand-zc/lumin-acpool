package occupancy

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
)

// Controller 账号占用控制器接口。
// 控制单个账号在同一时刻的并发使用数量，防止多个请求同时选中同一个账号导致超过其限额或触发限流。
// 通过 balancer.WithOccupancyController 注入到 Balancer 中，作为 Pick/Report 流程的一个环节。
//
// 工作流程：
//   - Pick 阶段: FilterAvailable 过滤已满账号 → Selector 选取 → Acquire 占用槽位
//   - Report 阶段: ReportSuccess/ReportFailure 中自动调用 Release 释放槽位
type Controller interface {
	// FilterAvailable 从候选账号中过滤出仍有并发余量的账号。
	// 在 Selector.Select 之前调用，将已满的账号提前排除。
	// 返回有余量的账号子集（保持原始顺序）。
	FilterAvailable(ctx context.Context, accounts []*account.Account) []*account.Account

	// Acquire 尝试为指定账号获取一个占用槽位。
	// 在 Selector.Select 选中账号后调用，使用原子操作确保竞态安全。
	// 返回 false 表示该账号并发已满（竞态场景下可能发生），Balancer 会将该账号排除后重试选取。
	Acquire(ctx context.Context, acct *account.Account) bool

	// Release 释放指定账号的一个占用槽位。
	// 在 ReportSuccess/ReportFailure 中自动调用，无需调用方手动管理。
	Release(ctx context.Context, accountID string)
}
