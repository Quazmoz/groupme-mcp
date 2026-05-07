package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore manages encrypted GroupMe tokens in Redis.
type RedisStore struct {
	client     *redis.Client
	key        []byte
	expiryDays int
	logger     *slog.Logger
}

// NewRedisStore creates a new Redis-backed token store.
func NewRedisStore(addr, password string, db int, encryptionKey string, expiryDays int, logger *slog.Logger) (*RedisStore, error) {
	if expiryDays <= 0 {
		expiryDays = 90
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Ping to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisStore{
		client:     client,
		key:        DeriveKey(encryptionKey),
		expiryDays: expiryDays,
		logger:     logger,
	}, nil
}

// Register stores an encrypted GroupMe token for a user in Redis.
func (s *RedisStore) Register(userID, groupmeToken string) error {
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

	// Serialize to JSON
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	ctx := context.Background()
	// Key: "mcp:token:{userID}"
	key := fmt.Sprintf("mcp:token:%s", userID)
	
	// Set with expiration
	expiry := time.Duration(s.expiryDays) * 24 * time.Hour
	if err := s.client.Set(ctx, key, data, expiry).Err(); err != nil {
		return fmt.Errorf("failed to save to Redis: %w", err)
	}

	s.logger.Info("Token registered for user (redis)", "user_id", userID)
	return nil
}

// GetDecrypted retrieves and decrypts the GroupMe token from Redis.
func (s *RedisStore) GetDecrypted(userID string) (string, error) {
	ctx := context.Background()
	key := fmt.Sprintf("mcp:token:%s", userID)

	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrTokenNotFound
	} else if err != nil {
		return "", fmt.Errorf("redis error: %w", err)
	}

	var token UserToken
	if err := json.Unmarshal([]byte(val), &token); err != nil {
		return "", fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Double check expiration (Redis should handle it, but good to be safe)
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
		ctx := context.Background()
		token.LastUsedAt = time.Now()
		
		// Re-marshal and update
		if data, err := json.Marshal(token); err == nil {
			// Keep existing TTL
			ttl := s.client.TTL(ctx, key).Val()
			s.client.Set(ctx, key, data, ttl)
		}
	}()

	return decrypted, nil
}

// Status returns the token metadata.
func (s *RedisStore) Status(userID string) (*UserToken, bool) {
	ctx := context.Background()
	key := fmt.Sprintf("mcp:token:%s", userID)

	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, false
	} else if err != nil {
		s.logger.Error("Redis status check failed", "error", err)
		return nil, false
	}

	var token UserToken
	if err := json.Unmarshal([]byte(val), &token); err != nil {
		s.logger.Error("Failed to unmarshal token during status check", "error", err)
		return nil, false
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, false
	}

	return &UserToken{
		UserID:     token.UserID,
		CreatedAt:  token.CreatedAt,
		ExpiresAt:  token.ExpiresAt,
		LastUsedAt: token.LastUsedAt,
	}, true
}

// Revoke removes the token from Redis.
func (s *RedisStore) Revoke(userID string) bool {
	ctx := context.Background()
	key := fmt.Sprintf("mcp:token:%s", userID)

	deleted, err := s.client.Del(ctx, key).Result()
	if err != nil {
		s.logger.Error("Redis revoke failed", "error", err)
		return false
	}
	
	if deleted > 0 {
		s.logger.Info("Token revoked for user (redis)", "user_id", userID)
		return true
	}
	return false
}
