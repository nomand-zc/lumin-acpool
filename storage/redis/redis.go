package redis

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func init() {
	redisRegistry = make(map[string][]ClientBuilderOpt)
}

var redisRegistry map[string][]ClientBuilderOpt

// Client 定义了 Redis 操作的接口。
// 抽象出通用的 Redis 命令，方便注入 mock 实现进行测试。
type Client interface {
	// Get 获取键对应的值。
	Get(ctx context.Context, key string) (string, error)
	// Set 设置键值对，0 表示无过期时间。
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	// Del 删除指定键。
	Del(ctx context.Context, keys ...string) error
	// Exists 判断键是否存在，返回存在的键的个数。
	Exists(ctx context.Context, keys ...string) (int64, error)

	// HSet 设置 Hash 中的字段值（支持多字段）。
	HSet(ctx context.Context, key string, values ...any) error
	// HGet 获取 Hash 中指定字段的值。
	HGet(ctx context.Context, key, field string) (string, error)
	// HGetAll 获取 Hash 中所有字段和值。
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	// HDel 删除 Hash 中的指定字段。
	HDel(ctx context.Context, key string, fields ...string) error
	// HIncrBy 对 Hash 中指定字段做整数增量操作。
	HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error)
	// HIncrByFloat 对 Hash 中指定字段做浮点数增量操作。
	HIncrByFloat(ctx context.Context, key, field string, incr float64) (float64, error)

	// SAdd 向 Set 中添加成员。
	SAdd(ctx context.Context, key string, members ...any) error
	// SRem 从 Set 中移除成员。
	SRem(ctx context.Context, key string, members ...any) error
	// SMembers 获取 Set 中的所有成员。
	SMembers(ctx context.Context, key string) ([]string, error)
	// SIsMember 判断是否为 Set 的成员。
	SIsMember(ctx context.Context, key string, member any) (bool, error)
	// SCard 获取 Set 的成员数量，O(1) 复杂度。
	SCard(ctx context.Context, key string) (int64, error)

	// Eval 执行 Lua 脚本。
	Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)

	// Pipeline 创建管道。
	Pipeline(ctx context.Context) Pipeline

	// Close 关闭 Redis 连接。
	Close() error
}

// Pipeline 定义了 Redis 管道操作接口。
type Pipeline interface {
	HGetAll(ctx context.Context, key string) *goredis.MapStringStringCmd
	Exec(ctx context.Context) ([]goredis.Cmder, error)
}

// goRedisClient 包装 go-redis 实现 Client 接口。
type goRedisClient struct {
	rdb goredis.UniversalClient
}

// WrapGoRedis 将 go-redis UniversalClient 包装为 Client。
func WrapGoRedis(rdb goredis.UniversalClient) Client {
	return &goRedisClient{rdb: rdb}
}

func (c *goRedisClient) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *goRedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

func (c *goRedisClient) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *goRedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.rdb.Exists(ctx, keys...).Result()
}

func (c *goRedisClient) HSet(ctx context.Context, key string, values ...any) error {
	return c.rdb.HSet(ctx, key, values...).Err()
}

func (c *goRedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	return c.rdb.HGet(ctx, key, field).Result()
}

func (c *goRedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.rdb.HGetAll(ctx, key).Result()
}

func (c *goRedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	return c.rdb.HDel(ctx, key, fields...).Err()
}

func (c *goRedisClient) HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error) {
	return c.rdb.HIncrBy(ctx, key, field, incr).Result()
}

func (c *goRedisClient) HIncrByFloat(ctx context.Context, key, field string, incr float64) (float64, error) {
	return c.rdb.HIncrByFloat(ctx, key, field, incr).Result()
}

func (c *goRedisClient) SAdd(ctx context.Context, key string, members ...any) error {
	return c.rdb.SAdd(ctx, key, members...).Err()
}

func (c *goRedisClient) SRem(ctx context.Context, key string, members ...any) error {
	return c.rdb.SRem(ctx, key, members...).Err()
}

func (c *goRedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *goRedisClient) SIsMember(ctx context.Context, key string, member any) (bool, error) {
	return c.rdb.SIsMember(ctx, key, member).Result()
}

func (c *goRedisClient) SCard(ctx context.Context, key string) (int64, error) {
	return c.rdb.SCard(ctx, key).Result()
}

func (c *goRedisClient) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return c.rdb.Eval(ctx, script, keys, args...).Result()
}

func (c *goRedisClient) Pipeline(ctx context.Context) Pipeline {
	return c.rdb.Pipeline()
}

func (c *goRedisClient) Close() error {
	return c.rdb.Close()
}

// clientBuilder 是构建 Client 实例的函数类型。
type clientBuilder func(builderOpts ...ClientBuilderOpt) (Client, error)

var globalBuilder clientBuilder = defaultClientBuilder

// SetClientBuilder 设置全局 Redis client 构建器。
// 可用于注入自定义的 client 创建逻辑（如 tracing 等）。
func SetClientBuilder(builder clientBuilder) {
	globalBuilder = builder
}

// GetClientBuilder 获取当前的 Redis client 构建器。
func GetClientBuilder() clientBuilder {
	return globalBuilder
}

// defaultClientBuilder 是默认的 Redis client 构建器。
func defaultClientBuilder(builderOpts ...ClientBuilderOpt) (Client, error) {
	o := &ClientBuilderOpts{}
	for _, opt := range builderOpts {
		opt(o)
	}

	if o.DSN == "" {
		return nil, fmt.Errorf("redis: DSN is required")
	}

	parsed, err := ParseRedisDSN(o.DSN)
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}

	rdb := goredis.NewClient(&goredis.Options{
		Addr:            parsed.Addr,
		Password:        parsed.Password,
		DB:              parsed.DB,
		MaxRetries:      o.MaxRetries,
		PoolSize:        o.PoolSize,
		MinIdleConns:    o.MinIdleConns,
		ConnMaxIdleTime: o.ConnMaxIdleTime,
		ConnMaxLifetime: o.ConnMaxLifetime,
	})

	// 测试连接。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	return &goRedisClient{rdb: rdb}, nil
}

// ClientBuilderOpt 是 Redis client 构建器的选项函数。
type ClientBuilderOpt func(*ClientBuilderOpts)

// ClientBuilderOpts 是 Redis client 构建器的选项。
type ClientBuilderOpts struct {
	// DSN 是 Redis 数据源名称（连接配置方式）。
	// 格式: redis://[:password@]host:port[/db]
	// 示例:
	//   redis://localhost:6379
	//   redis://:secret@localhost:6379/1
	//   redis://user:secret@redis.example.com:6380/2
	DSN string
	// MaxRetries 是最大重试次数（默认不重试）。
	MaxRetries int
	// PoolSize 是连接池大小。
	PoolSize int
	// MinIdleConns 是最小空闲连接数。
	MinIdleConns int
	// ConnMaxIdleTime 是连接最大空闲时间。
	ConnMaxIdleTime time.Duration
	// ConnMaxLifetime 是连接最大生命周期。
	ConnMaxLifetime time.Duration
	// KeyPrefix 是所有键的前缀（用于命名空间隔离）。
	KeyPrefix string
	// ExtraOptions 是自定义 client 构建器的额外选项。
	ExtraOptions []any
}

// WithClientBuilderDSN 设置 Redis client 的 DSN 连接字符串。
// 格式: redis://[:password@]host:port[/db]
func WithClientBuilderDSN(dsn string) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.DSN = dsn
	}
}

// WithMaxRetries 设置最大重试次数。
func WithMaxRetries(n int) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.MaxRetries = n
	}
}

// WithPoolSize 设置连接池大小。
func WithPoolSize(n int) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.PoolSize = n
	}
}

// WithMinIdleConns 设置最小空闲连接数。
func WithMinIdleConns(n int) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.MinIdleConns = n
	}
}

// WithConnMaxIdleTime 设置连接最大空闲时间。
func WithConnMaxIdleTime(d time.Duration) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ConnMaxIdleTime = d
	}
}

// WithConnMaxLifetime 设置连接最大生命周期。
func WithConnMaxLifetime(d time.Duration) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ConnMaxLifetime = d
	}
}

// WithKeyPrefix 设置所有键的前缀。
func WithKeyPrefix(prefix string) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.KeyPrefix = prefix
	}
}

// WithExtraOptions 设置自定义 client 构建器的额外选项。
func WithExtraOptions(extraOptions ...any) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ExtraOptions = append(opts.ExtraOptions, extraOptions...)
	}
}

// RegisterInstance 注册一个 Redis 实例配置。
// 通过名称标识，可在多个 store 间共享同一实例配置。
func RegisterInstance(name string, opts ...ClientBuilderOpt) {
	redisRegistry[name] = append(redisRegistry[name], opts...)
}

// GetInstance 根据名称获取已注册的 Redis 实例配置。
func GetInstance(name string) ([]ClientBuilderOpt, bool) {
	if _, ok := redisRegistry[name]; !ok {
		return nil, false
	}
	return redisRegistry[name], true
}

// RedisDSNParts 保存从 Redis DSN 中解析出的各个部分。
type RedisDSNParts struct {
	Addr     string // host:port
	Password string // 认证密码
	DB       int    // 数据库编号
}

// ParseRedisDSN 解析 Redis DSN 字符串。
//
// 支持的格式:
//   - redis://host:port
//   - redis://host:port/db
//   - redis://:password@host:port
//   - redis://:password@host:port/db
//   - redis://user:password@host:port/db
//   - host:port                          （无 scheme 的简写格式）
//
// 返回解析后的各部分。如果 DSN 格式有误，返回错误。
func ParseRedisDSN(dsn string) (*RedisDSNParts, error) {
	if dsn == "" {
		return nil, fmt.Errorf("dsn is empty")
	}

	// 兼容无 scheme 的简写格式: "host:port" 或 "host:port/db"
	if !strings.Contains(dsn, "://") {
		dsn = "redis://" + dsn
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid redis dsn %q: %w", dsn, err)
	}

	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return nil, fmt.Errorf("unsupported scheme %q in redis dsn, expected \"redis\" or \"rediss\"", u.Scheme)
	}

	parts := &RedisDSNParts{}

	// 解析 host:port
	parts.Addr = u.Host
	if parts.Addr == "" {
		return nil, fmt.Errorf("missing host in redis dsn %q", dsn)
	}
	// 如果没有指定端口，默认 6379
	if !strings.Contains(parts.Addr, ":") {
		parts.Addr += ":6379"
	}

	// 解析密码
	if u.User != nil {
		parts.Password, _ = u.User.Password()
	}

	// 解析 DB 编号（path 部分，如 /1）
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		db, err := strconv.Atoi(path)
		if err != nil {
			return nil, fmt.Errorf("invalid db number %q in redis dsn", path)
		}
		parts.DB = db
	}

	return parts, nil
}
