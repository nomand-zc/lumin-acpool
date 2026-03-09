package balancer

import "errors"

var (
	// ErrModelRequired 未指定模型
	ErrModelRequired = errors.New("balancer: model is required")
	// ErrNoAvailableProvider 没有可用的供应商
	ErrNoAvailableProvider = errors.New("balancer: no available provider")
	// ErrNoAvailableAccount 没有可用的账号
	ErrNoAvailableAccount = errors.New("balancer: no available account")
	// ErrModelNotSupported 请求的模型无任何供应商支持
	ErrModelNotSupported = errors.New("balancer: model not supported by any provider")
	// ErrProviderNotFound 指定的供应商不存在
	ErrProviderNotFound = errors.New("balancer: specified provider not found")
	// ErrMaxRetriesExceeded 超过最大重试次数仍未获取到可用账号
	ErrMaxRetriesExceeded = errors.New("balancer: max retries exceeded")
	// ErrAccountNotFound 上报结果时找不到对应的账号
	ErrAccountNotFound = errors.New("balancer: account not found")
)
