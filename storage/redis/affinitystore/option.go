package affinitystore

import (
	"fmt"

	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

// Option 是 affinitystore 的配置选项函数。
type Option func(*Options)

// Options 是 affinitystore 的配置参数。
type Options struct {
	// InstanceName 是已注册的 Redis 实例名称（优先级最高）。
	InstanceName string
	// DSN 是 Redis 连接字符串（优先级次于 InstanceName）。
	// 格式: redis://[:password@]host:port[/db]
	DSN string
	// KeyPrefix 是 Redis 键前缀，默认 "acpool:"。
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
func WithInstanceName(name string) Option {
	return func(o *Options) {
		o.InstanceName = name
	}
}

// WithDSN 设置 Redis DSN 连接字符串。
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
		var ok bool
		builderOpts, ok = storeRedis.GetInstance(o.InstanceName)
		if !ok {
			return nil, fmt.Errorf("affinitystore: redis instance %q not found", o.InstanceName)
		}
	} else if o.DSN != "" {
		builderOpts = []storeRedis.ClientBuilderOpt{
			storeRedis.WithClientBuilderDSN(o.DSN),
		}
	}

	if len(o.ExtraOptions) > 0 {
		builderOpts = append(builderOpts, storeRedis.WithExtraOptions(o.ExtraOptions...))
	}

	return builder(builderOpts...)
}
