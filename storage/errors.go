package storage

import "errors"

var (
	// ErrNotFound indicates that the resource does not exist.
	ErrNotFound = errors.New("storage: not found")
	// ErrAlreadyExists indicates that the resource already exists (primary key conflict).
	ErrAlreadyExists = errors.New("storage: already exists")
	// ErrVersionConflict 表示乐观锁冲突，当前版本与数据库版本不一致（已被其他实例更新）。
	ErrVersionConflict = errors.New("storage: version conflict")
)
