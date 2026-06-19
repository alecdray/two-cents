package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

func newTestService(t *testing.T) *Service {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return NewService(db.WrapSqlDB(sqlDB))
}

func testCtx() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

func TestAuthenticate(t *testing.T) {
	t.Run("returns the user id for the correct password", func(t *testing.T) {
		s := newTestService(t)
		ctx := testCtx()
		if err := s.SetPassword(ctx, "correct horse"); err != nil {
			t.Fatalf("set password: %v", err)
		}

		id, err := s.Authenticate(ctx, "correct horse")
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if id != userID {
			t.Fatalf("user id = %q, want %q", id, userID)
		}
	})

	t.Run("rejects the wrong password", func(t *testing.T) {
		s := newTestService(t)
		ctx := testCtx()
		if err := s.SetPassword(ctx, "correct horse"); err != nil {
			t.Fatalf("set password: %v", err)
		}

		if _, err := s.Authenticate(ctx, "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("err = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("reports no credential when none is seeded", func(t *testing.T) {
		s := newTestService(t)

		if _, err := s.Authenticate(testCtx(), "anything"); !errors.Is(err, ErrNoCredential) {
			t.Fatalf("err = %v, want ErrNoCredential", err)
		}
	})

	t.Run("a rotated password replaces the old one", func(t *testing.T) {
		s := newTestService(t)
		ctx := testCtx()
		if err := s.SetPassword(ctx, "first"); err != nil {
			t.Fatalf("set password: %v", err)
		}
		if err := s.SetPassword(ctx, "second"); err != nil {
			t.Fatalf("rotate password: %v", err)
		}

		if _, err := s.Authenticate(ctx, "first"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("old password err = %v, want ErrInvalidCredentials", err)
		}
		if _, err := s.Authenticate(ctx, "second"); err != nil {
			t.Fatalf("new password should authenticate: %v", err)
		}
	})
}

func TestSetPassword(t *testing.T) {
	t.Run("rejects an empty password", func(t *testing.T) {
		s := newTestService(t)
		if err := s.SetPassword(testCtx(), ""); err == nil {
			t.Fatal("expected an error for an empty password")
		}
	})
}

func TestHasCredential(t *testing.T) {
	t.Run("false before seeding, true after", func(t *testing.T) {
		s := newTestService(t)
		ctx := testCtx()

		has, err := s.HasCredential(ctx)
		if err != nil {
			t.Fatalf("has credential: %v", err)
		}
		if has {
			t.Fatal("expected no credential before seeding")
		}

		if err := s.SetPassword(ctx, "pw"); err != nil {
			t.Fatalf("set password: %v", err)
		}
		has, err = s.HasCredential(ctx)
		if err != nil {
			t.Fatalf("has credential: %v", err)
		}
		if !has {
			t.Fatal("expected a credential after seeding")
		}
	})
}
