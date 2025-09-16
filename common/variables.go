package common

import (
	"math/rand"
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
	r := rand.Intn(20)
	if r < 1 {
		heightMax = 0
	}
	return heightMax
}

func SetHeightMax(hmax int64) {
	heightMutex.Lock()
	defer heightMutex.Unlock()
	heightMax = hmax
}
