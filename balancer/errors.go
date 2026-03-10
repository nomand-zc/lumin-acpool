package balancer

import "errors"

var (
	// ErrModelRequired indicates that no model was specified.
	ErrModelRequired = errors.New("balancer: model is required")
	// ErrNoAvailableProvider indicates that no available provider was found.
	ErrNoAvailableProvider = errors.New("balancer: no available provider")
	// ErrNoAvailableAccount indicates that no available account was found.
	ErrNoAvailableAccount = errors.New("balancer: no available account")
	// ErrModelNotSupported indicates that no provider supports the requested model.
	ErrModelNotSupported = errors.New("balancer: model not supported by any provider")
	// ErrProviderNotFound indicates that the specified provider does not exist.
	ErrProviderNotFound = errors.New("balancer: specified provider not found")
	// ErrMaxRetriesExceeded indicates that the maximum retries were exceeded without obtaining an available account.
	ErrMaxRetriesExceeded = errors.New("balancer: max retries exceeded")
	// ErrAccountNotFound indicates that the account was not found when reporting results.
	ErrAccountNotFound = errors.New("balancer: account not found")
)
