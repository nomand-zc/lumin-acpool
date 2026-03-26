package memory

import (
	"sync"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// 编译期接口合规性检查。
var (
	_ storage.AccountStorage  = (*Store)(nil)
	_ storage.ProviderStorage = (*Store)(nil)
	_ storage.StatsStore      = (*Store)(nil)
	_ storage.UsageStore      = (*Store)(nil)
	_ storage.OccupancyStore  = (*Store)(nil)
	_ storage.AffinityStore   = (*Store)(nil)
)

// StoreOption 是 Store 的配置选项。
type StoreOption func(*Store)

// WithMaxAffinityEntries 设置亲和映射表的最大条目数（默认：10000）。
// 超过上限时清空映射表重建，以防内存无限增长。
func WithMaxAffinityEntries(n int) StoreOption {
	return func(s *Store) {
		if n > 0 {
			s.affinityMaxEntries = n
		}
	}
}

// Store 是基于内存的统一存储实现，实现所有 store 接口。
// 使用多个细粒度的读写锁保证并发安全。
type Store struct {
	// ---- Account 相关 ----
	acctMu sync.RWMutex
	// accounts 是主存储：id -> Account
	accounts map[string]*account.Account
	// acctProviderIndex 是二级索引：ProviderKey -> id set
	acctProviderIndex map[account.ProviderKey]map[string]struct{}
	// acctConverter 是账号条件转换器。
	acctConverter *AccountConverter

	// ---- Provider 相关 ----
	provMu sync.RWMutex
	// providers 是主存储：ProviderKey -> ProviderInfo
	providers map[account.ProviderKey]*account.ProviderInfo
	// provTypeIndex 是类型索引：providerType -> ProviderKey set
	provTypeIndex map[string]map[account.ProviderKey]struct{}
	// provModelIndex 是模型索引：model -> ProviderKey set
	provModelIndex map[string]map[account.ProviderKey]struct{}
	// provConverter 是供应商条件转换器。
	provConverter *ProviderConverter

	// ---- Affinity 相关 ----
	affinityMu         sync.RWMutex
	affinityBindings   map[string]string
	affinityMaxEntries int

	// ---- Occupancy 相关 ----
	// occupancyStore 存储 per-account 的 *atomic.Int64，使用 sync.Map 避免全局锁。
	occupancyStore sync.Map // key: string(accountID), value: *atomic.Int64

	// ---- Stats 相关 ----
	statsMu    sync.Mutex
	statsStore map[string]*account.AccountStats

	// ---- Usage 相关 ----
	usageMu    sync.Mutex
	usageStore map[string][]*account.TrackedUsage
}

// NewStore 创建一个新的内存统一存储实例。
func NewStore(opts ...StoreOption) *Store {
	s := &Store{
		accounts:          make(map[string]*account.Account),
		acctProviderIndex: make(map[account.ProviderKey]map[string]struct{}),
		acctConverter:     &AccountConverter{},

		providers:      make(map[account.ProviderKey]*account.ProviderInfo),
		provTypeIndex:  make(map[string]map[account.ProviderKey]struct{}),
		provModelIndex: make(map[string]map[account.ProviderKey]struct{}),
		provConverter:  &ProviderConverter{},

		affinityBindings:   make(map[string]string),
		affinityMaxEntries: 10000,

		statsStore: make(map[string]*account.AccountStats),
		usageStore: make(map[string][]*account.TrackedUsage),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
