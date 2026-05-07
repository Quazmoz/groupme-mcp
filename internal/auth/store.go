package auth

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

var (
	ErrTokenNotFound       = errors.New("token not found for user")
	ErrInvalidGroupMeToken = errors.New("invalid GroupMe token: must be at least 20 characters")
)

// UserToken represents a stored GroupMe token for a user.
type UserToken struct {
	UserID         string    `json:"user_id"`
	EncryptedToken string    `json:"encrypted_token"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	LastUsedAt     time.Time `json:"last_used_at"`
}

// TokenStore is the interface for managing user tokens.
type TokenStore interface {
	Register(userID, groupmeToken string) error
	GetDecrypted(userID string) (string, error)
	Status(userID string) (*UserToken, bool)
	Revoke(userID string) bool
}

// MemoryStore manages encrypted GroupMe tokens in memory.
type MemoryStore struct {
	tokens     map[string]*UserToken
	mu         sync.RWMutex
	key        []byte
	expiryDays int
	logger     *slog.Logger
}

// NewMemoryStore creates a new in-memory token store.
func NewMemoryStore(encryptionKey string, expiryDays int, logger *slog.Logger) *MemoryStore {
	if expiryDays <= 0 {
		expiryDays = 90 // Default to 90 days
	}
	return &MemoryStore{
		tokens:     make(map[string]*UserToken),
		key:        DeriveKey(encryptionKey),
		expiryDays: expiryDays,
		logger:     logger,
	}
}

// Register stores an encrypted GroupMe token for a user.
func (s *MemoryStore) Register(userID, groupmeToken string) error {
	// Validate token
	if len(groupmeToken) < 20 {
		return ErrInvalidGroupMeToken
	}

	// Encrypt the token
	encrypted, err := Encrypt(groupmeToken, s.key)
	if err != nil {
		return err
	}

	now := time.Now()
	token := &UserToken{
		UserID:         userID,
		EncryptedToken: encrypted,
		CreatedAt:      now,
		ExpiresAt:      now.AddDate(0, 0, s.expiryDays),
		LastUsedAt:     now,
	}

	s.mu.Lock()
	s.tokens[userID] = token
	s.mu.Unlock()

	s.logger.Info("Token registered for user (memory)", "user_id", userID)
	return nil
}

// GetDecrypted retrieves and decrypts the GroupMe token for a user.
func (s *MemoryStore) GetDecrypted(userID string) (string, error) {
	s.mu.RLock()
	token, exists := s.tokens[userID]
	s.mu.RUnlock()

	if !exists {
		return "", ErrTokenNotFound
	}

	// Check if expired
	if time.Now().After(token.ExpiresAt) {
		s.Revoke(userID)
		return "", ErrTokenNotFound
	}

	// Decrypt
	decrypted, err := Decrypt(token.EncryptedToken, s.key)
	if err != nil {
		return "", err
	}

	// Update last used (fire and forget)
	go func() {
		s.mu.Lock()
		if t, ok := s.tokens[userID]; ok {
			t.LastUsedAt = time.Now()
		}
		s.mu.Unlock()
	}()

	return decrypted, nil
}

// Status returns the token metadata for a user (without the actual token).
func (s *MemoryStore) Status(userID string) (*UserToken, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token, exists := s.tokens[userID]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(token.ExpiresAt) {
		return nil, false
	}

	// Return a copy without the encrypted token for safety
	return &UserToken{
		UserID:     token.UserID,
		CreatedAt:  token.CreatedAt,
		ExpiresAt:  token.ExpiresAt,
		LastUsedAt: token.LastUsedAt,
	}, true
}

// Revoke removes the token for a user.
func (s *MemoryStore) Revoke(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tokens[userID]; exists {
		delete(s.tokens, userID)
		s.logger.Info("Token revoked for user (memory)", "user_id", userID)
		return true
	}
	return false
}
