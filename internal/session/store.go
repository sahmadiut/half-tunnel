// Package session provides UUID-based session tracking for the Half-Tunnel system.
package session

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store provides thread-safe storage for sessions with TTL eviction.
type Store struct {
	sessions   map[uuid.UUID]*Session
	mu         sync.RWMutex
	ttl        time.Duration
	cleanupCtx context.Context
	cancelFunc context.CancelFunc
}

// NewStore creates a new session store with the given TTL for session eviction.
func NewStore(ttl time.Duration) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{
		sessions:   make(map[uuid.UUID]*Session),
		ttl:        ttl,
		cleanupCtx: ctx,
		cancelFunc: cancel,
	}
	go s.cleanupLoop()
	return s
}

// Get retrieves a session by ID.
func (s *Store) Get(id uuid.UUID) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, exists := s.sessions[id]
	return session, exists
}

// GetOrCreate retrieves an existing session or creates a new one.
func (s *Store) GetOrCreate(id uuid.UUID) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, exists := s.sessions[id]; exists {
		session.Touch()
		return session
	}

	session := NewWithID(id)
	s.sessions[id] = session
	return session
}

// Create creates a new session and stores it.
func (s *Store) Create() *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := New()
	s.sessions[session.ID] = session
	return session
}

// Remove removes a session by ID.
func (s *Store) Remove(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// Count returns the number of active sessions.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// Close stops the cleanup goroutine.
func (s *Store) Close() {
	s.cancelFunc()
}

// cleanupLoop periodically removes expired sessions.
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(s.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-s.cleanupCtx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup removes all expired sessions.
func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.sessions {
		if session.IsExpired(s.ttl) {
			delete(s.sessions, id)
		}
	}
}
