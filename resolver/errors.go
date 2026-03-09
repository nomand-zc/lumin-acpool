package resolver

import "errors"

var (
	// ErrProviderNotFound 指定的供应商不存在
	ErrProviderNotFound = errors.New("resolver: provider not found")
	// ErrProviderInactive 指定的供应商不在活跃状态
	ErrProviderInactive = errors.New("resolver: provider is not active")
	// ErrModelNotSupported 指定的供应商不支持请求的模型
	ErrModelNotSupported = errors.New("resolver: model not supported by provider")
)
