package storage

import "errors"

var (
	// ErrNotFound 资源不存在
	ErrNotFound = errors.New("storage: not found")
	// ErrAlreadyExists 资源已存在（主键冲突）
	ErrAlreadyExists = errors.New("storage: already exists")
)
