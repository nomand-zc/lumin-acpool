module github.com/nomand-zc/lumin-acpool

go 1.24.11

require (
	github.com/go-sql-driver/mysql v1.9.3
	github.com/mattn/go-sqlite3 v1.14.34
	github.com/nomand-zc/lumin-client v0.0.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.8 // indirect
	github.com/aws/smithy-go v1.22.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/juju/errors v1.0.0 // indirect
	github.com/panjf2000/ants/v2 v2.11.5 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tiktoken-go/tokenizer v0.7.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	golang.org/x/sync v0.11.0 // indirect
)

replace github.com/nomand-zc/lumin-client => ../lumin-client
