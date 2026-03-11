package health

import "context"

// leaderKeyHealthChecker 是健康检查后台任务的 leader election key。
const leaderKeyHealthChecker = "lumin-acpool:health-checker"

// LeaderElector 用于在集群部署中选举领导者。
// 后台周期性任务（如健康检查）通过此接口判断当前实例是否应该执行。
//
// 在单机部署时，无需注入此接口，所有实例默认执行后台任务。
// 在集群部署时，业务方应注入基于分布式锁（Redis/MySQL/etcd）的实现，
// 确保同一时刻只有一个实例执行后台健康检查，避免 API 请求放大。
//
// 实现注意事项：
//   - 锁应设置合理的过期时间（TTL），防止实例宕机后锁无法释放。
//     推荐 TTL 为健康检查 tick 间隔的 2~3 倍。
//   - 如果分布式锁服务不可用，建议 IsLeader 返回 true（宁可重复执行也不要全部停止）。
//
// 使用方式：
//
//	health.NewHealthChecker(
//	    health.WithLeaderElector(myRedisLeaderElector),
//	)
type LeaderElector interface {
	// IsLeader 判断当前实例在指定 key 下是否为 leader。
	// key 用于区分不同的后台任务（如 "health-checker"、"usage-sync" 等），
	// 每个 key 独立选举，互不影响。
	//
	// 返回 true 表示当前实例应执行该任务；返回 false 表示跳过。
	// 如果判断过程出错，建议返回 true 以保证可用性（宁可重复执行也不要全部停止）。
	IsLeader(ctx context.Context, key string) bool
}
