package selector

import "errors"

var (
	// ErrNoAvailableAccount indicates that no available candidate account was found.
	ErrNoAvailableAccount = errors.New("selector: no available account")
	// ErrNoAvailableProvider indicates that no available candidate provider was found.
	ErrNoAvailableProvider = errors.New("selector: no available provider")
	// ErrEmptyCandidates indicates that the candidate list is empty.
	ErrEmptyCandidates = errors.New("selector: empty candidates")
)
