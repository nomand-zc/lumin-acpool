package selector

import "errors"

var (
	// ErrNoAvailableAccount 没有可用的候选账号
	ErrNoAvailableAccount = errors.New("selector: no available account")
	// ErrNoAvailableProvider 没有可用的候选供应商
	ErrNoAvailableProvider = errors.New("selector: no available provider")
	// ErrEmptyCandidates 候选列表为空
	ErrEmptyCandidates = errors.New("selector: empty candidates")
)
