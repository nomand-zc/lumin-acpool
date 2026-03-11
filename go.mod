module github.com/nomand-zc/lumin-acpool

go 1.24.11

require github.com/nomand-zc/lumin-client v0.0.0

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-sql-driver/mysql v1.9.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.34 // indirect
	github.com/redis/go-redis/v9 v9.18.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace github.com/nomand-zc/lumin-client => ../lumin-client
