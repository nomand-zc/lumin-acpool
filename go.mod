module github.com/nomand-zc/lumin-acpool

go 1.24.11

require github.com/nomand-zc/lumin-client v0.0.0

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/go-sql-driver/mysql v1.9.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
)

replace github.com/nomand-zc/lumin-client => ../lumin-client
