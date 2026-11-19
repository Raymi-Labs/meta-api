package common

import (
	"sync"
	"time"
)

const rateLimiterCleanupInterval = 60 * time.Second

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	expirationDuration time.Duration
	mutex              sync.Mutex
	started            bool
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.store == nil {
		l.store = make(map[string]*[]int64)
	}
	if expirationDuration > l.expirationDuration {
		l.expirationDuration = expirationDuration
	}
	if !l.started && l.expirationDuration > 0 {
		l.started = true
		go l.clearExpiredItems()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(rateLimiterCleanupInterval)
		l.clearExpiredItemsAt(time.Now().Unix())
	}
}

func (l *InMemoryRateLimiter) clearExpiredItemsAt(now int64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	expirationSeconds := int64(l.expirationDuration.Seconds())
	if expirationSeconds <= 0 {
		expirationSeconds = int64(rateLimiterCleanupInterval.Seconds())
	}
	for key, queue := range l.store {
		size := len(*queue)
		// Remove entries idle for more than the configured retention window.
		if size == 0 || now-(*queue)[size-1] > expirationSeconds {
			delete(l.store, key)
		}
	}
}

// QueryStatus returns the current count of requests within the window and the oldest request timestamp.
// This is a read-only operation that does not record a new request.
func (l *InMemoryRateLimiter) QueryStatus(key string, duration int64) (count int, oldestTs int64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	queue, ok := l.store[key]
	if !ok || len(*queue) == 0 {
		return 0, 0
	}

	now := time.Now().Unix()
	windowStart := now - duration

	// Find the first entry within the window
	startIdx := -1
	for i, ts := range *queue {
		if ts > windowStart {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return 0, 0
	}

	count = len(*queue) - startIdx
	oldestTs = (*queue)[startIdx]
	return count, oldestTs
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			return true
		} else {
			if now-(*queue)[0] >= duration {
				*queue = (*queue)[1:]
				*queue = append(*queue, now)
				return true
			} else {
				return false
			}
		}
	} else {
		s := make([]int64, 0, maxRequestNum)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}
