package redis

import (
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

// Store 是基于 Redis 的统一存储实现，实现所有 store 接口。
// 共享同一个 Client 连接。
type Store struct {
	client            Client
	keyPrefix         string
	accountEvaluator  *FilterEvaluator
	providerEvaluator *FilterEvaluator
}

// NewStore 创建一个新的 Redis 统一存储实例。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultStoreOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildStoreClient(o)
	if err != nil {
		return nil, err
	}

	store := &Store{
		client:            client,
		keyPrefix:         o.KeyPrefix,
		accountEvaluator:  NewFilterEvaluator(accountFieldExtractor),
		providerEvaluator: NewFilterEvaluator(providerFieldExtractor),
	}

	return store, nil
}

// Close 关闭 Redis 连接。
func (s *Store) Close() error {
	return s.client.Close()
}

// pipelineCmd 包装 Pipeline 命令结果。
type pipelineCmd struct {
	cmd interface {
		Result() (map[string]string, error)
	}
}
