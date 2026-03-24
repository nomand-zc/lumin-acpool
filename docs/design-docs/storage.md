# 存储层设计

## 6 个子接口

| 接口 | 职责 |
|------|------|
| `AccountStorage` | Account CRUD，部分字段更新（UpdateField 位掩码） |
| `ProviderStorage` | ProviderInfo CRUD |
| `StatsStore` | 运行时统计，原子 IncrSuccess/IncrFailure |
| `UsageStore` | 用量追踪，原子 IncrLocalUsed/CalibrateRule |
| `OccupancyStore` | 并发占用计数，原子 Incr/Decr |
| `AffinityStore` | 亲和绑定 affinityKey → targetID |

消费端应依赖子接口（接口隔离），聚合接口 Storage 仅用于一体化后端初始化传递。

## 4 种后端

| 后端 | 适用场景 |
|------|---------|
| Memory (`storage/memory/`) | 单机开发/测试（全部 6 个接口） |
| SQLite (`storage/sqlite/`) | 单机持久化（lumin-desktop） |
| MySQL (`storage/mysql/`) | 集群生产环境 |
| Redis (`storage/redis/`) | 集群高性能（全部 6 个接口） |

典型集群组合：MySQL（持久化） + Redis（高频操作 + 占用 + 亲和）

## UpdateField 位掩码

`UpdateAccount(acct, fields)` 按位掩码部分更新，避免全量覆盖：

- `UpdateFieldCredential` — 凭证
- `UpdateFieldStatus` — 状态 + CooldownUntil + CircuitOpenUntil
- `UpdateFieldPriority` — 优先级
- `UpdateFieldTags` — 标签
- `UpdateFieldMetadata` — 元数据
- `UpdateFieldUsageRules` — 用量规则

**重要**：Status 变更时存储层应在事务内同步更新 Provider.AvailableAccountCount。

## SearchFilter

高频索引字段提升为一级字段（ProviderType/Name/Status/SupportedModel），Redis 等可直接定位索引。
ExtraCond 使用 `filtercond.Filter` 表达式树，各后端实现 `Converter[T]` 转为具体查询格式。

## DDL 文件

MySQL 和 SQLite 建表语句以 `.sql` 嵌入文件存储在各自目录中。
