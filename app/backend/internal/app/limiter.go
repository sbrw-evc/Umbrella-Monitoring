package app

import (
	"sync"
	"time"
)

type Limiter struct {
	max    int
	window time.Duration
	block  time.Duration

	mu    sync.Mutex
	marks map[string]*attempts
}

type attempts struct {
	count int
	first time.Time
	until time.Time
}

func NewLimiter(max int, window, block time.Duration) *Limiter {
	return &Limiter{max: max, window: window, block: block, marks: map[string]*attempts{}}
}

func (l *Limiter) Locked(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		if at := l.marks[k]; at != nil && now.Before(at.until) {
			return true
		}
	}
	return false
}

func (l *Limiter) Fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		at := l.marks[k]
		if at == nil || now.Sub(at.first) > l.window {
			at = &attempts{first: now}
			l.marks[k] = at
		}
		at.count++
		if at.count >= l.max {
			at.until, at.count, at.first = now.Add(l.block), 0, now
		}
	}
	if len(l.marks) > 10000 {
		for k, at := range l.marks {
			if now.Sub(at.first) > l.window && now.After(at.until) {
				delete(l.marks, k)
			}
		}
	}
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.marks, key)
}
