package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ===================== 顶层配置 =====================

// Config 是 CLI 的顶层配置。
type Config struct {
	// Storage 后端存储配置（通用: Driver + DSN）。
	Storage StorageConfig `yaml:"storage"`
	// Balancer 负载均衡器配置（驱动所有子组件的初始化）。
	Balancer BalancerConfig `yaml:"balancer"`
	// Log 日志配置。
	Log LogConfig `yaml:"log"`
}

// StorageConfig 通用后端存储配置。
type StorageConfig struct {
	// Driver 存储驱动类型: "memory" | "mysql" | "redis" | "sqlite"。
	// memory 不需要 DSN；mysql/redis/sqlite 必须提供 DSN。
	Driver string `yaml:"driver"`
	// DSN 数据源连接字符串。
	// MySQL 格式: user:password@tcp(host:port)/dbname?parseTime=true
	// Redis 格式: redis://[:password@]host:port[/db]
	// SQLite 格式: /path/to/db.sqlite3 或 file:test.db?cache=shared&mode=memory
	// Memory 时此字段忽略。
	DSN string `yaml:"dsn"`
}

// LogConfig 日志配置。
type LogConfig struct {
	// Level 日志级别: "debug" | "info" | "warn" | "error"，默认 "info"。
	Level string `yaml:"level"`
	// Dir 日志输出目录。为空时输出到 stdout。
	Dir string `yaml:"dir"`
}

// ===================== Balancer 配置 =====================

// BalancerConfig 负载均衡器完整配置。
type BalancerConfig struct {
	// Selector 账号级选择策略（Decoder 模式：策略名作为 key）。
	//   round_robin: {}
	//   affinity:
	//     fallback: weighted
	Selector *TypedConfig[SelectorStrategy] `yaml:"selector"`

	// GroupSelector 供应商级选择策略（Decoder 模式：策略名作为 key）。
	//   group_priority: {}
	//   group_affinity:
	//     fallback: group_round_robin
	GroupSelector *TypedConfig[GroupSelectorStrategy] `yaml:"group_selector"`

	// Occupancy 并发占用控制策略（Decoder 模式：策略名作为 key）。
	//   unlimited: {}
	//   fixed:
	//     default_limit: 10
	//   adaptive:
	//     factor: 0.8
	Occupancy *TypedConfig[OccupancyStrategy] `yaml:"occupancy"`

	// CircuitBreaker 熔断器配置（单实现，直接结构体，nil 不启用）。
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker"`

	// Cooldown 冷却管理器配置（单实现，直接结构体，nil 不启用）。
	Cooldown *CooldownConfig `yaml:"cooldown"`

	// UsageTracker 用量追踪器配置（单实现，直接结构体，nil 不启用）。
	UsageTracker *UsageTrackerConfig `yaml:"usage_tracker"`

	// DefaultMaxRetries 默认最大重试次数（默认 0，不重试）。
	DefaultMaxRetries int `yaml:"default_max_retries"`

	// DefaultEnableFailover 默认是否启用故障转移（默认 false）。
	DefaultEnableFailover bool `yaml:"default_enable_failover"`
}

// ===================== TypedConfig 通用延迟解码容器 =====================

// TypedConfig 通过「策略名作为 YAML key」确定类型，value 为延迟解码的原始配置。
// T 约束为策略类型的枚举（string 别名）。
//
// YAML 示例:
//
//	# 无参数策略
//	round_robin: {}
//
//	# 带参数策略
//	fixed:
//	  default_limit: 10
//
// 反序列化后：Strategy = "fixed", RawConfig 持有 {default_limit: 10} 的 yaml.Node。
// 同一层级下应只出现一个 key，多个 key 时报错。
type TypedConfig[T ~string] struct {
	// Strategy 从 YAML key 解析得到的策略类型。
	Strategy T
	// RawConfig 策略专属配置的原始 YAML 节点，延迟解码。
	RawConfig yaml.Node
}

// UnmarshalYAML 自定义反序列化：将 map 的唯一 key 作为 Strategy，value 作为 RawConfig。
func (tc *TypedConfig[T]) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("TypedConfig: expected mapping, got kind %v", node.Kind)
	}
	if len(node.Content) < 2 {
		return fmt.Errorf("TypedConfig: expected exactly one key, got empty mapping")
	}
	if len(node.Content) > 2 {
		return fmt.Errorf("TypedConfig: expected exactly one strategy key, got %d keys", len(node.Content)/2)
	}

	// Content[0] = key 节点, Content[1] = value 节点
	tc.Strategy = T(node.Content[0].Value)
	tc.RawConfig = *node.Content[1]
	return nil
}

// Decode 将 RawConfig 解码到目标结构体。
func (tc *TypedConfig[T]) Decode(target any) error {
	if tc.RawConfig.Kind == 0 {
		return nil // 空节点（如 `round_robin: {}`），保持 target 零值
	}
	return tc.RawConfig.Decode(target)
}

// IsZero 判断是否为零值（未配置）。
func (tc *TypedConfig[T]) IsZero() bool {
	return tc.Strategy == ""
}

// ===================== 策略类型枚举 =====================

// SelectorStrategy 账号级选择策略类型。
type SelectorStrategy string

const (
	SelectorRoundRobin SelectorStrategy = "round_robin"
	SelectorPriority   SelectorStrategy = "priority"
	SelectorWeighted   SelectorStrategy = "weighted"
	SelectorLeastUsed  SelectorStrategy = "least_used"
	SelectorAffinity   SelectorStrategy = "affinity"
)

// GroupSelectorStrategy 供应商级选择策略类型。
type GroupSelectorStrategy string

const (
	GroupSelectorPriority      GroupSelectorStrategy = "group_priority"
	GroupSelectorRoundRobin    GroupSelectorStrategy = "group_round_robin"
	GroupSelectorWeighted      GroupSelectorStrategy = "group_weighted"
	GroupSelectorMostAvailable GroupSelectorStrategy = "group_most_available"
	GroupSelectorAffinity      GroupSelectorStrategy = "group_affinity"
)

// OccupancyStrategy 占用控制策略类型。
type OccupancyStrategy string

const (
	OccupancyUnlimited OccupancyStrategy = "unlimited"
	OccupancyFixed     OccupancyStrategy = "fixed"
	OccupancyAdaptive  OccupancyStrategy = "adaptive"
)

// ===================== 各策略专属配置结构体 =====================

// AffinityConfig 是 account affinity 策略的专属配置。
type AffinityConfig struct {
	// Fallback 亲和未命中时的退化策略名称，默认 "round_robin"。
	Fallback SelectorStrategy `yaml:"fallback"`
}

// GroupAffinityConfig 是 group_affinity 策略的专属配置。
type GroupAffinityConfig struct {
	// Fallback 亲和未命中时的退化策略名称，默认 "group_priority"。
	Fallback GroupSelectorStrategy `yaml:"fallback"`
}

// FixedLimitConfig 是 fixed 占用控制策略的专属配置。
type FixedLimitConfig struct {
	// DefaultLimit 全局默认并发上限（必填）。
	DefaultLimit int64 `yaml:"default_limit"`
}

// AdaptiveLimitConfig 是 adaptive 占用控制策略的专属配置。
type AdaptiveLimitConfig struct {
	// Factor 调控因子（默认 1.0）。
	Factor float64 `yaml:"factor"`
	// MinLimit 最小并发上限（默认 1）。
	MinLimit int64 `yaml:"min_limit"`
	// MaxLimit 最大并发上限（默认 0 不限制）。
	MaxLimit int64 `yaml:"max_limit"`
	// FallbackLimit 回退并发上限（默认 1）。
	FallbackLimit int64 `yaml:"fallback_limit"`
}

// ===================== 单实现组件配置 =====================

// CircuitBreakerConfig 熔断器配置。
type CircuitBreakerConfig struct {
	// DefaultThreshold 默认连续失败阈值（默认 5）。
	DefaultThreshold int `yaml:"default_threshold"`
	// DefaultTimeout 默认熔断恢复时间（默认 "60s"）。
	DefaultTimeout time.Duration `yaml:"default_timeout"`
	// ThresholdRatio 动态阈值比例（默认 0.5）。
	ThresholdRatio float64 `yaml:"threshold_ratio"`
	// MinThreshold 最小阈值（默认 3）。
	MinThreshold int `yaml:"min_threshold"`
}

// CooldownConfig 冷却管理器配置。
type CooldownConfig struct {
	// DefaultDuration 默认冷却时长（默认 "30s"）。
	DefaultDuration time.Duration `yaml:"default_duration"`
}

// UsageTrackerConfig 用量追踪器配置。
type UsageTrackerConfig struct {
	// SafetyRatio 安全阈值比例（默认 0.95）。
	SafetyRatio float64 `yaml:"safety_ratio"`
}

// ===================== 配置加载 =====================

// Load 从 YAML 文件加载配置。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	cfg.applyDefaults()
	return cfg, nil
}

// Validate 校验配置合法性。
func (c *Config) Validate() error {
	// 校验 Storage
	if c.Storage.Driver == "" {
		return fmt.Errorf("storage.driver is required")
	}
	switch c.Storage.Driver {
	case "memory":
		// memory 不需要 DSN
	case "mysql", "redis", "sqlite":
		if c.Storage.DSN == "" {
			return fmt.Errorf("storage.dsn is required when driver is %q", c.Storage.Driver)
		}
	default:
		return fmt.Errorf("unsupported storage driver: %q", c.Storage.Driver)
	}

	// 校验 Log Level
	switch c.Log.Level {
	case "", "debug", "info", "warn", "error":
		// 合法值
	default:
		return fmt.Errorf("unsupported log level: %q, valid values: debug, info, warn, error", c.Log.Level)
	}

	// 校验 Occupancy
	if c.Balancer.Occupancy != nil {
		switch c.Balancer.Occupancy.Strategy {
		case OccupancyFixed:
			var cfg FixedLimitConfig
			if err := c.Balancer.Occupancy.Decode(&cfg); err != nil {
				return fmt.Errorf("occupancy.fixed: %w", err)
			}
			if cfg.DefaultLimit <= 0 {
				return fmt.Errorf("occupancy.fixed.default_limit must be > 0")
			}
		case OccupancyAdaptive:
			if c.Balancer.UsageTracker == nil {
				return fmt.Errorf("occupancy.adaptive requires usage_tracker to be configured")
			}
		}
	}

	return nil
}

// applyDefaults 为未配置的字段设置默认值。
func (c *Config) applyDefaults() {
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
}
