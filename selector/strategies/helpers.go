package strategies

import (
	"math/rand/v2"
	"sync/atomic"
)

// randIntN 生成 [0, n) 范围内的随机整数
func randIntN(n int) int {
	return rand.IntN(n)
}

// atomicAddUint64 原子增加 uint64 值
func atomicAddUint64(addr *uint64, delta uint64) uint64 {
	return atomic.AddUint64(addr, delta)
}
