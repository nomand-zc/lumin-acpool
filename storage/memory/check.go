package memory

import (
	"github.com/nomand-zc/lumin-acpool/storage"
)

// 编译期接口一致性检查
var (
	_ storage.AccountStorage  = (*AccountStore)(nil)
	_ storage.ProviderStorage = (*ProviderStore)(nil)
)
