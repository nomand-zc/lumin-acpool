package resolver

import "errors"

var (
	// ErrProviderNotFound indicates that the specified provider does not exist.
	ErrProviderNotFound = errors.New("resolver: provider not found")
	// ErrProviderInactive indicates that the specified provider is not active.
	ErrProviderInactive = errors.New("resolver: provider is not active")
	// ErrModelNotSupported indicates that the specified provider does not support the requested model.
	ErrModelNotSupported = errors.New("resolver: model not supported by provider")
	// ErrNoAccount indicates that no account is available for the requested operation.
	ErrNoAccount = errors.New("resolver: no account available")
)
