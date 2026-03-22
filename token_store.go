package main

import (
	"strings"
	"sync"
)

// contextTokenStore caches latest context_token per peer (from_user_id) for outbound sends.
type contextTokenStore struct {
	mu   sync.RWMutex
	byPeer map[string]string // peerID -> token
}

func newContextTokenStore() *contextTokenStore {
	return &contextTokenStore{byPeer: make(map[string]string)}
}

func (s *contextTokenStore) set(peerID, token string) {
	peerID = strings.TrimSpace(peerID)
	token = strings.TrimSpace(token)
	if peerID == "" || token == "" {
		return
	}
	s.mu.Lock()
	s.byPeer[peerID] = token
	s.mu.Unlock()
}

func (s *contextTokenStore) get(peerID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byPeer[strings.TrimSpace(peerID)]
}
