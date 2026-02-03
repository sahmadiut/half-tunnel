package session

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewStore(t *testing.T) {
	store := NewStore(time.Minute)
	defer store.Close()

	if store.Count() != 0 {
		t.Errorf("New store should have 0 sessions, got %d", store.Count())
	}
}

func TestStoreCreate(t *testing.T) {
	store := NewStore(time.Minute)
	defer store.Close()

	s := store.Create()
	if s == nil {
		t.Fatal("Create should return a session")
	}

	if s.ID == uuid.Nil {
		t.Error("Session should have a valid ID")
	}

	if store.Count() != 1 {
		t.Errorf("Store should have 1 session, got %d", store.Count())
	}
}

func TestStoreGet(t *testing.T) {
	store := NewStore(time.Minute)
	defer store.Close()

	s := store.Create()

	// Get existing session
	retrieved, exists := store.Get(s.ID)
	if !exists {
		t.Error("Session should exist")
	}
	if retrieved != s {
		t.Error("Should return same session instance")
	}

	// Get non-existing session
	_, exists = store.Get(uuid.New())
	if exists {
		t.Error("Non-existing session should not be found")
	}
}

func TestStoreGetOrCreate(t *testing.T) {
	store := NewStore(time.Minute)
	defer store.Close()

	id := uuid.New()

	// Create new session
	s1 := store.GetOrCreate(id)
	if s1.ID != id {
		t.Errorf("Session ID mismatch: got %v, want %v", s1.ID, id)
	}

	if store.Count() != 1 {
		t.Errorf("Store should have 1 session, got %d", store.Count())
	}

	// Get existing session
	s2 := store.GetOrCreate(id)
	if s1 != s2 {
		t.Error("Should return same session instance")
	}

	if store.Count() != 1 {
		t.Errorf("Store should still have 1 session, got %d", store.Count())
	}
}

func TestStoreRemove(t *testing.T) {
	store := NewStore(time.Minute)
	defer store.Close()

	s := store.Create()
	id := s.ID

	if store.Count() != 1 {
		t.Errorf("Store should have 1 session, got %d", store.Count())
	}

	store.Remove(id)

	if store.Count() != 0 {
		t.Errorf("Store should have 0 sessions after removal, got %d", store.Count())
	}

	_, exists := store.Get(id)
	if exists {
		t.Error("Session should not exist after removal")
	}
}

func TestStoreCleanup(t *testing.T) {
	// Use very short TTL for testing
	ttl := 50 * time.Millisecond
	store := NewStore(ttl)
	defer store.Close()

	store.Create()

	if store.Count() != 1 {
		t.Errorf("Store should have 1 session, got %d", store.Count())
	}

	// Wait for cleanup (TTL + cleanup interval)
	time.Sleep(ttl + ttl/2 + 50*time.Millisecond)

	if store.Count() != 0 {
		t.Errorf("Store should have 0 sessions after cleanup, got %d", store.Count())
	}
}

func TestStoreConcurrency(t *testing.T) {
	store := NewStore(time.Minute)
	defer store.Close()

	// Create multiple sessions concurrently
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			s := store.Create()
			s.GetStream(1)
			s.Touch()
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if store.Count() != 100 {
		t.Errorf("Store should have 100 sessions, got %d", store.Count())
	}
}
