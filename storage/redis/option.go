package redis

import (
	"fmt"
)

// Option 是 Redis store 的配置选项函数。
type Option func(*Options)

// Options 是 Redis store 的配置参数。
type Options struct {
	// InstanceName 是已注册的 Redis 实例名称（优先级最高）。
	InstanceName string

	// DSN 是 Redis 连接字符串（优先级次于 InstanceName）。
	DSN string

	// KeyPrefix 是 Redis 键前缀，用于命名空间隔离，默认 "acpool:"。
	KeyPrefix string

	// ExtraOptions 是传递给 client builder 的额外选项。
	ExtraOptions []any
}

// DefaultOptions 返回默认配置。
func DefaultStoreOptions() *Options {
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

// WithStoreKeyPrefix 设置 Redis 键前缀。
func WithStoreKeyPrefix(prefix string) Option {
	return func(o *Options) {
		o.KeyPrefix = prefix
	}
}

// WithStoreExtraOptions 设置传递给 client builder 的额外选项。
func WithStoreExtraOptions(extraOptions ...any) Option {
	return func(o *Options) {
		o.ExtraOptions = append(o.ExtraOptions, extraOptions...)
	}
}

// buildStoreClient 根据 Options 构建 Redis Client。
func buildStoreClient(o *Options) (Client, error) {
	builder := GetClientBuilder()
	var builderOpts []ClientBuilderOpt

	if o.InstanceName != "" {
		var ok bool
		builderOpts, ok = GetInstance(o.InstanceName)
		if !ok {
			return nil, fmt.Errorf("redis store: instance %q not found", o.InstanceName)
		}
	} else if o.DSN != "" {
		builderOpts = []ClientBuilderOpt{
			WithClientBuilderDSN(o.DSN),
		}
	}

	if len(o.ExtraOptions) > 0 {
		builderOpts = append(builderOpts, WithExtraOptions(o.ExtraOptions...))
	}

	return builder(builderOpts...)
}
