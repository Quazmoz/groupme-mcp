package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/lib/pq"
)

// PostgresStore manages encrypted GroupMe tokens in PostgreSQL.
type PostgresStore struct {
	db         *sql.DB
	key        []byte
	expiryDays int
	logger     *slog.Logger
}

// NewPostgresStore creates a new PostgreSQL-backed token store.
// databaseURL should be in format: postgresql://user:pass@host:5432/dbname
func NewPostgresStore(databaseURL, encryptionKey string, expiryDays int, logger *slog.Logger) (*PostgresStore, error) {
	if expiryDays <= 0 {
		expiryDays = 90
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Ping to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	store := &PostgresStore{
		db:         db,
		key:        DeriveKey(encryptionKey),
		expiryDays: expiryDays,
		logger:     logger,
	}

	// Auto-migrate: create table if not exists
	if err := store.migrate(ctx); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	logger.Info("PostgreSQL token store initialized")
	return store, nil
}

// migrate creates the user_tokens table if it doesn't exist.
func (s *PostgresStore) migrate(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS user_tokens (
			user_id VARCHAR(255) PRIMARY KEY,
			encrypted_token TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			last_used_at TIMESTAMP DEFAULT NOW()
		);
	`
	_, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create user_tokens table: %w", err)
	}
	s.logger.Info("Database migration complete (user_tokens table)")
	return nil
}

// Register stores an encrypted GroupMe token for a user in PostgreSQL.
func (s *PostgresStore) Register(userID, groupmeToken string) error {
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
	expiresAt := now.AddDate(0, 0, s.expiryDays)

	ctx := context.Background()
	query := `
		INSERT INTO user_tokens (user_id, encrypted_token, expires_at, created_at, updated_at, last_used_at)
		VALUES ($1, $2, $3, $4, $4, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			encrypted_token = EXCLUDED.encrypted_token,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`

	_, err = s.db.ExecContext(ctx, query, userID, encrypted, expiresAt, now)
	if err != nil {
		return fmt.Errorf("failed to save token to PostgreSQL: %w", err)
	}

	s.logger.Info("Token registered for user (postgres)", "user_id", userID)
	return nil
}

// GetDecrypted retrieves and decrypts the GroupMe token from PostgreSQL.
func (s *PostgresStore) GetDecrypted(userID string) (string, error) {
	ctx := context.Background()
	query := `
		SELECT encrypted_token, expires_at 
		FROM user_tokens 
		WHERE user_id = $1
	`

	var encryptedToken string
	var expiresAt time.Time

	err := s.db.QueryRowContext(ctx, query, userID).Scan(&encryptedToken, &expiresAt)
	if err == sql.ErrNoRows {
		return "", ErrTokenNotFound
	} else if err != nil {
		return "", fmt.Errorf("postgres query error: %w", err)
	}

	// Check if expired
	if time.Now().After(expiresAt) {
		s.Revoke(userID)
		return "", ErrTokenNotFound
	}

	// Decrypt
	decrypted, err := Decrypt(encryptedToken, s.key)
	if err != nil {
		return "", err
	}

	// Update last used (fire and forget)
	go func() {
		ctx := context.Background()
		_, _ = s.db.ExecContext(ctx,
			"UPDATE user_tokens SET last_used_at = NOW() WHERE user_id = $1",
			userID)
	}()

	return decrypted, nil
}

// Status returns the token metadata for a user.
func (s *PostgresStore) Status(userID string) (*UserToken, bool) {
	ctx := context.Background()
	query := `
		SELECT user_id, expires_at, created_at, last_used_at 
		FROM user_tokens 
		WHERE user_id = $1
	`

	var token UserToken
	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&token.UserID, &token.ExpiresAt, &token.CreatedAt, &token.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, false
	} else if err != nil {
		s.logger.Error("Postgres status check failed", "error", err)
		return nil, false
	}

	// Check if expired
	if time.Now().After(token.ExpiresAt) {
		return nil, false
	}

	return &token, true
}

// Revoke removes the token from PostgreSQL.
func (s *PostgresStore) Revoke(userID string) bool {
	ctx := context.Background()
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM user_tokens WHERE user_id = $1", userID)
	if err != nil {
		s.logger.Error("Postgres revoke failed", "error", err)
		return false
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		s.logger.Info("Token revoked for user (postgres)", "user_id", userID)
		return true
	}
	return false
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
