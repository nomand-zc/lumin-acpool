package memory

import (
	"github.com/nomand-zc/lumin-acpool/storage"
)

// Compile-time interface compliance checks.
var (
	_ storage.AccountStorage  = (*AccountStore)(nil)
	_ storage.ProviderStorage = (*ProviderStore)(nil)
)
