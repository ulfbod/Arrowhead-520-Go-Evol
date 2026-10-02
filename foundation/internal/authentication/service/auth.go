// Package service implements Authentication business logic.
// AH5 responsibility: provide, manage, and validate system identities.
package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"arrowhead/foundation/internal/authentication/model"
	"arrowhead/foundation/internal/authentication/repository"
)

var (
	ErrMissingSystemName  = errors.New("systemName is required")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrStorage wraps a failed write in the token or identity store (500 in the API).
	ErrStorage = errors.New("storage error")
)

// AuthService manages identity tokens.
type AuthService struct {
	repo          repository.Repository
	identityRepo  repository.IdentityRepository // nil = no credential verification
	tokenDuration time.Duration
}

func NewAuthService(repo repository.Repository, tokenDuration time.Duration) *AuthService {
	return NewAuthServiceWithCleanup(repo, tokenDuration, 5*time.Minute)
}

// NewAuthServiceWithCleanup creates an AuthService that periodically removes
// expired tokens from the repository every cleanupInterval.
func NewAuthServiceWithCleanup(repo repository.Repository, tokenDuration, cleanupInterval time.Duration) *AuthService {
	svc := &AuthService{repo: repo, tokenDuration: tokenDuration}
	go func() {
		for range time.Tick(cleanupInterval) {
			repo.DeleteExpired()
		}
	}()
	return svc
}

// NewAuthServiceFull creates an AuthService with credential verification.
// Bootstrap: if the identity store is empty, a Sysop identity is created
// using the SYSOP_PASSWORD env var (default: "arrowhead").
func NewAuthServiceFull(tokenRepo repository.Repository, identityRepo repository.IdentityRepository, tokenDuration time.Duration) *AuthService {
	svc := &AuthService{
		repo:          tokenRepo,
		identityRepo:  identityRepo,
		tokenDuration: tokenDuration,
	}
	go func() {
		for range time.Tick(5 * time.Minute) {
			tokenRepo.DeleteExpired()
		}
	}()
	// Bootstrap Sysop identity if store is empty.
	if len(identityRepo.All()) == 0 {
		sysopPassword := os.Getenv("SYSOP_PASSWORD")
		if sysopPassword == "" {
			sysopPassword = "arrowhead"
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(sysopPassword), 12)
		identityRepo.Save(repository.Identity{
			SystemName:   "Sysop",
			PasswordHash: string(hash),
			Sysop:        true,
		})
	}
	return svc
}

// Login creates and stores an identity token for the given system.
// When an identityRepo is configured, credentials are verified against the
// stored bcrypt hash and 401 is returned on mismatch or unknown systemName.
func (s *AuthService) Login(req model.LoginRequest) (*model.LoginResponse, error) {
	if strings.TrimSpace(req.SystemName) == "" {
		return nil, ErrMissingSystemName
	}
	sysop := false
	if s.identityRepo != nil {
		id, ok := s.identityRepo.Get(req.SystemName)
		if !ok {
			return nil, ErrInvalidCredentials
		}
		password := ""
		if creds, ok := req.CredentialsMap["password"]; ok {
			password = creds
		}
		if err := bcrypt.CompareHashAndPassword([]byte(id.PasswordHash), []byte(password)); err != nil {
			return nil, ErrInvalidCredentials
		}
		sysop = id.Sysop
	}
	now := time.Now()
	token := &model.IdentityToken{
		Token:      generateToken(),
		SystemName: req.SystemName,
		ExpiresAt:  now.Add(s.tokenDuration),
		LoginTime:  now,
	}
	if err := s.repo.Save(token); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	return &model.LoginResponse{
		Token:          token.Token,
		SystemName:     token.SystemName,
		ExpirationTime: token.ExpiresAt,
		Sysop:          sysop,
	}, nil
}

// Logout invalidates an identity token.
func (s *AuthService) Logout(token string) error {
	if !s.repo.Delete(token) {
		return ErrInvalidToken
	}
	return nil
}

// Verify checks whether a token is valid and not expired.
func (s *AuthService) Verify(token string) (*model.VerifyResponse, error) {
	t, ok := s.repo.FindByToken(token)
	if !ok {
		return &model.VerifyResponse{Verified: false}, nil
	}
	if time.Now().After(t.ExpiresAt) {
		s.repo.Delete(token)
		return &model.VerifyResponse{Verified: false}, nil
	}
	sysop := false
	if s.identityRepo != nil {
		if id, ok := s.identityRepo.Get(t.SystemName); ok {
			sysop = id.Sysop
		}
	}
	return &model.VerifyResponse{
		Verified:       true,
		SystemName:     t.SystemName,
		LoginTime:      t.LoginTime.Format(time.RFC3339),
		ExpirationTime: t.ExpiresAt.Format(time.RFC3339),
		Sysop:          sysop,
	}, nil
}

// ChangeCredentials accepts a credential rotation for the named system.
// Returns ErrInvalidToken if the system has no active session.
func (s *AuthService) ChangeCredentials(systemName string) error {
	if _, ok := s.repo.FindBySystemName(systemName); !ok {
		return ErrInvalidToken
	}
	return nil
}

// ─── Identity management ──────────────────────────────────────────────────────

// IdentityRecord is the safe (no hash) view returned by management endpoints.
type IdentityRecord struct {
	SystemName string `json:"systemName"`
	Sysop      bool   `json:"sysop"`
	CreatedBy  string `json:"createdBy,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

type CreateIdentityRequest struct {
	SystemName  string            `json:"systemName"`
	Credentials map[string]string `json:"credentials"`
	Sysop       bool              `json:"sysop"`
	CreatedBy   string            `json:"createdBy"`
}

// CreateIdentities hashes each identity's password and stores the record.
func (s *AuthService) CreateIdentities(reqs []CreateIdentityRequest) ([]IdentityRecord, error) {
	if s.identityRepo == nil {
		return nil, errors.New("identity store not configured")
	}
	var result []IdentityRecord
	for _, req := range reqs {
		password := req.Credentials["password"]
		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return nil, err
		}
		id := repository.Identity{
			SystemName:   req.SystemName,
			PasswordHash: string(hash),
			Sysop:        req.Sysop,
			CreatedBy:    req.CreatedBy,
		}
		if err := s.identityRepo.Save(id); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrStorage, err)
		}
		saved, _ := s.identityRepo.Get(req.SystemName)
		result = append(result, toRecord(saved))
	}
	return result, nil
}

// UpdateIdentities re-hashes and updates existing identity records.
func (s *AuthService) UpdateIdentities(reqs []CreateIdentityRequest) ([]IdentityRecord, error) {
	if s.identityRepo == nil {
		return nil, errors.New("identity store not configured")
	}
	var result []IdentityRecord
	for _, req := range reqs {
		existing, ok := s.identityRepo.Get(req.SystemName)
		if !ok {
			return nil, fmt.Errorf("identity not found: %s", req.SystemName)
		}
		password := req.Credentials["password"]
		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return nil, err
		}
		existing.PasswordHash = string(hash)
		existing.Sysop = req.Sysop
		existing.CreatedBy = req.CreatedBy
		if err := s.identityRepo.Save(existing); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrStorage, err)
		}
		saved, _ := s.identityRepo.Get(req.SystemName)
		result = append(result, toRecord(saved))
	}
	return result, nil
}

// DeleteIdentities removes identity records by system name. A failed write
// stops the loop and returns an ErrStorage-wrapped error.
func (s *AuthService) DeleteIdentities(names []string) error {
	if s.identityRepo == nil {
		return nil
	}
	for _, name := range names {
		if err := s.identityRepo.Delete(name); err != nil {
			return fmt.Errorf("%w: %v", ErrStorage, err)
		}
	}
	return nil
}

// QueryIdentities returns all stored identity records (no passwords).
func (s *AuthService) QueryIdentities() []IdentityRecord {
	if s.identityRepo == nil {
		return nil
	}
	all := s.identityRepo.All()
	result := make([]IdentityRecord, 0, len(all))
	for _, id := range all {
		result = append(result, toRecord(id))
	}
	return result
}

// ─── Session management ───────────────────────────────────────────────────────

// SessionRecord is the safe view of an active token.
type SessionRecord struct {
	Token          string `json:"token"`
	SystemName     string `json:"systemName"`
	LoginTime      string `json:"loginTime"`
	ExpirationTime string `json:"expirationTime"`
}

// HasIdentityRepo returns true when an identity store is configured (credential
// verification is active). Used by the handler to enforce G43 credential validation.
func (s *AuthService) HasIdentityRepo() bool {
	return s.identityRepo != nil
}

// QuerySessions returns all active (non-expired) token records.
func (s *AuthService) QuerySessions() []SessionRecord {
	all := s.repo.All()
	now := time.Now()
	var result []SessionRecord
	for _, t := range all {
		if now.Before(t.ExpiresAt) {
			result = append(result, SessionRecord{
				Token:          t.Token,
				SystemName:     t.SystemName,
				LoginTime:      t.LoginTime.Format(time.RFC3339),
				ExpirationTime: t.ExpiresAt.Format(time.RFC3339),
			})
		}
	}
	return result
}

// RevokeSessions deletes all tokens for the given system names. A failed write
// stops the loop and returns an ErrStorage-wrapped error: the tokens of that
// system may still verify, so the caller must not report success.
func (s *AuthService) RevokeSessions(names []string) error {
	for _, name := range names {
		if err := s.repo.DeleteBySystemName(name); err != nil {
			return fmt.Errorf("%w: %v", ErrStorage, err)
		}
	}
	return nil
}

func toRecord(id repository.Identity) IdentityRecord {
	return IdentityRecord{
		SystemName: id.SystemName,
		Sysop:      id.Sysop,
		CreatedBy:  id.CreatedBy,
		CreatedAt:  id.CreatedAt,
		UpdatedAt:  id.UpdatedAt,
	}
}

func generateToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
