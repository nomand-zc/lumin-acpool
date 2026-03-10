package storage

import "errors"

var (
	// ErrNotFound indicates that the resource does not exist.
	ErrNotFound = errors.New("storage: not found")
	// ErrAlreadyExists indicates that the resource already exists (primary key conflict).
	ErrAlreadyExists = errors.New("storage: already exists")
)
