package account

// Status represents the account status enumeration.
type Status int

const (
	// StatusAvailable means the account is available and can participate in selection.
	StatusAvailable Status = iota + 1
	// StatusCoolingDown means the account is cooling down due to rate limiting, waiting for auto-recovery.
	StatusCoolingDown
	// StatusCircuitOpen means the circuit breaker is open due to too many consecutive failures, temporarily excluded from selection.
	StatusCircuitOpen
	// StatusExpired means the credential has expired and needs to be refreshed.
	StatusExpired
	// StatusInvalidated means the credential is permanently invalid (e.g., refresh token is invalid).
	StatusInvalidated
	// StatusBanned means the account is banned by the platform and requires manual intervention.
	StatusBanned
	// StatusDisabled means the account is manually disabled by an administrator.
	StatusDisabled
)

// IsSelectable returns whether the account can participate in selection.
func (s Status) IsSelectable() bool {
	return s == StatusAvailable
}

// IsRecoverable returns whether the account can potentially auto-recover.
func (s Status) IsRecoverable() bool {
	switch s {
	case StatusCoolingDown, StatusCircuitOpen, StatusExpired:
		return true
	default:
		return false
	}
}

// String returns a human-readable string representation of the status.
func (s Status) String() string {
	switch s {
	case StatusAvailable:
		return "available"
	case StatusCoolingDown:
		return "cooling_down"
	case StatusCircuitOpen:
		return "circuit_open"
	case StatusExpired:
		return "expired"
	case StatusInvalidated:
		return "invalidated"
	case StatusBanned:
		return "banned"
	case StatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}
