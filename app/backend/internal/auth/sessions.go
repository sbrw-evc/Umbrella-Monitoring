package auth

import (
	"sync"
	"time"
)

// Session lifetimes.
var (
	IdleTimeout = 12 * time.Hour
	MaxLifetime = 7 * 24 * time.Hour
)

// Session is a browser sign-in. Sessions live in memory: a restart signs
// everybody out.
type Session struct {
	ID       string
	UserID   string
	CSRF     string
	Created  time.Time
	LastSeen time.Time
}

// Sessions is the session table.
type Sessions struct {
	mu sync.Mutex
	m  map[string]*Session
}

// NewSessions creates an empty table.
func NewSessions() *Sessions { return &Sessions{m: map[string]*Session{}} }

// Create starts a session for the user.
func (s *Sessions) Create(userID string) *Session {
	now := time.Now()
	ss := &Session{ID: RandomToken("", 32), UserID: userID, CSRF: RandomToken("", 24), Created: now, LastSeen: now}
	s.mu.Lock()
	s.m[TokenHash(ss.ID)] = ss
	s.mu.Unlock()
	return ss
}

// Get returns a live session and extends it.
func (s *Sessions) Get(id string) *Session {
	if id == "" {
		return nil
	}
	key := TokenHash(id)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.m[key]
	if ss == nil {
		return nil
	}
	if now.Sub(ss.LastSeen) > IdleTimeout || now.Sub(ss.Created) > MaxLifetime {
		delete(s.m, key)
		return nil
	}
	ss.LastSeen = now
	c := *ss
	return &c
}

// Delete ends one session.
func (s *Sessions) Delete(id string) {
	s.mu.Lock()
	delete(s.m, TokenHash(id))
	s.mu.Unlock()
}

// DeleteUser ends every session of a user, except keep (may be empty).
func (s *Sessions) DeleteUser(userID, keep string) {
	k := ""
	if keep != "" {
		k = TokenHash(keep)
	}
	s.mu.Lock()
	for key, ss := range s.m {
		if ss.UserID == userID && key != k {
			delete(s.m, key)
		}
	}
	s.mu.Unlock()
}

// Count returns the number of live sessions per user.
func (s *Sessions) Count() map[string]int {
	out := map[string]int{}
	s.mu.Lock()
	for _, ss := range s.m {
		out[ss.UserID]++
	}
	s.mu.Unlock()
	return out
}
