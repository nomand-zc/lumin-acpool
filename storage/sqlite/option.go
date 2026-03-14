package sqlite

import (
	"fmt"
)

// Option 是 SQLite store 的配置选项函数。
type Option func(*Options)

// Options 是 SQLite store 的配置参数。
type Options struct {
	// InstanceName 是已注册的 SQLite 实例名称（优先级最高）。
	// 通过 RegisterInstance() 预先注册。
	InstanceName string

	// DSN 是 SQLite 连接字符串（优先级次于 InstanceName）。
	DSN string

	// SkipInitDB 是否跳过数据库初始化（建表等），默认 false 即执行初始化。
	SkipInitDB bool

	// ExtraOptions 是传递给 client builder 的额外选项。
	ExtraOptions []any
}

// DefaultOptions 返回默认配置。
func DefaultOptions() *Options {
	return &Options{}
}

// WithInstanceName 设置已注册的 SQLite 实例名称。
func WithInstanceName(name string) Option {
	return func(o *Options) {
		o.InstanceName = name
	}
}

// WithDSN 设置 SQLite DSN 连接字符串。
func WithDSN(dsn string) Option {
	return func(o *Options) {
		o.DSN = dsn
	}
}

// WithSkipInitDB 设置是否跳过数据库初始化（建表等）。
func WithSkipInitDB(skip bool) Option {
	return func(o *Options) {
		o.SkipInitDB = skip
	}
}

// WithStoreExtraOptions 设置传递给 client builder 的额外选项。
func WithStoreExtraOptions(extraOptions ...any) Option {
	return func(o *Options) {
		o.ExtraOptions = append(o.ExtraOptions, extraOptions...)
	}
}

// buildClient 根据 Options 构建 SQLite Client。
// 优先级：InstanceName > DSN。
func buildClient(o *Options) (Client, error) {
	builder := GetClientBuilder()
	var builderOpts []ClientBuilderOpt

	if o.InstanceName != "" {
		var ok bool
		builderOpts, ok = GetInstance(o.InstanceName)
		if !ok {
			return nil, fmt.Errorf("sqlite store: instance %q not found", o.InstanceName)
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
