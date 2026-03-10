package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func init() {
	mysqlRegistry = make(map[string][]ClientBuilderOpt)
}

var mysqlRegistry map[string][]ClientBuilderOpt

// Client 定义了 MySQL 数据库操作的接口。
// 使用回调模式抽象通用数据库操作，方便注入 mock 实现进行测试。
type Client interface {
	// Exec 执行不返回行的查询（如 INSERT、UPDATE、DELETE）。
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)

	// Query 执行返回行的查询，对每一行调用 next 回调函数。
	Query(ctx context.Context, next NextFunc, query string, args ...any) error

	// QueryRow 执行预期最多返回一行的查询，并扫描到 dest 中。
	QueryRow(ctx context.Context, dest []any, query string, args ...any) error

	// Transaction 在事务中执行函数。
	Transaction(ctx context.Context, fn TxFunc, opts ...TxOption) error

	// Close 关闭数据库连接。
	Close() error
}

// NextFunc 对查询结果中的每一行调用。
// 返回 ErrBreak 可提前终止迭代，返回其他错误则中止并报错。
type NextFunc func(*sql.Rows) error

// TxFunc 是用户事务函数。
// 返回 nil 则提交，返回错误则回滚。
type TxFunc func(*sql.Tx) error

// TxOption 配置事务选项。
type TxOption func(*sql.TxOptions)

// ErrBreak 可在 NextFunc 中返回，用于提前终止迭代而不产生错误。
var ErrBreak = errors.New("mysql scan rows break")

// sqlDBClient 包装 *sql.DB 实现 Client 接口。
type sqlDBClient struct {
	db *sql.DB
}

// WrapSQLDB 将 *sql.DB 连接包装为 Client。
// 注意：此函数主要用于内部或测试场景。
func WrapSQLDB(db *sql.DB) Client {
	return &sqlDBClient{db: db}
}

// Exec 实现 Client.Exec。
func (c *sqlDBClient) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

// Query 实现 Client.Query，使用回调模式。
func (c *sqlDBClient) Query(ctx context.Context, next NextFunc, query string, args ...any) error {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := next(rows); err != nil {
			if errors.Is(err, ErrBreak) {
				break
			}
			return err
		}
	}

	return rows.Err()
}

// QueryRow 实现 Client.QueryRow。
func (c *sqlDBClient) QueryRow(ctx context.Context, dest []any, query string, args ...any) error {
	row := c.db.QueryRowContext(ctx, query, args...)
	return row.Scan(dest...)
}

// Transaction 实现 Client.Transaction，使用回调模式。
func (c *sqlDBClient) Transaction(ctx context.Context, fn TxFunc, opts ...TxOption) error {
	txOpts := &sql.TxOptions{}
	for _, opt := range opts {
		opt(txOpts)
	}

	tx, err := c.db.BeginTx(ctx, txOpts)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		}
	}()

	err = fn(tx)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Close 实现 Client.Close。
func (c *sqlDBClient) Close() error {
	return c.db.Close()
}

// clientBuilder 是构建 Client 实例的函数类型。
type clientBuilder func(builderOpts ...ClientBuilderOpt) (Client, error)

var globalBuilder clientBuilder = defaultClientBuilder

// SetClientBuilder 设置全局 MySQL client 构建器。
// 可用于注入自定义的 client 创建逻辑（如连接池管理、tracing 等）。
func SetClientBuilder(builder clientBuilder) {
	globalBuilder = builder
}

// GetClientBuilder 获取当前的 MySQL client 构建器。
func GetClientBuilder() clientBuilder {
	return globalBuilder
}

// defaultClientBuilder 是默认的 MySQL client 构建器。
func defaultClientBuilder(builderOpts ...ClientBuilderOpt) (Client, error) {
	o := &ClientBuilderOpts{}
	for _, opt := range builderOpts {
		opt(o)
	}

	if o.DSN == "" {
		return nil, errors.New("mysql: dsn is empty")
	}

	db, err := sql.Open("mysql", o.DSN)
	if err != nil {
		return nil, fmt.Errorf("mysql: open connection %s: %w", o.DSN, err)
	}

	// 设置连接池参数。
	if o.MaxOpenConns > 0 {
		db.SetMaxOpenConns(o.MaxOpenConns)
	}
	if o.MaxIdleConns > 0 {
		db.SetMaxIdleConns(o.MaxIdleConns)
	}
	if o.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(o.ConnMaxLifetime)
	}
	if o.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(o.ConnMaxIdleTime)
	}

	// 测试连接。
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("mysql: ping failed: %w", err)
	}

	return &sqlDBClient{db: db}, nil
}

// ClientBuilderOpt 是 MySQL client 构建器的选项函数。
type ClientBuilderOpt func(*ClientBuilderOpts)

// ClientBuilderOpts 是 MySQL client 构建器的选项。
type ClientBuilderOpts struct {
	// DSN 是 MySQL 数据源名称。
	// 格式: [username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
	// 示例: user:password@tcp(localhost:3306)/dbname?parseTime=true
	DSN string

	// MaxOpenConns 是数据库最大打开连接数。
	MaxOpenConns int

	// MaxIdleConns 是空闲连接池中的最大连接数。
	MaxIdleConns int

	// ConnMaxLifetime 是连接可复用的最大时间。
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime 是连接可空闲的最大时间。
	ConnMaxIdleTime time.Duration

	// ExtraOptions 是自定义 client 构建器的额外选项。
	ExtraOptions []any
}

// WithClientBuilderDSN 设置 MySQL client 的 DSN。
func WithClientBuilderDSN(dsn string) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.DSN = dsn
	}
}

// WithMaxOpenConns 设置数据库最大打开连接数。
func WithMaxOpenConns(n int) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.MaxOpenConns = n
	}
}

// WithMaxIdleConns 设置空闲连接池中的最大连接数。
func WithMaxIdleConns(n int) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.MaxIdleConns = n
	}
}

// WithConnMaxLifetime 设置连接可复用的最大时间。
func WithConnMaxLifetime(d time.Duration) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ConnMaxLifetime = d
	}
}

// WithConnMaxIdleTime 设置连接可空闲的最大时间。
func WithConnMaxIdleTime(d time.Duration) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ConnMaxIdleTime = d
	}
}

// WithExtraOptions 设置自定义 client 构建器的额外选项。
func WithExtraOptions(extraOptions ...any) ClientBuilderOpt {
	return func(opts *ClientBuilderOpts) {
		opts.ExtraOptions = append(opts.ExtraOptions, extraOptions...)
	}
}

// RegisterInstance 注册一个 MySQL 实例配置。
// 通过名称标识，可在多个 store 间共享同一实例配置。
func RegisterInstance(name string, opts ...ClientBuilderOpt) {
	mysqlRegistry[name] = append(mysqlRegistry[name], opts...)
}

// GetInstance 根据名称获取已注册的 MySQL 实例配置。
func GetInstance(name string) ([]ClientBuilderOpt, bool) {
	if _, ok := mysqlRegistry[name]; !ok {
		return nil, false
	}
	return mysqlRegistry[name], true
}


