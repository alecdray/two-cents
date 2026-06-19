package auth

import (
	"fmt"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"golang.org/x/crypto/bcrypt"
)

// Service owns the single local credential and the login decision. It writes
// only its own table and reaches no other domain; the session token and cookie
// are core's job (ADR-0007).
type Service struct {
	db *db.DB
}

// NewService builds an auth Service over the database.
func NewService(d *db.DB) *Service {
	return &Service{db: d}
}

func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}

// Authenticate verifies the submitted password against the stored credential
// and returns the user id on success. It returns ErrNoCredential when none has
// been seeded and ErrInvalidCredentials when the password does not match — the
// caller surfaces both as the same inline "incorrect password" message so a
// probe can't distinguish "no account" from "wrong password".
func (s *Service) Authenticate(ctx contextx.ContextX, password string) (string, error) {
	cred, ok, err := s.repo().GetCredential(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load credential: %w", err)
	}
	if !ok {
		return "", ErrNoCredential
	}
	if bcrypt.CompareHashAndPassword([]byte(cred.hash), []byte(password)) != nil {
		return "", ErrInvalidCredentials
	}
	return cred.id, nil
}

// HasCredential reports whether a credential has been seeded, so the login page
// can show the "no account configured" state instead of a password prompt.
func (s *Service) HasCredential(ctx contextx.ContextX) (bool, error) {
	_, ok, err := s.repo().GetCredential(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to load credential: %w", err)
	}
	return ok, nil
}

// SetPassword hashes the password and upserts the single credential row. It is
// the bootstrap-and-rotation path the operator runs out-of-band (the
// set-password command); re-running it replaces the stored hash. An empty
// password is rejected.
func (s *Service) SetPassword(ctx contextx.ContextX, password string) error {
	if password == "" {
		return fmt.Errorf("password must not be empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	if err := s.repo().UpsertCredential(ctx, defaultUsername, string(hash)); err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}
	return nil
}
