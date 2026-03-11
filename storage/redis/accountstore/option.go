package accountstore

import (
	"fmt"

	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

// Option 是 accountstore 的配置选项函数。
type Option func(*Options)

// Options 是 accountstore 的配置参数。
type Options struct {
	// InstanceName 是已注册的 Redis 实例名称（优先级最高）。
	// 通过 storeRedis.RegisterInstance() 预先注册。
	InstanceName string

	// DSN 是 Redis 连接字符串（优先级次于 InstanceName）。
	// 格式: redis://[:password@]host:port[/db]
	// 示例: redis://:secret@localhost:6379/1
	DSN string

	// KeyPrefix 是 Redis 键前缀，用于命名空间隔离，默认 "acpool:"。
	KeyPrefix string
	// ExtraOptions 是传递给 client builder 的额外选项。
	ExtraOptions []any
}

// DefaultOptions 返回默认配置。
func DefaultOptions() *Options {
	return &Options{
		KeyPrefix: "acpool:",
	}
}

// WithInstanceName 设置已注册的 Redis 实例名称。
// 当设置了 InstanceName 时，DSN 将被忽略。
func WithInstanceName(name string) Option {
	return func(o *Options) {
		o.InstanceName = name
	}
}

// WithDSN 设置 Redis DSN 连接字符串。
// 格式: redis://[:password@]host:port[/db]
func WithDSN(dsn string) Option {
	return func(o *Options) {
		o.DSN = dsn
	}
}

// WithKeyPrefix 设置 Redis 键前缀。
func WithKeyPrefix(prefix string) Option {
	return func(o *Options) {
		o.KeyPrefix = prefix
	}
}

// WithExtraOptions 设置传递给 client builder 的额外选项。
func WithExtraOptions(extraOptions ...any) Option {
	return func(o *Options) {
		o.ExtraOptions = append(o.ExtraOptions, extraOptions...)
	}
}

// buildClient 根据 Options 构建 Redis Client。
// 优先级：InstanceName > DSN。
func buildClient(o *Options) (storeRedis.Client, error) {
	builder := storeRedis.GetClientBuilder()
	var builderOpts []storeRedis.ClientBuilderOpt

	if o.InstanceName != "" {
		// 优先级 1：通过实例名称获取配置
		var ok bool
		builderOpts, ok = storeRedis.GetInstance(o.InstanceName)
		if !ok {
			return nil, fmt.Errorf("accountstore: redis instance %q not found", o.InstanceName)
		}
	} else if o.DSN != "" {
		// 优先级 2：通过 DSN 创建
		builderOpts = []storeRedis.ClientBuilderOpt{
			storeRedis.WithClientBuilderDSN(o.DSN),
		}
	}

	if len(o.ExtraOptions) > 0 {
		builderOpts = append(builderOpts, storeRedis.WithExtraOptions(o.ExtraOptions...))
	}

	return builder(builderOpts...)
}
