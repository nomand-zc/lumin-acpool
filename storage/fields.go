package storage

// Account filterable field constants.
const (
	// AccountFieldID is the account ID.
	AccountFieldID = "id"
	// AccountFieldProviderType is the provider type.
	AccountFieldProviderType = "provider_type"
	// AccountFieldProviderName is the provider instance name.
	AccountFieldProviderName = "provider_name"
	// AccountFieldStatus is the account status.
	AccountFieldStatus = "status"
	// AccountFieldPriority is the priority.
	AccountFieldPriority = "priority"
	// AccountFieldCooldownUntil is the cooldown expiration time.
	AccountFieldCooldownUntil = "cooldown_until"
	// AccountFieldCircuitOpenUntil is the circuit breaker expiration time.
	AccountFieldCircuitOpenUntil = "circuit_open_until"
	// AccountFieldCreatedAt is the creation time.
	AccountFieldCreatedAt = "created_at"
	// AccountFieldUpdatedAt is the last update time.
	AccountFieldUpdatedAt = "updated_at"
)

// ProviderInfo filterable field constants.
const (
	// ProviderFieldType is the provider type.
	ProviderFieldType = "provider_type"
	// ProviderFieldName is the provider instance name.
	ProviderFieldName = "provider_name"
	// ProviderFieldStatus is the provider status.
	ProviderFieldStatus = "provider_status"
	// ProviderFieldPriority is the priority.
	ProviderFieldPriority = "priority"
	// ProviderFieldWeight is the weight.
	ProviderFieldWeight = "weight"
	// ProviderFieldSupportedModel is the supported model.
	ProviderFieldSupportedModel = "supported_model"
	// ProviderFieldAccountCount is the total account count.
	ProviderFieldAccountCount = "account_count"
	// ProviderFieldAvailableAccountCount is the available account count.
	ProviderFieldAvailableAccountCount = "available_account_count"
	// ProviderFieldCreatedAt is the creation time.
	ProviderFieldCreatedAt = "created_at"
	// ProviderFieldUpdatedAt is the last update time.
	ProviderFieldUpdatedAt = "updated_at"
)
