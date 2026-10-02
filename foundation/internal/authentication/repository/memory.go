// Package repository provides in-memory token storage for Authentication.
package repository

import (
	"sync"
	"time"

	"arrowhead/foundation/internal/authentication/model"
)

// Repository defines token storage operations.
type Repository interface {
	// Save stores a token. A non-nil error reports a failed write.
	Save(token *model.IdentityToken) error
	FindByToken(token string) (*model.IdentityToken, bool)
	FindBySystemName(name string) (*model.IdentityToken, bool)
	Delete(token string) bool
	// DeleteBySystemName removes every token of a system. A non-nil error
	// reports a failed write (the tokens may still verify).
	DeleteBySystemName(name string) error
	DeleteExpired()
	All() []*model.IdentityToken
}

// MemoryRepository is a thread-safe, in-memory token store.
type MemoryRepository struct {
	mu     sync.RWMutex
	tokens map[string]*model.IdentityToken
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{tokens: make(map[string]*model.IdentityToken)}
}

func (r *MemoryRepository) Save(t *model.IdentityToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[t.Token] = t
	return nil
}

func (r *MemoryRepository) FindByToken(token string) (*model.IdentityToken, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tokens[token]
	return t, ok
}

func (r *MemoryRepository) Delete(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tokens[token]; !ok {
		return false
	}
	delete(r.tokens, token)
	return true
}

func (r *MemoryRepository) FindBySystemName(name string) (*model.IdentityToken, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	for _, t := range r.tokens {
		if t.SystemName == name && !now.After(t.ExpiresAt) {
			return t, true
		}
	}
	return nil, false
}

func (r *MemoryRepository) DeleteBySystemName(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, t := range r.tokens {
		if t.SystemName == name {
			delete(r.tokens, k)
		}
	}
	return nil
}

func (r *MemoryRepository) All() []*model.IdentityToken {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*model.IdentityToken, 0, len(r.tokens))
	for _, t := range r.tokens {
		result = append(result, t)
	}
	return result
}

func (r *MemoryRepository) DeleteExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for k, t := range r.tokens {
		if now.After(t.ExpiresAt) {
			delete(r.tokens, k)
		}
	}
}
