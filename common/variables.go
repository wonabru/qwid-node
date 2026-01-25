package common

import (
	"sync"
	"sync/atomic"
)

var height int64
var heightMax int64
var heightMutex sync.RWMutex
var BlockMutex sync.Mutex
var NonceMutex sync.Mutex
var IsSyncing = atomic.Bool{}

func GetHeight() int64 {
	heightMutex.RLock()
	defer heightMutex.RUnlock()
	return height
}

func SetHeight(h int64) {
	heightMutex.Lock()
	defer heightMutex.Unlock()
	height = h
}

func GetHeightMax() int64 {
	heightMutex.RLock()
	defer heightMutex.RUnlock()
	return heightMax
}

func SetHeightMax(hmax int64) {
	heightMutex.Lock()
	defer heightMutex.Unlock()
	heightMax = hmax
}
